package relay

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestIsPublicIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",       // loopback
		"::1",             // loopback v6
		"10.1.2.3",        // private
		"192.168.1.10",    // private
		"172.16.0.5",      // private
		"169.254.169.254", // cloud metadata
		"0.0.0.0",         // unspecified
		"224.0.0.1",       // multicast
		"fd00::1",         // unique local
		"fe80::1",         // link local
	}
	for _, addr := range blocked {
		if isPublicIP(net.ParseIP(addr)) {
			t.Errorf("%s must not be reachable through mirroring", addr)
		}
	}

	for _, addr := range []string{"1.1.1.1", "93.184.216.34", "2606:4700:4700::1111"} {
		if !isPublicIP(net.ParseIP(addr)) {
			t.Errorf("%s is a public address and should be allowed", addr)
		}
	}
}

func blossomAuth(t *testing.T, sk, action string, expires time.Time) string {
	t.Helper()
	evt := nostr.Event{
		Kind:      24242,
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"t", action},
			{"expiration", strconv.FormatInt(expires.Unix(), 10)},
		},
	}
	if err := evt.Sign(sk); err != nil {
		t.Fatalf("signing: %v", err)
	}
	raw, _ := json.Marshal(evt)
	return "Nostr " + base64.StdEncoding.EncodeToString(raw)
}

func TestReadBlossomAuth(t *testing.T) {
	sk := nostr.GeneratePrivateKey()
	pk, _ := nostr.GetPublicKey(sk)

	request := func(header string) *http.Request {
		r := httptest.NewRequest("PUT", "/mirror", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		return r
	}

	t.Run("valid", func(t *testing.T) {
		evt, err := readBlossomAuth(request(blossomAuth(t, sk, "upload", time.Now().Add(time.Minute))), "upload")
		if err != nil {
			t.Fatalf("rejected a valid authorization: %v", err)
		}
		if evt.PubKey != pk {
			t.Errorf("pubkey = %s, want %s", evt.PubKey, pk)
		}
	})

	t.Run("expired", func(t *testing.T) {
		if _, err := readBlossomAuth(request(blossomAuth(t, sk, "upload", time.Now().Add(-time.Minute))), "upload"); err == nil {
			t.Error("an expired authorization was accepted")
		}
	})

	t.Run("wrong action", func(t *testing.T) {
		if _, err := readBlossomAuth(request(blossomAuth(t, sk, "get", time.Now().Add(time.Minute))), "upload"); err == nil {
			t.Error("an authorization signed for another action was accepted")
		}
	})

	t.Run("missing header", func(t *testing.T) {
		if _, err := readBlossomAuth(request(""), "upload"); err == nil {
			t.Error("a request without authorization was accepted")
		}
	})

	t.Run("tampered event", func(t *testing.T) {
		header := blossomAuth(t, sk, "upload", time.Now().Add(time.Minute))
		raw, _ := base64.StdEncoding.DecodeString(header[len("Nostr "):])
		var evt nostr.Event
		json.Unmarshal(raw, &evt)
		evt.Tags = append(evt.Tags, nostr.Tag{"t", "delete"})
		forged, _ := json.Marshal(evt)
		if _, err := readBlossomAuth(request("Nostr "+base64.StdEncoding.EncodeToString(forged)), "delete"); err == nil {
			t.Error("an event whose tags were changed after signing was accepted")
		}
	})
}
