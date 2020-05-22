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
	dbw = &DBWrapper{dbAddress: "", rdbGoPy:cfg.redisChannelGoPy}
	dbw.initDB()

	peer := NewPeer(cfg)
	err := peer.peerInit()

	if err != nil {
		fmt.Println("Initializing peer failed")
		os.Exit(1)
	}

	slist := SListener{channelName:cfg.redisChannelPyGo, dbAddress:cfg.redisDb, peer:peer}
	go slist.dbInit()

	go runTests(cfg.redisDb, cfg.redisChannelPyGo)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	<- ch
	fmt.Printf("\nReceived signal, shutting down...\n")

	peer.close()
	os.Exit(0)
}

func testPort(listenPort int) {
	portStr := strconv.Itoa(listenPort)
	socket, err := net.Listen("tcp", ":" + portStr)

	if err != nil {
		panicMsg := fmt.Sprintf("can't listen on port %q: %s", portStr, err)
		panic(panicMsg)
	}

	_ = socket.Close()
}

func renameFilesAndChannels(cfg *config){
	if cfg.renameWithPort {
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