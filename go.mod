module github.com/StreamSpace/ss-light-client

go 1.13

require (
	github.com/StreamSpace/scp v1.0.0
	github.com/benbjohnson/clock v1.0.3 // indirect
	github.com/davidlazar/go-crypto v0.0.0-20190912175916-7055855a373f // indirect
	github.com/fortytw2/leaktest v1.3.0 // indirect
	github.com/glendc/go-external-ip v0.0.0-20170425150139-139229dcdddd
	github.com/ipfs/go-bitswap v0.3.3
	github.com/ipfs/go-blockservice v0.1.4
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-datastore v0.4.5
	github.com/ipfs/go-ipfs-blockstore v1.0.3
	github.com/ipfs/go-ipfs-chunker v0.0.5 // indirect
	github.com/ipfs/go-ipfs-exchange-offline v0.0.1
	github.com/ipfs/go-ipld-cbor v0.0.4
	github.com/ipfs/go-ipld-format v0.2.0
	github.com/ipfs/go-ipns v0.0.2
	github.com/ipfs/go-log/v2 v2.1.1
	github.com/ipfs/go-merkledag v0.3.2
	github.com/ipfs/go-unixfs v0.2.4
	github.com/libp2p/go-libp2p v0.12.0
	github.com/libp2p/go-libp2p-connmgr v0.2.4
	github.com/libp2p/go-libp2p-core v0.7.0
	github.com/libp2p/go-libp2p-kad-dht v0.11.1
	github.com/libp2p/go-libp2p-record v0.1.3
	github.com/libp2p/go-sockaddr v0.1.0 // indirect
	github.com/mailru/easyjson v0.0.0-20190312143242-1de009706dbe // indirect
	github.com/multiformats/go-multiaddr v0.3.1
	github.com/multiformats/go-multihash v0.0.14
	github.com/olivere/elastic v6.2.34+incompatible
	github.com/teris-io/shortid v0.0.0-20171029131806-771a37caa5cf
	golang.org/x/crypto v0.0.0-20200510223506-06a226fb4e37 // indirect
	golang.org/x/net v0.0.0-20200519113804-d87ec0cfa476 // indirect
	golang.org/x/sys v0.0.0-20200519105757-fe76b779f299 // indirect
	golang.org/x/tools v0.0.0-20200117012304-6edc0a871e69 // indirect
)

replace github.com/StreamSpace/scp => ./scp
