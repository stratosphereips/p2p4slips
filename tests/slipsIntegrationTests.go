package tests

import (
	"fmt"
	"github.com/go-redis/redis/v7"
	"time"
)

func RunTests(dbAddress string, pyGoChannel string){
	rdb := redis.NewClient(&redis.Options{
		Addr:     dbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	pongErr := rdb.Ping().Err()

	if pongErr != nil {
		fmt.Println("[PEER] Database connection failed -", pongErr)
		return
	}

	if pyGoChannel != "p2p_pygo6667"{
		return
	}

	fmt.Println("[RUNNING TESTS]")

	time.Sleep(3 * time.Second)
	var data string

	//data = "{\"message\": \"ewogICAgImtleV90eXBlIjogImlwIiwKICAgICJrZXkiOiAiMS4yLjMuNDAiLAogICAgImV........jYKfQ==\"" +
	//	", \"recipient\": \"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N\"}"
	// rdb.Publish(pyGoChannel, data)

	// missing recipient
	data = "{\"message\": \"ewogICAgImtleV90eXBlIjogImlwIiwKICAgICJrZXkiOiAiMS4yLjMuNDAiLAogICAgImV........jYKfQ==\"}"
	rdb.Publish(pyGoChannel, data)

	// missing message
	data = "{\"recipient\": \"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N\"}"
	rdb.Publish(pyGoChannel, data)

	// additional field
	data = "{\"message\": \"ewogICAgImtleV90eXBlIjogImlwIiwKICAgICJrZXkiOiAiMS4yLjMuNDAiLAogICAgImV........jYKfQ==\"" +
		", \"recipient\": \"*\", \"foo\": 3}"
	rdb.Publish(pyGoChannel, data)
}
