package peer

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"
)

func TestNoiseWithoutEarlyMuxerNegotiationConnects(t *testing.T) {
	keyA, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	keyB, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err)
	}
	options := func(key crypto.PrivKey) []libp2p.Option {
		return []libp2p.Option{
			libp2p.Identity(key),
			libp2p.Transport(tcp.NewTCPTransport),
			libp2p.Security(noise.ID, newNoiseTransport),
			libp2p.DisableRelay(),
		}
	}
	listener, err := libp2p.New(
		append(options(keyA), libp2p.ListenAddrs(multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")))...,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	dialer, err := libp2p.New(options(keyB)...)
	if err != nil {
		t.Fatal(err)
	}
	defer dialer.Close()

	peerInfo := libp2ppeer.AddrInfo{ID: listener.ID(), Addrs: listener.Addrs()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := dialer.Connect(ctx, peerInfo); err != nil {
		t.Fatalf("failed to connect using compatible Noise negotiation: %v", err)
	}
	if dialer.Network().Connectedness(listener.ID()) != network.Connected {
		t.Fatal("peer connection did not remain active after the handshake")
	}
}

func TestMultiaddrEndpointSupportsIPv4AndIPv6TCP(t *testing.T) {
	tests := []struct {
		address string
		ip      string
		port    string
	}{
		{"/ip4/192.0.2.10/tcp/6668", "192.0.2.10", "6668"},
		{"/ip6/2001:db8::10/tcp/51000", "2001:db8::10", "51000"},
	}
	for _, test := range tests {
		address, err := multiaddr.NewMultiaddr(test.address)
		if err != nil {
			t.Fatal(err)
		}
		ip, port, ok := multiaddrEndpoint(address)
		if !ok || ip != test.ip || port != test.port {
			t.Fatalf("got (%q, %q, %v) for %s", ip, port, ok, test.address)
		}
	}
}

func TestMultiaddrEndpointRejectsNonTCPAddress(t *testing.T) {
	address, err := multiaddr.NewMultiaddr("/ip4/192.0.2.10/udp/6668")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := multiaddrEndpoint(address); ok {
		t.Fatal("UDP address must not enter the authenticated TCP registry")
	}
}
