package main

import (
	"context"
	"fmt"

	"github.com/awalterschulze/gographviz"
	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-log/v2"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/multiformats/go-multiaddr"
)

var (
	testCID = "QmS4ustL54uo8FzR9455qaxZwuMiUhyvMcX9Ba8nUH4uVv"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.SetLogLevel("*", "warn")

	ds := ipfslite.NewInMemoryDatastore()
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
		ds,
		ipfslite.Libp2pOptionsExtra...,
	)

	if err != nil {
		panic(err)
	}

	lite, err := ipfslite.New(ctx, ds, h, dht, nil)
	if err != nil {
		panic(err)
	}

	lite.Bootstrap(ipfslite.DefaultBootstrapPeers())

	c, _ := cid.Decode(testCID)
	node, err := lite.Get(ctx, c)
	if err != nil {
		panic(err)
	}
	graphAst, err := gographviz.ParseString(`digraph G {}`)
	if err != nil {
		panic(err)
	}
	graph := gographviz.NewGraph()
	if err := gographviz.Analyse(graphAst, graph); err != nil {
		panic(err)
	}
	navNode := format.NewNavigableIPLDNode(node, lite.DAGService)

	rootNode := node.Cid().String()

	// add the root node to the graph
	if err := graph.AddNode("Example", rootNode, nil); err != nil {
		panic(err)
	}

	for i := 0; i < int(navNode.ChildTotal()); i++ {
		childNode, err := navNode.FetchChild(ctx, uint(i))
		if err != nil {
			panic(err)
		}
		n := format.ExtractIPLDNode(childNode)
		childCID := n.Cid().String()

		if err := graph.AddNode("Example", childCID, nil); err != nil {
			panic(err)
		}
		if err := graph.AddEdge(rootNode, childCID, true, nil); err != nil {
			panic(err)
		}
	}
	graphOut := graph.String()
	fmt.Println(graphOut)
}
