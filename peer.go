package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v7"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/multiformats/go-multiaddr"
	"io"
	"os"
	"strings"
	"time"
)

type Peer struct {
	rdb           *redis.Client
	rdbGoPy       string
	activePeers   string
	allPeers      string
	host          host.Host
	port          int
	hostname      string
	protocol      string
	dbAddress     string
	rendezVous    string
	ctx           context.Context
	peerstore     PeerStore
	privKey       crypto.PrivKey
	keyFile       string
	resetKey      bool
	peerstoreFile string
}

type Report struct {
	Reporter  string `json:"reporter"`
	ReportTme int64  `json:"report_time"`
	Message   string `json:"message"`
}

func (p *Peer) peerInit() error {
	err := p.redisInit()
	if err != nil {
		fmt.Println("[PEER] Database connection failed -", err)
		return err
	}

	// prepare p2p host
	p.p2pInit(p.keyFile, p.resetKey)

	p.activePeers = p.host.ID().Pretty() + "-active"
	p.allPeers = p.host.ID().Pretty() + "-all"

	// link to a listener for new connections
	p.host.SetStreamHandler(protocol.ID(p.protocol), p.listener)

	p.peerstore = PeerStore{store: p.host.Peerstore(), saveFile: p.peerstoreFile}
	p.peerstore.readFromFile(p.privKey)

	// run peer discovery in the background
	err = p.discoverPeers()
	if err != nil {
		return err
	}

	go p.pingLoop()
	return nil
}

func (p *Peer) redisInit() error {
	// connect to the database
	p.rdb = redis.NewClient(&redis.Options{
		Addr:     p.dbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// delete db
	// TODO: remove for production
	p.rdb.FlushAll()

	pongErr := p.rdb.Ping().Err()

	if pongErr != nil {
		fmt.Println("[PEER] Database connection failed -", pongErr)
	}

	return pongErr
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
			peerAddress := <-peerChan // will block until we discover a peerAddress
			peerId := peerAddress.ID.Pretty()
			fmt.Println(">>> Found peerAddress:", peerId)

			var remotePeerData *PeerData

			// active peer
			if remotePeerData = p.peerstore.isActivePeer(peerId); remotePeerData != nil {
				fmt.Println("I know him and he is active, skipping")
				continue
			}

			// known but inactive peer
			if remotePeerData = p.peerstore.isKnown(peerId); remotePeerData != nil {
				fmt.Println("I know him, but we've met a long time ago, skipping")
				// add him to the active peers list
				p.peerstore.activePeers[peerId] = remotePeerData
				continue
			}

			// unknown peer
			fmt.Println("This is a new node, contacting him...")
			go p.sayHello(peerAddress)

			// sleep, because a new peer might be found twice, and we want to save him before the second message is read
			time.Sleep(2 * time.Second)
		}
	}()
	return nil
}

func (p *Peer) setPeerOffline(peerid string) {
	// check if he is in all
	// if not, copy him from active
	// update info in all
	// delete from active
}

func (p *Peer) setPeerOnline(peerid string) {
	// get data from all
	// copy data to active
}

func (p *Peer) talker(rw *bufio.ReadWriter) {
	stdReader := bufio.NewReader(os.Stdin)
	fmt.Println("I am talking now")

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from stdin")
			panic(err)
		}

		_, err = rw.WriteString(fmt.Sprintf("%s\n", sendData))
		if err != nil {
			fmt.Println("Error writing to buffer")
			panic(err)
		}
		err = rw.Flush()
		if err != nil {
			fmt.Println("Error flushing buffer")
			panic(err)
		}
	}
}

func (p *Peer) listener(stream network.Stream) {
	defer p.closeStream(stream)

	remotePeer := stream.Conn().RemotePeer()
	remotePeerStr := remotePeer.Pretty()
	remoteMA := fmt.Sprintf("%s/p2p/%s", stream.Conn().RemoteMultiaddr(), remotePeerStr)

	var remotePeerData *PeerData
	remotePeerData = p.peerstore.isKnownWithUpdate(remotePeerStr)
	if remotePeerData == nil {
		// add a simple record into db
		remotePeerData = &PeerData{PeerID: remotePeerStr}
		p.peerstore.activePeers[remotePeerStr] = remotePeerData
	}
	remotePeerData.checkAndUpdateActivePeerMultiaddr(remoteMA)

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
		p.handleHello(remotePeerStr, remotePeerData, stream, &commands)
		return
	} else if str == "ping" {
		fmt.Println("[", remotePeer, "] says ping")
		p.handlePing(remotePeerStr, remotePeerData, stream)
		return
	} else {
		fmt.Println("[", remotePeer, "] sent an unknown message:", str)
		p.handleGenericMessage(remotePeerStr, str)
	}

}

func (p *Peer) sayHello(peerAddress peer.AddrInfo) {
	// add a simple record into db
	remotePeer := peerAddress.ID
	remotePeerStr := remotePeer.Pretty()
	remoteMA := fmt.Sprintf("%s/p2p/%s", peerAddress.Addrs[0], remotePeerStr)

	var remotePeerData *PeerData
	// add a simple record into db
	remotePeerData = &PeerData{PeerID: remotePeerStr}
	p.peerstore.addNewPeer(remotePeerData)
	remotePeerData.checkAndUpdateActivePeerMultiaddr(remoteMA)

	if err := p.host.Connect(p.ctx, peerAddress); err != nil {
		fmt.Println("Connection failed:", err)
		remotePeerData.addBasicInteraction(0)
		return
	}

	response, ok := p.sendMessageToPeerData(remotePeerData, "hello version1\n", 10 * time.Second)

	if !ok {
		return
	}

	fmt.Println("text:", response)

	command := strings.Fields(response)
	var remoteVersion string

	if len(command) != 2 || command[0] != "hello" {
		fmt.Println("Peer sent invalid hello reply")
		remotePeerData.addBasicInteraction(0)
		return
	} else {
		remoteVersion = command[1]
	}

	fmt.Println("PeerOld response ok, updating reputation")
	remotePeerData.addBasicInteraction(1)
	p.peerstore.updatePeerVersion(remotePeerStr, remoteVersion)
}

func (p *Peer) handleHello(remotePeerStr string, remotePeerData *PeerData, stream network.Stream, command *[]string) {

	// parse contents of hello message
	var remoteVersion string
	if len(*command) != 2 {
		fmt.Println("Invalid Hello format")
		remotePeerData.addBasicInteraction(0)
		return
	} else {
		remoteVersion = (*command)[1]
	}

	if remotePeerData.Version != remoteVersion {
		// update this info
		fmt.Println("updating version info")
		p.peerstore.updatePeerVersion(remotePeerStr, remoteVersion)
	} else {
		// hello message should not be sent unless the version changed or the peer is unknown (changes version as well)
		fmt.Println("Peer sent unsolicited hello")
		remotePeerData.addBasicInteraction(0)
	}

	// say hello
	fmt.Println("Sending hello reply...")

	_, ok := p.sendMessageToStream(stream, "hello version1\n", 0)

	if !ok {
		fmt.Println("Something went wrong when sending hello reply")
		remotePeerData.addBasicInteraction(0)
		return
	}

	remotePeerData.addBasicInteraction(1)
}

func (p *Peer) sendPing(remotePeerData *PeerData) {
	// check last ping time, if it was recently, do not ping at all
	if !remotePeerData.shouldIPingPeer() {
		fmt.Println("Peer was contacted recently, no need for ping")
		return
	}

	response, ok := p.sendMessageToPeerData(remotePeerData, "ping\n", 10 * time.Second)

	if !ok {
		return
	}

	fmt.Printf("ping response: '%s'\n", response)

	if response == "pong\n" {
		remotePeerData.addBasicInteraction(1)
	} else {
		fmt.Println("Peer sent wrong pong reply")
		remotePeerData.addBasicInteraction(0)
	}
}

func (p *Peer) handlePing(remotePeerStr string, remotePeerData *PeerData, stream network.Stream) {
	// did he send hello before pinging?
	if remotePeerData.Version == "" {
		fmt.Println("Peer is sending pings before saying hello")
		remotePeerData.addBasicInteraction(0)
	}

	// is he not pinging me too early?
	if !remotePeerData.shouldIPingPeer() {
		fmt.Println("Peer is sending pings too ofter")
		remotePeerData.addBasicInteraction(0)
	}

	// reply to ping
	fmt.Println("Sending ping reply...")
	_, ok := p.sendMessageToStream(stream, "pong\n", 0)
	if !ok {
		fmt.Println("Something went wrong when sending ping reply")
		remotePeerData.addBasicInteraction(0)
		return
	}

	remotePeerData.addBasicInteraction(1)
}

func (p *Peer) handleGenericMessage(peerID string, message string) {
	report := Report{
		Reporter:  peerID,
		ReportTme: time.Now().Unix(),
		Message:   message,
	}

	reportString, err := json.Marshal(report)

	if err != nil {
		fmt.Println(err)
		return
	}

	p.rdb.Publish(p.rdbGoPy, reportString)
}

func (p *Peer) GetActivePeers() *map[string]*Reputation {
	data := p.rdb.HGetAll(p.activePeers)
	reputations := make(map[string]*Reputation)

	for peerId, jsonData := range data.Val() {
		reputations[peerId] = Json2rep(jsonData)
	}

	return &reputations
}

func (p *Peer) GetAllPeers() *map[string]*Reputation {

	data := p.rdb.HGetAll(p.allPeers)
	reputations := make(map[string]*Reputation)

	for peerId, jsonData := range data.Val() {
		reputations[peerId] = Json2rep(jsonData)
	}

	return &reputations
}

func (p *Peer) IsActivePeerIP(ipAddress string) *Reputation {
	peers := p.GetActivePeers()

	for _, rep := range *peers {
		if (*rep).Ip == ipAddress {
			return rep
		}
	}
	return nil
}

func (p *Peer) IsPeerIP(ipAddress string) *Reputation {
	peers := p.GetAllPeers()

	for _, p := range *peers {
		if p.Ip == ipAddress {
			return p
		}
	}
	return nil
}

func (p *Peer) close() {
	p.peerstore.saveToFile(p.privKey)

	// shut the node down
	if err := p.host.Close(); err != nil {
		panic(err)
	}
}

func (p *Peer) pingLoop() {
	for {
		fmt.Println("[LOOP] printing active peers:")
		for peerID := range p.peerstore.activePeers {
			peerData := p.peerstore.activePeers[peerID]
			fmt.Printf("[LOOP] peer %s: %d\n", peerID, peerData.BasicInteractions)
			p.sendPing(peerData)
		}
		fmt.Println("[LOOP] done, sleeping 10s")
		time.Sleep(10 * time.Second)
	}
	// sleep
	// for each active peer
	// should i ping?
	// ping
	// too many failures?
	// remove peer from actives
}

// Send specified message to the provided peer. Return the peer's reply (string) and success (bool)
// All connection errors affect the peer's reliability, there is no need to update it based on the success bool
// peerData: data of the target peer
// message: the string to send to the target peer
// timeout: timeout to wait for reply. If timeout is set to 0, the stream is closed right after sending, without reading any replies.
// return response string: the response sent by the peer. Empty string if timeout is zero or if there were errors
// return success bool: true if everything went smoothly, false in case of errors (or no reply from peer)
func (p *Peer) sendMessageToPeerData(peerData *PeerData, message string, timeout time.Duration) (string, bool){
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
	remoteMA := peerData.LastMultiAddress

	// new multiaddress from string
	multiaddress, err := multiaddr.NewMultiaddr(remoteMA)
	if err != nil {
		fmt.Printf("Error parsing multiaddress '%s': %s\n", remoteMA, err)
		return nil
	}

	// addrInfo from multiaddress
	remotePeer, err := peer.AddrInfoFromP2pAddr(multiaddress)
	if err != nil {
		fmt.Println("Error creating addrInfo from multiaddress:", err)
		return nil
	}

	// open stream
	fmt.Println("YYY remotepeer:", remotePeer.ID)
	fmt.Printf("YYY remotepeertype: %T\n", remotePeer.ID)
	stream, err := p.host.NewStream(p.ctx, remotePeer.ID, protocol.ID(p.protocol))
	if err != nil {
		fmt.Println("Error opening stream:", err)
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
	fmt.Printf("Sending message: '%s'\n", message)
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
	var contactList map[string]*PeerData

	// handle * as recipient
	if peerId == "*" {
		contactList = p.peerstore.activePeers
		// TODO: choose 50 peers
		// TODO: consider broadcasting
	} else {
		contactList = make(map[string]*PeerData)
		peerData := p.peerstore.isActivePeer(peerId)

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
