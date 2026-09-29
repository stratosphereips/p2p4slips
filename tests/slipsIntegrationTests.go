package tests

import (
	"fmt"
	"time"

	"github.com/stratosphereips/p2p4slips/database"
)

func RunTests(dbAddress string, redisPassword string, pyGoChannel string) {
	rdb, err := database.NewRedisClient(dbAddress, redisPassword)
	if err != nil {
		fmt.Println("[PEER] Database connection failed -", err)
		return
	}
	defer rdb.Close()

	if pyGoChannel != "p2p_pygo6667" {
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
