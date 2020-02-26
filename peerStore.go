package main

import (
	"encoding/json"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"io/ioutil"
)

type PeerData struct {
	PeerID     string
	LastUsedIP string
	Version    string
	GoodCount  int
}

type PeerStore struct {
	store       peerstore.Peerstore
	saveFile    string
	allPeers    map[string]*PeerData
	activePeers map[string]*PeerData
}

func (ps *PeerStore) saveToFile (key crypto.PrivKey) error {
	// save all data from peerstore to file, encrypted by private key

	// join two peer maps together, overwriting the old data with newer stats:
	for peerId, peerData := range ps.activePeers {
		ps.allPeers[peerId] = peerData
	}

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


func (ps *PeerStore) isActivePeer(peerId string) *PeerData {
	peerData, ok := ps.activePeers[peerId]
	if ok {
		return peerData
	}
	return nil
}

func (ps *PeerStore) isKnown(peerId string) *PeerData {
	peerData, ok := ps.allPeers[peerId]
	if ok {
		return peerData
	}
	peerData, ok = ps.activePeers[peerId]
	if ok {
		return peerData
	}
	return nil
}

func (ps *PeerStore) updatePeerIP(peerId string, value string){
	ps.activePeers[peerId].LastUsedIP = value
}

func (ps *PeerStore) updatePeerVersion(peerId string, value string){
	ps.activePeers[peerId].Version = value
}

func (ps *PeerStore) increaseGoodCount(peerId string) {
	ps.activePeers[peerId].GoodCount = ps.activePeers[peerId].GoodCount + 1
}

func (ps *PeerStore) decreaseGoodCount(peerId string) {
	ps.activePeers[peerId].GoodCount = ps.activePeers[peerId].GoodCount - 1
}

//func (ps *PeerStore) updatePeerIP(peerId peer.ID, value string){
//	data := ps.getPeerData(peerId)
//	data.LastUsedIP = value
//	ps.updatePeerData(data)
//}
//
//func (ps *PeerStore) updatePeerVersion(peerId peer.ID, value string){
//	data := ps.getPeerData(peerId)
//	data.Version = value
//	ps.updatePeerData(data)
//}
//
//func (ps *PeerStore) increaseGoodCount(peerId peer.ID) {
//	data := ps.getPeerData(peerId)
//	data.GoodCount = data.GoodCount + 1
//	ps.updatePeerData(data)
//}
//
//func (ps *PeerStore) decreaseGoodCount(peerId peer.ID) {
//	data := ps.getPeerData(peerId)
//	data.GoodCount = data.GoodCount - 1
//	ps.updatePeerData(data)
//}
//
//func (ps *PeerStore) updatePeerData(data *PeerData) {
//	peerId := data.PeerID
//	err := ps.store.Put(peerId, "PeerData", data)
//	if err != nil {
//		fmt.Printf("[PEERSTORE] Error updating data for peer %s, error %s\n", peerId, err)
//	}
//}
//
//func (ps *PeerStore) getPeerData(id peer.ID) *PeerData {
//	data, err := ps.store.Get(id, "PeerData")
//	if err != nil {
//		fmt.Printf("[PEERSTORE] Loading peer data failed for peer %s, error %s\n", id, err)
//		return &PeerData{PeerID:id}
//	}
//
//	peerData, ok := data.(*PeerData)
//
//	if !ok {
//		fmt.Println("Parsing did not go ok")
//	}
//
//	fmt.Println("[PEERSTORE] Peer data", peerData)
//
//	return peerData
//}
//
//func (ps *PeerStore) getAllPeers() map[string]*PeerData {
//	allPeers := make(map[string]*PeerData)
//	for _, peerId := range ps.store.Peers() {
//		allPeers[peerId.Pretty()] = ps.getPeerData(peerId)
//	}
//	return allPeers
//}