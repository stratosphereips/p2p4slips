package database

import "testing"

func TestConnectionIDChangesWithReconnectPort(t *testing.T) {
	first := AuthenticatedConnection{
		PeerID: "peer-a", Protocol: "tcp", LocalIP: "192.0.2.10",
		LocalPort: "6668", RemoteIP: "198.51.100.20", RemotePort: "43123",
	}
	second := first
	second.RemotePort = "51000"

	if ConnectionID(first) == ConnectionID(second) {
		t.Fatal("connections with changed ports must have different registry IDs")
	}
}
