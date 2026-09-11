package database

import (
	"errors"
	"os"
	"testing"
)

func TestConnectionEntryChangesWithReconnectPort(t *testing.T) {
	first := AuthenticatedConnection{
		PeerID: "peer-a", Protocol: "tcp", LocalIP: "192.0.2.10",
		LocalPort: "6668", RemoteIP: "198.51.100.20", RemotePort: "43123",
	}
	second := first
	second.RemotePort = "51000"

	if ConnectionEntry(first) == ConnectionEntry(second) {
		t.Fatal("connections with changed ports must have different registry entries")
	}
}

func TestReadRequirepassEmptyPathMeansNoPassword(t *testing.T) {
	password, err := ReadRequirepass("")
	if err != nil || password != "" {
		t.Fatalf("got (%q, %v), want (\"\", nil)", password, err)
	}
}

func TestReadRequirepassParsesSlipsAuthConf(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{"plain", "requirepass s3cret\n", "s3cret", false},
		{"with comments and spaces", "# slips redis auth\n\n  requirepass   abc-123_XYZ  \n", "abc-123_XYZ", false},
		{"missing line", "maxmemory 1gb\n", "", true},
		{"empty password", "requirepass \n", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/redis_auth.conf"
			if err := os.WriteFile(path, []byte(test.content), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := ReadRequirepass(path)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestReadRequirepassMissingFile(t *testing.T) {
	if _, err := ReadRequirepass(t.TempDir() + "/does-not-exist.conf"); err == nil {
		t.Fatal("expected an error for a missing conf file")
	}
}

func TestNoPasswordNeededMatchesBothRedisWordings(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("ERR Client sent AUTH, but no password is set"), true},
		{errors.New("ERR AUTH <password> called without any password configured for the default user"), true},
		{errors.New("NOAUTH Authentication required."), false},
		{nil, false},
	}
	for _, test := range tests {
		if got := noPasswordNeeded(test.err); got != test.want {
			t.Fatalf("noPasswordNeeded(%v) = %v, want %v", test.err, got, test.want)
		}
	}
}
