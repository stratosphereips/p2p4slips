package peer

import (
	"testing"

	"github.com/multiformats/go-multiaddr"
)

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
