package engine

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/ipfs/go-peertaskqueue"
	"github.com/ipfs/go-peertaskqueue/peertask"
	peer "github.com/libp2p/go-libp2p-core/peer"

	lpb "github.com/StreamSpace/ss-light-client/scp/engine/ledger"
	"github.com/StreamSpace/ss-light-client/scp/message"
	hspb "github.com/StreamSpace/ss-light-client/scp/message/handshake"
	mppb "github.com/StreamSpace/ss-light-client/scp/message/micropayment"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("ssEngine")

type handshakeState struct {
	Sent     bool
	Received bool
	Role     string
	Tries    int
	DeviceId string
	SentAt   int64
}

type SSReceipt struct {
	Partner       string
	PartnerDevice string
	Role          string
	Sent          float64
	Recvd         float64
	Exchanges     int
	Whitelisted   bool
	SignedTxn     string
	Metadata      []byte `json:",omitempty"`
	BillCycle     int
	BytesPaid     uint64
	BytesPayRecvd uint64
	BlocksSent    uint64
	BlocksRecvd   uint64
	BytesSent     uint64
	BytesRecvd    uint64
}

type Envelope struct {
	Peer peer.ID

	Message message.ScpMsg

	Sent func()
}

type Engine struct {
	// peerRequestQueue is a priority queue of requests received from peers.
	// Requests are popped from the queue, packaged up, and placed in the
	// outbox.
	peerRequestQueue *peertaskqueue.PeerTaskQueue

	// FIXME it's a bit odd for the client and the worker to both share memory
	// (both modify the peerRequestQueue) and also to communicate over the
	// workSignal channel. consider sending requests over the channel and
	// allowing the worker to have exclusive access to the peerRequestQueue. In
	// that case, no lock would be required.
	workSignal chan struct{}

	// outbox contains outgoing messages to peers. This is owned by the
	// taskWorker goroutine
	outbox chan (<-chan *Envelope)

	lock sync.RWMutex // protects the fields immediatly below
	// ledgerMap lists Ledgers by their Partner key.
	ledgerMap       map[peer.ID]*lpb.SSLedger
	taskWorkerLock  sync.Mutex
	taskWorkerCount int

	// Streamspace engine fields
	ssStore      lpb.Store
	ssConf       SSConfig
	msgSigner    MessageSigner
	wlChecker    WhitelistChecker
	signVerifier SignatureVerifier
	sentQueue    chan peer.ID
	handshakeMap sync.Map

	ticker     *time.Ticker
	workerStop context.CancelFunc
}

func (e *Engine) defaultOpts() {
	if e.ssConf == nil {
		log.Warn("Streamspace configuration not provided. This will disable" +
			"certain features.")
		e.ssStore = &lpb.DummyStore{}
		e.ssConf = &dummyConf{}
	}
	if e.ssStore == nil {
		e.ssStore = lpb.NewMapLedgerStore()
	}
	if e.msgSigner == nil {
		e.msgSigner = &dummyMessageSigner{}
	}
	if e.wlChecker == nil {
		e.wlChecker = &dummyWhitelistChecker{}
	}
	if e.signVerifier == nil {
		e.signVerifier = &dummySignatureVerifier{}
	}
}

func NewEngine(ctx context.Context, opts []Option) *Engine {
	e := &Engine{
		ledgerMap:       make(map[peer.ID]*lpb.SSLedger),
		taskWorkerCount: 1,
		outbox:          make(chan (<-chan *Envelope), 0),
		workSignal:      make(chan struct{}, 1),
		ticker:          time.NewTicker(time.Millisecond * 100),
		sentQueue:       make(chan peer.ID, 200),
	}
	for _, opt := range opts {
		opt(e)
	}
	e.defaultOpts()
	e.peerRequestQueue = peertaskqueue.New(
		peertaskqueue.TaskMerger(newTaskMerger()),
		peertaskqueue.IgnoreFreezing(true),
	)
	go e.taskWorker(ctx)
	go e.ssTaskWorker(ctx)
	return e
}

// This function takes in ssLedger as parameter. So it should be called whilst
// holding the ledger lock.
func (e *Engine) isWhitelisted(currLedger *lpb.SSLedger) bool {
	if time.Since(time.Unix(int64(currLedger.LastWhitelistCheck), 0)) > 24*time.Hour {
		currLedger.Whitelisted = e.wlChecker.IsWhitelisted(currLedger.Partner)
		currLedger.LastWhitelistCheck = time.Now().Unix()
	}
	return currLedger.Whitelisted
}

func (e *Engine) getBillingCycle() int64 {
	return int64(math.Ceil(
		float64(time.Since(e.ssConf.Epoch())) / float64(e.ssConf.Cycle())))
}

func (e *Engine) commitLiveLedgers(endBillingCycle bool) {
	e.lock.Lock()
	defer e.lock.Unlock()

	for _, ledger := range e.ledgerMap {
		log.Debugf("Commiting ledger %s", ledger.String())
		err := e.ssStore.Store(ledger)
		if endBillingCycle && err == nil {
			p := ledger.Partner
			deviceId := ledger.DeviceId
			role := ledger.Role
			ledger.Reset()
			ledger.Partner = p
			ledger.Role = role
			ledger.DeviceId = deviceId
		}
	}
}

func (e *Engine) endBillingCycle(newCycle int64) error {
	e.commitLiveLedgers(true)
	log.Debugf("Updating billing cycle from %d to %d",
		e.ssStore.BillingCycle(), newCycle)

	err := e.ssStore.Update(newCycle)
	if err != nil {
		log.Error("Failed creating new ledger Err:" + err.Error())
		return err
	}
	return nil
}

func (e *Engine) findOrCreate(p peer.ID) *lpb.SSLedger {
	// Take a read lock (as it's less expensive) to check if we have a ledger
	// for the peer
	e.lock.RLock()
	l, ok := e.ledgerMap[p]
	e.lock.RUnlock()
	if ok {
		return l
	}

	// There's no ledger, so take a write lock, then check again and create the
	// ledger if necessary
	e.lock.Lock()
	defer e.lock.Unlock()
	l, ok = e.ledgerMap[p]
	if !ok {
		l = e.newSSLedger(p)
		e.ledgerMap[p] = l
	}
	return l
}

func (e *Engine) signalNewWork() {
	// Signal task generation to restart (if stopped!)
	select {
	case e.workSignal <- struct{}{}:
	default:
	}
}

func (e *Engine) newSSLedger(p peer.ID) *lpb.SSLedger {
	val := new(lpb.SSLedger)
	val.Partner = p.Pretty()
	err := e.ssStore.Get(val)
	if err != nil {
		err = e.ssStore.Store(val)
	}
	if err != nil {
		//TODO:Find better way to handle maybe.
		panic("Not able to create SSLedger. Can't recover.")
	}
	return val
}

func (e *Engine) handshakeDone(p peer.ID) bool {
	result, loaded := e.handshakeMap.LoadOrStore(p.Pretty(), &handshakeState{})
	log.Debugf("Handshake %v Loaded %b", result, loaded)
	if loaded {
		hs, ok := result.(*handshakeState)
		log.Debugf("Handshake : %+v OK : %b", hs, ok)
		if ok && hs.Sent && hs.Received {
			l := e.findOrCreate(p)
			l.Role = hs.Role
			l.DeviceId = hs.DeviceId
			log.Debugf("Handshake done for %s. Got Role %s",
				p.Pretty(), hs.Role)
			return true
		} else if hs.Tries > 5 {
			log.Errorf("Handshake tried 5 times and failed. " +
				"Returning done so it will be cleaned up")
			return true
		}
	}
	return false
}

func (e *Engine) sendHandshake(p peer.ID) {
	deviceID := e.ssConf.DeviceId()
	msg := hspb.NewHandshake(e.ssConf.Role(), deviceID)

	e.peerRequestQueue.PushTasks(p, peertask.Task{
		Topic:    "Handshake",
		Priority: 100,
		Work:     1,
		Data: &Envelope{
			Peer:    p,
			Message: msg,
			Sent: func() {
				res, ok := e.handshakeMap.Load(p.Pretty())
				if ok {
					hs := res.(*handshakeState)
					hs.Sent = true
					hs.SentAt = time.Now().Unix()
					hs.Tries++
					e.handshakeMap.Store(p.Pretty(), hs)
					log.Debugf("Updated Handshake state. %v", hs)
				}
			},
		},
	})
	log.Debugf("Enqueued handshake msg. %s", msg.String())
	e.signalNewWork()
}

func (e *Engine) HandshakeDone(p peer.ID) bool {
	if e.handshakeDone(p) {
		return true
	}
	// If handshake is not done, send the message
	e.sendHandshake(p)
	return false
}

func (e *Engine) Outbox() <-chan (<-chan *Envelope) {
	return e.outbox
}

func (e *Engine) ReceivedFrom(p peer.ID, count, totalLen uint64) {
	l := e.findOrCreate(p)

	e.lock.Lock()
	defer e.lock.Unlock()
	l.BlocksRecvd += count
	l.BytesRecvd += totalLen
	return
}

func (e *Engine) SentTo(p peer.ID, count, totalLen uint64) {
	l := e.findOrCreate(p)

	e.lock.Lock()
	defer e.lock.Unlock()
	l.BlocksSent += count
	l.BytesSent += totalLen
	return
}

func (e *Engine) HandleMsg(p peer.ID, msg message.ScpMsg) {
	switch msg.(type) {
	case *hspb.HandshakeMsg:
		e.HandleHandshake(p, msg.(*hspb.HandshakeMsg))
	case *mppb.MicropaymentMsg:
		e.HandleMicroPayment(p, msg.(*mppb.MicropaymentMsg))
	}
}

func (e *Engine) HandleHandshake(p peer.ID, hs *hspb.HandshakeMsg) {

	result, ok := e.handshakeMap.LoadOrStore(p.Pretty(), &handshakeState{})
	if ok {
		savedState, _ := result.(*handshakeState)
		savedState.Received = true
		savedState.Role = hs.Role
		savedState.DeviceId = hs.DeviceId
		e.handshakeMap.Store(p.Pretty(), savedState)
		log.Debugf("HandleHandshake %s %v", p.Pretty(), savedState)
		// If a peer gets disconnected and reconnects, we will have the handshake
		// state in memory. We save this to reduce the no. of messages. However,
		// because of this we will not send back the handshake.
		// Two cases:
		// 1. Peer is connecting/disconnecting due to network issues.
		//    Interval should be big enough so we don't overload and keep sending
		// 2. Valid case when peer went offline for sometime and came back.
		// So here we check when we sent the last handshake. When we connect first time
		// interval will be in seconds. So we will not resend the handshake. But if
		// last handshake happened sometime back and peer is trying to connect again
		// we will resend the message.
		if time.Since(time.Unix(savedState.SentAt, 0)) > time.Minute {
			log.Debug("Resending message to peer to complete handshake")
			e.sendHandshake(p)
		}
	}
	return
}

func (e *Engine) HandleMicroPayment(p peer.ID, mp *mppb.MicropaymentMsg) {
	l := e.findOrCreate(p)

	if e.ssConf.Role() != INVALID {
		if int64(mp.BillingCycle) != e.ssStore.BillingCycle() {
			// Should not happen
			log.Warnf("Got payment for different billing cycle "+
				"Exp:%d Curr:%d Msg:%s", e.ssStore.BillingCycle(), mp.BillingCycle,
				mp.String())
			return
		}
		if mp.Amount < l.Recvd {
			// Could be an older payment. Do nothing.
			log.Warn("Current received amount is less.")
			return
		}
		if mp.Receiver != e.ssConf.DeviceId() {
			// Should not happen
			log.Errorf("Got payment for some other peer %s",
				mp.Receiver)
			return
		}
		if !e.signVerifier.VerifyTxn(string(p), mp) {
			log.Errorf("Txn Signature invalid %s", mp.String())
			return
		}
	}

	e.lock.Lock()
	defer e.lock.Unlock()

	l.Recvd = mp.Amount
	bfRecvd, bfRate := big.NewFloat(l.Recvd), big.NewFloat(e.ssConf.Rate())
	bfBytesPayRecvd := new(big.Float).Quo(bfRecvd, bfRate)
	l.BytesPayRecvd, _ = bfBytesPayRecvd.Uint64()
	l.MpExchangeCount++
	l.LastMpExchange = time.Now().Unix()
	l.SignedMp = mp.TxnHash
	l.Metadata = mp.RawMetadataBytes()
	log.Debugf("Incoming Micropayment handled %v", l.Loggable())
}

func (e *Engine) GenerateMicroPayment(p peer.ID, blkLen int) {
	l := e.findOrCreate(p)

	e.lock.Lock()
	defer e.lock.Unlock()

	l.Invoice += float64(blkLen) * e.ssConf.Rate()
	// Convert to 9 decimals after 0. Float multiplication giving weird results.
	l.Invoice, _ = strconv.ParseFloat(fmt.Sprintf("%.9f", l.Invoice), 9)
	msg := mppb.NewMicroPayment(
		l.Invoice,
		e.ssStore.BillingCycle(),
		l.DeviceId,
		e.ssConf.RawMetadata(),
	)
	err := e.msgSigner.SignTxn(msg)
	if err != nil {
		log.Errorf("Failed creating Micropayment signature err: %s", err.Error())
		return
	}
	e.peerRequestQueue.PushTasks(p, peertask.Task{
		Topic:    "Micropayment",
		Priority: 100,
		Work:     1,
		Data: &Envelope{
			Peer:    p,
			Message: msg,
			Sent: func() {
				e.lock.Lock()
				defer e.lock.Unlock()

				l.Sent = msg.Amount
				bfSent, bfRate := big.NewFloat(l.Sent), big.NewFloat(e.ssConf.Rate())
				bfBytesPaid := new(big.Float).Quo(bfSent, bfRate)
				l.BytesPaid, _ = bfBytesPaid.Uint64()
				l.MpExchangeCount++
				l.LastMpExchange = time.Now().Unix()
				log.Debugf("Updated SSLedger. %v", l.Loggable())
			},
		},
	})
	log.Debugf("Enqueued micropayment task. %s", msg.String())
}

func (e *Engine) GetCurrentTxns() ([]*SSReceipt, error) {
	e.commitLiveLedgers(false)
	list, err := e.ssStore.List()
	if err != nil {
		log.Error("Failed listing current txns Err:" + err.Error())
		return []*SSReceipt{}, err
	}

	rcpts := make([]*SSReceipt, 0)

	cycle := e.ssStore.BillingCycle()

	for _, v := range list {
		log.Debugf("Returning %v", v.Loggable())
		rcpt := &SSReceipt{
			Partner:       v.Partner,
			PartnerDevice: v.DeviceId,
			Role:          v.Role,
			Sent:          v.Sent,
			Recvd:         v.Recvd,
			Exchanges:     int(v.MpExchangeCount),
			Whitelisted:   v.Whitelisted,
			SignedTxn:     v.SignedMp,
			Metadata:      v.Metadata,
			BillCycle:     int(cycle),
			BytesPaid:     v.BytesPaid,
			BytesPayRecvd: v.BytesPayRecvd,
			BlocksSent:    v.BlocksSent,
			BlocksRecvd:   v.BlocksRecvd,
			BytesSent:     v.BytesSent,
			BytesRecvd:    v.BytesRecvd,
		}
		rcpts = append(rcpts, rcpt)
	}

	return rcpts, nil
}

func (e *Engine) GetPendingTxns() ([]*SSReceipt, error) {
	tmap, err := e.ssStore.GetPending()
	if err != nil {
		log.Errorf("Failed getting pending txns Err:%s", err)
		return nil, err
	}
	rcpts := make([]*SSReceipt, 0)
	for k, l := range tmap {
		for _, v := range l {
			if len(v.SignedMp) == 0 && (v.Recvd == 0 && v.BytesPaid == 0) {
				log.Debugf("Ignoring empty pending txn %v", v.Loggable())
				continue
			}
			rcpt := &SSReceipt{
				Partner:       v.Partner,
				PartnerDevice: v.DeviceId,
				Role:          v.Role,
				Sent:          v.Sent,
				Recvd:         v.Recvd,
				Exchanges:     int(v.MpExchangeCount),
				Whitelisted:   v.Whitelisted,
				SignedTxn:     v.SignedMp,
				Metadata:      v.Metadata,
				BillCycle:     k,
				BytesPaid:     v.BytesPaid,
				BytesPayRecvd: v.BytesPayRecvd,
				BlocksSent:    v.BlocksSent,
				BlocksRecvd:   v.BlocksRecvd,
				BytesSent:     v.BytesSent,
				BytesRecvd:    v.BytesRecvd,
			}
			rcpts = append(rcpts, rcpt)
		}
	}
	return rcpts, nil
}

func (e *Engine) ClearPendingTxns(cycles []int) ([]int, error) {
	return e.ssStore.ClearPending(cycles)
}
