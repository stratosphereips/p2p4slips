package peer

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	libpeer "github.com/libp2p/go-libp2p/core/peer"
)

func TestCatchupSignatureAndRelay(t *testing.T) {
	key, _, _ := crypto.GenerateEd25519Key(nil)
	id, _ := libpeer.IDFromPrivateKey(key)
	item := catchupItem{Record: map[string]interface{}{"author": id.String(), "target": "192.0.2.1", "last_seen": 123.0}}
	if err := signIncident(key, &item); err != nil {
		t.Fatal(err)
	}
	item.Record["target"] = "forged"
	if err := verifyIncident(&item); err != nil {
		t.Fatal(err)
	}
	if item.Record["target"] != "192.0.2.1" || item.VerifiedAuthor != id.String() {
		t.Fatal("origin payload was not preserved")
	}
	item.Seal = base64.StdEncoding.EncodeToString([]byte("forged signature"))
	if verifyIncident(&item) == nil {
		t.Fatal("forgery accepted")
	}
}

func TestCatchupCannotSignAnotherOrigin(t *testing.T) {
	key, _, _ := crypto.GenerateEd25519Key(nil)
	item := catchupItem{Record: map[string]interface{}{"author": "somebody else"}}
	if signIncident(key, &item) == nil {
		t.Fatal("signed for another peer")
	}
}

func TestCatchupPageVerifiesEveryRecord(t *testing.T) {
	key, _, _ := crypto.GenerateEd25519Key(nil)
	id, _ := libpeer.IDFromPrivateKey(key)
	p := &Peer{privKey: key}
	raw, _ := json.Marshal(map[string]interface{}{"message_type": "catchup_page", "items": []catchupItem{{Record: map[string]interface{}{"author": id.String(), "target": "192.0.2.2"}}}})
	signed, err := p.transformCatchup(base64.StdEncoding.EncodeToString(raw), true)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := p.transformCatchup(signed, false)
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(verified))
	var result map[string]interface{}
	_ = json.Unmarshal(bytes, &result)
	if result["verified"] != true {
		t.Fatal("missing locally verified marker")
	}
}

func TestLimitedLineRejectsUnterminatedAndLargeFrames(t *testing.T) {
	for _, value := range []string{strings.Repeat("x", 10000), "short"} {
		if _, err := readLimitedLine(strings.NewReader(value), 100); err == nil {
			t.Fatal("unbounded frame accepted")
		}
	}
	if value, err := readLimitedLine(strings.NewReader("hello\n"), 100); err != nil || value != "hello\n" {
		t.Fatal("valid line rejected")
	}
}

func TestHelloAdvertisesOnlyEnabledCatchup(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		p := &Peer{catchupEnabled: enabled}
		if strings.Contains(p.helloMessage(), "catchup1") != enabled {
			t.Fatal("capability advertisement does not match setting")
		}
	}
}
