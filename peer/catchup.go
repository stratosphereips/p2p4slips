package peer

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/record"
)

const catchupDomain = "slips-network-incident-v1"
const maxCatchupPage = 65536
const maxCatchupRecord = 8192

// helloMessage advertises catch-up only when the local worker enables it.
func (p *Peer) helloMessage() string {
	if p.catchupEnabled {
		return "hello version1-catchup1\n"
	}
	return "hello version1\n"
}

// incidentRecord preserves signed observation bytes across untrusted forwarders.
type incidentRecord struct{ Data []byte }

func (*incidentRecord) Domain() string                   { return catchupDomain }
func (*incidentRecord) Codec() []byte                    { return []byte("/slips/network-incident/1") }
func (r *incidentRecord) MarshalRecord() ([]byte, error) { return r.Data, nil }
func (r *incidentRecord) UnmarshalRecord(data []byte) error {
	if len(data) > maxCatchupRecord {
		return errors.New("incident exceeds size limit")
	}
	r.Data = append([]byte(nil), data...)
	return nil
}
func init() { record.RegisterType(&incidentRecord{}) }

// readLimitedLine bounds allocation before a peer-controlled newline arrives.
func readLimitedLine(reader io.Reader, limit int64) (string, error) {
	line, err := bufio.NewReader(io.LimitReader(reader, limit+1)).ReadString('\n')
	if int64(len(line)) > limit || !strings.HasSuffix(line, "\n") {
		return "", errors.New("oversized or incomplete peer frame")
	}
	return line, err
}

// catchupMessage identifies versioned catch-up JSON without changing legacy messages.
func catchupMessage(message string) (map[string]json.RawMessage, bool) {
	if len(message) > 512*1024 {
		return nil, false
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(message))
	if err != nil {
		return nil, false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil {
		return nil, false
	}
	var kind string
	_ = json.Unmarshal(object["message_type"], &kind)
	return object, kind == "catchup_request" || kind == "catchup_page"
}

type catchupItem struct {
	Record         map[string]interface{} `json:"record"`
	Seal           string                 `json:"seal"`
	VerifiedAuthor string                 `json:"verified_author,omitempty"`
}

// signIncident signs first-hand records only; an origin cannot be replaced by a relay.
func signIncident(key crypto.PrivKey, item *catchupItem) error {
	origin, err := libpeer.IDFromPrivateKey(key)
	if err != nil {
		return err
	}
	if item.Record["author"] != origin.String() {
		return errors.New("cannot sign another author's incident")
	}
	payload, err := json.Marshal(item.Record)
	if err != nil || len(payload) > maxCatchupRecord {
		return errors.New("invalid incident payload")
	}
	envelope, err := record.Seal(&incidentRecord{Data: payload}, key)
	if err != nil {
		return err
	}
	encoded, err := envelope.Marshal()
	if err != nil {
		return err
	}
	item.Seal = base64.StdEncoding.EncodeToString(encoded)
	return nil
}

// verifyIncident uses the signed payload as authoritative, ignoring supplied display fields.
func verifyIncident(item *catchupItem) error {
	if len(item.Seal) > 16384 {
		return errors.New("oversized signature envelope")
	}
	encoded, err := base64.StdEncoding.DecodeString(item.Seal)
	if err != nil {
		return err
	}
	envelope, payload, err := record.ConsumeEnvelope(encoded, catchupDomain)
	if err != nil {
		return err
	}
	incident, ok := payload.(*incidentRecord)
	if !ok {
		return errors.New("wrong signed record type")
	}
	var object map[string]interface{}
	if json.Unmarshal(incident.Data, &object) != nil {
		return errors.New("invalid signed JSON")
	}
	author, err := libpeer.IDFromPublicKey(envelope.PublicKey)
	if err != nil {
		return err
	}
	if object["author"] != author.String() {
		return errors.New("signed author does not match peer key")
	}
	item.Record = object
	item.VerifiedAuthor = author.String()
	return nil
}

// transformCatchup signs outgoing own records or verifies every incoming origin.
func (p *Peer) transformCatchup(message string, outgoing bool) (string, error) {
	object, ok := catchupMessage(message)
	if !ok {
		return message, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(message))
	if err != nil || len(raw) > maxCatchupPage {
		return "", errors.New("oversized catch-up page")
	}
	var kind string
	_ = json.Unmarshal(object["message_type"], &kind)
	if kind == "catchup_page" {
		var items []catchupItem
		if json.Unmarshal(object["items"], &items) != nil || len(items) > 50 {
			return "", errors.New("invalid catch-up records")
		}
		for i := range items {
			if outgoing && items[i].Seal == "" {
				err = signIncident(p.privKey, &items[i])
			} else {
				err = verifyIncident(&items[i])
			}
			if err != nil {
				return "", err
			}
			if outgoing {
				items[i].VerifiedAuthor = ""
			}
		}
		object["items"], err = json.Marshal(items)
		if err != nil {
			return "", err
		}
		if outgoing {
			delete(object, "verified")
		} else {
			object["verified"] = json.RawMessage("true")
		}
	}
	raw, err = json.Marshal(object)
	if err != nil || len(raw) > maxCatchupPage {
		return "", errors.New("catch-up page exceeds limit after signing")
	}
	return base64.StdEncoding.EncodeToString(raw) + "\n", nil
}

// allowCatchup limits verification and signing work globally and by authenticated peer.
func (p *Peer) allowCatchup(peerID string) bool {
	p.catchupMu.Lock()
	defer p.catchupMu.Unlock()
	now := time.Now()
	if now.Sub(p.catchupWindow) >= time.Second {
		p.catchupWindow = now
		p.catchupCount = 0
		p.catchupPeers = make(map[string]int)
	}
	if p.catchupCount >= 16 || p.catchupPeers[peerID] >= 4 {
		return false
	}
	p.catchupCount++
	p.catchupPeers[peerID]++
	return true
}
