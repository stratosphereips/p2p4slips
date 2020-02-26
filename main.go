package main

import (
	"flag"
	"fmt"
	"github.com/go-redis/redis/v7"
	"os"
)

func redisDemo(){
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	pubsub := rdb.Subscribe("new_ip")

	// Wait for confirmation that subscription is created before publishing anything.
	_, err := pubsub.Receive()
	if err != nil {
		panic(err)
	}

	// Go channel which receives messages.
	ch := pubsub.Channel()

	// Consume messages.
	for msg := range ch {
		fmt.Println(msg.Channel, msg.Payload)
	}
}

func main() {
	help := flag.Bool("help", false, "Display Help")
	cfg := parseFlags()

	if *help {
		fmt.Printf("Simple example for peer discovery using mDNS. mDNS is great when you have multiple peers in local LAN.")
		fmt.Printf("Usage: \n   Run './chat-with-mdns'\nor Run './chat-with-mdns -host [host] -port [port] -rendezvous [string] -pid [proto ID]'\n")

		os.Exit(0)
	}

	name, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	fmt.Println("hostname:", name)

	host := cfg.listenHost
	port := cfg.listenPort
	keyFile := cfg.keyFile
	keyReset := cfg.resetKeys

	fmt.Println("host is", host, "and port is", port)

	peer, err := peerInit(port, keyFile, keyReset)

	if err != nil {
		fmt.Println("Initializing peer failed")
		return
	}

	slist := SListener{channelName:"gotest", dbAddress:"localhost:6379", peer:peer}
	slist.dbInit()
	select {}
}
