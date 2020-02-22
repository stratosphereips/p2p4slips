package main

import (
	"fmt"
	"github.com/go-redis/redis/v7"
	"net"
	"strings"
)

type SListener struct {
	dbAddress string
	channelName string
	rdb  *redis.Client
	peer *Peer
}

func (s *SListener) dbInit(){

	// connect to the database
	// TODO: not crashing when database is offline would be nice
	s.rdb = redis.NewClient(&redis.Options{
		Addr:     s.dbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	// taken from https://godoc.org/github.com/go-redis/redis#example-PubSub-Receive
	// TODO: name channel, so multiple peers can run with one redis
	pubsub := s.rdb.Subscribe(s.channelName)

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
		s.handleCommand(msg.Payload)
	}
}

func (s *SListener) handleCommand(message string) {
	fmt.Println("[SLIST] New message from REDIS:", message)

	// split message to two parts, command (first word) and the rest
	parsedMessage := strings.SplitN(message, " ", 2)

	switch parsedMessage[0] {
	case "BLAME":
		s.blame(parsedMessage[1])
	case "BROADCAST":
		s.broadcast(parsedMessage[1])
	case "ASK":
		s.ask(parsedMessage[1])
	default:
		fmt.Println("[SLIST] Invalid command:", parsedMessage[0])
	}
}

func (s *SListener) blame (data string){
	if net.ParseIP(data) == nil {
		fmt.Printf("[SLIST] Can't blame '%s' - not a valid IP\n", data)
		return
	}

	s.peer.Blame(data)
}

func (s *SListener) broadcast (data string){

}

func (s *SListener) ask (data string){

}