package database

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-redis/redis/v7"
)

var DBW *DBWrapper

// requirepassPrefix is the redis.conf directive slips writes the shared
// password with; the python side parses the same line.
const requirepassPrefix = "requirepass "

// ReadRequirepass returns the password from the "requirepass <password>"
// line of the given redis conf file. An empty path means "no password"
// and yields "" without touching the disk.
func ReadRequirepass(confPath string) (string, error) {
	if confPath == "" {
		return "", nil
	}
	// #nosec G304 -- confPath is the -redis-auth-conf flag chosen by slips, not remote input
	file, err := os.Open(confPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, requirepassPrefix) {
			password := strings.TrimSpace(strings.TrimPrefix(line, requirepassPrefix))
			if password == "" {
				return "", fmt.Errorf("%s has an empty requirepass line", confPath)
			}
			return password, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("%s has no requirepass line", confPath)
}

// noPasswordNeeded reports whether redis answered an AUTH with "I have no
// password configured" (worded differently by redis < 6 and >= 6), which
// means the server predates slips' redis auth and wants no password.
func noPasswordNeeded(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "no password is set") ||
		strings.Contains(text, "without any password configured")
}

// NewRedisClient connects to addr, authenticating with password when it is
// not empty. If the server says it has no password configured, it retries
// once without one, mirroring slips' python redis clients.
func NewRedisClient(addr string, password string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0, // use default DB
	})
	err := client.Ping().Err()
	if err == nil {
		return client, nil
	}
	if password == "" || !noPasswordNeeded(err) {
		_ = client.Close()
		return nil, err
	}
	fmt.Println("[db] redis has no password configured, connecting without one")
	_ = client.Close()
	client = redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   0,
	})
	if err := client.Ping().Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

type DBWrapper struct {
	DbAddress    string
	Password     string
	Rdb          *redis.Client
	RdbGoPy      string
	RdbPyGo      string
	SlipsVersion string
	Ch           <-chan *redis.Message // this channel is only to be used by the slipsListener
}

func (dw *DBWrapper) InitDB() bool {
	client, err := NewRedisClient(dw.DbAddress, dw.Password)
	if err != nil {
		fmt.Println("[PEER] Database connection failed -", err)
		if errors.Is(err, redis.Nil) || strings.Contains(err.Error(), "NOAUTH") {
			fmt.Println("[PEER] redis requires a password: pass slips' redis auth conf with -redis-auth-conf")
		}
		return false
	}
	dw.Rdb = client

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
	PeerID     string `json:"peer_id"`
	Protocol   string `json:"protocol"`
	LocalIP    string `json:"local_ip"`
	LocalPort  string `json:"local_port"`
	RemoteIP   string `json:"remote_ip"`
	RemotePort string `json:"remote_port"`
}

// connectionsSet is the single redis set slips reads to know which
// exact tuples are Slips's own authenticated P2P connections.
const connectionsSet = "p2p:connections"

// ConnectionEntry is the member stored in connectionsSet, matching the
// 5-tuple slips uses to match a flow: protocol|local_ip|local_port|
// remote_ip|remote_port.
func ConnectionEntry(connection AuthenticatedConnection) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s",
		connection.Protocol, connection.LocalIP, connection.LocalPort,
		connection.RemoteIP, connection.RemotePort)
}

func (dw *DBWrapper) StoreAuthenticatedConnection(connection AuthenticatedConnection) {
	dw.Rdb.SAdd(connectionsSet, ConnectionEntry(connection))
	dw.publishConnectionUpdate(connection, true)
}

func (dw *DBWrapper) RemoveAuthenticatedConnection(connection AuthenticatedConnection) {
	dw.Rdb.SRem(connectionsSet, ConnectionEntry(connection))
	dw.publishConnectionUpdate(connection, false)
}

func (dw *DBWrapper) publishConnectionUpdate(connection AuthenticatedConnection, connected bool) {
	message := map[string]interface{}{
		"message_type": "connection_update",
		"message_contents": map[string]interface{}{
			"peer_id":     connection.PeerID,
			"protocol":    connection.Protocol,
			"local_ip":    connection.LocalIP,
			"local_port":  connection.LocalPort,
			"remote_ip":   connection.RemoteIP,
			"remote_port": connection.RemotePort,
			"connected":   connected,
		},
	}
	payload, err := json.Marshal(message)
	if err != nil {
		fmt.Println("[db] Could not serialize connection update -", err)
		return
	}
	dw.SendStringToChannel(string(payload))
}
