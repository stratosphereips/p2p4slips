package main

import (
	"encoding/json"
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"io/ioutil"
)

type PeerData struct {
	PeerID peer.ID
	LastUsedIP string
	Version string
	GoodCount int
}

type PeerStore struct {
	store peerstore.Peerstore
	saveFile string
}

func (ps *PeerStore) Experiment() {
	fmt.Println("[PEERSTORE - EXPERIMENT] peerstore", ps.store.Peers())

	for i, peerid := range ps.store.Peers() {
		fmt.Println("[PEERSTORE - EXPERIMENT] peer:", i, peerid)
	}

	peerid := ps.store.Peers()[0]

	fmt.Println(ps.store.PeerInfo(ps.store.Peers()[0]))
	err := ps.store.Put(peerid, "rep", Reputation{Ip: "1.2.3.4", Multiaddr:"multiaddr"})
	fmt.Println("[PEERSTORE - EXPERIMENT] put err:", err)

	iff, err := ps.store.Get(peerid, "rep")

	fmt.Println("[PEERSTORE - EXPERIMENT] rep from store:", iff, err)

	fmt.Println("[PEERSTORE - EXPERIMENT] Examining peer", peerid)
	fmt.Println(ps.store.GetProtocols(peerid))
	fmt.Println(ps.store.LatencyEWMA(peerid))
	fmt.Println(ps.store.PeerInfo(peerid))
}

//
//func (p *Peer) GetActivePeers() *map[string]*Reputation {
//	data := p.rdb.HGetAll(p.activePeers)
//	reputations := make(map[string]*Reputation)
//
//	for peerId, jsonData := range data.Val() {
//		reputations[peerId] = Json2rep(jsonData)
//	}
//
//	return &reputations
//}
//
//func (p *Peer) IsActivePeerIP(ipAddress string) *Reputation {
//	peers := p.GetActivePeers()
//
//	for _, rep := range *peers {
//		if (*rep).Ip == ipAddress {
//			return rep
//		}
//	}
//	return nil
//}
//
//func (p *Peer) IsPeerIP(ipAddress string) *Reputation {
//	peers := p.GetAllPeers()
//
//	for _, p := range *peers {
//		if p.Ip == ipAddress {
//			return p
//		}
//	}
//	return nil
//}

func (ps *PeerStore) saveToFile (key crypto.PrivKey) error {
	// save all data from peerstore to file, encrypted by private key
	peerData := ps.store.Peers()

	marshaledPeerData, err := json.Marshal(peerData)

	// TODO: catch err

	// TODO: implement signing/encryption
	//r := rand.Reader
	//encryptedData, err := rsa.EncryptPKCS1v15(r, &key, marshaledPeerData)
	encryptedData := marshaledPeerData

	err = ioutil.WriteFile(ps.saveFile, encryptedData, 0600)
	if err != nil {
		fmt.Println("[PEERSTORE] PeerStore saving failed:", err)
		return err
	}

	return nil
}

func (ps *PeerStore) readFromFile (privateKey crypto.PrivKey) {
	encryptedData, err := ioutil.ReadFile(ps.saveFile)

	if err != nil {
		fmt.Println("[PEERSTORE] PeerStore loading failed:", err)
		fmt.Println("[PEERSTORE] Using empty peerstore")
		return
	}

	// TODO: implement verification/decryption
	marshaledData := encryptedData

	var data peer.IDSlice
	err = json.Unmarshal(marshaledData, &data)

	if err != nil {
		fmt.Println("[PEERSTORE] PeerStore loading failed:", err)
		fmt.Println("[PEERSTORE] Using empty peerstore")
		return
	}

	fmt.Println("[PEERSTORE] Loaded peerstore")
	fmt.Println(data)
	fmt.Println("[PEERSTORE] --- END OF PEERSTORE DATA ---")
	// serialize object (add all addresses by hand)
}

func (ps *PeerStore) updatePeerIP(peerId peer.ID, value string){
	data := ps.getPeerData(peerId)
	data.LastUsedIP = value
	ps.updatePeerData(data)
}

func (ps *PeerStore) updatePeerVersion(peerId peer.ID, value string){
	data := ps.getPeerData(peerId)
	data.Version = value
	ps.updatePeerData(data)
}

func (ps *PeerStore) increaseGoodCount(peerId peer.ID) {
	data := ps.getPeerData(peerId)
	data.GoodCount = data.GoodCount + 1
	ps.updatePeerData(data)
}

func (ps *PeerStore) decreaseGoodCount(peerId peer.ID) {
	data := ps.getPeerData(peerId)
	data.GoodCount = data.GoodCount - 1
	ps.updatePeerData(data)
}

func (ps *PeerStore) updatePeerData(data *PeerData) {
	peerId := data.PeerID
	err := ps.store.Put(peerId, "PeerData", data)
	if err != nil {
		fmt.Printf("[PEERSTORE] Error updating data for peer %s, error %s\n", peerId, err)
	}
}

func (ps *PeerStore) getPeerData(id peer.ID) *PeerData {
	data, err := ps.store.Get(id, "PeerData")
	if err != nil {
		fmt.Printf("[PEERSTORE] Loading peer data failed for peer %s, error %s\n", id, err)
		return &PeerData{PeerID:id}
	}

	peerData, ok := data.(*PeerData)

	if !ok {
		fmt.Println("Parsing did not go ok")
	}

	fmt.Println("[PEERSTORE] Peer data", peerData)

	return peerData
}

func (ps *PeerData) getAllPeers() map[string]*PeerData {
	return nil
}
