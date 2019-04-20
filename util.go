package ipfslite

import (
	"context"
	"fmt"
	"os"
	"os/user"

	"github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	config "github.com/ipfs/go-ipfs-config"
	"github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	ipnet "github.com/libp2p/go-libp2p-interface-pnet"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	pnet "github.com/libp2p/go-libp2p-pnet"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	"github.com/multiformats/go-multiaddr"
)

// DefaultBootstrapPeers returns the default go-ipfs bootstrap peers (for use
// with NewLibp2pHost.
func DefaultBootstrapPeers() []peerstore.PeerInfo {
	// conversion copied from go-ipfs
	defaults, _ := config.DefaultBootstrapPeers()
	pinfos := make(map[peer.ID]*peerstore.PeerInfo)
	for _, bootstrap := range defaults {
		pinfo, ok := pinfos[bootstrap.ID()]
		if !ok {
			pinfo = new(peerstore.PeerInfo)
			pinfos[bootstrap.ID()] = pinfo
			pinfo.ID = bootstrap.ID()
		}

		pinfo.Addrs = append(pinfo.Addrs, bootstrap.Transport())
	}

	var peers []peerstore.PeerInfo
	for _, pinfo := range pinfos {
		peers = append(peers, *pinfo)
	}
	return peers
}

// IPFSBadgerDatastore returns the Badger datastore used by the IPFS daemon
// (from `~/.ipfs/datastore`). Do not use the default datastore when the
// regular IFPS daemon is running at the same time.
func IPFSBadgerDatastore() (datastore.Batching, error) {
	home := os.Getenv("HOME")
	if home == "" {
		usr, err := user.Current()
		if err != nil {
			panic(fmt.Sprintf("cannot get current user: %s", err))
		}
		home = usr.HomeDir
	}

	path, err := config.DataStorePath(home)
	if err != nil {
		return nil, err
	}
	return BadgerDatastore(path)
}

// BadgerDatastore returns a new instance of Badger-DS persisting
// to the given path with the default options.
func BadgerDatastore(path string) (datastore.Batching, error) {
	return badger.NewDatastore(path, &badger.DefaultOptions)
}

// SetupLibp2p returns a routed host and DHT instances that can be used to
// easily create a ipfslite Peer. The DHT is NOT bootstrapped. You may consider
// to use Peer.Bootstrap() after creating the IPFS-Lite Peer.
func SetupLibp2p(
	ctx context.Context,
	hostKey crypto.PrivKey,
	secret []byte,
	listenAddrs []multiaddr.Multiaddr,
) (host.Host, *dht.IpfsDHT, error) {

	var prot ipnet.Protector
	var err error

	// Create protector if we have a secret.
	if secret != nil && len(secret) > 0 {
		var key [32]byte
		copy(key[:], secret)
		prot, err = pnet.NewV1ProtectorFromBytes(&key)
		if err != nil {
			return nil, nil, err
		}
	}

	h, err := libp2p.New(
		ctx,
		libp2p.Identity(hostKey),
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.PrivateNetwork(prot),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, nil, err
	}

	idht, err := dht.New(ctx, h)
	if err != nil {
		h.Close()
		return nil, nil, err
	}

	rHost := routedhost.Wrap(h, idht)
	return rHost, idht, nil
}
