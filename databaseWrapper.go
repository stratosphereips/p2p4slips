package main


import (
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v7"
	"time"
)

type PeerDataUpdate struct {
	PeerID       string    `json:"peerid"`
	Ip           string    `json:"ip,omitempty"`
	Reliability  float64   `json:"reliability,omitempty"`
	Timestamp    int64     `json:"timestamp"`
	// `json...` makes the properties to parse correctly when using json.Marshal
}

func (p *PeerDataUpdate) pdu2json() string {
	byteData, err := json.Marshal(p)
	data := string(byteData)
	if err != nil {
		fmt.Println(err)
	}
	return data
}

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

func (dw *DBWrapper) sharePeerDataUpdate(data *PeerData){
	peerID := data.peerID
	ip := data.lastUsedIP
	reliability := data.Reliability

	pdu := &PeerDataUpdate{
		PeerID:      peerID,
		Ip:          ip,
		Reliability: reliability,
		Timestamp:   time.Now().Unix(),
	}

	strJson := pdu.pdu2json()

	dw.sendStringToChannel("peer_update " + strJson)
}

func (dw *DBWrapper) sendStringToChannel(message string){

	fmt.Println("[MESSAGE TO p2p_gopy]", message)

	dw.rdb.Publish(dw.rdbGoPy, message)
}

func (dw *DBWrapper) sendBytesToChannel(message []byte){

	fmt.Println("[MESSAGE TO p2p_gopy]", message)

	dw.rdb.Publish(dw.rdbGoPy, message)
}

func (dw *DBWrapper) subscribeToPyGo() bool {
	// taken from https://godoc.org/github.com/go-redis/redis#example-PubSub-Receive
	pubsub := dbw.rdb.Subscribe(dw.rdbPyGo)

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
