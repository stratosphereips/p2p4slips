package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/go-redis/redis/v7"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/multiformats/go-multiaddr"
	"os"
	"strings"
	"time"
)

type Peer struct {
	rdb  *redis.Client
	activePeers string
	allPeers string
	host       host.Host
	port       int
	hostname   string
	protocol   string
	dbAddress  string
	rendezVous string
	ctx        context.Context
	peerstore  PeerStore
	privKey    crypto.PrivKey
	keyFile    string
	resetKey   bool
	peerstoreFile string
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
	// TODO: this can't be tested that easily on localhost, as they will connect to the same db. Perhaps more redises?
	// TODO: this needs access to the db object. It can be global or passed in a function:
	// TODO:     https://stackoverflow.com/questions/26211954/how-do-i-pass-arguments-to-my-handler
	p.host.SetStreamHandler(protocol.ID(p.protocol), p.listener)

	p.peerstore = PeerStore{store:p.host.Peerstore(), saveFile: p.peerstoreFile}
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
	// TODO: not crashing when database is offline would be nice
	//  also when database shuts down while this is still running...
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
	go func() {	for {
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
	}}()
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
	remotePeer := stream.Conn().RemotePeer()
	remotePeerStr := remotePeer.Pretty()
	remoteMA := fmt.Sprintf("%s/p2p/%s", stream.Conn().RemoteMultiaddr(), remotePeerStr)
	fmt.Println("remotema listener:", remoteMA)

	var remotePeerData *PeerData
	remotePeerData = p.peerstore.isKnownWithUpdate(remotePeerStr)
	if remotePeerData == nil {
		// add a simple record into db
		remotePeerData = &PeerData{PeerID: remotePeerStr}
		p.peerstore.activePeers[remotePeerStr] = remotePeerData
	}
	remotePeerData.checkAndUpdateActivePeerMultiaddr(remoteMA)

	// Create a buffer stream for non blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	str, err := rw.ReadString('\n')

	// remove trailing newlines
	str = str[:len(str)-1]

	if err != nil {
		fmt.Println("Error reading from buffer")
		// TODO: instead of panic, disconnect and stop listening
		panic(err)
	}

	commands := strings.Fields(str)

	if commands[0] == "hello" {
		fmt.Println("[", remotePeer, "] New peer says hello to me")
		p.handleHello(remotePeerStr, remotePeerData, rw, &commands)
		p.GetActivePeers()
		return
	}

	if str == "ping" {
		fmt.Println("[", remotePeer, "] says ping")
		p.handlePing(stream, rw, &commands)
		return
	}

	if str == "\n" {
		fmt.Println("[", remotePeer, "] sent an empty string")
		return
	}

	// Green console colour: 	\x1b[32m
	// Reset console colour: 	\x1b[0m
	fmt.Println("[", remotePeer, "] sent an unknown message:", str)
	p.peerstore.decreaseGoodCount(remotePeer.Pretty())
}

func (p *Peer) sayHello(peerAddress peer.AddrInfo){
	// add a simple record into db
	remotePeer := peerAddress.ID
	remotePeerStr := remotePeer.Pretty()
	remoteMA := fmt.Sprintf("%s/p2p/%s", peerAddress.Addrs[0], remotePeerStr)
	fmt.Println("remotema sayhello:", remoteMA)

	var remotePeerData *PeerData
	// add a simple record into db
	remotePeerData = &PeerData{PeerID: remotePeerStr}
	p.peerstore.activePeers[remotePeerStr] = remotePeerData
	remotePeerData.checkAndUpdateActivePeerMultiaddr(remoteMA)

	if err := p.host.Connect(p.ctx, peerAddress); err != nil {
		fmt.Println("Connection failed:", err)
		p.peerstore.decreaseGoodCount(remotePeerStr)
		return
	}

	// open a stream, this stream will be handled by handleStream other end
	fmt.Printf("PROTOCOL: %s\n", p.protocol)
	stream, err := p.host.NewStream(p.ctx, remotePeer, protocol.ID(p.protocol))

	if err != nil {
		fmt.Println("Stream open failed", err)
		p.peerstore.decreaseGoodCount(remotePeerStr)
		return
	}

	input := make(chan string)
	p.sendMessageToStream(stream, "hello version1\n", input)

	response := ""
	select{
	case response = <- input:
		break
	case <-time.After(5 * time.Second):
		fmt.Println("timeout")
	}
	fmt.Println("text:", response)

	command := strings.Fields(response)
	var remoteVersion string

	if len(command) != 2 || command[0] != "hello" {
		p.peerstore.decreaseGoodCount(remotePeerStr)
		return
	} else {
		remoteVersion = command[1]
	}

	fmt.Println("PeerOld response ok, updating reputation")
	p.peerstore.increaseGoodCount(remotePeerStr)
	p.peerstore.updatePeerVersion(remotePeerStr, remoteVersion)

	// is this a proper close?
	err = stream.Close()
	if err != nil {
		fmt.Println("Error closing")
		// TODO: do something here?
		return
	}
}

func (p *Peer) handleHello(remotePeerStr string, remotePeerData *PeerData, rw *bufio.ReadWriter, command *[]string) {

	// parse contents of hello message
	var remoteVersion string
	if len(*command) != 2 {
		fmt.Println("Invalid Hello format")
		// TODO: malformed hello message, lower rep?
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
		p.peerstore.decreaseGoodCount(remotePeerStr)
	}

	// say hello
	fmt.Println("Sending hello reply...")

	_, err := rw.WriteString("hello version1\n")
	if err != nil {
		// TODO: don't panic
		fmt.Println("Error writing to buffer")
		// panic(err)
	}

	err = rw.Flush()
	if err != nil {
		// TODO: don't panic
		fmt.Println("Error flushing buffer")
		// panic(err)
	}

	p.peerstore.increaseGoodCount(remotePeerStr)
	return
}

func (p *Peer) sendPing(peerData *PeerData) {
	// check last ping time, if it was recently, do not ping at all
	if !peerData.shouldIPingPeer() {
		fmt.Println("Peer was contacted recently, no need for ping")
		return
	}

	pingStream := p.openStreamFromPeerData(peerData)
	if pingStream == nil {
		// failure
	}

	output := make(chan string)
	p.sendMessageToStream(pingStream, "ping\n", output)

	response := ""
	select{
	case response = <- output:
		break
	case <-time.After(5 * time.Second):
		fmt.Println("timeout")
	}

	fmt.Println("ping response:", response)
	// TODO: handle ping response
}

func (p *Peer) handlePing(stream network.Stream, rw *bufio.ReadWriter, command *[]string) {
	// do i have reputation for him in all?
	// if not: lower reputation. Send hello.

	// if last contact was recently, lower score, this is not necessary

	// reply to ping
	// increment last contact in reputation
	// also increment this in all contacts (hello etc)
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

func rw2channel(input chan string, rw *bufio.ReadWriter) {
	for {
		result, err := rw.ReadString('\n')
		if err != nil {
			fmt.Println("err on rw2channel")
		}

		input <- result
	}
}

func send2rw (rw *bufio.ReadWriter, message string) bool {
	_, err := rw.WriteString(message)

	if err != nil {
		return false
	}
	err = rw.Flush()
	if err != nil {
		return false
	}

	return true
}

// public functions follow

func (p *Peer) Blame (ipAddress string) {
	b := p.IsPeerIP(ipAddress)

	if b == nil {
		fmt.Printf("[PEER ] Can't blame '%s' - not a peer", ipAddress)
		return
	}

	// TODO: implement
	fmt.Printf("[PEER ] Can't blame '%s' - not implemented yet", ipAddress)
	return
}

func (p *Peer) Send (data string) {
	// for now, use the entire active list
	// TODO: choose 50 peers
	// TODO: consider broadcasting
	peerList := p.GetActivePeers()
	fmt.Println(peerList)
}

func (p *Peer) SendAndWait (data string, timeout int) string {
	// for now, use the entire active list
	// TODO: choose 50 peers
	//// TODO: consider broadcasting
	//peerList := p.GetActivePeers()
	//
	//peerstore.AddrBook()
	return ""
}

func (p *Peer) close(){
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
			fmt.Printf("[LOOP] peer %s: %d\n", peerID, peerData.GoodCount)
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

func (p *Peer) openStreamFromPeerData(peerData *PeerData) network.Stream{
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
	stream, err := p.host.NewStream(p.ctx, remotePeer.ID, protocol.ID(p.protocol))
	if err != nil {
		fmt.Println("Error opening stream:", err)
		return nil
	}

	return stream
}

func (p *Peer) sendMessageToStream(stream network.Stream, msg string, output chan string) {

	// open rw
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	fmt.Printf("Sending message: '%s'\n", msg)
	if !send2rw(rw, msg) {
		fmt.Println("error sending")
		// p.peerstore.decreaseGoodCount(remotePeerStr)
		return
	}

	go rw2channel(output, rw)

	// is this a proper close?
	err := stream.Close()
	if err != nil {
		fmt.Println("Error closing")
		// TODO: do something here?
		return
	}
}