package main

import (
	"encoding/json"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"io/ioutil"
	"time"
)

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
