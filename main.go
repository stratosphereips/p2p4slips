package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/stratosphereips/p2p4slips/database"
	"github.com/stratosphereips/p2p4slips/peer"
	"github.com/stratosphereips/p2p4slips/slistener"
	"github.com/stratosphereips/p2p4slips/tests"
	"github.com/stratosphereips/p2p4slips/utils"
)

func main() {

	cfg := utils.ParseFlags()

	if cfg.ShowHelp {
		fmt.Println("This is the P2P component of the Stratosphere Linux IPS.")
		fmt.Println("Run './p2p4slips' to start it.")
		fmt.Println("For testing multiple peers on one machine, use './p2p4slips -port [port]'")

		fmt.Println()
		fmt.Println("Usage:")
		flag.PrintDefaults()

		os.Exit(0)
	}

	if cfg.RunTests {
		fmt.Println("Running tests...")
		tests.RunTests("127.0.0.1", "foo")
		os.Exit(0)
	}
	fmt.Printf("[DEBUGGG] LISTENING on IP: %s\n", cfg.ListenHost)

	// check if port is available - if not, panic
	testPort(cfg.ListenPort)
	if cfg.RenameWithPort {
		fmt.Printf("[DEBUGGING]: renaming channels and files with port %d\n", cfg.ListenPort)
		// add port to file names and channels (if config specifies it)
		renameFilesAndChannels(cfg)
	}

	fmt.Printf("[MAIN] Pigeon is starting on TCP Port %d\n", cfg.ListenPort)

	// initialize database interface
	database.DBW = &database.DBWrapper{DbAddress: "", RdbGoPy: cfg.RedisChannelGoPy,
		RdbPyGo: cfg.RedisChannelPyGo}
	var dbSuccess = database.DBW.InitDB()

	if !dbSuccess {
		fmt.Println("[PEER] Initializing database failed")
		os.Exit(1)
	}

	// initialize peer
	peer := peer.NewPeer(cfg)
	// defer means do it at the	end of the function
	defer peer.PeerShutdown()
	err := peer.PeerInit()

	if err != nil {
		fmt.Println("[PEER] Initializing peer failed")
		os.Exit(1)
	}

	// initialize the node listening for data from slips
	slist := slistener.SListener{Peer: peer}
	go slist.Run()

	// run tests
	// TODO: remove tests for production
	go tests.RunTests(cfg.RedisDb, cfg.RedisChannelPyGo)

	// neatly exit when termination signal is received
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	<-ch
	fmt.Printf("\nReceived signal, shutting down...\n")

	peer.Close()
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
func renameFilesAndChannels(cfg *utils.Config) {
	if cfg.RenameWithPort {
		// if file name is empty, it means that file saving should not be used
		// therefore port should not be added to empty file names, as this would make them no longer empty
		if cfg.KeyFile != "" {
			cfg.KeyFile = fmt.Sprintf("%s%d", cfg.KeyFile, cfg.ListenPort)
		}
		if cfg.PeerstoreFile != "" {
			cfg.PeerstoreFile = fmt.Sprintf("%s%d", cfg.PeerstoreFile, cfg.ListenPort)
		}
		cfg.RedisChannelGoPy = fmt.Sprintf("%s%d", cfg.RedisChannelGoPy, cfg.ListenPort)
		cfg.RedisChannelPyGo = fmt.Sprintf("%s%d", cfg.RedisChannelPyGo, cfg.ListenPort)
	}
}
