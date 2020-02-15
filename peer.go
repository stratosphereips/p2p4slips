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
	rdb *redis.Client
	rdhash string
	host host.Host
	port int
	hostname string
	protocol string
	dbAddress string
	rendezVous string
	ctx context.Context
	canTalk bool
}

// dbInit function for a peer. It creates the database connection, starts listening for channels. It also initializes
// everything needed for libp2p and sets up a Host that listens for new connections.
// TODO: call this from somewhere?
func peerInit(canTalk bool, port int) *Peer {
	var p Peer
	p.port = port
	p.hostname = "0.0.0.0"
	p.protocol = "/chat/1.1.0"
	p.dbAddress = "localhost:6379"
	p.canTalk = canTalk

	// connect to the database
	// TODO: not crashing when database is offline would be nice
	p.rdb = redis.NewClient(&redis.Options{
		Addr:     p.dbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// prepare p2p host
	p.p2pInit()

	p.rdhash = p.host.ID().Pretty()

	// subscribe to SLIPS channel
	go p.redisSubscribe()


	fmt.Println("setting handler")
	// link to a listener for new connections
	// TODO: this can't be tested that easily on localhost, as they will connect to the same db. Perhaps more redises?
	// TODO: this needs access to the db object. It can be global or passed in a function:
	// TODO:     https://stackoverflow.com/questions/26211954/how-do-i-pass-arguments-to-my-handler
	p.host.SetStreamHandler(protocol.ID(p.protocol), p.listener)

	fmt.Println("foo")

	// run peer discovery in the background
	go p.discoverPeers()
	return &p
}

// subscribe to a channel and print all messages that arrive
// taken from https://godoc.org/github.com/go-redis/redis#example-PubSub-Receive
func (p *Peer) redisSubscribe() {
	pubsub := p.rdb.Subscribe("new_ip")

	// Wait for confirmation that subscription is created before publishing anything.
	_, err := pubsub.Receive()
	if err != nil {
		panic(err)
	}

	// Go channel which receives messages.
	ch := pubsub.Channel()

	// TODO: there was a part here that prevented the sample from working alongside SLIPS. I need to look into that.
	// time.AfterFunc(time.Second, func() {
	//    // When pubsub is closed channel is closed too.
	//    _ = pubsub.Close()
	//})

	// Consume messages.
	for msg := range ch {
		fmt.Println(msg.Channel, msg.Payload)
	}
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

	peerChan := initMDNS(p.ctx, p.host, p.rendezVous)

	for {
		peerAddress := <-peerChan // will block until we discover a peerAddress
		peerid := peerAddress.ID.Pretty()
		fmt.Println(">>> Found peerAddress:", peerid)

		if p.isKnown(peerid) {
			fmt.Println("I know him already, skipping")
			continue
		} else {
			fmt.Println("This is a new node, contacting him...")
		}

		go p.sayHello(peerAddress)
		time.Sleep(2 * time.Second)
	}
}

func (p *Peer) isKnown(peerid string) bool {
	return p.rdb.HExists(p.rdhash, peerid).Val()
}

func (p *Peer) setReputation(reputation Reputation) {
	data := reputation.rep2json()
	fmt.Println("data", data)
	p.rdb.HSet(p.rdhash, reputation.Peerid, data)
}

func (p *Peer) getReputation(peerid string) Reputation {
	data := p.rdb.HGet(p.rdhash, peerid).Val()
	reputation := Reputation{}
	_ = json.Unmarshal([]byte(data), &reputation)
	return reputation
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
	peer := stream.Conn().RemotePeer()
	fmt.Println("[", peer, "] A peer is contacting me")

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
		fmt.Println("[", peer, "] says hello")
		p.handleHello(stream, rw, &commands)
		return
	}

	if str == "ping" {
		fmt.Println("[", peer, "] says ping")
		return
	}

	if str == "\n" {
		fmt.Println("[", peer, "] sent an empty string")
		return
	}

	// Green console colour: 	\x1b[32m
	// Reset console colour: 	\x1b[0m
	fmt.Println("[", peer, "] sent an unknown message:", str)
}

func (p *Peer) sayHello(peerAddress peer.AddrInfo){
	// add a simple record into db
	reputation := Reputation{
		Peerid:       peerAddress.ID.Pretty(),
		Multiaddr:    peerAddress.String(),
		Version:      "",
		Interactions: nil,
		Uptime:       nil,
	}
	p.setReputation(reputation)

	if err := p.host.Connect(p.ctx, peerAddress); err != nil {
		fmt.Println("Connection failed:", err)
	}

	// open a stream, this stream will be handled by handleStream other end
	stream, err := p.host.NewStream(p.ctx, peerAddress.ID, protocol.ID(p.protocol))

	if err != nil {
		fmt.Println("Stream open failed", err)
	}

	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	fmt.Println("Saying hello")
	_, err = rw.WriteString("hello version1\n")
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

func (p *Peer) handleHello(stream network.Stream, rw *bufio.ReadWriter, command *[]string){
	remotePeer := stream.Conn().RemotePeer()
	peerid := remotePeer.Pretty()
	remoteAddr := stream.Conn().RemoteMultiaddr().String()

	// parse contents of hello message
	var remoteVersion string
	if len(*command) != 2 {
		// TODO: malformed hello message, lower his score
	} else {
		remoteVersion = (*command)[1]
	}

	if !p.isKnown(peerid) {
		// add a simple record into db
		reputation := Reputation{
			Peerid:       peerid,
			Multiaddr:    remoteAddr,
			Version:      remoteVersion,
			Interactions: nil,
			Uptime:       nil,
		}
		p.setReputation(reputation)

		// say hello
		rw.WriteString("hello version1\n")
		return
	}

	// if he is not known, check if his information is new
	reputation := p.getReputation(peerid)

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