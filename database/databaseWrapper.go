package database

import (
	"fmt"

	"github.com/go-redis/redis/v7"
)

var DBW *DBWrapper

type DBWrapper struct {
	DbAddress string
	Rdb       *redis.Client
	RdbGoPy   string
	RdbPyGo   string
	Ch        <-chan *redis.Message // this channel is only to be used by the slipsListener
}

func (dw *DBWrapper) InitDB() bool {
	dw.Rdb = redis.NewClient(&redis.Options{
		Addr:     dw.DbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	pongErr := dw.Rdb.Ping().Err()

	if pongErr != nil {
		fmt.Println("[PEER] Database connection failed -", pongErr)
		return false
	}

	if !dw.subscribeToPyGo() {
		fmt.Println("[PEER] Channel subscription failed")
		return false
	}
	fmt.Println("[db] Database connection succeeded - ", dw.DbAddress)

	return true
}

func (dw *DBWrapper) SaveMultiAddress(multiAddress string) {
	// expiration 0 means the key won't expire

	//fmt.Printf("[db] setting multiaddr to -  %s \n", multiAddress)
	dw.Rdb.Set("multiAddress", multiAddress, 0)

}

func (dw *DBWrapper) SendStringToChannel(message string) {
	// sending the msg taken from the peer, to slips python module
	//fmt.Println("[MESSAGE TO p2p_gopy]", message)
	//fmt.Println("[pigeon -> p2p module]", message)
	dw.Rdb.Publish(dw.RdbGoPy, message)
}

func (dw *DBWrapper) subscribeToPyGo() bool {
	// taken from https://godoc.org/github.com/go-redis/redis#example-PubSub-Receive
	pubsub := dw.Rdb.Subscribe(dw.RdbPyGo)

	// Wait for confirmation that subscription is created before publishing anything.
	_, err := pubsub.Receive()
	if err != nil {
		fmt.Printf("[ERROR] Database connection failed - %s\n", err)
		return false
	}

	dw.Ch = pubsub.Channel()

	// TODO: there was a part here that prevented the sample from working alongside SLIPS. I need to look into that.
	// time.AfterFunc(time.Second, func() {
	//    // When pubsub is closed channel is closed too.
	//    _ = pubsub.Close()
	//})
	return true
}
