package database

import (
	"encoding/json"
	"net"
	"testing"
)

func TestNetworkForPeerRequiresLiveLocalMembership(t *testing.T) {
	tests := []struct {
		name       string
		peerIP     string
		hostIP     string
		connected  bool
		gatewayMAC string
		want       string
	}{
		{"same network", "192.168.1.170", "192.168.1.163", true, "00:11:22:33:44:55", "00:11:22:33:44:55|192.168.1.0/24"},
		{"different subnet", "10.0.0.170", "192.168.1.163", true, "00:11:22:33:44:55", ""},
		{"interface disconnected", "192.168.1.170", "192.168.1.163", false, "00:11:22:33:44:55", ""},
		{"stale interface IP", "192.168.1.170", "10.0.0.163", true, "00:11:22:33:44:55", ""},
		{"gateway unknown", "192.168.1.170", "192.168.1.163", true, "", ""},
		{"same subnet different gateway", "192.168.1.170", "192.168.1.163", true, "00:aa:bb:cc:dd:ee", "00:aa:bb:cc:dd:ee|192.168.1.0/24"},
		{"IPv6 peer", "fe80::170", "192.168.1.163", true, "00:11:22:33:44:55", "00:11:22:33:44:55|192.168.1.0/24"},
		{"invalid IP", "invalid", "192.168.1.163", true, "00:11:22:33:44:55", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := map[string]interface{}{
				"connected": test.connected, "host_ip": "192.168.1.163",
				"local_network": "192.168.1.0/24", "gateway_mac": test.gatewayMAC,
				"addresses": []map[string]string{{"network": "fe80::/64"}},
			}
			encoded, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			states := map[string]string{"en0": string(encoded)}
			addresses := map[string][]net.IP{"en0": {net.ParseIP(test.hostIP)}}
			if got := networkForPeer(states, addresses, test.peerIP); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
