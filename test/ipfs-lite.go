package main

import (
	"context"
	"fmt"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-log"
	crypto "github.com/libp2p/go-libp2p-crypto"
	"github.com/multiformats/go-multiaddr"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.SetLogLevel("*", "warn")

	ds, err := ipfslite.IPFSBadgerDatastore()
	if err != nil {
		panic(err)
	}
	priv, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		panic(err)
	}

	listen, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/4005")

	h, dht, err := ipfslite.SetupLibp2p(
		ctx,
		priv,
		nil,
		[]multiaddr.Multiaddr{listen},
		ipfslite.DefaultBootstrapPeers(),
	)

	if err != nil {
		panic(err)
	}

	lite, err := ipfslite.New(ctx, ds, h, dht, nil)
	if err != nil {
		panic(err)
	}

	c, _ := cid.Decode("QmVBw81ZCSdddvoVuT3G3jNpNr7eG27UVfVpvgZ72jKreN")
	n, err := lite.Get(ctx, c)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(n.RawData()))
}
