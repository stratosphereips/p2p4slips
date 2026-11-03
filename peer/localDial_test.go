package peer

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// testSubnet parses one fixed network for deterministic address policy tests.
func testSubnet(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	return subnet
}

func TestLocalDialRejectsForeignAdvertisedAddresses(t *testing.T) {
	gater := &localDialGater{networks: func() []*net.IPNet {
		return []*net.IPNet{testSubnet(t, "192.168.1.0/24"), testSubnet(t, "fd00:1::/64")}
	}}
	for _, test := range []struct {
		address string
		allowed bool
	}{
		{"/ip4/192.168.1.170/tcp/6668", true},
		{"/ip4/192.168.1.200/tcp/6669", true},
		{"/ip4/172.17.0.1/tcp/6668", false},
		{"/ip4/192.168.12.1/tcp/6668", false},
		{"/ip4/127.0.0.1/tcp/6668", false},
		{"/ip4/0.0.0.0/tcp/6668", false},
		{"/ip4/224.0.0.251/tcp/6668", false},
		{"/ip4/8.8.8.8/tcp/6668", false},
		{"/ip6/fd00:1::170/tcp/6668", true},
		{"/ip6/fd00:2::170/tcp/6668", false},
		{"/ip4/192.168.1.170/udp/6668", false},
	} {
		t.Run(test.address, func(t *testing.T) {
			address := multiaddr.StringCast(test.address)
			if got := gater.InterceptAddrDial("peer", address); got != test.allowed {
				t.Fatalf("allowed=%v, want %v", got, test.allowed)
			}
		})
	}
	advertised := []multiaddr.Multiaddr{
		multiaddr.StringCast("/ip4/127.0.0.1/tcp/6668"),
		multiaddr.StringCast("/ip4/172.17.0.1/tcp/6668"),
		multiaddr.StringCast("/ip4/192.168.1.170/tcp/6668"),
	}
	filtered := gater.filter(advertised)
	if len(filtered) != 1 || !filtered[0].Equal(advertised[2]) {
		t.Fatal("must choose the LAN endpoint, not loopback or a foreign Docker bridge")
	}
}

func TestLocalDialUsesCurrentInterfacesAfterNetworkChange(t *testing.T) {
	networks := []*net.IPNet{testSubnet(t, "192.168.1.0/24")}
	gater := &localDialGater{networks: func() []*net.IPNet { return networks }}
	old := multiaddr.StringCast("/ip4/192.168.1.170/tcp/6668")
	current := multiaddr.StringCast("/ip4/172.20.10.3/tcp/6668")
	if !gater.allows(old) || gater.allows(current) {
		t.Fatal("initial network membership is incorrect")
	}
	networks = []*net.IPNet{testSubnet(t, "172.20.10.0/28")}
	if gater.allows(old) || !gater.allows(current) {
		t.Fatal("a network change must invalidate old dial permissions")
	}
	networks = nil
	if gater.allows(current) {
		t.Fatal("missing interfaces must not permit arbitrary dialing")
	}
}

func TestLocalDialGateRejectsCachedAddressAndKeepsValidConnection(t *testing.T) {
	gater := &localDialGater{allowLoopback: true, networks: func() []*net.IPNet { return nil }}
	dialer, err := libp2p.New(libp2p.ConnectionGater(gater), libp2p.DisableRelay())
	if err != nil {
		t.Fatal(err)
	}
	defer dialer.Close()
	listener, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"), libp2p.DisableRelay())
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bad := libp2ppeer.AddrInfo{ID: listener.ID(), Addrs: []multiaddr.Multiaddr{
		multiaddr.StringCast("/ip4/172.17.0.1/tcp/6668"),
	}}
	if err := dialer.Connect(ctx, bad); err == nil {
		t.Fatal("an off-LAN address must be rejected before TCP dialing")
	}
	// Keep the rejected address cached and verify a valid address still connects.
	good := libp2ppeer.AddrInfo{ID: listener.ID(), Addrs: listener.Addrs()}
	if err := dialer.Connect(ctx, good); err != nil {
		t.Fatalf("local peer must remain reachable: %v", err)
	}
}
