package pushgateway

import (
	"net/http"
	"testing"
	"time"
)

func TestServerRateLimit(t *testing.T) {
	now := testNow
	g := newTestGateway(t, func(c *Config) {
		c.ServerPerSecond = 2
		c.Now = func() time.Time { return now }
	})
	gw, _ := g.(*gateway)
	android, _ := drivers(gw)
	handle := registerHandle(t, gw, "android", "tok-1", "")
	send := func() int {
		return signedSend(t, g, 7, showBody(handle), now.Unix()).Code
	}
	for i := 1; i <= 2; i++ {
		if got := send(); got != http.StatusAccepted {
			t.Fatalf("send %d: got %d, want 202", i, got)
		}
	}
	rec := signedSend(t, g, 7, showBody(handle), now.Unix())
	wantBody(t, rec, http.StatusTooManyRequests, `{"error":"rate limited"}`)
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if len(android.got) != 2 {
		t.Fatalf("driver calls = %d, want 2", len(android.got))
	}
	now = now.Add(time.Second)
	if got := send(); got != http.StatusAccepted {
		t.Fatalf("after 1s: got %d, want 202", got)
	}
}

func TestHandleRateLimit(t *testing.T) {
	g := newTestGateway(t, func(c *Config) {
		c.HandlePerMinute = 2
		c.ServerPerSecond = 100
	})
	gw, _ := g.(*gateway)
	a := registerHandle(t, gw, "android", "tok-a", "")
	b := registerHandle(t, gw, "android", "tok-b", "")
	for i := 1; i <= 2; i++ {
		if rec := signedSend(t, g, 7, showBody(a), testNow.Unix()); rec.Code != http.StatusAccepted {
			t.Fatalf("send %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	rec := signedSend(t, g, 7, showBody(a), testNow.Unix())
	wantBody(t, rec, http.StatusTooManyRequests, `{"error":"rate limited"}`)
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want 30", got)
	}
	if rec := signedSend(t, g, 7, showBody(b), testNow.Unix()); rec.Code != http.StatusAccepted {
		t.Fatalf("handle B: %d %s", rec.Code, rec.Body)
	}
}

func TestLimiterSweepsIdleEntries(t *testing.T) {
	set := newLimiterSet(1, 1)
	for _, k := range []string{"a", "b", "c"} {
		set.allow(k, testNow)
	}
	if len(set.m) != 3 {
		t.Fatalf("len = %d, want 3", len(set.m))
	}
	set.allow("d", testNow.Add(11*time.Minute))
	if len(set.m) != 1 {
		t.Fatalf("len after sweep = %d, want 1", len(set.m))
	}
}
