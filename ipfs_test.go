package ipfslite

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"testing"

	datastore "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	cbor "github.com/ipfs/go-ipld-cbor"
	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	pnet "github.com/libp2p/go-libp2p-pnet"
	multiaddr "github.com/multiformats/go-multiaddr"
	multihash "github.com/multiformats/go-multihash"
)

var secret []byte = []byte("2cc2c79ea52c9cc85dfd3061961dd8c4230cce0b09f182a0822c1536bf1d5f21")

func setupPeers(t *testing.T) (p1, p2 *Peer, closer func(t *testing.T)) {
	ctx := context.Background()

	// Workaround https://github.com/libp2p/go-libp2p-kad-dht/issues/308
	var key [32]byte
	copy(key[:], secret)
	prot, err := pnet.NewV1ProtectorFromBytes(&key)
	if err != nil {
		t.Fatal(err)
	}
	signalHost, err := libp2p.New(
		ctx,
		libp2p.PrivateNetwork(prot),
	)
	if err != nil {
		t.Fatal(err)
	}
	sdht, err := dht.New(ctx, signalHost)
	if err != nil {
		signalHost.Close()
		t.Fatal(err)
	}

	pinfo := peerstore.PeerInfo{
		ID:    signalHost.ID(),
		Addrs: signalHost.Addrs(),
	}

	ds1 := dssync.MutexWrap(datastore.NewMapDatastore())
	ds2 := dssync.MutexWrap(datastore.NewMapDatastore())
	priv1, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		t.Fatal(err)
	}
	priv2, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		t.Fatal(err)
	}

	listen, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/0")
	h1, dht1, err := SetupLibp2p(
		ctx,
		priv1,
		secret,
		[]multiaddr.Multiaddr{listen},
		[]peerstore.PeerInfo{pinfo},
	)
	if err != nil {
		t.Fatal(err)
	}
	h2, dht2, err := SetupLibp2p(
		ctx,
		priv2,
		secret,
		[]multiaddr.Multiaddr{listen},
		[]peerstore.PeerInfo{pinfo},
	)
	if err != nil {
		t.Fatal(err)
	}

	dht1.Update(ctx, h2.ID())
	dht2.Update(ctx, h1.ID())

	closer = func(t *testing.T) {
		for _, cl := range []io.Closer{sdht, signalHost, dht1, dht2, h1, h2} {
			err := cl.Close()
			if err != nil {
				t.Error(err)
			}
		}
	}
	p1, err = New(ctx, ds1, h1, dht1, nil)
	if err != nil {
		closer(t)
		t.Fatal(err)
	}
	p2, err = New(ctx, ds2, h2, dht2, nil)
	if err != nil {
		closer(t)
		t.Fatal(err)
	}
	return
}

func TestDAG(t *testing.T) {
	ctx := context.Background()
	p1, p2, closer := setupPeers(t)
	defer closer(t)

	m := map[string]string{
		"akey": "avalue",
	}

	codec := uint64(multihash.SHA2_256)
	node, err := cbor.WrapObject(m, codec, multihash.DefaultLengths[codec])
	if err != nil {
		t.Fatal(err)
	}

	t.Log("created node: ", node.Cid())
	err = p1.Add(ctx, node)
	if err != nil {
		t.Fatal(err)
	}

	_, err = p2.Get(ctx, node.Cid())
	if err != nil {
		t.Error(err)
	}

	err = p1.Remove(ctx, node.Cid())
	if err != nil {
		t.Error(err)
	}

	err = p2.Remove(ctx, node.Cid())
	if err != nil {
		t.Error(err)
	}

	if ok, err := p1.BlockStore().Has(node.Cid()); ok || err != nil {
		t.Error("block should have been deleted")
	}

	if ok, err := p2.BlockStore().Has(node.Cid()); ok || err != nil {
		t.Error("block should have been deleted")
	}
}

func TestSession(t *testing.T) {
	ctx := context.Background()
	p1, p2, closer := setupPeers(t)
	defer closer(t)

	m := map[string]string{
		"akey": "avalue",
	}

	codec := uint64(multihash.SHA2_256)
	node, err := cbor.WrapObject(m, codec, multihash.DefaultLengths[codec])
	if err != nil {
		t.Fatal(err)
	}

	t.Log("created node: ", node.Cid())
	err = p1.Add(ctx, node)
	if err != nil {
		t.Fatal(err)
	}

	sesGetter := p2.Session(ctx)
	_, err = sesGetter.Get(ctx, node.Cid())
	if err != nil {
		t.Fatal(err)
	}
}

func TestFiles(t *testing.T) {
	p1, p2, closer := setupPeers(t)
	defer closer(t)

	content := []byte("hola")
	buf := bytes.NewReader(content)
	n, err := p1.AddFile(context.Background(), buf, nil)
	if err != nil {
		t.Fatal(err)
	}

	rsc, err := p2.GetFile(context.Background(), n.Cid())
	if err != nil {
		t.Fatal(err)
	}
	defer rsc.Close()

	content2, err := ioutil.ReadAll(rsc)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(content, content2) {
		t.Error(string(content))
		t.Error(string(content2))
		t.Error("different content put and retrieved")
	}
}
