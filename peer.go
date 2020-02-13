package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"github.com/go-redis/redis/v7"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/multiformats/go-multiaddr"
	"os"
)

type Peer struct {
	rdb *redis.Client
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
		peer := <-peerChan // will block until we discover a peer
		fmt.Println(">>> Found peer:", peer, ", connecting")

		if err := p.host.Connect(p.ctx, peer); err != nil {
			fmt.Println("Connection failed:", err)
		}

		// open a stream, this stream will be handled by handleStream other end
		stream, err := p.host.NewStream(p.ctx, peer.ID, protocol.ID(p.protocol))

		if err != nil {
			fmt.Println("Stream open failed", err)
		} else if p.canTalk {
			rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
			go p.talker(rw)
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

	if str == "hello" {
		fmt.Println("[", peer, "] says hello")
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