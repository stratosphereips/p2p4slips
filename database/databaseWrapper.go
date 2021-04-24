package database


import (
	"fmt"
	"github.com/go-redis/redis/v7"
)

type DBWrapper struct {
	dbAddress  string
	rdb  *redis.Client
	rdbGoPy    string
	rdbPyGo    string
	ch         <- chan *redis.Message // this channel is only to be used by the slipsListener
}

func (dw *DBWrapper) initDB() bool {
	dw.rdb = redis.NewClient(&redis.Options{
		Addr:     dw.dbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	pongErr := dw.rdb.Ping().Err()

	if pongErr != nil {
		fmt.Println("[PEER] Database connection failed -", pongErr)
		return false
	}

	if !dw.subscribeToPyGo() {
		fmt.Println("[PEER] Channel subscription failed")
		return false
	}

	return true
}


func (dw *DBWrapper) sendStringToChannel(message string){

	fmt.Println("[MESSAGE TO p2p_gopy]", message)

	dw.rdb.Publish(dw.rdbGoPy, message)
}

func (dw *DBWrapper) subscribeToPyGo() bool {
	// taken from https://godoc.org/github.com/go-redis/redis#example-PubSub-Receive
	pubsub := dw.rdb.Subscribe(dw.rdbPyGo)

	// Wait for confirmation that subscription is created before publishing anything.
	_, err := pubsub.Receive()
	if err != nil {
		fmt.Printf("[ERROR] Database connection failed - %s\n", err)
		return false
	}

	dw.ch = pubsub.Channel()

	// TODO: there was a part here that prevented the sample from working alongside SLIPS. I need to look into that.
	// time.AfterFunc(time.Second, func() {
	//    // When pubsub is closed channel is closed too.
	//    _ = pubsub.Close()
	//})
	return true
}
