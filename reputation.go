package main

import (
	"encoding/json"
	"fmt"
)

type Reputation struct {
	Peerid       string
	Multiaddr    string
	Version      string
	Interactions []int
	Uptime       []int
}

func (r *Reputation) rep2json() string {
	byteData, err := json.Marshal(r)
	data := string(byteData)
	if err != nil {
		fmt.Println(err)
	}
	return data
}