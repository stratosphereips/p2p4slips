package peer

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"
	"github.com/stratosphereips/p2p4slips/database"
	"github.com/stratosphereips/p2p4slips/utils"
)

type Peer struct {
	host          host.Host
	port          int
	hostname      string
	protocol      string
	rendezVous    string
	ctx           context.Context
	peerstore     PeerStore
	privKey       crypto.PrivKey
	keyFile       string
	resetKey      bool
	peerstoreFile string
	closing       bool
	// mdns is the running peer discovery service, closed on shutdown
	mdns io.Closer
}

func NewPeer(cfg *utils.Config) *Peer {
	p := &Peer{
		port:          cfg.ListenPort,
		hostname:      cfg.ListenHost,
		protocol:      cfg.ProtocolID,
		rendezVous:    cfg.RendezvousString,
		peerstore:     PeerStore{},
		privKey:       nil,
		keyFile:       cfg.KeyFile,
		resetKey:      cfg.ResetKeys,
		peerstoreFile: cfg.PeerstoreFile,
		closing:       false,
	}
	return p
}

func (p *Peer) PeerInit(bootstrapPeers string) error {
	// prepare p2p host
	if err := p.p2pInit(p.keyFile, p.resetKey); err != nil {
		return err
	}

	p.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, connection network.Conn) {
			p.authenticateConnection(connection)
		},
		DisconnectedF: func(_ network.Network, connection network.Conn) {
			p.disconnectConnection(connection)
		},
	})

	// link to a listener for new connections
	p.host.SetStreamHandler(protocol.ID(p.protocol),
		p.listener)

	p.peerstore = PeerStore{Store: p.host.Peerstore(),
		SaveFile: p.peerstoreFile}
	p.peerstore.ReadFromFile(p.privKey)

	// run peer discovery in the background
	err := p.discoverPeers(bootstrapPeers)
	if err != nil {
		return err
	}

	go p.pingLoop()
	return nil
}

func (p *Peer) PeerShutdown() {
	p.stopDiscovery()
	if p.host == nil {
		// host creation failed, nothing to close
		return
	}
	if err := p.host.Close(); err != nil {
		fmt.Println("[PEER] Error closing host -", err)
	}
	fmt.Printf("[PEER] Closed port %d\n", p.port)
}

// stopDiscovery stops the mDNS service so it no longer announces this peer
// or delivers newly found peers. Safe to call more than once.
func (p *Peer) stopDiscovery() {
	if p.mdns == nil {
		return
	}
	if err := p.mdns.Close(); err != nil {
		fmt.Println("[PEER] Error stopping peer discovery -", err)
	}
	p.mdns = nil
}

func (p *Peer) p2pInit(keyFile string, keyReset bool) error {
	p.ctx = context.Background()

	prvKey := utils.LoadKey(keyFile, keyReset)

	p.privKey = prvKey

	if p.hostname == "" {
		// 0.0.0.0 will listen on any interface device.
		p.hostname = "0.0.0.0"
	}
	sourceMultiAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", p.hostname, p.port))
	if err != nil {
		fmt.Println("[PEER] Invalid listen address -", err)
		return err
	}

	// libp2p.New constructs a new libp2p Host.
	// Pigeon only ever talks over plain TCP on the local network, so the
	// host is limited to the TCP transport: slips records every
	// authenticated peer connection as a TCP 5-tuple, and relays, QUIC and
	// the other default transports would only add listeners it can't track.
	p.host, err = libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(prvKey),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.DisableRelay(),
		libp2p.DisableMetrics(),
	)

	if err != nil {
		fmt.Println("[PEER] P2P initialization failed -", err)
		return err
	}

	multiAddress := fmt.Sprintf("/ip4/%s/tcp/%v/p2p/%s", p.hostname, p.port, p.host.ID().String())
	fmt.Println("\n[*] Your Multiaddress Is: ", multiAddress)
	database.DBW.SaveMultiAddress(multiAddress)
	return nil
}

func (p *Peer) discoverPeers(bootstrapPeers string) error {
	fmt.Println("Looking for peers")

	peerChan, mdnsService, err := utils.InitMDNS(p.host, p.rendezVous)

	if err != nil {
		return err
	}
	p.mdns = mdnsService
	go func() {
		for {
			if p.closing {
				return
			}
			peerAddress := <-peerChan // will block until we discover a peerAddress
			peerId := peerAddress.ID.String()
			fmt.Println(">>> Found peerAddress:", peerId)

			if peerAddress.ID == p.host.ID() || len(peerAddress.Addrs) == 0 {
				// mDNS can report our own announcement, or a peer without
				// a usable address; neither can be contacted
				continue
			}

			wasActive := p.peerstore.IsActivePeer(peerId) != nil
			if p.host.Network().Connectedness(peerAddress.ID) != network.Connected {
				// Peerstore entries survive restarts, but their old TCP
				// connections do not. Redial known peers after discovery.
				ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
				err := p.host.Connect(ctx, peerAddress)
				cancel()
				if err != nil {
					fmt.Printf("[PEER] Connection to %s failed: %s\n", peerId, err)
					continue
				}
			}

			peerData, _ := p.peerstore.ActivatePeer(peerId)
			remoteMA := fmt.Sprintf("%s/p2p/%s", peerAddress.Addrs[0], peerId)
			peerData.SetMultiaddr(remoteMA)
			if !wasActive {
				go p.sayHello(peerData)
			}
		}
	}()
	for _, address := range strings.Split(bootstrapPeers, ",") {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		multiAddress, parseErr := multiaddr.NewMultiaddr(address)
		if parseErr != nil {
			fmt.Printf("[BOOTSTRAP] Invalid peer address %q: %s\n", address, parseErr)
			continue
		}
		peerAddress, parseErr := libp2ppeer.AddrInfoFromP2pAddr(multiAddress)
		if parseErr != nil {
			fmt.Printf("[BOOTSTRAP] Invalid peer identity in %q: %s\n", address, parseErr)
			continue
		}
		peerChan <- *peerAddress
	}
	return nil
}

func multiaddrEndpoint(address multiaddr.Multiaddr) (string, string, bool) {
	ip, err := address.ValueForProtocol(multiaddr.P_IP4)
	if err != nil {
		ip, err = address.ValueForProtocol(multiaddr.P_IP6)
	}
	if err != nil {
		return "", "", false
	}
	port, err := address.ValueForProtocol(multiaddr.P_TCP)
	if err != nil {
		return "", "", false
	}
	return ip, port, true
}

func authenticatedConnection(connection network.Conn) (database.AuthenticatedConnection, bool) {
	localIP, localPort, ok := multiaddrEndpoint(connection.LocalMultiaddr())
	if !ok {
		return database.AuthenticatedConnection{}, false
	}
	remoteIP, remotePort, ok := multiaddrEndpoint(connection.RemoteMultiaddr())
	if !ok {
		return database.AuthenticatedConnection{}, false
	}
	return database.AuthenticatedConnection{
		PeerID:     connection.RemotePeer().String(),
		Protocol:   "tcp",
		LocalIP:    localIP,
		LocalPort:  localPort,
		RemoteIP:   remoteIP,
		RemotePort: remotePort,
	}, true
}

func (p *Peer) authenticateConnection(connection network.Conn) {
	record, ok := authenticatedConnection(connection)
	if !ok {
		return
	}
	database.DBW.StoreAuthenticatedConnection(record)
}

func (p *Peer) refreshAuthenticatedConnection(connection network.Conn) {
	p.authenticateConnection(connection)
}

func (p *Peer) disconnectConnection(connection network.Conn) {
	record, ok := authenticatedConnection(connection)
	if !ok {
		return
	}
	database.DBW.RemoveAuthenticatedConnection(record)
}

func (p *Peer) listener(stream network.Stream) {
	defer p.closeStream(stream)

	remotePeer := stream.Conn().RemotePeer()
	remotePeerStr := remotePeer.String()
	remoteMA := fmt.Sprintf("%s/p2p/%s", stream.Conn().RemoteMultiaddr(), remotePeerStr)

	remotePeerData, _ := p.peerstore.ActivatePeer(remotePeerStr)
	remotePeerData.SetMultiaddr(remoteMA)

	// Create a buffer stream for non blocking read and write.
	reader := bufio.NewReader(stream)

	str, err := reader.ReadString('\n')

	if err != nil {
		fmt.Println("Error reading from buffer")
		remotePeerData.AddBasicInteraction(0)
		return
	}

	if len(str) == 0 {
		fmt.Println("Peer sent empty message")
		remotePeerData.AddBasicInteraction(0)
		return
	}

	// remove trailing newlines
	// fmt.Println("String:", str)
	// fmt.Println("Err:", err)
	str = str[:len(str)-1]

	commands := strings.Fields(str)

	if len(commands) == 0 {
		// peer sent empty message
		fmt.Println("[", remotePeer, "] sent an empty string")
		remotePeerData.AddBasicInteraction(0)
		return
	}

	if commands[0] == "hello" {
		p.refreshAuthenticatedConnection(stream.Conn())
		fmt.Println("[", remotePeer, "] New peer says hello to me")
		p.handleHello(remotePeerData, stream, &commands)
		return
	} else if str == "ping" {
		p.refreshAuthenticatedConnection(stream.Conn())
		dt := time.Now()
		//fmt.Println("[", remotePeer, "] says ping at ", dt.Format(time.UnixDate))
		fmt.Printf("[ %s ] says ping at %s \n", remotePeer, dt.Format(time.UnixDate))
		p.handlePing(remotePeerData, stream)
		return
	} else if str == "goodbye" {
		p.refreshAuthenticatedConnection(stream.Conn())
		fmt.Println("[", remotePeer, "] says goodbye")
		p.handleGoodbye(remotePeerData)
		return
	} else {
		//fmt.Println("[", remotePeer, "] sent an unknown message:", str)

		// log the received msg
		rawDecodedText, err := base64.StdEncoding.DecodeString(str)
		if err == nil {
			p.refreshAuthenticatedConnection(stream.Conn())
			fmt.Printf("Received from [ %s ] : %s\n", remotePeer, rawDecodedText)
		}

		//now forward this msg to slips p2p module to deal with it
		p.handleGenericMessage(remotePeerStr, str)
	}
}

func (p *Peer) sayHello(peerData *PeerData) {
	if p.closing {
		return
	}

	response, ok := p.sendMessageToPeerData(peerData, "hello version1\n", 10*time.Second)

	if !ok {
		return
	}

	fmt.Println("text:", response)

	command := strings.Fields(response)
	var remoteVersion string

	if len(command) != 2 || command[0] != "hello" {
		fmt.Println("Peer sent invalid hello reply")
		peerData.AddBasicInteraction(0)
		return
	} else {
		remoteVersion = command[1]
	}

	fmt.Println("PeerOld response ok, updating reputation")
	peerData.AddBasicInteraction(1)
	peerData.SetVersion(remoteVersion)
}

func (p *Peer) handleHello(remotePeerData *PeerData, stream network.Stream, command *[]string) {
	if p.closing {
		return
	}

	// parse contents of hello message
	var remoteVersion string
	if len(*command) != 2 {
		fmt.Println("Invalid Hello format")
		remotePeerData.AddBasicInteraction(0)
		return
	} else {
		remoteVersion = (*command)[1]
	}

	if !remotePeerData.SetVersion(remoteVersion) {
		// hello message should not be sent unless the version changed or the peer is unknown (changes version as well)
		fmt.Println("Peer sent unsolicited hello")
		remotePeerData.AddBasicInteraction(0)
	}

	_, ok := p.sendMessageToStream(stream, "hello version1\n", 0)

	if !ok {
		fmt.Println("Something went wrong when sending hello reply")
		remotePeerData.AddBasicInteraction(0)
		return
	}

	remotePeerData.AddBasicInteraction(1)
}

func (p *Peer) sendPing(remotePeerData *PeerData) {
	if p.closing {
		return
	}
	//fmt.Println("[PEER PING]")

	// check last ping time, if it was recently, do not ping at all
	if !remotePeerData.ShouldIPingPeer() {
		//fmt.Printf("[PEER PING] Peer %s was contacted recently, no need for ping\n", remotePeerData.PeerID)
		return
	}
	dt := time.Now()
	fmt.Printf("[PEER PING] Sending ping to [ %s ] at %s \n", remotePeerData.PeerID, dt.Format(time.UnixDate))
	response, ok := p.sendMessageToPeerData(remotePeerData, "ping\n", 10*time.Second)

	if response == "pong\n" && ok {
		remotePeerData.AddBasicInteraction(1)
		remotePeerData.LastGoodPing = time.Now()
		fmt.Printf("[PEER PING] Peer %s sent pong reply\n", remotePeerData.PeerID)
	} else {
		fmt.Printf("[PEER PING] Peer %s sent wrong pong reply (or none at all)\n", remotePeerData.PeerID)
		remotePeerData.AddBasicInteraction(0)
		if remotePeerData.ShouldIDeactivatePeer() {
			fmt.Printf("[PEER PING] It's been to long since the peer %s has been online, deactivating him\n", remotePeerData.PeerID)
			p.peerstore.DeactivatePeer(remotePeerData.PeerID)
		}
	}
}

func (p *Peer) handlePing(remotePeerData *PeerData, stream network.Stream) {
	if p.closing {
		return
	}
	//fmt.Println("[PEER PING] Received ping at \n", time.Now())
	//fmt.Println("[PEER PING] from %s\n ", remotePeerData.PeerID)

	rating := 1.0

	// is he not pinging me too early?
	if !remotePeerData.CanHePingMe() {
		fmt.Printf("[PEER PING REPLY] Peer %s is sending pings too often\n", remotePeerData.PeerID)
		rating = 0
	}

	// reply to ping
	//fmt.Printf("[PEER PING REPLY] Sending ping reply to %s\n", remotePeerData.PeerID)
	_, ok := p.sendMessageToStream(stream, "pong\n", 0)
	if !ok {
		fmt.Printf("[PEER PING REPLY] Something went wrong when sending ping reply to %s\n", remotePeerData.PeerID)
		rating = 0
	} else {
		remotePeerData.LastGoodPing = time.Now()
		fmt.Printf("[PEER PING REPLY] Ping reply successfully sent to %s\n", remotePeerData.PeerID)
	}

	remotePeerData.AddBasicInteraction(rating)
}

func (p *Peer) handleGoodbye(remotePeerData *PeerData) {
	p.peerstore.DeactivatePeer(remotePeerData.PeerID)
}

func (p *Peer) handleGenericMessage(peerID string, message string) {
	report := &ReportStruct{
		Reporter:  peerID,
		ReportTme: time.Now().Unix(),
		Message:   message,
	}

	ShareReport(report)
}

func (p *Peer) Close() {
	// set closing to true, to stop all outgoing communication, wait till everything is sent
	p.closing = true
	time.Sleep(1 * time.Second)

	// tell peers that this node is shutting down
	p.SendMessageToPeerId("goodbye\n", "*")
	// wait till the message is sent, otherwise the host is closed too early and sending fails
	time.Sleep(1 * time.Second)

	if err := p.peerstore.SaveToFile(p.privKey); err != nil {
		fmt.Println("[PEER] Peerstore was not saved -", err)
	}

	// shut the node down
	p.stopDiscovery()
	if err := p.host.Close(); err != nil {
		panic(err)
	}
}

func (p *Peer) pingLoop() {
	for {
		//fmt.Println("[LOOP] printing active peers:")
		for peerID := range p.peerstore.ActivePeers {
			peerData := p.peerstore.ActivePeers[peerID]
			fmt.Printf("[LOOP] Listing active peer: %s\n", peerID)
			p.sendPing(peerData)
		}
		//fmt.Println("[LOOP] printing all peers:")
		//for peerID := range p.peerstore.AllPeers {
		// 			peerData := p.peerstore.AllPeers[peerID]
		//fmt.Printf("[LOOP] Listing all peers %s\n", peerID)
		//}
		//fmt.Println("[LOOP] done, sleeping 10s")
		time.Sleep(10 * time.Second)
	}
}

// Send specified message to the provided peer. Return the peer's reply (string) and success (bool)
// All connection errors affect the peer's reliability, there is no need to update it based on the success bool
// peerData: data of the target peer
// message: the string to send to the target peer
// timeout: timeout to wait for reply. If timeout is set to 0, the stream is closed right after sending,
// without reading any replies.
// return response string: the response sent by the peer. Empty string if timeout is zero or if there were errors
// return success bool: true if everything went smoothly, false in case of errors (or no reply from peer)
func (p *Peer) sendMessageToPeerData(peerData *PeerData, message string, timeout time.Duration) (string, bool) {

	// log the sent msg
	rawDecodedText, err := base64.StdEncoding.DecodeString(message)
	if err == nil {
		fmt.Printf("Sent to [ %s ] : %s \n", peerData.PeerID, string(rawDecodedText))
	}
	//fmt.Println("sending ", message, " to:", peerData.PeerID)

	// open stream
	stream := p.openStreamFromPeerData(peerData)
	// close stream when this function exits (useful to have it here, since there are multiple returns)
	defer p.closeStream(stream)

	// give up if stream opening failed, lower peer's reliability
	if stream == nil {
		fmt.Println("Couldn't open the stream")
		peerData.AddBasicInteraction(0)
		return "", false
	}

	// send message to the stream, read response
	response, ok := p.sendMessageToStream(stream, message, timeout)

	// lower peer's reputation in case of errors
	if !ok {
		fmt.Println("Couldn't send the message")
		peerData.AddBasicInteraction(0)
	}

	return response, ok
}

// Open a stream to the remote peer. Return the stream, or nil in case of errors. Peer reliability is not modified.
// peerData: data of the target peer
// return stream network.Stream: a stream with the given peer, or nil in case of errors
func (p *Peer) openStreamFromPeerData(peerData *PeerData) network.Stream {
	//fmt.Printf("DEBUGGINGGG %+v\n", peerData)
	remoteMA := peerData.LastMultiAddress

	// new multiaddress from string
	multiaddress, err := multiaddr.NewMultiaddr(remoteMA)
	if err != nil {
		fmt.Printf("[OPEN STREAM] Error parsing multiaddress '%s': %s\n", remoteMA, err)
		return nil
	}

	// addrInfo from multiaddress
	remotePeer, err := libp2ppeer.AddrInfoFromP2pAddr(multiaddress)
	if err != nil {
		fmt.Println("[OPEN STREAM] Error creating addrInfo from multiaddress:", err)
		return nil
	}

	// make sure the host knows how to reach the peer even if the original
	// connection was dropped, so the stream can dial it again
	p.host.Peerstore().AddAddrs(remotePeer.ID, remotePeer.Addrs, peerstore.TempAddrTTL)

	// open stream
	stream, err := p.host.NewStream(p.ctx, remotePeer.ID, protocol.ID(p.protocol))
	if err != nil {
		fmt.Println("[OPEN STREAM] Opening stream failed")
		return nil
	}

	return stream
}

// Send specified message to the provided stream. Return the reply (string) and success (bool). Peer reliability is not modified.
// stream: stream to the target peer
// message: the string to send to the stream
// timeout: timeout to wait for reply. If timeout is set to 0, the function exits without reading any replies.
// return response string: the response read from the stream. Empty string if timeout is zero or if there were errors
// return success bool: true if everything went smoothly, false in case of errors (or no reply from peer)
func (p *Peer) sendMessageToStream(stream network.Stream, message string, timeout time.Duration) (response string, ok bool) {

	// open rw
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	// send message
	// fmt.Printf("Sending message: '%s'\n", message)
	if !send2rw(rw, message) {
		fmt.Println("error sending")
		return "", false
	}

	// if timeout is zero, end here
	p.refreshAuthenticatedConnection(stream.Conn())
	if timeout == 0 {
		return "", true
	}

	// collect the reply from the stream to a channel. Room for one message,
	// so the reader never blocks (and leaks) when this function has
	// already given up waiting
	output := make(chan string, 1)
	go rw2channel(output, rw)
	data := ""

	// wait for whichever process returns first: reading from the stream, or timeout
	select {
	case data = <-output:
		// peer sent something
		break
	case <-time.After(timeout):
		// peer didn't respond in time
		fmt.Println("timeout reached without msgs from peer")
		return data, false
	}

	return data, true
}

// Send message to the read writer. Return true if it went ok, false in case of errors
func send2rw(rw *bufio.ReadWriter, message string) bool {
	_, err := rw.WriteString(message)

	if err != nil {
		fmt.Println(err)
		return false
	}
	err = rw.Flush()
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}

// Read one reply line from the read writer and direct it to the channel.
// Every exchange with a peer is one message and one reply, so the reader
// stops after the first line; reading on would only hit the stream reset
// that closing the stream causes. The channel must have room for one
// message so the reader never blocks after the caller stopped waiting.
func rw2channel(input chan string, rw *bufio.ReadWriter) {
	result, err := rw.ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Println("Error while reading msg from peer: ", err)
	}
	if result == "" {
		// nothing usable arrived, let the caller time out
		return
	}
	input <- result
}

// close the stream nicely (no return value, since we don't care if it crashed - it is closed either way)
func (p *Peer) closeStream(stream network.Stream) {
	if stream == nil {
		// nil streams cause SIGSEGV errs when they are closed
		// fmt.Println("Stream is nil, not closing")
		return
	}
	err := stream.Close()
	if err != nil {
		fmt.Println("Error closing stream")
	}
}

// send a string message to a peer identified by peerId (or to all peers)
// If the given peerid doesn't exist, doesn't reply etc, it is skipped
// message: the string to send
// peerid: the peerid of the peer. Or * to broadcast to multiple peers
func (p *Peer) SendMessageToPeerId(message string, peerId string) {
	// the functions should:
	var contactList map[string]*PeerData

	// handle * as recipient
	if peerId == "*" {
		contactList = p.peerstore.ActivePeers
		// TODO: choose 50 peers
		// TODO: consider broadcasting
	} else {
		contactList = make(map[string]*PeerData)
		peerData := p.peerstore.IsActivePeer(peerId)

		if peerData == nil {
			fmt.Println("[PEER] peerid doesn't belong to any active peer: ", peerId)
			return
		}

		contactList[peerId] = peerData
	}

	for peerID := range contactList {
		peerData := contactList[peerID]

		go p.sendMessageToPeerData(peerData, message, 0)
	}
}
