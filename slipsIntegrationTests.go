package main

import (
	"github.com/go-redis/redis/v7"
	"time"
)

func runTests(dbAddress string){
	rdb := redis.NewClient(&redis.Options{
		Addr:     dbAddress,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	time.Sleep(10 * time.Second)
	var data string

	//data = "{\"message\": \"ewogICAgImtleV90eXBlIjogImlwIiwKICAgICJrZXkiOiAiMS4yLjMuNDAiLAogICAgImV........jYKfQ==\"" +
	//	", \"recipient\": \"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N\"}"
	//rdb.Publish("p2p_pygo", data)

	// missing recipient
	data = "{\"message\": \"ewogICAgImtleV90eXBlIjogImlwIiwKICAgICJrZXkiOiAiMS4yLjMuNDAiLAogICAgImV........jYKfQ==\"}"
	rdb.Publish("p2p_pygo", data)

	// missing message
	data = "{\"recipient\": \"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N\"}"
	rdb.Publish("p2p_pygo", data)

	// additional field
	data = "{\"message\": \"ewogICAgImtleV90eXBlIjogImlwIiwKICAgICJrZXkiOiAiMS4yLjMuNDAiLAogICAgImV........jYKfQ==\"" +
		", \"recipient\": \"*\", \"foo\": 3}"
	rdb.Publish("p2p_pygo", data)
}