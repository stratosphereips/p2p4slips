package utils

import (
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/host"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

// discoveredPeersBuffer is how many discovered peers can wait in the channel
// before the mDNS callback blocks. Discovery is bursty when several peers
// start at once, so a small buffer keeps the mDNS goroutine responsive.
const discoveredPeersBuffer = 16

type discoveryNotifee struct {
	PeerChan chan libp2ppeer.AddrInfo
}

// HandlePeerFound is called by the mDNS service whenever a new peer is found.
func (n *discoveryNotifee) HandlePeerFound(pi libp2ppeer.AddrInfo) {
	n.PeerChan <- pi
}

// MdnsServiceName turns the rendezvous string into a DNS-SD service name.
// go-libp2p uses the name verbatim as the mDNS service type, so it must have
// the "_<name>._udp" shape (the libp2p default is "_p2p._udp").
func MdnsServiceName(rendezvous string) string {
	if rendezvous == "" {
		return mdns.ServiceName
	}
	return fmt.Sprintf("_%s._udp", rendezvous)
}

// InitMDNS starts the mDNS discovery service on the given host. Peers found
// on the local network are delivered on the returned channel. The returned
// closer stops the service; the caller must close it on shutdown.
func InitMDNS(peerhost host.Host, rendezvous string) (chan libp2ppeer.AddrInfo, io.Closer, error) {
	n := &discoveryNotifee{}
	n.PeerChan = make(chan libp2ppeer.AddrInfo, discoveredPeersBuffer)

	ser := mdns.NewMdnsService(peerhost, MdnsServiceName(rendezvous), n)
	if err := ser.Start(); err != nil {
		fmt.Printf("[ERROR] Couldn't start host discovery - %s\n", err)
		return nil, nil, err
	}

	return n.PeerChan, ser, nil
}
