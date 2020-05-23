package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v7"
	"strings"
)

type SListener struct {
	peer *Peer
}

type PigeonScroll struct {
	Message   string `json:"message"`
	Recipient string `json:"recipient"`
}

func (s *SListener) dbInit() error {

	// Go channel which receives messages.
	ch := make(<-chan *redis.Message)
	if !dbw.subscribeToPyGo(ch) {
		fmt.Println("subscribing failed")
		return errors.New("subscribing to db failed")
	}
	fmt.Println("subscribing ok")
	// TODO: actually not subscribed...

	// TODO: there was a part here that prevented the sample from working alongside SLIPS. I need to look into that.
	// time.AfterFunc(time.Second, func() {
	//    // When pubsub is closed channel is closed too.
	//    _ = pubsub.Close()
	//})

	// Consume messages.
	for msg := range ch {
		// if redis is stopped, golang will show an error: pubsub.go:160: redis: discarding bad PubSub connection: EOF
		// I don't know where to catch this, but it is not a problem. When redis is restarted, pubsub listens again
		s.handleCommand(msg.Payload)
	}

	return nil
}

func (s *SListener) handleCommand(message string) {
	fmt.Println("[SLISTENER] New message from REDIS:", message)

	ps, err := s.parseJson(message)

	if err != nil {
		fmt.Println("[SLISTENER] invalid json received from Slips")
		return
	}

	fmt.Println("[SLISTENER] Message data sent from Slips", ps)

	// send the message to the peer specified in the scroll
	s.peer.sendMessageToPeerId(ps.Message, ps.Recipient)

	// the responses should be processed by remote peers eventually and should be processed by the peer listening loop
	// and saved to slips database from there
}

func (s *SListener) parseJson(message string) (*PigeonScroll, error) {
	ps := &PigeonScroll{}

	if err := json.Unmarshal([]byte(message), ps); err != nil {
		fmt.Println("[SLISTENER] ", err)
		return nil, err
	}

	if ps.Message == "" {
		fmt.Println("[SLISTENER] JSON is missing the Message field")
		return nil, errors.New("message field missing")
	}

	if ps.Recipient == "" {
		fmt.Println("[SLISTENER] JSON is missing the Recipient field")
		return nil, errors.New("recipient field missing")
	}

	if !strings.HasSuffix(ps.Message, "\n") {
		fmt.Println("Adding newline at the end of slips message...")
		ps.Message = ps.Message + "\n"
	}

	return ps, nil
}
