package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery"
)

type discoveryNotifee struct {
	PeerChan chan libp2ppeer.AddrInfo
}

//interface to be called when new  peer is found
func (n *discoveryNotifee) HandlePeerFound(pi libp2ppeer.AddrInfo) {
	n.PeerChan <- pi
}

//Initialize the MDNS service
func InitMDNS(ctx context.Context, peerhost host.Host, rendezvous string) (chan libp2ppeer.AddrInfo, error) {
	// An hour might be a long long period in practical applications. But this is fine for us
	ser, err := discovery.NewMdnsService(ctx, peerhost, time.Hour, rendezvous)
	if err != nil {
		fmt.Printf("[ERROR] Couldn't start host discovery - %s\n", err)
		return nil, err
	}

	//register with service so that we get notified about peer discovery
	n := &discoveryNotifee{}
	n.PeerChan = make(chan libp2ppeer.AddrInfo)

	ser.RegisterNotifee(n)
	return n.PeerChan, nil
}
