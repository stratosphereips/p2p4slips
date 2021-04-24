package peer

import (
	"encoding/json"
	"fmt"
	"time"
)

type PeerUpdateStruct struct {
	PeerID       string    `json:"peerid"`
	Ip           string    `json:"ip,omitempty"`
	Reliability  float64   `json:"reliability,omitempty"`
	Timestamp    int64     `json:"timestamp"`
	// `json...` makes the properties to parse correctly when using json.Marshal
}

type ReportStruct struct {
	Reporter  string `json:"reporter"`
	ReportTme int64  `json:"report_time"`
	Message   string `json:"message"`
}

type UpdateMessage struct {
	MessageType string `json:"message_type"`
	MessageContents PeerUpdateStruct `json:"message_contents"`
}

type ReportMessage struct {
	MessageType string `json:"message_type"`
	MessageContents ReportStruct `json:"message_contents"`
}

func (p *UpdateMessage) pdu2json() string {
	byteData, err := json.Marshal(p)
	data := string(byteData)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(data)
	return data
}

func (p *ReportMessage) pdu2json() string {
	byteData, err := json.Marshal(p)
	data := string(byteData)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(data)
	return data
}


func (dw *DBWrapper) shareReport(data *ReportStruct){
	pdum := &ReportMessage{
		MessageType:     "go_data",
		MessageContents: *data,
	}

	strJson := pdum.pdu2json()

	dw.sendStringToChannel(strJson)
}

func (dw *DBWrapper) sharePeerDataUpdate(data *PeerData){
	peerID := data.peerID
	ip := data.lastUsedIP
	reliability := data.Reliability

	pdum := &UpdateMessage{
		MessageType:     "peer_update",
		MessageContents: PeerUpdateStruct{
			PeerID:      peerID,
			Ip:          ip,
			Reliability: reliability,
			Timestamp:   time.Now().Unix(),
		},
	}

	strJson := pdum.pdu2json()

	dw.sendStringToChannel(strJson)
}
