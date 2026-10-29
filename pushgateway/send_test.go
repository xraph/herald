package pushgateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/push"
)

const (
	testKID      = "0011223344556677"
	testCollapse = "c0ffee0123456789"
)

func drivers(g *gateway) (android, ios *fakeDriver) {
	a, _ := g.android.Driver.(*fakeDriver)
	i, _ := g.ios.Driver.(*fakeDriver)
	return a, i
}

func registerHandle(t *testing.T, g *gateway, platform, token, env string) string {
	const seed = 7
	t.Helper()
	pub, _ := serverKeys(seed)
	rec := post(g, "/v1/registrations", registerBody(platform, token, env, pub))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	return handleFrom(t, rec.Body.Bytes())
}

func sendRaw(g http.Handler, seed byte, raw []byte, ts int64) *httptest.ResponseRecorder {
	pub, priv := serverKeys(seed)
	tsStr := strconv.FormatInt(ts, 10)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/send", bytes.NewReader(raw))
	req.Header.Set(push.HeaderKey, b64(pub))
	req.Header.Set(push.HeaderTimestamp, tsStr)
	req.Header.Set(push.HeaderSignature, b64(ed25519.Sign(priv, push.SignatureBase(tsStr, raw))))
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	return rec
}

func signedSend(t *testing.T, g http.Handler, seed byte, body push.SendBody, ts int64) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return sendRaw(g, seed, raw, ts)
}

func showBody(handle string) push.SendBody {
	return push.SendBody{
		Handle: handle, Sealed: b64([]byte("sealed-bytes")), KeyID: testKID,
		Urgency: "critical", Kind: "show", CollapseID: testCollapse, TTLSeconds: 3600,
	}
}

func wantBody(t *testing.T, rec *httptest.ResponseRecorder, code int, body string) {
	t.Helper()
	if rec.Code != code || strings.TrimSpace(rec.Body.String()) != body {
		t.Fatalf("got %d %s, want %d %s", rec.Code, rec.Body, code, body)
	}
}

func TestSendAndroidShowMapsToFCM(t *testing.T) {
	g := testGW(t, nil)
	android, ios := drivers(g)
	h := registerHandle(t, g, "android", "fcm-token-1", "")
	body := showBody(h)
	rec := signedSend(t, g, 7, body, testNow.Unix())
	wantBody(t, rec, 202, `{"id":"fcm-msg-1"}`)
	if len(android.got) != 1 || len(ios.got) != 0 {
		t.Fatalf("calls: android %d ios %d", len(android.got), len(ios.got))
	}
	m := android.got[0]
	if m.To != "fcm-token-1" || m.Title != "" || m.Text != "" {
		t.Fatalf("message = %+v", m)
	}
	if want := map[string]string{"service_account": "sa", "project_id": "p1"}; !maps.Equal(m.Data, want) {
		t.Fatalf("data = %v", m.Data)
	}
	want := &driver.PushOptions{
		DataOnly: true, Urgency: "critical", CollapseID: testCollapse, TTL: time.Hour,
		Data: map[string]string{"sealed": body.Sealed, "kid": testKID, "kind": "show", "collapse_id": testCollapse},
	}
	if !reflect.DeepEqual(m.Push, want) {
		t.Fatalf("push = %+v, want %+v", m.Push, want)
	}
}

func TestSendAndroidWithdraw(t *testing.T) {
	g := testGW(t, nil)
	android, _ := drivers(g)
	h := registerHandle(t, g, "android", "fcm-token-1", "")
	body := push.SendBody{Handle: h, KeyID: testKID, Kind: "withdraw", CollapseID: testCollapse}
	wantBody(t, signedSend(t, g, 7, body, testNow.Unix()), 202, `{"id":"fcm-msg-1"}`)
	want := map[string]string{"kid": testKID, "kind": "withdraw", "collapse_id": testCollapse}
	if got := android.got[0].Push.Data; !maps.Equal(got, want) {
		t.Fatalf("data = %v", got)
	}
}

func TestSendIOSShowMapsToAPNs(t *testing.T) {
	g := testGW(t, nil)
	_, ios := drivers(g)
	prod := registerHandle(t, g, "ios", "apns-tok", "production")
	body := showBody(prod)
	body.Urgency = "time_sensitive"
	wantBody(t, signedSend(t, g, 7, body, testNow.Unix()), 202, `{"id":"apns-msg-1"}`)
	m := ios.got[0]
	if _, ok := m.Data["sandbox"]; ok || m.To != "apns-tok" {
		t.Fatalf("message = %+v", m)
	}
	if m.Title != "Notification" || m.Text != "New notification" {
		t.Fatalf("placeholder = %q %q", m.Title, m.Text)
	}
	want := &driver.PushOptions{
		MutableContent: true, Urgency: "time_sensitive", CollapseID: testCollapse, TTL: time.Hour,
		Data: map[string]string{"sealed": body.Sealed, "kid": testKID, "collapse_id": testCollapse},
	}
	if !reflect.DeepEqual(m.Push, want) {
		t.Fatalf("push = %+v", m.Push)
	}

	before := maps.Clone(g.ios.Settings)
	sbx := registerHandle(t, g, "ios", "apns-tok2", "sandbox")
	body.Handle = sbx
	wantBody(t, signedSend(t, g, 7, body, testNow.Unix()), 202, `{"id":"apns-msg-1"}`)
	if got := ios.got[1].Data["sandbox"]; got != "true" {
		t.Fatalf("sandbox = %q", got)
	}
	if !maps.Equal(g.ios.Settings, before) {
		t.Fatalf("platform settings changed: %v", g.ios.Settings)
	}
}

func TestSendIOSCustomPlaceholder(t *testing.T) {
	g := testGW(t, func(c *Config) { c.PlaceholderTitle = "Example" })
	_, ios := drivers(g)
	h := registerHandle(t, g, "ios", "apns-tok", "production")
	signedSend(t, g, 7, showBody(h), testNow.Unix())
	if ios.got[0].Title != "Example" {
		t.Fatalf("title = %q", ios.got[0].Title)
	}
}

func TestSendIOSWithdraw(t *testing.T) {
	g := testGW(t, nil)
	_, ios := drivers(g)
	h := registerHandle(t, g, "ios", "apns-tok", "production")
	body := push.SendBody{Handle: h, KeyID: testKID, Kind: "withdraw", CollapseID: testCollapse, Urgency: "critical"}
	wantBody(t, signedSend(t, g, 7, body, testNow.Unix()), 202, `{"id":"apns-msg-1"}`)
	m := ios.got[0]
	want := &driver.PushOptions{
		ContentAvailable: true, Urgency: "passive", Data: map[string]string{"withdraw": testCollapse},
	}
	if !reflect.DeepEqual(m.Push, want) || m.Title != "" || m.Text != "" {
		t.Fatalf("message = %+v push = %+v", m, m.Push)
	}
}

func TestSendSignatureChecks(t *testing.T) {
	now := testNow.Unix()
	tests := []struct {
		name string
		do   func(g *gateway, h string) *httptest.ResponseRecorder
		code int
		msg  string
	}{
		{"other key", func(g *gateway, h string) *httptest.ResponseRecorder {
			raw, _ := json.Marshal(showBody(h))
			pub, _ := serverKeys(7)
			_, other := serverKeys(9)
			ts := strconv.FormatInt(now, 10)
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/send", bytes.NewReader(raw))
			req.Header.Set(push.HeaderKey, b64(pub))
			req.Header.Set(push.HeaderTimestamp, ts)
			req.Header.Set(push.HeaderSignature, b64(ed25519.Sign(other, push.SignatureBase(ts, raw))))
			rec := httptest.NewRecorder()
			g.ServeHTTP(rec, req)
			return rec
		}, 401, `{"error":"bad signature"}`},
		{"body changed", func(g *gateway, h string) *httptest.ResponseRecorder {
			raw, _ := json.Marshal(showBody(h))
			pub, priv := serverKeys(7)
			ts := strconv.FormatInt(now, 10)
			sig := ed25519.Sign(priv, push.SignatureBase(ts, raw))
			raw = bytes.Replace(raw, []byte("critical"), []byte("passive"), 1)
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/send", bytes.NewReader(raw))
			req.Header.Set(push.HeaderKey, b64(pub))
			req.Header.Set(push.HeaderTimestamp, ts)
			req.Header.Set(push.HeaderSignature, b64(sig))
			rec := httptest.NewRecorder()
			g.ServeHTTP(rec, req)
			return rec
		}, 401, `{"error":"bad signature"}`},
		{"missing signature", func(g *gateway, h string) *httptest.ResponseRecorder {
			raw, _ := json.Marshal(showBody(h))
			pub, _ := serverKeys(7)
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/send", bytes.NewReader(raw))
			req.Header.Set(push.HeaderKey, b64(pub))
			req.Header.Set(push.HeaderTimestamp, strconv.FormatInt(now, 10))
			rec := httptest.NewRecorder()
			g.ServeHTTP(rec, req)
			return rec
		}, 401, `{"error":"bad signature"}`},
		{"too old", func(g *gateway, h string) *httptest.ResponseRecorder {
			return signedSend(t, g, 7, showBody(h), now-301)
		}, 401, `{"error":"timestamp outside the allowed window"}`},
		{"too new", func(g *gateway, h string) *httptest.ResponseRecorder {
			return signedSend(t, g, 7, showBody(h), now+301)
		}, 401, `{"error":"timestamp outside the allowed window"}`},
		{"edge of window", func(g *gateway, h string) *httptest.ResponseRecorder {
			return signedSend(t, g, 7, showBody(h), now-299)
		}, 202, `{"id":"fcm-msg-1"}`},
		{"other server", func(g *gateway, h string) *httptest.ResponseRecorder {
			return signedSend(t, g, 8, showBody(h), now)
		}, 401, `{"error":"key does not match the handle"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := testGW(t, nil)
			android, ios := drivers(g)
			h := registerHandle(t, g, "android", "fcm-token-1", "")
			wantBody(t, tt.do(g, h), tt.code, tt.msg)
			calls := len(android.got) + len(ios.got)
			if (tt.code == 202) != (calls == 1) || (tt.code != 202 && calls != 0) {
				t.Fatalf("driver calls = %d", calls)
			}
		})
	}
}

func TestSendHandleUnderPreviousKey(t *testing.T) {
	old := testGW(t, func(c *Config) { c.SealingKeys = c.SealingKeys[1:] })
	h := registerHandle(t, old, "android", "fcm-token-1", "")
	g := testGW(t, nil)
	wantBody(t, signedSend(t, g, 7, showBody(h), testNow.Unix()), 202, `{"id":"fcm-msg-1"}`)
}

func TestSendRetiredKeyIs410(t *testing.T) {
	gone := testGW(t, func(c *Config) {
		c.SealingKeys = []SealingKey{
			{ID: "k2025z", Key: bytes.Repeat([]byte{3}, 32)},
			{ID: "key3", Key: bytes.Repeat([]byte{4}, 32)},
		}
	})
	h := registerHandle(t, gone, "android", "fcm-token-1", "")
	g := testGW(t, nil)
	android, _ := drivers(g)
	wantBody(t, signedSend(t, g, 7, showBody(h), testNow.Unix()), 410, `{"error":"address gone"}`)
	if len(android.got) != 0 {
		t.Fatal("driver was called")
	}
}

func TestSendRejectsMalformed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(b *push.SendBody)
		raw    string
	}{
		{name: "bad json", raw: "{nope"},
		{name: "urgency", mutate: func(b *push.SendBody) { b.Urgency = "loud" }},
		{name: "kind", mutate: func(b *push.SendBody) { b.Kind = "shout" }},
		{name: "no sealed", mutate: func(b *push.SendBody) { b.Sealed = "" }},
		{name: "bad sealed", mutate: func(b *push.SendBody) { b.Sealed = "%%%" }},
		{name: "sealed too big", mutate: func(b *push.SendBody) { b.Sealed = b64(make([]byte, 2609)) }},
		{name: "ttl negative", mutate: func(b *push.SendBody) { b.TTLSeconds = -1 }},
		{name: "ttl too big", mutate: func(b *push.SendBody) { b.TTLSeconds = 2419201 }},
		{name: "collapse long", mutate: func(b *push.SendBody) { b.CollapseID = strings.Repeat("a", 65) }},
		{name: "key id", mutate: func(b *push.SendBody) { b.KeyID = "XYZ" }},
		{name: "tampered handle", mutate: func(b *push.SendBody) { b.Handle = b.Handle[:len(b.Handle)-2] + "AA" }},
		{name: "oversized body", mutate: func(b *push.SendBody) { b.CollapseID = strings.Repeat("a", 20<<10) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := testGW(t, nil)
			android, _ := drivers(g)
			h := registerHandle(t, g, "android", "fcm-token-1", "")
			var rec *httptest.ResponseRecorder
			if tt.raw != "" {
				rec = sendRaw(g, 7, []byte(tt.raw), testNow.Unix())
			} else {
				b := showBody(h)
				tt.mutate(&b)
				rec = signedSend(t, g, 7, b, testNow.Unix())
			}
			if rec.Code != 400 || len(android.got) != 0 {
				t.Fatalf("got %d %s, calls %d", rec.Code, rec.Body, len(android.got))
			}
		})
	}
}

func TestSendGoneIs410(t *testing.T) {
	g := testGW(t, nil)
	android, _ := drivers(g)
	android.sendErr = fmt.Errorf("fcm: %w", driver.ErrAddressGone)
	h := registerHandle(t, g, "android", "fcm-token-1", "")
	wantBody(t, signedSend(t, g, 7, showBody(h), testNow.Unix()), 410, `{"error":"address gone"}`)
}

func TestSendUpstreamFailureIs502(t *testing.T) {
	g := testGW(t, nil)
	android, _ := drivers(g)
	android.sendErr = errors.New("apns: API error 500: secret-upstream-text")
	h := registerHandle(t, g, "android", "fcm-token-1", "")
	rec := signedSend(t, g, 7, showBody(h), testNow.Unix())
	wantBody(t, rec, 502, `{"error":"delivery failed"}`)
	if strings.Contains(rec.Body.String(), "secret-upstream-text") {
		t.Fatal("upstream text leaked")
	}
}

func TestSendNeverLogsSecrets(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	g := testGW(t, func(c *Config) { c.Logger = log })
	android, _ := drivers(g)
	h := registerHandle(t, g, "android", "fcm-token-1", "")
	body := showBody(h)
	if rec := signedSend(t, g, 7, body, testNow.Unix()); rec.Code != 202 {
		t.Fatalf("send 1: %d", rec.Code)
	}
	android.sendErr = fmt.Errorf("secret-upstream-text: %w", driver.ErrAddressGone)
	if rec := signedSend(t, g, 7, body, testNow.Unix()); rec.Code != 410 {
		t.Fatalf("send 2: %d", rec.Code)
	}
	if rec := signedSend(t, g, 7, body, testNow.Unix()-1000); rec.Code != 401 {
		t.Fatalf("send 3: %d", rec.Code)
	}
	android.sendErr = errors.New("secret-upstream-text")
	signedSend(t, g, 7, body, testNow.Unix())

	pub, _ := serverKeys(7)
	logs := buf.String()
	for name, secret := range map[string]string{
		"token": "fcm-token-1", "handle": h, "sealed": body.Sealed, "key id": testKID,
		"collapse id": testCollapse, "server key": b64(pub), "upstream": "secret-upstream-text",
	} {
		if strings.Contains(logs, secret) {
			t.Errorf("log contains the %s", name)
		}
	}
	for _, want := range []string{`"outcome":202`, `"platform":"android"`, `"urgency":"critical"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("log lacks %s:\n%s", want, logs)
		}
	}
}
