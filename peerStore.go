package main

import (
	"fmt"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peerstore"
)

type PeerStore struct {
	store peerstore.Peerstore
}

func (ps *PeerStore) Experiment() {
	fmt.Println("peerstore", ps.store.Peers())

	for i, peerid := range ps.store.Peers() {
		fmt.Println("peer:", i, peerid)
	}

	peerid := ps.store.Peers()[0]

	fmt.Println(ps.store.PeerInfo(ps.store.Peers()[0]))
	err := ps.store.Put(peerid, "rep", Reputation{Ip: "1.2.3.4", Multiaddr:"multiaddr"})
	fmt.Println("put err:", err)

	iff, err := ps.store.Get(peerid, "rep")

	fmt.Println("rep from store:", iff, err)

	fmt.Println("Examining peer", peerid)
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

func (ps *PeerStore) saveToFile (filename string) {
	// save all data from peerstore to file, encrypted by private key
	// permissions 0444 (readonly)
}

func (ps *PeerStore) readFromFile (filename string, privateKey crypto.PrivKey) {
	// read from file
	// decrypt
	// serialize object (add all addresses by hand)
}
