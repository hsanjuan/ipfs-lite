package scp

import (
	"context"
	"io"
	"sync"
	"time"

	ssConf "github.com/StreamSpace/ss-light-client/scp/config"
	ssCrypto "github.com/StreamSpace/ss-light-client/scp/crypto"
	"github.com/StreamSpace/ss-light-client/scp/engine"
	"github.com/StreamSpace/ss-light-client/scp/message"
	bsmsg "github.com/ipfs/go-bitswap/message"
	bsnet "github.com/ipfs/go-bitswap/network"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/helpers"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	msgio "github.com/libp2p/go-msgio"
)

type Params struct {
	DeviceID string
	Role     string
	Mtdt     map[string]interface{}
	Rate     string
}

type Hook string

var (
	PeerConnected Hook = "PeerConnected"
)

func NewScpModule(
	ctx context.Context,
	h host.Host,
	r routing.Routing,
	params Params,
) (*Scp, error) {
	cfg, err := ssConf.New(h, params.DeviceID, params.Role, params.Mtdt, params.Rate)
	if err != nil {
		return nil, err
	}
	opts := []engine.Option{
		engine.WithMsgSigner(ssCrypto.New(h)),
		engine.WithSignVerifier(ssCrypto.New(h)),
		engine.WithSSConfig(cfg),
	}
	e := engine.NewEngine(ctx, opts)
	return NewScp(ctx, h, r, e, true), nil
}

var log = logging.Logger("scp")

type Scp struct {
	bsnet.BitSwapNetwork
	bsnet.Receiver

	h          host.Host
	engine     *engine.Engine
	stat       *stats
	workerStop context.CancelFunc
	hooks      map[Hook]func()
}

func NewScp(
	ctx context.Context,
	h host.Host,
	r routing.Routing,
	e *engine.Engine,
	online bool,
) *Scp {
	if !online {
		return &Scp{}
	}
	s := &Scp{
		h:              h,
		stat:           &stats{stat: make(map[string]*Stat)},
		BitSwapNetwork: bsnet.NewFromIpfsHost(h, r),
		engine:         e,
		hooks:          make(map[Hook]func()),
	}
	s.h.SetStreamHandler(message.HandshakeProto, s.handleScpStream)
	s.h.SetStreamHandler(message.MicropaymentProto, s.handleScpStream)
	go s.taskWorker(ctx, 1)
	return s
}

func (s *Scp) SetDelegate(r bsnet.Receiver) {
	s.Receiver = r
	s.BitSwapNetwork.SetDelegate(s)
}

// Sender override
func (s *Scp) SendMessage(
	ctx context.Context,
	p peer.ID,
	outgoing bsmsg.BitSwapMessage,
) error {
	// Let bitswap handle it first
	err := s.BitSwapNetwork.SendMessage(ctx, p, outgoing)
	if err == nil {
		blks := outgoing.Blocks()
		totalLen := 0
		for _, b := range blks {
			totalLen += len(b.RawData())
		}
		if totalLen > 0 {
			log.Debugf("Updating sent info for %s total %d", p, totalLen)
			s.engine.SentTo(p, uint64(len(blks)), uint64(totalLen))
			s.stat.blksSent(len(blks), totalLen)
		}
	}
	return err
}

// Receiver override
func (s *Scp) ReceiveMessage(
	ctx context.Context,
	sender peer.ID,
	incoming bsmsg.BitSwapMessage,
) {
	// Let bitswap handle it first
	s.Receiver.ReceiveMessage(ctx, sender, incoming)
	blks := incoming.Blocks()
	totalLen := 0
	for _, b := range blks {
		totalLen += len(b.RawData())
	}
	if totalLen > 0 {
		log.Debugf("Generating micropayment for %s byes %d", sender, totalLen)
		s.engine.GenerateMicroPayment(sender, totalLen)
		s.engine.ReceivedFrom(sender, uint64(len(blks)), uint64(totalLen))
		s.stat.blksReceived(len(blks), totalLen)
	}
}

func (s *Scp) AddHook(h Hook, fn func()) {
	s.hooks[h] = fn
}

func (s *Scp) PeerConnected(p peer.ID) {
	if s.engine.HandshakeDone(p) {
		log.Infof("SCP Handshake done for %s. Sending notification to bitswap", p)
		s.Receiver.PeerConnected(p)
		if hk, ok := s.hooks[PeerConnected]; ok {
			hk()
		}
	}
}

func (s *Scp) PeerDisconnected(p peer.ID) {
	s.Receiver.PeerDisconnected(p)
}

var sendMessageTimeout = time.Minute * 10

func (s *Scp) msgToStream(
	ctx context.Context,
	st network.Stream,
	msg message.ScpMsg,
) error {
	deadline := time.Now().Add(sendMessageTimeout)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}

	if err := st.SetWriteDeadline(deadline); err != nil {
		log.Warnf("error setting deadline: %s", err)
	}

	if err := msg.ToStream(st); err != nil {
		log.Errorf("Failed putting message %s on stream err %s", msg.ID(), err.Error())
		return err
	}

	if err := st.SetWriteDeadline(time.Time{}); err != nil {
		log.Warnf("error resetting deadline: %s", err)
	}
	return nil
}

func (s *Scp) sendScpMessage(
	ctx context.Context,
	p peer.ID,
	msg message.ScpMsg,
) error {
	st, err := s.h.NewStream(ctx, p, msg.ID())
	if err != nil {
		return err
	}

	err = s.msgToStream(ctx, st, msg)
	if err != nil {
		return err
	}

	go helpers.AwaitEOF(st)
	return st.Close()
}

func (s *Scp) handleScpStream(st network.Stream) {
	defer st.Close()

	if s.engine == nil {
		_ = st.Reset()
		return
	}

	reader := msgio.NewVarintReaderSize(st, message.SizeMax)
	for {
		msg, err := message.FromReader(st.Protocol(), reader)
		if err != nil {
			if err != io.EOF {
				log.Errorf("Received error in SCP stream err:%s", err.Error())
			}
			return
		}
		s.engine.HandleMsg(st.Conn().RemotePeer(), msg)
		s.stat.received(msg)
		if msg.ID() == message.HandshakeProto {
			s.PeerConnected(st.Conn().RemotePeer())
		}
		log.Debugf("received SCP msg %s from %s", msg.ID(), st.Conn().RemotePeer())
	}
}

func (s *Scp) taskWorker(ctx context.Context, id int) {
	defer log.Debug("SCP task worker shutting down...")
	log := log.With("ID", id)
	for {
		log.Debug("SCP.TaskWorker.Loop")
		select {
		case nextEnvelope := <-s.engine.Outbox():
			select {
			case envelope, ok := <-nextEnvelope:
				if !ok {
					continue
				}
				s.sendMsg(ctx, envelope)
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scp) sendMsg(ctx context.Context, env *engine.Envelope) {
	// Blocks need to be sent synchronously to maintain proper backpressure
	// throughout the network stack
	defer env.Sent()

	err := s.sendScpMessage(ctx, env.Peer, env.Message)
	if err != nil {
		log.Debugf("failed to send SCP message %s to %s", env.Message.ID(), env.Peer)
		return
	}
	s.stat.sent(env.Message)
	log.Debugf("sent SCP message %s to %s", env.Message.ID(), env.Peer)
}

type stats struct {
	lk   sync.Mutex
	stat map[string]*Stat
}

type Stat struct {
	Sent     int
	Received int
}

func (st *stats) sent(msg message.ScpMsg) {
	st.lk.Lock()
	defer st.lk.Unlock()
	val, ok := st.stat[string(msg.ID())]
	if !ok {
		st.stat[string(msg.ID())] = &Stat{Sent: 1}
		return
	}
	val.Sent++
	return
}

func (st *stats) received(msg message.ScpMsg) {
	st.lk.Lock()
	defer st.lk.Unlock()

	val, ok := st.stat[string(msg.ID())]
	if !ok {
		st.stat[string(msg.ID())] = &Stat{Received: 1}
		return
	}
	val.Received++
	return
}

func (st *stats) blksSent(count, totalLen int) {
	st.lk.Lock()
	defer st.lk.Unlock()

	cnt, ok := st.stat["blocks"]
	if !ok {
		st.stat["blocks"] = &Stat{Sent: count}
	} else {
		cnt.Sent += count
	}
	tot, ok := st.stat["total"]
	if !ok {
		st.stat["total"] = &Stat{Sent: totalLen}
	} else {
		tot.Sent += totalLen
	}
	return
}

func (st *stats) blksReceived(count, totalLen int) {
	st.lk.Lock()
	defer st.lk.Unlock()

	cnt, ok := st.stat["blocks"]
	if !ok {
		st.stat["blocks"] = &Stat{Received: count}
	} else {
		cnt.Received += count
	}
	tot, ok := st.stat["total"]
	if !ok {
		st.stat["total"] = &Stat{Received: totalLen}
	} else {
		tot.Received += totalLen
	}
	return
}

func (st *stats) snapshot() map[string]Stat {
	st.lk.Lock()
	defer st.lk.Unlock()

	retMap := make(map[string]Stat)
	for k, v := range st.stat {
		retMap[k] = *v
	}
	return retMap
}

// Interface to other clients
func (s *Scp) GetMicroPayments() ([]*engine.SSReceipt, error) {
	return s.engine.GetCurrentTxns()
}

func (s *Scp) GetPendingMicroPayments() ([]*engine.SSReceipt, error) {
	return s.engine.GetPendingTxns()
}

func (s *Scp) ClearPendingCycles(cycles []int) ([]int, error) {
	return s.engine.ClearPendingTxns(cycles)
}

func (s *Scp) ScpStats() map[string]Stat {
	return s.stat.snapshot()
}
