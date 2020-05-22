package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
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

	name, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	fmt.Println("hostname:", name)

	portStr := strconv.Itoa(cfg.listenPort)
	ln, err := net.Listen("tcp", ":" + portStr)

	if err != nil {
		panicMsg := fmt.Sprintf("can't listen on port %q: %s",portStr, err)
		panic(panicMsg)
	}

	_ = ln.Close()
	fmt.Printf("[MAIN] Pigeon is starting on TCP Port %q\n", portStr)

	keyFile := cfg.keyFile
	peerstoreFile := cfg.peerstoreFile
	if cfg.renameWithPort {
		if keyFile != "" {
			keyFile = fmt.Sprintf("%s%d", keyFile, cfg.listenPort)
		}
		if peerstoreFile != "" {
			peerstoreFile = fmt.Sprintf("%s%d", peerstoreFile, cfg.listenPort)
		}
		cfg.redisChannelGoPy = fmt.Sprintf("%s%d", cfg.redisChannelGoPy, cfg.listenPort)
		cfg.redisChannelPyGo = fmt.Sprintf("%s%d", cfg.redisChannelPyGo, cfg.listenPort)
	}

	dbw = &DBWrapper{dbAddress: "", rdbGoPy:cfg.redisChannelGoPy}
	dbw.initDB()

	peer := Peer{
		dbAddress:cfg.redisDb,
		redisDelete:cfg.redisDelete,
		rdbGoPy:cfg.redisChannelGoPy,
		port:cfg.listenPort,
		protocol:cfg.ProtocolID,
		hostname:cfg.listenHost,
		rendezVous:cfg.RendezvousString,
		peerstoreFile:peerstoreFile,
		keyFile:keyFile,
		resetKey:cfg.resetKeys,
	}

	err = peer.peerInit()

	if err != nil {
		fmt.Println("Initializing peer failed")
		os.Exit(1)
	}

	slist := SListener{channelName:cfg.redisChannelPyGo, dbAddress:cfg.redisDb, peer:&peer}
	go slist.dbInit()

	go runTests(cfg.redisDb, cfg.redisChannelPyGo)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	<- ch
	fmt.Printf("\nReceived signal, shutting down...\n")

	peer.close()
	time.Sleep(20 * time.Second)
	os.Exit(0)
}
