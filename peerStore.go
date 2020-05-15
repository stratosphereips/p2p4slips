package main

import (
	"encoding/json"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"io/ioutil"
	"strings"
	"time"
)

type PeerData struct {
	PeerID                string
	LastUsedIP            string
	Version               string
	Reliability           float64
	LastInteraction       time.Time
	LastGoodPing          time.Time
	LastMultiAddress      string
	BasicInteractions     []float64
	BasicInteractionTimes []time.Time
	// TODO: move all manipulation to getters and setters, which notify slips

}

func (pd *PeerData) checkAndUpdateActivePeerMultiaddr(multiAddress string){
	if multiAddress != pd.LastMultiAddress {
		fmt.Println("Updating multiaddr")
		// if addresses are different
		pd.LastMultiAddress = multiAddress
		remoteIP := strings.Split(multiAddress, "/")[2]
		fmt.Println("IP address changed", remoteIP)
		if pd.LastUsedIP != remoteIP {
			pd.LastUsedIP = remoteIP
			dbw.sharePeerDataUpdate(pd)
		}
	}
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
	if timeSinceLastPing < 60 * time.Second {
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
	if timeSinceLastPing < 5 * time.Second {
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

	// TODO: improve reliability computation
	// TODO: only share updates if there is something new
	pd.Reliability = average(pd.BasicInteractions)
	dbw.sharePeerDataUpdate(pd)
}

type PeerStore struct {
	store       peerstore.Peerstore
	saveFile    string
	allPeers    map[string]*PeerData
	activePeers map[string]*PeerData
}

func (ps *PeerStore) saveToFile (key crypto.PrivKey) error {
	if ps.saveFile == "" {
		fmt.Println("[PEERSTORE] No save file provided, data is not saved.")
		return nil
	}

	// save all data from peerstore to file, encrypted by private key

	marshaledPeerData, err := json.Marshal(ps.allPeers)
	// TODO: catch err

	// TODO: implement signing/encryption
	//r := rand.Reader
	//encryptedData, err := rsa.EncryptPKCS1v15(r, &key, marshaledPeerData)
	encryptedData := marshaledPeerData

	err = ioutil.WriteFile(ps.saveFile, encryptedData, 0600)
	if err != nil {
		// TODO: there is no error happening here, but data is not really associated with peers. It can not be found
		//  later when the peer reconnects
		fmt.Println("[PEERSTORE] PeerStore saving failed:", err)
		return err
	}

	return nil
}

func (ps *PeerStore) readFromFile (privateKey crypto.PrivKey) {
	ps.allPeers = make(map[string]*PeerData)
	ps.activePeers = make(map[string]*PeerData)

	if ps.saveFile == "" {
		fmt.Println("[PEERSTORE] Using empty peerstore")
		return
	}

	encryptedData, err := ioutil.ReadFile(ps.saveFile)

	if err != nil {
		fmt.Println("[PEERSTORE] PeerStore loading failed:", err)
		fmt.Println("[PEERSTORE] Using empty peerstore")
		return
	}

	// TODO: implement verification/decryption
	marshaledData := encryptedData

	err = json.Unmarshal(marshaledData, &ps.allPeers)

	if err != nil {
		fmt.Println("[PEERSTORE] PeerStore loading failed:", err)
		fmt.Println("[PEERSTORE] Using empty peerstore")
		return
	}

	fmt.Println("[PEERSTORE] Loaded peerstore")
	fmt.Println(ps.allPeers)
	fmt.Println("[PEERSTORE] --- END OF PEERSTORE DATA ---")
}

func (ps *PeerStore) activatePeer(peerId string) (peerData *PeerData, isNew bool) {

	// check if he is active already
	peerData, ok := ps.activePeers[peerId]
	if ok {
		peerData.LastInteraction = time.Now()
		dbw.sharePeerDataUpdate(peerData)
		return peerData, false
	}

	// check if he was contacted ever before
	peerData, ok = ps.allPeers[peerId]
	if ok {
		// if yes, update his info and move him to active peer list
		peerData.LastInteraction = time.Now()
		dbw.sharePeerDataUpdate(peerData)
		ps.activePeers[peerId] = peerData
		return peerData, false
	}

	// the peer is completely new, he should be created...
	peerData = ps.createNewPeer(peerId)
	dbw.sharePeerDataUpdate(peerData)

	return peerData, true
}

func (ps *PeerStore) deactivatePeer(peerId string) {
	delete(ps.activePeers, peerId)
}

func (ps *PeerStore) createNewPeer(peerId string) *PeerData {

	peerData := &PeerData{PeerID: peerId, LastInteraction: time.Now()}
	ps.activePeers[peerId] = peerData
	ps.allPeers[peerId] = peerData

	return peerData
}

func (ps *PeerStore) isActivePeer(peerId string) *PeerData {
	peerData, ok := ps.activePeers[peerId]
	if ok {
		return peerData
	}
	return nil
}

func (ps *PeerStore) isKnown(peerId string) *PeerData {
	peerData, ok := ps.activePeers[peerId]
	if ok {
		return peerData
	}
	peerData, ok = ps.allPeers[peerId]
	if ok {
		return peerData
	}
	return nil
}

func (ps *PeerStore) updatePeerVersion(peerId string, value string){
	ps.activePeers[peerId].Version = value
	dbw.sharePeerDataUpdate(ps.activePeers[peerId])
}

func average(xs []float64) float64 {
	total := 0.0
	for _,v := range xs {
		total += v
	}
	return total / float64(len(xs))
}
