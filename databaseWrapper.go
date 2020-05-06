package main


import (
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v7"
)

type PeerDataUpdate struct {
	PeerID       string    `json:"peerid"`
	Ip           string    `json:"ip"`
	Reliability  int       `json:"reliability,omitempty"`
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
	// TODO: use the correct reliability measure here, I am not sure what GoodCound really is
	reliability := data.GoodCount

	pdu := &PeerDataUpdate{
		PeerID:      peerID,
		Ip:          ip,
		Reliability: reliability,
	}

	strJson := pdu.pdu2json()

	fmt.Println("JSON of peerdata")
	fmt.Println(strJson)
	fmt.Println("")

}