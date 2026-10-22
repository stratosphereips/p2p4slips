# Peer to peer component for the Stratosphere Linux IPS (SLIPS)

This program is a demo for the P2P part of SLIPS. It enables sharing network information between SLIPS nodes, which can improve overall network security.

## How to build this example?

You need Go 1.21 or newer: `go.mod` pins the exact toolchain (see its
`toolchain` line) and Go downloads it on the first build.

```
go build -buildvcs=false
```

`go build` also fetches the module dependencies (go-libp2p, go-redis,
go-multiaddr) listed in `go.mod`.

## Demo usage

Use two different terminal windows to run

```
./p2p-experiments
./p2p-experiments -port=4002
```
You will see the peers start and contact each other.


## Usage with SLIPS

This program is called from the slips P2P module. Save files for Peer storage and for encryption keys can be set up to use the same identity after restart.

Pigeon uses the Noise security protocol and negotiates the stream muxer after
the secure handshake. This keeps it interoperable with older Slips Pigeon
builds that do not support early muxer negotiation inside Noise.
