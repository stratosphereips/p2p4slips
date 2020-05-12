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

	data := "{\"message\": \"ewogICAgImtleV90eXBlIjogImlwIiwKICAgICJrZXkiOiAiMS4yLjMuNDAiLAogICAgImV........jYKfQ==\"" +
		", \"recipient\": \"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N\"}"
	rdb.Publish("p2p_pygo", data)

}