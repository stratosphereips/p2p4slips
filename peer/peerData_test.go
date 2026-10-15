package peer

import (
	"encoding/json"
	"testing"
)

func TestReliabilityPausesOutsidePeersNetwork(t *testing.T) {
	data := &PeerData{LastNetworkID: "home", Reliability: 1, BasicInteractions: []float64{1}}
	for _, context := range []string{"", "coffee-shop", "coffee-shop"} {
		if data.addBasicInteraction(0, context) {
			t.Fatal("an unavailable peer on another network changed reliability")
		}
	}
	if data.Reliability != 1 || len(data.BasicInteractions) != 1 {
		t.Fatal("ratings accumulated while away from the peer's network")
	}
	if !data.addBasicInteraction(0, "home") || data.Reliability != 0.5 {
		t.Fatal("a failure on the peer's actual network must affect reliability")
	}
}

func TestSuccessfulInteractionEstablishesNewNetwork(t *testing.T) {
	data := &PeerData{LastNetworkID: "home", Reliability: 1, BasicInteractions: []float64{1}}
	data.addBasicInteraction(1, "office")
	if data.LastNetworkID != "office" {
		t.Fatal("successful authenticated interaction did not establish new membership")
	}
	if !data.addBasicInteraction(0, "office") || len(data.BasicInteractions) != 3 {
		t.Fatal("reliability must resume after joining the peer's new network")
	}
}

func TestNetworkMembershipSurvivesPeerstoreReload(t *testing.T) {
	data := &PeerData{LastNetworkID: "home", Reliability: 1, BasicInteractions: []float64{1}}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var restored PeerData
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	restored.addBasicInteraction(0, "coffee-shop")
	if restored.Reliability != 1 || restored.LastNetworkID != "home" {
		t.Fatal("reloading peer history permitted a failure on another network")
	}
}

func TestZeroReliabilityIsIncludedInUpdate(t *testing.T) {
	encoded, err := json.Marshal(PeerUpdateStruct{PeerID: "peer", Reliability: 0})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if value, ok := fields["reliability"]; !ok || value != float64(0) {
		t.Fatal("zero reliability was omitted, leaving stale trust in Slips")
	}
}
