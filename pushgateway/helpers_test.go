package pushgateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xraph/herald/driver"
)

var testNow = time.Date(2026, 10, 28, 20, 44, 0, 0, time.UTC)

type fakeDriver struct {
	name, channel string
	err           error
	got           []*driver.OutboundMessage
}

func (f *fakeDriver) Name() string    { return f.name }
func (f *fakeDriver) Channel() string { return f.channel }

func (f *fakeDriver) Send(_ context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	c := *msg
	c.Data = maps.Clone(msg.Data)
	f.got = append(f.got, &c)
	return &driver.DeliveryResult{ProviderMessageID: f.name + "-msg-1"}, nil
}

func (f *fakeDriver) Validate(_, _ map[string]string) error { return f.err }

func testPlatforms() (*Platform, *Platform) {
	return &Platform{
			Driver:      &fakeDriver{name: "fcm", channel: "push"},
			Credentials: map[string]string{"service_account": "sa"},
			Settings:    map[string]string{"project_id": "p1"},
		}, &Platform{
			Driver:      &fakeDriver{name: "apns", channel: "push"},
			Credentials: map[string]string{"private_key": "pem"},
			Settings:    map[string]string{"key_id": "K1", "team_id": "T1", "bundle_id": "com.example.app"},
		}
}

func testConfig() Config {
	android, ios := testPlatforms()
	return Config{
		SealingKeys: []SealingKey{
			{ID: "k2026b", Key: bytes.Repeat([]byte{1}, 32)},
			{ID: "k2026a", Key: bytes.Repeat([]byte{2}, 32)},
		},
		Android: android,
		IOS:     ios,
		Now:     func() time.Time { return testNow },
	}
}

func newTestGateway(t *testing.T, mutate func(*Config)) http.Handler {
	t.Helper()
	cfg := testConfig()
	if mutate != nil {
		mutate(&cfg)
	}
	h, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

func testGW(t *testing.T, mutate func(*Config)) *gateway {
	t.Helper()
	g, ok := newTestGateway(t, mutate).(*gateway)
	if !ok {
		t.Fatal("New did not return *gateway")
	}
	return g
}

// serverKeys returns a deterministic Ed25519 key pair from a one-byte seed.
//
//nolint:unparam // the private key is for the send tests that follow
func serverKeys(seed byte) (ed25519.PublicKey, ed25519.PrivateKey) {
	priv := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	pub, _ := priv.Public().(ed25519.PublicKey)
	return pub, priv
}

func post(h http.Handler, path string, body any) *httptest.ResponseRecorder {
	var raw []byte
	switch b := body.(type) {
	case []byte:
		raw = b
	case string:
		raw = []byte(b)
	default:
		raw, _ = json.Marshal(b)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// serviceAccountJSON builds a valid-looking service account for driver validation.
func serviceAccountJSON(t *testing.T, tokenURL, projectID string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	out, err := json.Marshal(map[string]string{
		"type":           "service_account",
		"project_id":     projectID,
		"private_key_id": "kid1",
		"private_key":    string(keyPEM),
		"client_email":   "sa@" + projectID + ".iam.gserviceaccount.com",
		"token_uri":      tokenURL,
	})
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(out)
}
