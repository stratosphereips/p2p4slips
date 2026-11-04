package peer

import (
	"net"

	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// localDialGater prevents discovery and learned peer addresses from dialing off-LAN.
type localDialGater struct {
	networks      func() []*net.IPNet
	allowLoopback bool
}

// connectedNetworks returns the currently up, non-loopback interface subnets.
func connectedNetworks() []*net.IPNet {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var networks []*net.IPNet
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, subnet, err := net.ParseCIDR(address.String())
			if err == nil && !ip.IsLoopback() && !ip.IsUnspecified() {
				networks = append(networks, subnet)
			}
		}
	}
	return networks
}

// allows checks one literal TCP endpoint against live directly connected subnets.
func (g *localDialGater) allows(address multiaddr.Multiaddr) bool {
	ipString, _, ok := multiaddrEndpoint(address)
	if !ok {
		return false
	}
	ip := net.ParseIP(ipString)
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	if ip.IsLoopback() {
		return g.allowLoopback
	}
	for _, subnet := range g.networks() {
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

// filter removes unreachable advertised addresses before choosing a saved endpoint.
func (g *localDialGater) filter(addresses []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	var allowed []multiaddr.Multiaddr
	for _, address := range addresses {
		if g.allows(address) {
			allowed = append(allowed, address)
		}
	}
	return allowed
}

// InterceptPeerDial lets the address policy decide once endpoints are resolved.
func (g *localDialGater) InterceptPeerDial(libp2ppeer.ID) bool { return true }

// InterceptAddrDial applies to every dial, including identify-learned addresses.
func (g *localDialGater) InterceptAddrDial(_ libp2ppeer.ID, address multiaddr.Multiaddr) bool {
	return g.allows(address)
}

// InterceptAccept leaves existing inbound authentication unchanged.
func (g *localDialGater) InterceptAccept(network.ConnMultiaddrs) bool { return true }

// InterceptSecured leaves existing authenticated connection handling unchanged.
func (g *localDialGater) InterceptSecured(network.Direction, libp2ppeer.ID, network.ConnMultiaddrs) bool {
	return true
}

// InterceptUpgraded allows connections that passed address gating and authentication.
func (g *localDialGater) InterceptUpgraded(network.Conn) (bool, control.DisconnectReason) {
	return true, 0
}
