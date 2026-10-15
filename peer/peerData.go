package peer

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/stratosphereips/p2p4slips/database"
)

type PeerData struct {
	PeerID                string
	LastUsedIP            string
	Version               string
	Reliability           float64
	Connected             bool
	LastInteraction       time.Time
	LastGoodPing          time.Time
	LastMultiAddress      string
	BasicInteractions     []float64
	BasicInteractionTimes []time.Time
	LastNetworkID         string
	interactionMu         sync.Mutex
	// TODO: move all manipulation to getters and setters, which notify slips

}

func (pd *PeerData) SetMultiaddr(multiAddress string) {
	if multiAddress == pd.LastMultiAddress {
		return
	}
	fmt.Println("Updating multiaddr")
	pd.LastMultiAddress = multiAddress

	remoteIP := strings.Split(multiAddress, "/")[2]
	fmt.Println("IP address changed", remoteIP)
	if pd.LastUsedIP != remoteIP {
		pd.LastUsedIP = remoteIP
		SharePeerDataUpdate(pd)
	}
}

func (pd *PeerData) SetVersion(value string) bool {
	if value == pd.Version {
		return false
	}
	pd.Version = value
	SharePeerDataUpdate(pd)
	return true
}

func (pd *PeerData) ShouldIPingPeer() bool {
	lastPing := pd.LastGoodPing

	//fmt.Println("[PEER PING] last contact was ", lastPing)

	if lastPing.IsZero() {
		return true
	}

	timeSinceLastPing := time.Since(lastPing)
	//fmt.Println("[PEER PING] time since last contact is ", timeSinceLastPing)

	// TODO: justify the constant
	// do not ping a peer that has been contacted less than five minutes ago
	return timeSinceLastPing >= 15*time.Second
}

func (pd *PeerData) ShouldIDeactivatePeer() bool {
	lastPing := pd.LastGoodPing

	if lastPing.IsZero() {
		// if no ping ever happened, do not deactivate
		return false
	}

	timeSinceLastPing := time.Since(lastPing)
	//fmt.Println("[PEER PING] time since last contact is ", timeSinceLastPing)

	// TODO: justify the constant
	// if last contact was less than a minute ago, the peer should be considered active (return false)
	// deactivate only if time since last (successful) ping is more than one minute (return false)
	return timeSinceLastPing >= 60*time.Second
}

func (pd *PeerData) CanHePingMe() bool {

	lastPing := pd.LastGoodPing

	//fmt.Println("[PEER PING] last contact was ", lastPing)

	if lastPing.IsZero() {
		return true
	}

	timeSinceLastPing := time.Since(lastPing)
	//fmt.Println("[PEER PING] time since last contact is ", timeSinceLastPing)

	// TODO: justify the constant
	// if this is less than five seconds, he should not be pinging me (return false)
	// if he has pinged me already, and it has been more than 5s ago, he can ping me (return true)
	return timeSinceLastPing >= 5*time.Second
}

func (pd *PeerData) AddBasicInteraction(rating float64) {
	if pd.addBasicInteraction(rating, database.DBW.NetworkForPeer(pd.LastUsedIP)) {
		SharePeerDataUpdate(pd)
	}
}

// ObserveNetwork records the local network of an authenticated peer connection.
func (pd *PeerData) ObserveNetwork() {
	networkID := database.DBW.NetworkForPeer(pd.LastUsedIP)
	if networkID == "" {
		return
	}
	pd.interactionMu.Lock()
	defer pd.interactionMu.Unlock()
	pd.LastNetworkID = networkID
}

// addBasicInteraction records rating only while the peer belongs to networkID.
// Returns whether reliability changed; successful ratings establish membership.
func (pd *PeerData) addBasicInteraction(rating float64, networkID string) bool {
	pd.interactionMu.Lock()
	defer pd.interactionMu.Unlock()
	if networkID == "" || (rating <= 0 && networkID != pd.LastNetworkID) {
		return false
	}
	if rating > 0 {
		pd.LastNetworkID = networkID
	}
	// TODO change ping to include latency in score
	timestamp := time.Now()
	pd.BasicInteractions = append(pd.BasicInteractions, rating)
	pd.BasicInteractionTimes = append(pd.BasicInteractionTimes, timestamp)

	reliability := ComputeReliability(pd.BasicInteractions)
	if reliability == pd.Reliability {
		return false
	}
	pd.Reliability = reliability
	return true
}
