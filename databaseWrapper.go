package main


import (
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v7"
)

type PeerDataUpdate struct {
	PeerID       string    `json:"peerid"`
	Ip           string    `json:"ip"`
	Reliability  float64   `json:"reliability,omitempty"`
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
}

func (dw *DBWrapper) initDB(){
	dw.rdb = redis.NewClient(&redis.Options{
		Addr:     dw.dbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	pongErr := dw.rdb.Ping().Err()

	if pongErr != nil {
		fmt.Println("[PEER] Database connection failed -", pongErr)
	}
}

func (dw *DBWrapper) sharePeerDataUpdate(data *PeerData){
	peerID := data.PeerID
	ip := data.LastUsedIP
	reliability := data.Reliability

	pdu := &PeerDataUpdate{
		PeerID:      peerID,
		Ip:          ip,
		Reliability: reliability,
	}

	strJson := pdu.pdu2json()

	dw.sendStringToChannel("peer_update " + strJson)
}

func (dw *DBWrapper) sendStringToChannel(message string){

	fmt.Println("[MESSAGE TO p2p_gopy]", message)

	dw.rdb.Publish(dw.rdbGoPy, message)
}
