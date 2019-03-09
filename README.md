# IPFS-Lite

IPFS-lite is an embeddable, super-lightweight IPFS peer which runs the minimal
setup to provide an `ipld.DAGService`. It can:

* Add, Get, Remove IPLD Nodes to/from the IPFS Network (remove is a local blockstore operation).
* Import content (chunk, build the DAG and Add) from a `io.Reader`.

It needs:

* A [libp2p Host](https://godoc.org/github.com/libp2p/go-libp2p#New)
* A [libp2p DHT](https://godoc.org/github.com/libp2p/go-libp2p-kad-dht#New)
* A [datastore](https://godoc.org/github.com/ipfs/go-datastore) like [BadgerDB](https://godoc.org/github.com/ipfs/go-ds-badger)

Some helper functions are provided to
[initialize these quickly](https://godoc.org/github.com/hsanjuan/ipfs-lite#SetupLibp2p).

It provides:

* An [`ipld.DAGService`](https://godoc.org/github.com/ipfs/go-ipld-format#DAGService)
* An [`Import` method](https://godoc.org/github.com/hsanjuan/ipfs-lite#Peer.Import) to add content from a reader.

The goal of IPFS-Lite is to run the **bare minimal** functionality for any
IPLD-based application to interact with the IPFS Network by getting and
putting blocks to it, rather than having to deal with the complexities of
using a full IPFS daemon and with the liberty of sharing the needed libp2p
Host and DHT for other things.

## License

Apache 2.0

