package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/stratosphereips/p2p4slips/mypeer"
)

type Peer struct {
	host          host.Host
	port          int
	hostname      string
	protocol      string
	rendezVous    string
	ctx           context.Context
	peerstore     mypeer.PeerStore
	privKey       crypto.PrivKey
	keyFile       string
	resetKey      bool
	peerstoreFile string
	closing       bool
}

func NewPeer(cfg *config) *Peer {
	p := &Peer{
		port:          cfg.listenPort,
		hostname:      cfg.listenHost,
		protocol:      cfg.ProtocolID,
		rendezVous:    cfg.RendezvousString,
		peerstore:     mypeer.PeerStore{},
		privKey:       nil,
		keyFile:       cfg.keyFile,
		resetKey:      cfg.resetKeys,
		peerstoreFile: cfg.peerstoreFile,
		closing:       false,
	}
	return p
}

func (p *Peer) peerInit() error {
	// prepare p2p host
	p.p2pInit(p.keyFile, p.resetKey)

	// link to a listener for new connections
	p.host.SetStreamHandler(protocol.ID(p.protocol), p.listener)

	p.peerstore = mypeer.PeerStore{Store: p.host.Peerstore(), SaveFile: p.peerstoreFile}
	p.peerstore.ReadFromFile(p.privKey)

	// run peer discovery in the background
	err := p.discoverPeers()
	if err != nil {
		return err
	}

	go p.pingLoop()
	return nil
}

func (p *Peer) p2pInit(keyFile string, keyReset bool) error {
	p.ctx = context.Background()

	prvKey := p.loadKey(keyFile, keyReset)

	p.privKey = prvKey

	// 0.0.0.0 will listen on any interface device.
	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", p.hostname, p.port))

	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here.
	var err error
	p.host, err = libp2p.New(
		p.ctx,
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(prvKey),
	)

	if err != nil {
		fmt.Println("[PEER] P2P initialization failed -", err)
		return err
	}

	fmt.Printf("\n[*] Your Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n", p.hostname, p.port, p.host.ID().Pretty())
	return nil
}

func (p *Peer) discoverPeers() error {
	fmt.Println("Looking for peers")

	peerChan, err := initMDNS(p.ctx, p.host, p.rendezVous)

	if err != nil {
		return err
	}
	go func() {
		for {
			if p.closing {
				return
			}
			peerAddress := <-peerChan // will block until we discover a peerAddress
			peerId := peerAddress.ID.Pretty()
			fmt.Println(">>> Found peerAddress:", peerId)

			peerData, isNew := p.peerstore.ActivatePeer(peerId)

			if isNew {
				// unknown peer
				fmt.Println("This is a new node, contacting him...")

				if err := p.host.Connect(p.ctx, peerAddress); err != nil {
					fmt.Println("Connection failed:", err)
					fmt.Println("Please make sure that the port of this Node is not used by other processes")
					peerData.addBasicInteraction(0)
					return
				}

				remoteMA := fmt.Sprintf("%s/p2p/%s", peerAddress.Addrs[0], peerId)
				peerData.SetMultiaddr(remoteMA)

				go p.sayHello(peerData)

				// sleep, because a new peer might be found twice, and we want to save him before the second message is read
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()
	return nil
}

func (p *Peer) listener(stream network.Stream) {
	defer p.closeStream(stream)

	remotePeer := stream.Conn().RemotePeer()
	remotePeerStr := remotePeer.Pretty()
	remoteMA := fmt.Sprintf("%s/p2p/%s", stream.Conn().RemoteMultiaddr(), remotePeerStr)

	remotePeerData, _ := p.peerstore.ActivatePeer(remotePeerStr)
	remotePeerData.SetMultiaddr(remoteMA)

	// Create a buffer stream for non blocking read and write.
	reader := bufio.NewReader(stream)

	str, err := reader.ReadString('\n')

	if err != nil {
		fmt.Println("Error reading from buffer")
		remotePeerData.addBasicInteraction(0)
		return
	}

	if len(str) == 0 {
		fmt.Println("Peer sent empty message")
		remotePeerData.addBasicInteraction(0)
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
		remotePeerData.addBasicInteraction(0)
		return
	}

	if commands[0] == "hello" {
		fmt.Println("[", remotePeer, "] New peer says hello to me")
		p.handleHello(remotePeerData, stream, &commands)
		return
	} else if str == "ping" {
		fmt.Println("[", remotePeer, "] says ping")
		p.handlePing(remotePeerData, stream)
		return
	} else if str == "goodbye" {
		fmt.Println("[", remotePeer, "] says goodbye")
		p.handleGoodbye(remotePeerData)
		return
	} else {
		fmt.Println("[", remotePeer, "] sent an unknown message:", str)
		p.handleGenericMessage(remotePeerStr, str)
	}
}

func (p *Peer) sayHello(peerData *mypeer.PeerData) {
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

func (p *Peer) handleHello(remotePeerData *peer.PeerData, stream network.Stream, command *[]string) {
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
		remotePeerData.addBasicInteraction(0)
	}

	_, ok := p.sendMessageToStream(stream, "hello version1\n", 0)

	if !ok {
		fmt.Println("Something went wrong when sending hello reply")
		remotePeerData.addBasicInteraction(0)
		return
	}

	remotePeerData.addBasicInteraction(1)
}

func (p *Peer) sendPing(remotePeerData *mypeer.PeerData) {
	if p.closing {
		return
	}
	fmt.Println("[PEER PING]")

	// check last ping time, if it was recently, do not ping at all
	if !remotePeerData.ShouldIPingPeer() {
		fmt.Println("[PEER PING] Peer was contacted recently, no need for ping")
		return
	}

	fmt.Println("[PEER PING] Sending ping at", time.Now())
	response, ok := p.sendMessageToPeerData(remotePeerData, "ping\n", 10*time.Second)

	if response == "pong\n" && ok {
		remotePeerData.AddBasicInteraction(1)
		remotePeerData.LastGoodPing = time.Now()
		fmt.Println("[PEER PING] Peer reply ok")
	} else {
		fmt.Println("[PEER PING] Peer sent wrong pong reply (or none at all)")
		remotePeerData.AddBasicInteraction(0)
		if remotePeerData.ShouldIDeactivatePeer() {
			fmt.Println("[PEER PING] It's been to long since the peer has been online, deactivating him")
			p.peerstore.DeactivatePeer(remotePeerData.peerID)
		}
	}
}

func (p *Peer) handlePing(remotePeerData *mypeer.PeerData, stream network.Stream) {
	if p.closing {
		return
	}
	fmt.Println("[PEER PING] Received ping at ", time.Now())
	rating := 1.0

	// is he not pinging me too early?
	if !remotePeerData.CanHePingMe() {
		fmt.Println("[PEER PING REPLY] Peer is sending pings too often")
		rating = 0
	}

	// reply to ping
	fmt.Println("[PEER PING REPLY] Sending ping reply")
	_, ok := p.sendMessageToStream(stream, "pong\n", 0)
	if !ok {
		fmt.Println("[PEER PING REPLY] Something went wrong when sending ping reply")
		rating = 0
	} else {
		remotePeerData.LastGoodPing = time.Now()
	}

	remotePeerData.AddBasicInteraction(rating)
}

func (p *Peer) handleGoodbye(remotePeerData *mypeer.PeerData) {
	p.peerstore.DeactivatePeer(remotePeerData.peerID)
}

func (p *Peer) handleGenericMessage(peerID string, message string) {
	report := &ReportStruct{
		Reporter:  peerID,
		ReportTme: time.Now().Unix(),
		Message:   message,
	}

	dbw.ShareReport(report)
}

func (p *Peer) close() {
	// set closing to true, to stop all outgoing communication, wait till everything is sent
	p.closing = true
	time.Sleep(1 * time.Second)

	// tell peers that this node is shutting down
	p.sendMessageToPeerId("goodbye\n", "*")
	// wait till the message is sent, otherwise the host is closed too early and sending fails
	time.Sleep(1 * time.Second)

	p.peerstore.SaveToFile(p.privKey)

	// shut the node down
	if err := p.host.Close(); err != nil {
		panic(err)
	}
}

func (p *Peer) pingLoop() {
	for {
		fmt.Println("[LOOP] printing active peers:")
		for peerID := range p.peerstore.ActivePeers {
			peerData := p.peerstore.ActivePeers[peerID]
			fmt.Printf("[LOOP] peer %s: %d\n", peerID, peerData.BasicInteractions)
			p.sendPing(peerData)
		}
		fmt.Println("[LOOP] printing all peers:")
		for peerID := range p.peerstore.allPeers {
			peerData := p.peerstore.allPeers[peerID]
			fmt.Printf("[LOOP] peer %s: %d\n", peerID, peerData.BasicInteractions)
		}
		fmt.Println("[LOOP] done, sleeping 10s")
		time.Sleep(10 * time.Second)
	}
}

// Send specified message to the provided peer. Return the peer's reply (string) and success (bool)
// All connection errors affect the peer's reliability, there is no need to update it based on the success bool
// peerData: data of the target peer
// message: the string to send to the target peer
// timeout: timeout to wait for reply. If timeout is set to 0, the stream is closed right after sending, without reading any replies.
// return response string: the response sent by the peer. Empty string if timeout is zero or if there were errors
// return success bool: true if everything went smoothly, false in case of errors (or no reply from peer)
func (p *Peer) sendMessageToPeerData(peerData *PeerData, message string, timeout time.Duration) (string, bool) {
	fmt.Println("sending ", message, " to:", peerData.peerID)
	// open stream
	stream := p.openStreamFromPeerData(peerData)
	// close stream when this function exits (useful to have it here, since there are multiple returns)
	defer p.closeStream(stream)

	// give up if stream opening failed, lower peer's reliability
	if stream == nil {
		fmt.Println("Couldn't open the stream")
		peerData.addBasicInteraction(0)
		return "", false
	}

	// send message to the stream, read response
	response, ok := p.sendMessageToStream(stream, message, timeout)

	// lower peer's reputation in case of errors
	if !ok {
		fmt.Println("Couldn't send the message")
		peerData.addBasicInteraction(0)
	}

	return response, ok
}

// Open a stream to the remote peer. Return the stream, or nil in case of errors. Peer reliability is not modified.
// peerData: data of the target peer
// return stream network.Stream: a stream with the given peer, or nil in case of errors
func (p *Peer) openStreamFromPeerData(peerData *PeerData) network.Stream {
	remoteMA := peerData.lastMultiAddress

	// new multiaddress from string
	multiaddress, err := multiaddr.NewMultiaddr(remoteMA)
	if err != nil {
		fmt.Printf("[OPEN STREAM] Error parsing multiaddress '%s': %s\n", remoteMA, err)
		return nil
	}

	// addrInfo from multiaddress
	remotePeer, err := peer.AddrInfoFromP2pAddr(multiaddress)
	if err != nil {
		fmt.Println("[OPEN STREAM] Error creating addrInfo from multiaddress:", err)
		return nil
	}

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
	if timeout == 0 {
		return "", true
	}

	// collect reply from the stream to a channel
	output := make(chan string)
	go rw2channel(output, rw)
	data := ""

	// wait for whichever process returns first: reading from the stream, or timeout
	select {
	case data = <-output:
		// peer sent something
		break
	case <-time.After(timeout):
		// peer didn't respond in time
		fmt.Println("timeout")
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

// Read messages from read writer and direct them to a channel
func rw2channel(input chan string, rw *bufio.ReadWriter) {
	for {
		result, err := rw.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			fmt.Println("err on rw2channel:", err)
		}

		input <- result
	}
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
func (p *Peer) sendMessageToPeerId(message string, peerId string) {
	// the functions should:
	var contactList map[string]*mypeer.PeerData

	// handle * as recipient
	if peerId == "*" {
		contactList = p.peerstore.ActivePeers
		// TODO: choose 50 peers
		// TODO: consider broadcasting
	} else {
		contactList = make(map[string]*mypeer.PeerData)
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

	return
}
