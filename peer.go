package main

import (
	"bufio"
	"context"
	"crypto/rand"
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
	"os"
	"strings"
	"time"
)

type Peer struct {
	rdb  *redis.Client
	activePeers string
	allPeers string
	host host.Host
	port int
	hostname string
	protocol string
	dbAddress string
	rendezVous string
	ctx context.Context
}

// dbInit function for a peer. It creates the database connection, starts listening for channels. It also initializes
// everything needed for libp2p and sets up a Host that listens for new connections.
// TODO: call this from somewhere?
func peerInit(port int) *Peer {
	var p Peer
	p.port = port
	p.hostname = "0.0.0.0"
	p.protocol = "/chat/1.1.0"
	p.dbAddress = "localhost:6379"
	p.rendezVous = "meetme"

	// connect to the database
	// TODO: not crashing when database is offline would be nice
	//  also when database shuts down while this is still running...
	p.rdb = redis.NewClient(&redis.Options{
		Addr:     p.dbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	p.rdb.FlushAll()

	// prepare p2p host
	p.p2pInit()

	p.activePeers = p.host.ID().Pretty() + "-active"
	p.allPeers = p.host.ID().Pretty() + "-all"

	// link to a listener for new connections
	// TODO: this can't be tested that easily on localhost, as they will connect to the same db. Perhaps more redises?
	// TODO: this needs access to the db object. It can be global or passed in a function:
	// TODO:     https://stackoverflow.com/questions/26211954/how-do-i-pass-arguments-to-my-handler
	p.host.SetStreamHandler(protocol.ID(p.protocol), p.listener)

	// run peer discovery in the background
	go p.discoverPeers()

	// also run a service to continuously check for active peers.

	return &p
}

func (p *Peer) p2pInit() {
	p.ctx = context.Background()
	r := rand.Reader

	// Creates a new RSA key pair for this host.
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		panic(err)
	}

	// 0.0.0.0 will listen on any interface device.
	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", p.hostname, p.port))

	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here.
	p.host, err = libp2p.New(
		p.ctx,
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(prvKey),
	)

	if err != nil {
		panic(err)
	}

	fmt.Printf("\n[*] Your Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n", p.hostname, p.port, p.host.ID().Pretty())
}

func (p *Peer) discoverPeers() {
	fmt.Println("Looking for peers")

	// TODO: handle panic: No multicast listeners could be started
	//  happens when network is disabled
	peerChan := initMDNS(p.ctx, p.host, p.rendezVous)

	for {
		peerAddress := <-peerChan // will block until we discover a peerAddress
		peerid := peerAddress.ID.Pretty()
		fmt.Println(">>> Found peerAddress:", peerid)

		if p.isActivePeer(peerid) {
			fmt.Println("I know him and he is active, skipping")
			continue
		} else if p.isKnown(peerid) {
			fmt.Println("I know him, but we've met a long time ago, skipping")
			p.setPeerOnline(peerid)
		} else {
			fmt.Println("This is a new node, contacting him...")
		}

		go p.sayHello(peerAddress)
		time.Sleep(2 * time.Second)
	}
}

func (p *Peer) isActivePeer(peerid string) bool {
	return p.rdb.HExists(p.activePeers, peerid).Val()
}

func (p *Peer) isKnown(peerid string) bool {
	return p.rdb.HExists(p.allPeers, peerid).Val()
}

func (p *Peer) setReputation(reputation *Reputation) {
	if reputation.Ip == "" {
		fmt.Println("no data")
	}
	data := reputation.rep2json()
	// fmt.Println("data", data)
	p.rdb.HSet(p.activePeers, reputation.Peerid, data)
}

func (p *Peer) getReputation(peerid string) Reputation {
	data := p.rdb.HGet(p.activePeers, peerid).Val()
	reputation := Reputation{}
	_ = json.Unmarshal([]byte(data), &reputation)
	return reputation
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
	// fmt.Println("[", remotePeer, "] Incoming stream")

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
		p.handleHello(stream, rw, &commands)
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
}

func (p *Peer) sayHello(peerAddress peer.AddrInfo){
	// add a simple record into db
	reputation := PeerAddress2rep(peerAddress)
	p.setReputation(reputation)

	if err := p.host.Connect(p.ctx, peerAddress); err != nil {
		fmt.Println("Connection failed:", err)
		// TODO: lower rep?
		return
	}

	// open a stream, this stream will be handled by handleStream other end
	stream, err := p.host.NewStream(p.ctx, peerAddress.ID, protocol.ID(p.protocol))

	if err != nil {
		fmt.Println("Stream open failed", err)
		// TODO: lower rep?
		return
	}

	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	fmt.Println("Saying hello")
	if !send2rw(rw, "hello version1\n") {
		fmt.Println("error sending")
		// TODO: lower rep?
		return
	}

	input := make(chan string)
	go rw2channel(input, rw)

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
		// TODO: malformed hello message, lower rep?
		return
	} else {
		remoteVersion = command[1]
	}

	fmt.Println("PeerOld response ok, updating reputation")
	reputation.Version = remoteVersion
	p.setReputation(reputation)

	// is this a proper close?
	err = stream.Close()
	if err != nil {
		fmt.Println("Error closing")
		// TODO: do something here?
		// panic(err)
		return
	}
}

func (p *Peer) handleHello(stream network.Stream, rw *bufio.ReadWriter, command *[]string){
	remotePeer := stream.Conn().RemotePeer()
	peerId := remotePeer.Pretty()
	remoteAddr := stream.Conn().RemoteMultiaddr().String()

	// parse contents of hello message
	var remoteVersion string
	if len(*command) != 2 {
		fmt.Println("Invalid Hello format")
		// TODO: malformed hello message, lower rep?
		return
	} else {
		remoteVersion = (*command)[1]
	}

	if !p.isActivePeer(peerId) {
		// add a simple record into db
		reputation := Stream2rep(stream)
		p.setReputation(reputation)

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
		return
	}

	// if he is not known, check if his information is new
	reputation := p.getReputation(peerId)

	if reputation.Version != remoteVersion {
		// update this info
		fmt.Println("updating version info")
	}

	if reputation.Multiaddr != remoteAddr {
		// update this info
		fmt.Println("updating address info")
	}

	if reputation.Multiaddr == remoteAddr && reputation.Version == remoteVersion {
		// lower his rep
	}
}

func (p *Peer) sendPing() {
	// check last ping time, if it was recently, do not ping at all

	// open stream
	// open rw
	// send ping to rw (flush!)
	// wait for reply

	// if successful: add a new good interaction, update reputation
	// if unsuccessful: add a new bad interaction. If there are three of them, set peer offline
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
		fmt.Println(peerId, ":::::", jsonData)
		reputations[peerId] = Json2rep(jsonData)
	}

	return &reputations
}


func (p *Peer) GetAllPeers() *[]*Reputation {
	peers := make([]*Reputation, 0)

	data := p.rdb.HGetAll(p.allPeers)
	fmt.Println("All neighbors:")
	fmt.Println(data)

	// TODO: this is obviously not oK
	return &peers
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
	// TODO change to all peers later
	peers := p.GetActivePeers()

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