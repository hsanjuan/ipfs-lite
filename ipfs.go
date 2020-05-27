// Package ipfslite is a lightweight IPFS peer which runs the minimal setup to
// provide an `ipld.DAGService`, "Add" and "Get" UnixFS files from IPFS.
package ipfslite

import (
	"context"
	"sync"
	"time"

	"github.com/StreamSpace/ss-light-client/scp"
	"github.com/ipfs/go-bitswap"
	blockservice "github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipfs/go-merkledag"
	ufsio "github.com/ipfs/go-unixfs/io"
	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	routing "github.com/libp2p/go-libp2p-core/routing"
)

func init() {
	ipld.Register(cid.DagProtobuf, merkledag.DecodeProtobufBlock)
	ipld.Register(cid.Raw, merkledag.DecodeRawBlock)
	ipld.Register(cid.DagCBOR, cbor.DecodeBlock) // need to decode CBOR
}

var logger = logging.Logger("ipfslite")

var (
	defaultReprovideInterval = 12 * time.Hour
)

// Config wraps configuration options for the Peer.
type Config struct {
	Offline bool
	Root    string
	Mtdt    map[string]interface{}
}

// Peer is an IPFS-Lite peer. It provides a DAG service that can fetch and put
// blocks from/to the IPFS network.
type Peer struct {
	ctx context.Context

	cfg *Config

	host  host.Host
	dht   routing.Routing
	store datastore.Batching

	ipld.DAGService // become a DAG service
	bstore          blockstore.Blockstore
	bserv           blockservice.BlockService
}

// New creates an IPFS-Lite Peer. It uses the given datastore, libp2p Host and
// Routing (usuall the DHT). The Host and the Routing may be nil if
// config.Offline is set to true, as they are not used in that case. Peer
// implements the ipld.DAGService interface.
func New(
	ctx context.Context,
	store datastore.Batching,
	host host.Host,
	dht routing.Routing,
	cfg *Config,
) (*Peer, error) {

	if cfg == nil {
		cfg = &Config{}
	}
	logging.SetLogLevel("*", "Debug")
	p := &Peer{
		ctx:   ctx,
		cfg:   cfg,
		host:  host,
		dht:   dht,
		store: store,
	}

	err := p.setupBlockstore()
	if err != nil {
		return nil, err
	}
	err = p.setupBlockService()
	if err != nil {
		return nil, err
	}
	err = p.setupDAGService()
	if err != nil {
		p.bserv.Close()
		return nil, err
	}

	go p.autoclose()

	return p, nil
}

func (p *Peer) setupBlockstore() error {
	bs := blockstore.NewBlockstore(p.store)
	bs = blockstore.NewIdStore(bs)
	cachedbs, err := blockstore.CachedBlockstore(p.ctx, bs, blockstore.DefaultCacheOpts())
	if err != nil {
		return err
	}
	p.bstore = cachedbs
	return nil
}

func (p *Peer) setupBlockService() error {
	if p.cfg.Offline {
		p.bserv = blockservice.New(p.bstore, offline.Exchange(p.bstore))
		return nil
	}

	scpModule, err := scp.NewScpModule(p.ctx, p.host, p.dht, scp.Params{
		Root:     p.cfg.Root,
		DeviceID: "lc_" + p.host.ID().Pretty(),
		Role:     "light-client",
		Mtdt:     p.cfg.Mtdt,
	})
	if err != nil {
		return err
	}
	bswap := bitswap.New(p.ctx, scpModule, p.bstore)
	p.bserv = blockservice.New(p.bstore, bswap)
	return nil
}

func (p *Peer) setupDAGService() error {
	p.DAGService = merkledag.NewDAGService(p.bserv)
	return nil
}

func (p *Peer) autoclose() {
	<-p.ctx.Done()
	p.bserv.Close()
}

// Bootstrap is an optional helper to connect to the given peers and bootstrap
// the Peer DHT (and Bitswap). This is a best-effort function. Errors are only
// logged and a warning is printed when less than half of the given peers
// could be contacted. It is fine to pass a list where some peers will not be
// reachable.
func (p *Peer) Bootstrap(peers []peer.AddrInfo) {
	connected := make(chan struct{})

	var wg sync.WaitGroup
	for _, pinfo := range peers {
		//h.Peerstore().AddAddrs(pinfo.ID, pinfo.Addrs, peerstore.PermanentAddrTTL)
		wg.Add(1)
		go func(pinfo peer.AddrInfo) {
			defer wg.Done()
			err := p.host.Connect(p.ctx, pinfo)
			if err != nil {
				logger.Warn(err)
				return
			}
			logger.Info("Connected to", pinfo.ID)
			connected <- struct{}{}
		}(pinfo)
	}

	go func() {
		wg.Wait()
		close(connected)
	}()

	i := 0
	for range connected {
		i++
	}
	if nPeers := len(peers); i < nPeers/2 {
		logger.Warnf("only connected to %d bootstrap peers out of %d", i, nPeers)
	}

	err := p.dht.Bootstrap(p.ctx)
	if err != nil {
		logger.Error(err)
		return
	}
}

// Session returns a session-based NodeGetter.
func (p *Peer) Session(ctx context.Context) ipld.NodeGetter {
	ng := merkledag.NewSession(ctx, p.DAGService)
	if ng == p.DAGService {
		logger.Warn("DAGService does not support sessions")
	}
	return ng
}

// GetFile returns a reader to a file as identified by its root CID. The file
// must have been added as a UnixFS DAG (default for IPFS).
func (p *Peer) GetFile(ctx context.Context, c cid.Cid) (ufsio.ReadSeekCloser, error) {
	n, err := p.Get(ctx, c)
	if err != nil {
		return nil, err
	}
	return ufsio.NewDagReader(ctx, n, p)
}
