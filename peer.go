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
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/multiformats/go-multiaddr"
)

type Peer struct {
	rdb *redis.Client
	host host.Host
	port string
	hostname string
	protocol string
	dbAddress string
	rendezVous string
	ctx context.Context
}

// dbInit function for a peer. It creates the database connection, starts listening for channels. It also initializes
// everything needed for libp2p and sets up a Host that listens for new connections.
// TODO: call this from somewhere?
func peerInit() *Peer {
	var p Peer
	p.port = "6666"
	p.hostname = "0.0.0.0"
	p.protocol = "/chat/1.1.0"
	p.dbAddress = "localhost:6379"

	// connect to the database
	p.rdb = redis.NewClient(&redis.Options{
		Addr:     p.dbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// subscribe to SLIPS channel
	redisSubscribe(&p)

	// prepare p2p host
	p.host = p2pInit(p.hostname, p.port)

	// link to a listener for new connections
	// TODO: this can't be tested that easily on localhost, as they will connect to the same db. Perhaps more redises?
	// TODO: this needs access to the db object. It can be global or passed in a function:
	// TODO:     https://stackoverflow.com/questions/26211954/how-do-i-pass-arguments-to-my-handler
	// TODO: also call a different function instead
	p.host.SetStreamHandler(protocol.ID(p.protocol), waitForConnections)

	// run peer discovery in the background
	go discoverPeers(&p)
	return &p
}

// subscribe to a channel and print all messages that arrive
// taken from https://godoc.org/github.com/go-redis/redis#example-PubSub-Receive
func redisSubscribe(p *Peer){
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

func p2pInit(ip string, port string) host.Host{
	ctx := context.Background()
	r := rand.Reader

	// Creates a new RSA key pair for this host.
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		panic(err)
	}

	// 0.0.0.0 will listen on any interface device.
	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ip, port))

	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here.
	p2pHost, err := libp2p.New(
		ctx,
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(prvKey),
	)

	if err != nil {
		panic(err)
	}

	fmt.Printf("\n[*] Your Multiaddress Is: /ip4/%s/tcp/%v/p2p/%s\n", ip, port, p2pHost.ID().Pretty())

	return p2pHost
}

func discoverPeers(p *Peer){

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
		} else {
			rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
			// TODO: change to custom function
			go ping(rw)
		}
	}
}