package pushgateway

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func sampleClaims() claims {
	return claims{V: 1, P: "android", T: "tok", F: "ff", IAT: 1793220240}
}

func TestHandleRoundTrip(t *testing.T) {
	g := testGW(t, nil)
	h, err := g.sealHandle(sampleClaims())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "h1.k2026b.") {
		t.Fatalf("handle = %q", h)
	}
	got, err := g.openHandle(h)
	if err != nil {
		t.Fatal(err)
	}
	if got != sampleClaims() {
		t.Fatalf("claims = %+v", got)
	}
}

func TestHandleUnderPreviousKeyOpens(t *testing.T) {
	old := testGW(t, func(c *Config) { c.SealingKeys = c.SealingKeys[1:] })
	h, err := old.sealHandle(sampleClaims())
	if err != nil {
		t.Fatal(err)
	}
	got, err := testGW(t, nil).openHandle(h)
	if err != nil || got != sampleClaims() {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestHandleRetiredKey(t *testing.T) {
	gone := testGW(t, func(c *Config) {
		c.SealingKeys = []SealingKey{{ID: "k2025z", Key: bytes.Repeat([]byte{3}, 32)}}
	})
	h, err := gone.sealHandle(sampleClaims())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = testGW(t, nil).openHandle(h); !errors.Is(err, errRetiredKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestHandleTampering(t *testing.T) {
	g := testGW(t, nil)
	h, err := g.sealHandle(sampleClaims())
	if err != nil {
		t.Fatal(err)
	}
	flipped := []byte(h)
	i := len(flipped) - 5
	if flipped[i] == 'A' {
		flipped[i] = 'B'
	} else {
		flipped[i] = 'A'
	}
	cases := map[string]string{
		"flipped payload": string(flipped),
		"swapped id":      strings.Replace(h, ".k2026b.", ".k2026a.", 1),
		"wrong prefix":    strings.Replace(h, "h1.", "h2.", 1),
		"truncated":       h[:len(h)-20],
		"too short":       "h1.k2026b.AAAA",
		"bad base64":      "h1.k2026b.***",
		"no segments":     "h1",
	}
	for name, in := range cases {
		if _, err := g.openHandle(in); !errors.Is(err, errBadHandle) {
			t.Errorf("%s: err = %v, want errBadHandle", name, err)
		}
	}
}

func TestHandleRejectsWrongVersion(t *testing.T) {
	g := testGW(t, nil)
	c := sampleClaims()
	c.V = 2
	h, err := g.sealHandle(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.openHandle(h); !errors.Is(err, errBadHandle) {
		t.Fatalf("err = %v", err)
	}
}
