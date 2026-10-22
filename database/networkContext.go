package database

import (
	"encoding/json"
	"net"
	"strings"
)

type localNetworkState struct {
	Interface    string `json:"interface"`
	Connected    bool   `json:"connected"`
	HostIP       string `json:"host_ip"`
	LocalNetwork string `json:"local_network"`
	GatewayMAC   string `json:"gateway_mac"`
	Addresses    []struct {
		Network string `json:"network"`
	} `json:"addresses"`
}

// NetworkForPeer returns the current local network containing peerIP.
// Missing network identity or a disconnected capture interface pauses ratings.
func (dw *DBWrapper) NetworkForPeer(peerIP string) string {
	if dw == nil || dw.Rdb == nil {
		return ""
	}
	states, err := dw.Rdb.HGetAll("network_states").Result()
	if err != nil {
		return ""
	}
	addresses := make(map[string][]net.IP)
	for name := range states {
		iface, err := net.InterfaceByName(name)
		if err != nil || iface.Flags&net.FlagUp == 0 {
			continue
		}
		values, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, value := range values {
			ip, _, err := net.ParseCIDR(value.String())
			if err == nil {
				addresses[name] = append(addresses[name], ip)
			}
		}
	}
	return networkForPeer(states, addresses, peerIP)
}

// networkForPeer matches peerIP to live interface addresses and gateway identity.
// Returns an empty identity when the network cannot be established.
func networkForPeer(states map[string]string, addresses map[string][]net.IP, peerIP string) string {
	remote := net.ParseIP(strings.SplitN(peerIP, "%", 2)[0])
	if remote == nil || remote.IsLoopback() || remote.IsUnspecified() {
		return ""
	}
	for name, encoded := range states {
		var state localNetworkState
		if json.Unmarshal([]byte(encoded), &state) != nil || !state.Connected {
			continue
		}
		gatewayMAC := strings.TrimSpace(state.GatewayMAC)
		mac, err := net.ParseMAC(gatewayMAC)
		if err != nil {
			octets := strings.Split(gatewayMAC, ":")
			if len(octets) == 6 {
				for index, octet := range octets {
					if len(octet) == 1 {
						octets[index] = "0" + octet
					}
				}
				mac, err = net.ParseMAC(strings.Join(octets, ":"))
			}
		}
		if err != nil || state.LocalNetwork == "" {
			continue
		}
		hostPresent := false
		for _, address := range addresses[name] {
			if address.Equal(net.ParseIP(state.HostIP)) {
				hostPresent = true
				break
			}
		}
		if !hostPresent {
			continue
		}
		networks := []string{state.LocalNetwork}
		for _, address := range state.Addresses {
			networks = append(networks, address.Network)
		}
		for _, value := range networks {
			_, subnet, err := net.ParseCIDR(value)
			if err == nil && subnet.Contains(remote) {
				return mac.String() + "|" + state.LocalNetwork
			}
		}
	}
	return ""
}
