package main

import (
	"flag"
)

type config struct {
	RendezvousString string
	ProtocolID       string
	keyFile          string
	listenHost       string
	listenPort       int
	resetKeys	     bool
}

func parseFlags() *config {
	c := &config{}

	flag.StringVar(&c.RendezvousString, "rendezvous", "meetme", "Unique string to identify group " +
		"of nodes. Share this with your friends to let them connect with you")
	flag.StringVar(&c.listenHost, "host", "0.0.0.0", "The bootstrap node host listen address\n")
	flag.StringVar(&c.ProtocolID, "pid", "/chat/1.1.0", "Sets a protocol id for stream headers")
	flag.StringVar(&c.keyFile, "key-file", "", "File containing keys. If it is provided, keys " +
		"will be loaded from the file and saved to it for later use. If no file is specified, one time keys will be " +
		"generated")
	flag.BoolVar(&c.resetKeys, "key-reset", false, "Delete old keys and create new ones")
	flag.IntVar(&c.listenPort, "port", 4001, "node listen port")

	flag.Parse()
	return c
}
