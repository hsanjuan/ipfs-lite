# IPFS-Lite

** This project is just a proof of concept **

IPFS-lite is an embeddable, super-lightweight IPFS peer which runs the minimal
setup to provide an `ipld.DAGService`. It can only do basic DAG functionality
like retrieving and putting blocks from/to the IPFS network. Blocks are stored
in a given datastore.

In other words: it uses a given libp2p `Host` and `IpfsDHT` instances to provide
a `merkledag.DAGService`.

The goal of IPFS-Lite is to run the **bare minimal** functionality for any
IPLD-based application to interact with the IPFS Network by getting and
putting blocks to it. 
