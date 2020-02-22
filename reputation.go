package main

import (
	"encoding/json"
	"fmt"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"strings"
)

type Reputation struct {
	Peerid       string
	Multiaddr    string
	Ip           string
	Version      string
	Interactions []int
	Uptime       []int
}

func (r *Reputation) rep2json() string {
	byteData, err := json.Marshal(r)
	data := string(byteData)
	if err != nil {
		fmt.Println(err)
	}
	return data
}

func Json2rep(data string) *Reputation {
	r := Reputation{}
	json.Unmarshal([]byte(data), &r)
	return &r
}

func PeerAddress2rep(peerAddress peer.AddrInfo) *Reputation {
	multiAddress := peerAddress.String()
	multiAddressParsed := strings.Split(multiAddress, "/")

	r := Reputation{
		Peerid:       peerAddress.ID.Pretty(),
		Multiaddr:    multiAddress,
		Ip:		      multiAddressParsed[2],
		Version:      "",
		Interactions: nil,
		Uptime:       nil,
	}
	fmt.Println(r)
	return &r
}

func Stream2rep(stream network.Stream) *Reputation {
	multiAddress := stream.Conn().RemoteMultiaddr().String()
	multiAddressParsed := strings.Split(multiAddress, "/")
	r := Reputation{
		Peerid:       multiAddressParsed[5],
		Multiaddr:    multiAddress,
		Ip:		      multiAddressParsed[1],
		Version:      "",
		Interactions: nil,
		Uptime:       nil,
	}
	fmt.Println(r)
	return &r
}