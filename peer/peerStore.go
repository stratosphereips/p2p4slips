package peer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peerstore"
)

type PeerStore struct {
	Store       peerstore.Peerstore
	SaveFile    string
	AllPeers    map[string]*PeerData
	ActivePeers map[string]*PeerData
}

func (ps *PeerStore) SaveToFile(key crypto.PrivKey) error {
	if ps.SaveFile == "" {
		fmt.Println("[PEERSTORE] No save file provided, data is not saved.")
		return nil
	}

	// save all data from peerstore to file, encrypted by private key

	marshaledPeerData, err := json.Marshal(ps.AllPeers)
	if err != nil {
		fmt.Println("[PEERSTORE] PeerStore saving failed:", err)
		return err
	}

	// TODO: implement signing/encryption
	//r := rand.Reader
	//encryptedData, err := rsa.EncryptPKCS1v15(r, &key, marshaledPeerData)
	encryptedData := marshaledPeerData

	err = ioutil.WriteFile(ps.SaveFile, encryptedData, 0600)
	if err != nil {
		fmt.Println("[PEERSTORE] PeerStore saving failed:", err)
		return err
	}

	return nil
}

func (ps *PeerStore) ReadFromFile(privateKey crypto.PrivKey) {
	ps.AllPeers = make(map[string]*PeerData)
	ps.ActivePeers = make(map[string]*PeerData)

	if ps.SaveFile == "" {
		fmt.Println("[PEERSTORE] Using empty peerstore")
		return
	}

	encryptedData, err := ioutil.ReadFile(ps.SaveFile)

	if err != nil {
		fmt.Println("[PEERSTORE] PeerStore loading failed:", err)
		fmt.Println("[PEERSTORE] Using empty peerstore")
		return
	}

	// TODO: implement verification/decryption
	marshaledData := encryptedData

	err = json.Unmarshal(marshaledData, &ps.AllPeers)

	if err != nil {
		fmt.Println("[PEERSTORE] PeerStore loading failed:", err)
		fmt.Println("[PEERSTORE] Using empty peerstore")
		return
	}

	fmt.Println("[PEERSTORE] Loaded peerstore")
	fmt.Println(ps.AllPeers)
	fmt.Println("[PEERSTORE] --- END OF PEERSTORE DATA ---")
}

func (ps *PeerStore) ActivatePeer(peerId string) (peerData *PeerData, isNew bool) {

	// check if he is active already
	peerData, ok := ps.ActivePeers[peerId]
	if ok {
		peerData.LastInteraction = time.Now()
		SharePeerDataUpdate(peerData)
		return peerData, false
	}

	// check if he was contacted ever before
	peerData, ok = ps.AllPeers[peerId]
	if ok {
		// if yes, update his info and move him to active peer list
		peerData.LastInteraction = time.Now()
		SharePeerDataUpdate(peerData)
		ps.ActivePeers[peerId] = peerData
		return peerData, false
	}

	// the peer is completely new, he should be created...
	peerData = ps.CreateNewPeer(peerId)
	SharePeerDataUpdate(peerData)

	return peerData, true
}

func (ps *PeerStore) DeactivatePeer(peerId string) {
	delete(ps.ActivePeers, peerId)
}

func (ps *PeerStore) CreateNewPeer(peerId string) *PeerData {

	peerData := &PeerData{PeerID: peerId, LastInteraction: time.Now()}
	ps.ActivePeers[peerId] = peerData
	ps.AllPeers[peerId] = peerData

	return peerData
}

func (ps *PeerStore) IsActivePeer(peerId string) *PeerData {
	peerData, ok := ps.ActivePeers[peerId]
	if ok {
		return peerData
	}
	return nil
}

func (ps *PeerStore) IsKnown(peerId string) *PeerData {
	peerData, ok := ps.ActivePeers[peerId]
	if ok {
		return peerData
	}
	peerData, ok = ps.AllPeers[peerId]
	if ok {
		return peerData
	}
	return nil
}
