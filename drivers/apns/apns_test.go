package apns_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
