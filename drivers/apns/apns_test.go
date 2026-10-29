package apns_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/drivers/apns"
	"github.com/xraph/herald/message"
)

func testKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestAPNSSend(t *testing.T) {
	var gotPath, gotAuth, gotTopic string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotTopic = r.Header.Get("apns-topic")
		w.Header().Set("apns-id", "apns-1")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := &apns.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "devicetoken", Title: "Hi", Text: "body",
		Data: map[string]string{
			"key_id": "K1", "team_id": "T1", "bundle_id": "com.example.app",
			"private_key": testKeyPEM(t), "base_url": srv.URL,
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "apns-1" {
		t.Errorf("res = %+v", res)
	}
	if gotPath != "/3/device/devicetoken" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "bearer ") {
		t.Errorf("auth = %q, want bearer <jwt>", gotAuth)
	}
	if gotTopic != "com.example.app" {
		t.Errorf("apns-topic = %q", gotTopic)
	}
}

func TestAPNSValidateBadKey(t *testing.T) {
	d := &apns.Driver{}
	err := d.Validate(map[string]string{
		"key_id": "K", "team_id": "T", "bundle_id": "b", "private_key": "not-a-key",
	}, nil)
	if err == nil {
		t.Error("expected error for invalid private_key")
	}
}

type recorded struct {
	body    []byte
	headers http.Header
}

func newRecorder(t *testing.T, status int, resp string) (*httptest.Server, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.body, _ = io.ReadAll(r.Body) //nolint:errcheck // test helper
		rec.headers = r.Header.Clone()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp)) //nolint:errcheck // test helper
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func sendMsg(t *testing.T, srv *httptest.Server, msg *driver.OutboundMessage) error {
	t.Helper()
	msg.To = "devicetoken"
	msg.Data = map[string]string{
		"key_id": "K1", "team_id": "T1", "bundle_id": "com.example.app",
		"private_key": testKeyPEM(t), "base_url": srv.URL,
	}
	_, err := (&apns.Driver{}).Send(context.Background(), msg)
	return err
}

func TestAPNSWithoutPushOptionsKeepsTheOldPayload(t *testing.T) {
	srv, rec := newRecorder(t, http.StatusOK, "")
	if err := sendMsg(t, srv, &driver.OutboundMessage{Title: "Hi", Text: "body"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	want := `{"aps":{"alert":{"title":"Hi","body":"body"},"sound":"default"}}`
	if string(rec.body) != want {
		t.Errorf("body = %s, want %s", rec.body, want)
	}
	if got := rec.headers.Get("apns-priority"); got != "" {
		t.Errorf("apns-priority = %q, want empty", got)
	}
	if got := rec.headers.Get("apns-push-type"); got != "alert" {
		t.Errorf("apns-push-type = %q", got)
	}
}

func TestAPNSPushOptions(t *testing.T) {
	tests := []struct {
		name      string
		msg       driver.OutboundMessage
		wantAps   string
		wantTop   map[string]string
		wantHdr   map[string]string
		noHeaders []string
	}{
		{
			name: "critical",
			msg: driver.OutboundMessage{Title: "Hi", Text: "body", Push: &driver.PushOptions{
				Urgency: driver.UrgencyCritical, Category: "alert", ThreadID: "prj_1",
				MutableContent: true, CollapseID: "c0ffee0123456789", TTL: time.Hour,
				Data: map[string]string{"sealed": "QUJD", "kid": "0011223344556677", "aps": "x"},
			}},
			wantAps: `{"alert":{"title":"Hi","body":"body"},"category":"alert","interruption-level":"critical","mutable-content":1,"sound":{"critical":1,"name":"default","volume":1},"thread-id":"prj_1"}`,
			wantTop: map[string]string{"sealed": "QUJD", "kid": "0011223344556677"},
			wantHdr: map[string]string{
				"apns-priority": "10", "apns-push-type": "alert", "apns-collapse-id": "c0ffee0123456789",
			},
		},
		{
			name:    "time sensitive",
			msg:     driver.OutboundMessage{Title: "Hi", Text: "body", Push: &driver.PushOptions{Urgency: driver.UrgencyTimeSensitive}},
			wantAps: `{"alert":{"title":"Hi","body":"body"},"interruption-level":"time-sensitive","sound":"default"}`,
			wantHdr: map[string]string{"apns-priority": "10"},
		},
		{
			name:    "passive",
			msg:     driver.OutboundMessage{Title: "Hi", Text: "body", Push: &driver.PushOptions{Urgency: driver.UrgencyPassive}},
			wantAps: `{"alert":{"title":"Hi","body":"body"},"interruption-level":"passive","sound":"default"}`,
			wantHdr: map[string]string{"apns-priority": "5"},
		},
		{
			name: "background",
			msg: driver.OutboundMessage{Push: &driver.PushOptions{
				ContentAvailable: true, Data: map[string]string{"withdraw": "c0ffee0123456789"},
			}},
			wantAps:   `{"content-available":1}`,
			wantTop:   map[string]string{"withdraw": "c0ffee0123456789"},
			wantHdr:   map[string]string{"apns-push-type": "background", "apns-priority": "5"},
			noHeaders: []string{"apns-collapse-id", "apns-expiration"},
		},
		{
			name: "background ignores urgency",
			msg: driver.OutboundMessage{Push: &driver.PushOptions{
				ContentAvailable: true, Urgency: driver.UrgencyPassive, TTL: time.Minute,
				Data: map[string]string{"withdraw": "x"},
			}},
			wantAps: `{"content-available":1}`,
			wantTop: map[string]string{"withdraw": "x"},
			wantHdr: map[string]string{"apns-push-type": "background", "apns-priority": "5"},
		},
		{
			name: "background ignores time sensitive",
			msg: driver.OutboundMessage{Push: &driver.PushOptions{
				ContentAvailable: true, Urgency: driver.UrgencyTimeSensitive, MutableContent: true,
			}},
			wantAps: `{"content-available":1}`,
			wantHdr: map[string]string{"apns-push-type": "background", "apns-priority": "5"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, rec := newRecorder(t, http.StatusOK, "")
			if err := sendMsg(t, srv, &tt.msg); err != nil {
				t.Fatalf("Send: %v", err)
			}
			var top map[string]json.RawMessage
			if err := json.Unmarshal(rec.body, &top); err != nil {
				t.Fatalf("body %s: %v", rec.body, err)
			}
			var aps, want any
			if err := json.Unmarshal(top["aps"], &aps); err != nil {
				t.Fatalf("aps: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.wantAps), &want); err != nil {
				t.Fatal(err)
			}
			gb, _ := json.Marshal(aps)  //nolint:errcheck // test
			wb, _ := json.Marshal(want) //nolint:errcheck // test
			if string(gb) != string(wb) {
				t.Errorf("aps = %s, want %s", gb, wb)
			}
			for k, v := range tt.wantTop {
				var got string
				if err := json.Unmarshal(top[k], &got); err != nil || got != v {
					t.Errorf("top-level %q = %s, want %q", k, top[k], v)
				}
			}
			for k, v := range tt.wantHdr {
				if got := rec.headers.Get(k); got != v {
					t.Errorf("header %s = %q, want %q", k, got, v)
				}
			}
			for _, k := range tt.noHeaders {
				if got := rec.headers.Get(k); got != "" {
					t.Errorf("header %s = %q, want absent", k, got)
				}
			}
			if tt.name == "critical" {
				exp, err := strconv.ParseInt(rec.headers.Get("apns-expiration"), 10, 64)
				if err != nil {
					t.Fatalf("apns-expiration: %v", err)
				}
				if d := exp - time.Now().Add(time.Hour).Unix(); d < -5 || d > 5 {
					t.Errorf("apns-expiration off by %ds", d)
				}
			}
		})
	}
}

func TestAPNSAddressGone(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		gone   bool
	}{
		{"unregistered", 410, `{"reason":"Unregistered"}`, true},
		{"bad device token", 400, `{"reason":"BadDeviceToken"}`, true},
		{"bad collapse id", 400, `{"reason":"BadCollapseId"}`, false},
		{"server error", 500, ``, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newRecorder(t, tt.status, tt.body)
			err := sendMsg(t, srv, &driver.OutboundMessage{Title: "Hi", Text: "body"})
			if err == nil {
				t.Fatal("expected error")
			}
			if got := errors.Is(err, driver.ErrAddressGone); got != tt.gone {
				t.Errorf("errors.Is(ErrAddressGone) = %v, want %v (err: %v)", got, tt.gone, err)
			}
			if strings.Contains(err.Error(), "devicetoken") {
				t.Errorf("error leaks the device token: %v", err)
			}
		})
	}
}

func TestAPNSTransportErrorHidesToken(t *testing.T) {
	srv, _ := newRecorder(t, http.StatusOK, "")
	url := srv.URL
	srv.Close()
	msg := &driver.OutboundMessage{To: "devicetoken", Title: "Hi", Data: map[string]string{
		"key_id": "K1", "team_id": "T1", "bundle_id": "b", "private_key": testKeyPEM(t), "base_url": url,
	}}
	_, err := (&apns.Driver{}).Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "devicetoken") {
		t.Errorf("error leaks the device token: %v", err)
	}
}

func TestAPNSMaxSealedPayloadFits(t *testing.T) {
	srv, rec := newRecorder(t, http.StatusOK, "")
	// The worst case the gateway can build: a max-size sealed show (2560 bytes
	// of envelope plus 48 of seal, 3480 base64 characters), the longest
	// placeholders its 256-byte budget allows, and a 64-byte collapse ID.
	msg := &driver.OutboundMessage{
		Title: strings.Repeat("T", 128), Text: strings.Repeat("B", 128),
		Push: &driver.PushOptions{
			Urgency: driver.UrgencyCritical, MutableContent: true, CollapseID: strings.Repeat("b", 64),
			Data: map[string]string{
				"sealed": strings.Repeat("A", 3480), "kid": strings.Repeat("a", 16), "collapse_id": strings.Repeat("b", 64),
			},
		},
	}
	if err := sendMsg(t, srv, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(rec.body) > 4096 {
		t.Errorf("body is %d bytes, over 4096", len(rec.body))
	}
}

func TestAPNSEscapesTheDeviceTokenInThePath(t *testing.T) {
	var uri string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri = r.RequestURI
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	msg := &driver.OutboundMessage{To: "ab/../cd?x=1#f", Title: "Hi", Data: map[string]string{
		"key_id": "K1", "team_id": "T1", "bundle_id": "b", "private_key": testKeyPEM(t), "base_url": srv.URL,
	}}
	if _, err := (&apns.Driver{}).Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if want := "/3/device/ab%2F..%2Fcd%3Fx=1%23f"; uri != want {
		t.Errorf("request URI = %q, want %q", uri, want)
	}
}

func TestAPNSNothingToSend(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	t.Cleanup(srv.Close)
	err := sendMsg(t, srv, &driver.OutboundMessage{Push: &driver.PushOptions{Urgency: driver.UrgencyActive}})
	if err == nil {
		t.Fatal("expected error")
	}
	if hits != 0 {
		t.Errorf("%d requests reached the server", hits)
	}
	if strings.Contains(err.Error(), "devicetoken") {
		t.Errorf("error leaks the device token: %v", err)
	}
}
