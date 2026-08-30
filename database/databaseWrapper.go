package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v7"
)

var DBW *DBWrapper

type DBWrapper struct {
	DbAddress    string
	Rdb          *redis.Client
	RdbGoPy      string
	RdbPyGo      string
	SlipsVersion string
	Ch           <-chan *redis.Message // this channel is only to be used by the slipsListener
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
	if dw.SlipsVersion != "" {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(message), &payload); err != nil {
			fmt.Println("[db] Could not version outgoing message -", err)
		} else {
			payload["version"] = dw.SlipsVersion
			versionedMessage, err := json.Marshal(payload)
			if err != nil {
				fmt.Println("[db] Could not serialize versioned message -", err)
			} else {
				message = string(versionedMessage)
			}
		}
	}
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

type AuthenticatedConnection struct {
	PeerID        string `json:"peer_id"`
	Protocol      string `json:"protocol"`
	LocalIP       string `json:"local_ip"`
	LocalPort     string `json:"local_port"`
	RemoteIP      string `json:"remote_ip"`
	RemotePort    string `json:"remote_port"`
	Authenticated bool   `json:"authenticated"`
	Connected     bool   `json:"connected"`
	LastSeen      int64  `json:"last_seen"`
}

const activeConnectionIndex = "p2p:active_connections"
const activeConnectionPrefix = "p2p:active_connection:"
const recentConnectionIndex = "p2p:recent_connections"
const recentConnectionPrefix = "p2p:recent_connection:"

func ConnectionID(connection AuthenticatedConnection) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s", connection.PeerID,
		connection.Protocol, connection.LocalIP, connection.LocalPort,
		connection.RemoteIP, connection.RemotePort)
}

func (dw *DBWrapper) StoreAuthenticatedConnection(connection AuthenticatedConnection, ttl time.Duration) {
	connection.Authenticated = true
	connection.Connected = true
	connection.LastSeen = time.Now().Unix()
	connectionID := ConnectionID(connection)
	payload, err := json.Marshal(connection)
	if err != nil {
		fmt.Println("[db] Could not serialize authenticated connection -", err)
		return
	}
	dw.Rdb.SAdd(activeConnectionIndex, connectionID)
	dw.Rdb.Set(activeConnectionPrefix+connectionID, payload, ttl)
	dw.Rdb.SRem(recentConnectionIndex, connectionID)
	dw.Rdb.Del(recentConnectionPrefix + connectionID)
	dw.publishConnectionUpdate(connection)
}

func (dw *DBWrapper) RemoveAuthenticatedConnection(connection AuthenticatedConnection, grace time.Duration) {
	connection.Authenticated = true
	connection.Connected = false
	connection.LastSeen = time.Now().Unix()
	connectionID := ConnectionID(connection)
	dw.Rdb.SRem(activeConnectionIndex, connectionID)
	dw.Rdb.Del(activeConnectionPrefix + connectionID)
	if grace > 0 {
		payload, err := json.Marshal(connection)
		if err == nil {
			dw.Rdb.SAdd(recentConnectionIndex, connectionID)
			dw.Rdb.Set(recentConnectionPrefix+connectionID, payload, grace)
		}
	}
	dw.publishConnectionUpdate(connection)
}

func (dw *DBWrapper) publishConnectionUpdate(connection AuthenticatedConnection) {
	message := map[string]interface{}{
		"message_type":     "connection_update",
		"message_contents": connection,
	}
	payload, err := json.Marshal(message)
	if err != nil {
		fmt.Println("[db] Could not serialize connection update -", err)
		return
	}
	dw.SendStringToChannel(string(payload))
}
