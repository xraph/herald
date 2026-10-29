package push_test

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/driver/push"
)

var sealedNow = time.Date(2026, 10, 28, 20, 44, 0, 0, time.UTC)

func sealedSeed() []byte { return bytes.Repeat([]byte{0x07}, 32) }

func sealedDevice(t *testing.T) *ecdh.PrivateKey {
	t.Helper()
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func sealedData(gateway string) map[string]string {
	return map[string]string{
		"signing_key": base64.StdEncoding.EncodeToString(sealedSeed()),
		"gateway_url": gateway,
	}
}

func sealedDriver() *push.SealedDriver {
	return &push.SealedDriver{Now: func() time.Time { return sealedNow }}
}

const sealedHandle = "h1.0011223344556677.abc"

func TestSealedSendSignsAndSeals(t *testing.T) {
	srv := drivertest.NewServer(t, 202, `{"id":"gw-1"}`)
	dev := sealedDevice(t)
	msg := &driver.OutboundMessage{
		To: sealedHandle, Title: "Checkout is failing", Text: "Error rate 14%",
		Data: sealedData(srv.URL), PublicKey: dev.PublicKey().Bytes(),
		Push: &driver.PushOptions{
			Urgency: driver.UrgencyCritical, Category: "alert", ThreadID: "prj_1",
			CollapseID: "c0ffee0123456789", TTL: time.Hour,
			Data: map[string]string{"kind": "agent.report"},
		},
	}
	res, err := sealedDriver().Send(t.Context(), msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.ProviderMessageID != "gw-1" {
		t.Errorf("ProviderMessageID = %q", res.ProviderMessageID)
	}
	c := srv.Captured
	if c.Method != "POST" || c.Path != "/v1/send" {
		t.Errorf("request = %s %s", c.Method, c.Path)
	}
	if got := c.Header.Get(push.HeaderTimestamp); got != "1793220240" {
		t.Errorf("timestamp = %q", got)
	}
	pub := ed25519.NewKeyFromSeed(sealedSeed()).Public().(ed25519.PublicKey)
	if got := c.Header.Get(push.HeaderKey); got != base64.StdEncoding.EncodeToString(pub) {
		t.Errorf("key header = %q", got)
	}
	sig, err := base64.StdEncoding.DecodeString(c.Header.Get(push.HeaderSignature))
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(pub, push.SignatureBase("1793220240", c.Body), sig) {
		t.Error("signature does not verify")
	}

	var body push.SendBody
	c.DecodeJSON(t, &body)
	if body.Handle != sealedHandle || body.KeyID != push.KeyID(dev.PublicKey().Bytes()) ||
		body.Urgency != "critical" || body.Kind != "show" ||
		body.CollapseID != "c0ffee0123456789" || body.TTLSeconds != 3600 {
		t.Errorf("body = %+v", body)
	}
	sealed, err := base64.StdEncoding.DecodeString(body.Sealed)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := push.Open(dev.Bytes(), sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var env push.Envelope
	if err := json.Unmarshal(plain, &env); err != nil {
		t.Fatal(err)
	}
	want := push.Envelope{V: 1, Title: "Checkout is failing", Body: "Error rate 14%", Category: "alert", Thread: "prj_1", Data: map[string]string{"kind": "agent.report"}}
	if !reflect.DeepEqual(env, want) {
		t.Errorf("envelope = %+v, want %+v", env, want)
	}
}

func TestSealedSendDefaults(t *testing.T) {
	srv := drivertest.NewServer(t, 202, `{"id":"gw-1"}`)
	dev := sealedDevice(t)
	msg := &driver.OutboundMessage{To: sealedHandle, Title: "t", Text: "b", Data: sealedData(srv.URL), PublicKey: dev.PublicKey().Bytes()}
	if _, err := sealedDriver().Send(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	srv.Captured.DecodeJSON(t, &raw)
	if raw["urgency"] != "active" || raw["kind"] != "show" {
		t.Errorf("body = %v", raw)
	}
	for _, k := range []string{"ttl_seconds", "collapse_id"} {
		if _, ok := raw[k]; ok {
			t.Errorf("%s present in %v", k, raw)
		}
	}
}

func TestSealedWithdrawCarriesNoSeal(t *testing.T) {
	srv := drivertest.NewServer(t, 202, `{"id":"gw-1"}`)
	dev := sealedDevice(t)
	msg := &driver.OutboundMessage{To: sealedHandle, Data: sealedData(srv.URL), PublicKey: dev.PublicKey().Bytes(),
		Push: &driver.PushOptions{Kind: driver.PushKindWithdraw, CollapseID: "c1"}}
	if _, err := sealedDriver().Send(t.Context(), msg); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	srv.Captured.DecodeJSON(t, &raw)
	if _, ok := raw["sealed"]; ok {
		t.Error("withdraw carries a sealed key")
	}
	if raw["kind"] != "withdraw" || raw["key_id"] == "" || raw["key_id"] == nil {
		t.Errorf("body = %v", raw)
	}
}

func TestSealedRefusesOversizedEnvelope(t *testing.T) {
	srv := drivertest.NewServer(t, 202, `{"id":"gw-1"}`)
	dev := sealedDevice(t)
	msg := &driver.OutboundMessage{To: sealedHandle, Title: "t", Text: strings.Repeat("x", 3000), Data: sealedData(srv.URL), PublicKey: dev.PublicKey().Bytes()}
	_, err := sealedDriver().Send(t.Context(), msg)
	if !errors.Is(err, push.ErrPayloadTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if srv.Captured.Method != "" {
		t.Error("the gateway saw a request")
	}
}

func TestSealedNeedsPublicKey(t *testing.T) {
	srv := drivertest.NewServer(t, 202, `{"id":"gw-1"}`)
	msg := &driver.OutboundMessage{To: sealedHandle, Title: "t", Text: "b", Data: sealedData(srv.URL)}
	_, err := sealedDriver().Send(t.Context(), msg)
	if !errors.Is(err, push.ErrMissingPublicKey) {
		t.Fatalf("err = %v", err)
	}
	if srv.Captured.Method != "" {
		t.Error("the gateway saw a request")
	}
}

func TestSealedResponses(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		gone    bool
		contain string
	}{
		{"gone", 410, `{"error":"address gone"}`, true, ""},
		{"stale", 401, `{"error":"timestamp outside the allowed window"}`, false, "timestamp outside the allowed window"},
		{"limited", 429, `{"error":"slow down"}`, false, "429"},
		{"bad gateway", 502, `not json`, false, "502"},
		{"long error", 500, `{"error":"` + strings.Repeat("e", 500) + `"}`, false, "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := drivertest.NewServer(t, tt.status, tt.body)
			dev := sealedDevice(t)
			msg := &driver.OutboundMessage{To: sealedHandle, Title: "t", Text: "b", Data: sealedData(srv.URL), PublicKey: dev.PublicKey().Bytes()}
			_, err := sealedDriver().Send(t.Context(), msg)
			if err == nil {
				t.Fatal("no error")
			}
			if errors.Is(err, driver.ErrAddressGone) != tt.gone {
				t.Errorf("ErrAddressGone = %v, want %v (%v)", !tt.gone, tt.gone, err)
			}
			if !strings.Contains(err.Error(), tt.contain) {
				t.Errorf("err %q lacks %q", err, tt.contain)
			}
			var body push.SendBody
			srv.Captured.DecodeJSON(t, &body)
			if strings.Contains(err.Error(), sealedHandle) || strings.Contains(err.Error(), body.Sealed) {
				t.Errorf("error leaks the handle or sealed string: %v", err)
			}
			if len(err.Error()) > 300 {
				t.Errorf("error is %d bytes", len(err.Error()))
			}
		})
	}
}

func TestSealedValidate(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(sealedSeed())
	short := base64.StdEncoding.EncodeToString(sealedSeed()[:31])
	tests := []struct {
		name    string
		key     string
		gateway string
		ok      bool
	}{
		{"missing key", "", "https://push.example.com", false},
		{"not base64", "!!", "https://push.example.com", false},
		{"31 bytes", short, "https://push.example.com", false},
		{"plain http", good, "http://push.example.com", false},
		{"ftp", good, "ftp://x", false},
		{"missing url", good, "", false},
		{"https", good, "https://push.example.com", true},
		{"loopback http", good, "http://127.0.0.1:8080", true},
		{"ipv6 loopback", good, "http://[::1]:8080", true},
	}
	d := &push.SealedDriver{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := d.Validate(map[string]string{"signing_key": tt.key}, map[string]string{"gateway_url": tt.gateway})
			if (err == nil) != tt.ok {
				t.Errorf("Validate = %v, want ok=%v", err, tt.ok)
			}
			if err != nil && tt.key != "" && strings.Contains(err.Error(), tt.key) {
				t.Errorf("error leaks the key: %v", err)
			}
		})
	}
}
