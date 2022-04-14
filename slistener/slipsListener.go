package slistener

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/stratosphereips/p2p4slips/database"
	"github.com/stratosphereips/p2p4slips/peer"
)

type SListener struct {
	Peer *peer.Peer
}

type PigeonScroll struct {
	Message   string `json:"message"`
	Recipient string `json:"recipient"`
}

func (s *SListener) Run() {

	// Consume messages. (msgs arriving here are the ones sent by slips to ask other peers about ips )
	for msg := range database.DBW.Ch {
		// if redis is stopped, golang will show an error:
		// pubsub.go:160: redis: discarding bad PubSub connection: EOF
		// I don't know where to catch this, but it is not a problem. When redis is restarted, pubsub listens again
		s.handleCommand(msg.Payload)
	}
}

func (s *SListener) handleCommand(message string) {
	//fmt.Println("[SLISTENER] New message from REDIS:", message)

	if message == "stop_process" {
		fmt.Println("[SLISTENER] Stop process received, awaiting termination signal..")
		s.Peer.PeerShutdown()
		return
	}

	ps, err := s.parseJson(message)

	if err != nil {
		fmt.Println("[SLISTENER] invalid json received from Slips")
		return
	}

	//fmt.Println("[SLISTENER] Message sent from Slips: ", ps)

	// send the message to the peer specified in the scroll
	s.Peer.SendMessageToPeerId(ps.Message, ps.Recipient)

	// the responses should be processed by remote peers eventually
	// and should be processed by the peer listening loop
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
		//fmt.Println("Adding newline at the end of slips message...")
		ps.Message = ps.Message + "\n"
	}

	return ps, nil
}
