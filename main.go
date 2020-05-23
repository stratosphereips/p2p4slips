package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

var dbw *DBWrapper

func main() {
	help := flag.Bool("help", false, "Display Help")
	cfg := parseFlags()

	if *help {
		fmt.Println("This is the P2P component of the Stratosphere Linux IPS.")
		fmt.Println("Run './p2p-experiments' to start it.")
		fmt.Println("For testing multiple peers on one machine, use './p2p-experiments -port [port]'")

		fmt.Println()
		fmt.Println("Usage:")
		flag.PrintDefaults()

		os.Exit(0)
	}

	// check if port is available - if not, panic
	testPort(cfg.listenPort)

	// add port to file names and channels (if config specifies it)
	renameFilesAndChannels(cfg)

	fmt.Printf("[MAIN] Pigeon is starting on TCP Port %d\n", cfg.listenPort)

	// initialize database interface
	dbw = &DBWrapper{dbAddress: "", rdbGoPy: cfg.redisChannelGoPy, rdbPyGo: cfg.redisChannelPyGo}
	dbw.initDB()

	// initialize peer
	peer := NewPeer(cfg)
	err := peer.peerInit()

	if err != nil {
		fmt.Println("Initializing peer failed")
		os.Exit(1)
	}

	// initialize the node listening for data from slips
	slist := SListener{peer: peer}
	go slist.dbInit()

	// run tests
	// TODO: remove tests for production
	go runTests(cfg.redisDb, cfg.redisChannelPyGo)

	// neatly exit when termination signal is received
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	<-ch
	fmt.Printf("\nReceived signal, shutting down...\n")

	peer.close()
	os.Exit(0)
}

// verify that the chosen port is not used by another service by trying to open a tcp socket on it
// if the port is already used, this function will panic
func testPort(listenPort int) {
	portStr := strconv.Itoa(listenPort)
	socket, err := net.Listen("tcp", ":"+portStr)

	if err != nil {
		panicMsg := fmt.Sprintf("can't listen on port %q: %s", portStr, err)
		panic(panicMsg)
	}

	_ = socket.Close()
}

// check if config requires port to be appended to config strings, and if so, append the port
// this affects file names (key file and peerstore file) and channels for communicating with python module
func renameFilesAndChannels(cfg *config) {
	if cfg.renameWithPort {
		// if file name is empty, it means that file saving should not be used
		// therefore port should not be added to empty file names, as this would make them no longer empty
		if cfg.keyFile != "" {
			cfg.keyFile = fmt.Sprintf("%s%d", cfg.keyFile, cfg.listenPort)
		}
		if cfg.peerstoreFile != "" {
			cfg.peerstoreFile = fmt.Sprintf("%s%d", cfg.peerstoreFile, cfg.listenPort)
		}
		cfg.redisChannelGoPy = fmt.Sprintf("%s%d", cfg.redisChannelGoPy, cfg.listenPort)
		cfg.redisChannelPyGo = fmt.Sprintf("%s%d", cfg.redisChannelPyGo, cfg.listenPort)
	}
}
