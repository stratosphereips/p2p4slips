package mypeer

import (
	"fmt"
	"strings"
	"time"
)

type PeerData struct {
	peerID                string
	lastUsedIP            string
	version               string
	Reliability           float64
	LastInteraction       time.Time
	LastGoodPing          time.Time
	lastMultiAddress      string
	BasicInteractions     []float64
	BasicInteractionTimes []time.Time
	// TODO: move all manipulation to getters and setters, which notify slips

}

func (pd *PeerData) SetMultiaddr(multiAddress string) {
	if multiAddress == pd.lastMultiAddress {
		return
	}
	fmt.Println("Updating multiaddr")
	pd.lastMultiAddress = multiAddress

	remoteIP := strings.Split(multiAddress, "/")[2]
	fmt.Println("IP address changed", remoteIP)
	if pd.lastUsedIP != remoteIP {
		pd.lastUsedIP = remoteIP
		SharePeerDataUpdate(dbw, pd)
	}
}

func (pd *PeerData) SetVersion(value string) bool {
	if value == pd.version {
		return false
	}
	pd.version = value
	SharePeerDataUpdate(dbw, pd)
	return true
}

func (pd *PeerData) shouldIPingPeer() bool {
	lastPing := pd.LastGoodPing

	fmt.Println("[PEER PING] last contact was ", lastPing)

	if lastPing.IsZero() {
		return true
	}

	timeSinceLastPing := time.Since(lastPing)
	fmt.Println("[PEER PING] time since last contact is ", timeSinceLastPing)

	// TODO: justify the constant
	if timeSinceLastPing < 15*time.Second {
		// do not ping a peer that has been contacted less than five minutes ago
		return false
	}

	return true
}

func (pd *PeerData) shouldIDeactivatePeer() bool {
	lastPing := pd.LastGoodPing

	if lastPing.IsZero() {
		// if no ping ever happened, do not deactivate
		return false
	}

	timeSinceLastPing := time.Since(lastPing)
	fmt.Println("[PEER PING] time since last contact is ", timeSinceLastPing)

	// TODO: justify the constant
	if timeSinceLastPing < 60*time.Second {
		// if last contact was less than a minute ago, the peer should be considered active
		return false
	}

	// deactivate only if time since last (successful) ping is more than one minute
	return true
}

func (pd *PeerData) canHePingMe() bool {

	lastPing := pd.LastGoodPing

	fmt.Println("[PEER PING] last contact was ", lastPing)

	if lastPing.IsZero() {
		return true
	}

	timeSinceLastPing := time.Since(lastPing)
	fmt.Println("[PEER PING] time since last contact is ", timeSinceLastPing)

	// TODO: justify the constant
	if timeSinceLastPing < 5*time.Second {
		// if this is less than five seconds, he should not be pinging me
		return false
	}

	// if he has pinged me already, and it has been more than 5s ago, he can ping me
	return true
}

func (pd *PeerData) addBasicInteraction(rating float64) {
	// TODO change ping to include latency in score
	timestamp := time.Now()
	pd.BasicInteractions = append(pd.BasicInteractions, rating)
	pd.BasicInteractionTimes = append(pd.BasicInteractionTimes, timestamp)

	reliability := ComputeReliability(pd.BasicInteractions)
	if reliability == pd.Reliability {
		return
	}
	pd.Reliability = reliability
	SharePeerDataUpdate(dbw, pd)
}
