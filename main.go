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

	host := cfg.listenHost
	port := cfg.listenPort
	cantalk := cfg.canTalk

	fmt.Println("host is", host, "and port is", port)

	if cantalk {
		fmt.Println("I can talk")
	} else {
		fmt.Println("I can't talk")
	}

	go peerInit(cantalk, port)
	select {}
}
