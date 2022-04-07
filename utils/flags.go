package utils

import (
	"flag"
)

type Config struct {
	RendezvousString string
	ProtocolID       string
	KeyFile          string
	PeerstoreFile    string
	RenameWithPort   bool
	ListenHost       string
	ListenPort       int
	ResetKeys        bool
	RedisDb          string
	RedisDelete      bool
	RedisChannelPyGo string
	RedisChannelGoPy string
	RunTests         bool
	ShowHelp         bool
}

func ParseFlags() *Config {
	c := &Config{}

	flag.StringVar(&c.RendezvousString, "rendezvous", "slips", "Unique string to identify group "+
		"of nodes. Share this with your friends to let them connect with you")
	flag.StringVar(&c.ListenHost, "host", "", "The bootstrap node host listen address\n")
	flag.StringVar(&c.ProtocolID, "pid", "/slips/1.0", "Sets a protocol id for stream headers")
	flag.IntVar(&c.ListenPort, "port", 4001, "node listen port")

	flag.StringVar(&c.KeyFile, "key-file", "", "File containing keys. If it is provided, keys "+
		"will be loaded from the file and saved to it for later use. If no file is specified, one time keys will be "+
		"generated")
	flag.BoolVar(&c.ResetKeys, "key-reset", false, "Delete old keys and create new ones")

	flag.StringVar(&c.PeerstoreFile, "peerstore-file", "", "File containing known peers. If it is"+
		" provided, peers will be loaded from the file and saved to it for later use. If no file is specified, or if "+
		"the file cannot be decrypted with the given private key, empty peerstore will be created")

	flag.BoolVar(&c.RenameWithPort, "rename-with-port", false, "Port is appended to filenames and "+
		"channels for convenient running of more peers on one host. Set to false to keep filenames unchanged")

	flag.StringVar(&c.RedisDb, "redis-db", "localhost:6379", "Remote redis database")

	flag.BoolVar(&c.RedisDelete, "redis-delete", false, "Delete database when starting the program")

	flag.BoolVar(&c.RunTests, "test", false, "Run a test script instead of the main program")

	flag.StringVar(&c.RedisChannelPyGo, "redis-channel-pygo", "p2p_pygo", "Channel for listening to commands")
	flag.StringVar(&c.RedisChannelGoPy, "redis-channel-gopy", "p2p_gopy", "Channel for sending data to slips")

	flag.BoolVar(&c.ShowHelp, "help", false, "Display Help")

	flag.Parse()
	return c
}
