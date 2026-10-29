package pushgateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xraph/herald/driver/push"
)

func registerBody(platform, token, env string, key []byte) map[string]string {
	return map[string]string{"platform": platform, "token": token, "apns_env": env, "server_key": b64(key)}
}

func handleFrom(t *testing.T, body []byte) string {
	t.Helper()
	var out struct {
		Handle string `json:"handle"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Handle == "" {
		t.Fatalf("body = %s, err = %v", body, err)
	}
	return out.Handle
}

func TestRegisterAndroidReturnsHandle(t *testing.T) {
	g := testGW(t, nil)
	pub, _ := serverKeys(7)
	rec := post(g, "/v1/registrations", registerBody("android", "fcm-token-1", "", pub))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body)
	}
	got, err := g.openHandle(handleFrom(t, rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(pub)
	want := claims{V: 1, P: "android", T: "fcm-token-1", F: hex.EncodeToString(sum[:])[:32], IAT: 1793220240}
	if got != want {
		t.Fatalf("claims = %+v, want %+v", got, want)
	}
}

func TestRegisterIOS(t *testing.T) {
	g := testGW(t, nil)
	pub, _ := serverKeys(7)
	rec := post(g, "/v1/registrations", registerBody("ios", "apns-tok", "sandbox", pub))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body)
	}
	got, err := g.openHandle(handleFrom(t, rec.Body.Bytes()))
	if err != nil || got.E != "sandbox" || got.P != "ios" {
		t.Fatalf("claims = %+v, err = %v", got, err)
	}
	for _, env := range []string{"", "staging"} {
		if rec = post(g, "/v1/registrations", registerBody("ios", "apns-tok", env, pub)); rec.Code != http.StatusBadRequest {
			t.Errorf("env %q: code = %d", env, rec.Code)
		}
	}
}

func TestRegisterRejects(t *testing.T) {
	pub, _ := serverKeys(7)
	const token = "secret-device-token"
	cases := map[string]any{
		"bad platform":     registerBody("web", token, "", pub),
		"empty token":      registerBody("android", "", "", pub),
		"long token":       registerBody("android", strings.Repeat("t", 4097), "", pub),
		"short server key": registerBody("android", token, "", pub[:31]),
		"bad server key":   map[string]string{"platform": "android", "token": token, "server_key": "%%%"},
		"android env":      registerBody("android", token, "production", pub),
		"malformed json":   `{"platform": "android", "token": "` + token,
		"huge body":        bytes.Repeat([]byte("x"), 1<<20),
	}
	g := newTestGateway(t, nil)
	for name, body := range cases {
		rec := post(g, "/v1/registrations", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d", name, rec.Code)
		}
		var out map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["error"] == "" {
			t.Errorf("%s: body = %s", name, rec.Body)
		}
		if strings.Contains(rec.Body.String(), token) {
			t.Errorf("%s: error echoes the token", name)
		}
	}
}

func TestRegisterPlatformOff(t *testing.T) {
	g := newTestGateway(t, func(c *Config) { c.IOS = nil })
	pub, _ := serverKeys(7)
	if rec := post(g, "/v1/registrations", registerBody("ios", "t", "production", pub)); rec.Code != http.StatusBadRequest {
		t.Fatalf("ios code = %d", rec.Code)
	}
	if rec := post(g, "/v1/registrations", registerBody("android", "t", "", pub)); rec.Code != http.StatusCreated {
		t.Fatalf("android code = %d", rec.Code)
	}
}

func TestRoutes(t *testing.T) {
	g := newTestGateway(t, nil)
	rec := post(g, "/v2/send", "{}")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("v2 code = %d", rec.Code)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/registrations", nil)
	rec = httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "POST" {
		t.Fatalf("get: code = %d allow = %q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	key := func(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }
	cases := map[string]func(*Config){
		"no keys":       func(c *Config) { c.SealingKeys = nil },
		"short key":     func(c *Config) { c.SealingKeys = []SealingKey{{ID: "a", Key: key(1)[:31]}} },
		"empty id":      func(c *Config) { c.SealingKeys = []SealingKey{{ID: "", Key: key(1)}} },
		"dotted id":     func(c *Config) { c.SealingKeys = []SealingKey{{ID: "a.b", Key: key(1)}} },
		"duplicate ids": func(c *Config) { c.SealingKeys = []SealingKey{{ID: "a", Key: key(1)}, {ID: "a", Key: key(2)}} },
		"no platforms":  func(c *Config) { c.Android, c.IOS = nil, nil },
		"nil driver":    func(c *Config) { c.Android = &Platform{} },
		"validate fails": func(c *Config) {
			c.Android = &Platform{Driver: &fakeDriver{name: "fcm", channel: "push", err: errBadHandle}}
		},
	}
	for name, mutate := range cases {
		cfg := testConfig()
		mutate(&cfg)
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: New succeeded", name)
		}
	}
}

func TestNewAcceptsZeroValueDrivers(t *testing.T) {
	cfg := testConfig()
	cfg.IOS = nil
	cfg.Android = &Platform{
		Driver:      &push.FCMDriver{},
		Credentials: map[string]string{"service_account": serviceAccountJSON(t, "https://example.test/token", "p1")},
		Settings:    map[string]string{"project_id": "p1"},
	}
	if _, err := New(cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestRegisterFingerprintFollowsServerKey(t *testing.T) {
	g := testGW(t, nil)
	var seen [2]string
	for i, seed := range []byte{7, 8} {
		pub, _ := serverKeys(seed)
		rec := post(g, "/v1/registrations", registerBody("android", "t", "", pub))
		c, err := g.openHandle(handleFrom(t, rec.Body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		seen[i] = c.F
	}
	if seen[0] == seen[1] {
		t.Fatal("different server keys gave the same fingerprint")
	}
}
