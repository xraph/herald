package contract

import (
	"errors"
	"strings"
	"testing"
)

func TestSendResolve(t *testing.T) {
	e := newEnv(t)
	p := e.provider(t, appA, "primary")

	got, err := sendResolveHandler(e.deps)(bg, sendResolveRequest{Channel: "email"}, as(appA))
	if err != nil || got.Provider == nil || got.Provider.ID != p.ID.String() || got.Via != "fallback" {
		t.Fatalf("send.resolve = %+v, %v", got, err)
	}
	if got.From.Email != "no-reply@example.com" {
		t.Errorf("from = %+v", got.From)
	}
	none, err := sendResolveHandler(e.deps)(bg, sendResolveRequest{Channel: "sms"}, as(appA))
	if err != nil || none.Provider != nil || none.Via != "none" {
		t.Errorf("nothing handles sms = %+v, %v; want a null provider, not an error", none, err)
	}
	theirs := e.provider(t, appB, "theirs")
	if _, err := sendResolveHandler(e.deps)(bg, sendResolveRequest{Channel: "email", ProviderID: theirs.ID.String()}, as(appA)); codeOf(err) != "NOT_FOUND" {
		t.Errorf("a chosen provider from another app: %v", err)
	}
}

func TestSendTestReportsEachOutcomeHonestly(t *testing.T) {
	e := newEnv(t)
	p := e.provider(t, appA, "primary")

	sent, err := sendTestHandler(e.deps)(bg, sendTestRequest{Channel: "email", Recipient: "ada@example.com", Subject: "Hi", Body: "Hello"}, as(appA))
	if err != nil {
		t.Fatalf("send.test: %v", err)
	}
	if sent.Status != "sent" || sent.ProviderMessageID != "vendor-1" || sent.Provider == nil || sent.Provider.ID != p.ID.String() || !sent.Logged {
		t.Errorf("sent = %+v", sent)
	}
	if len(e.drv.sent) != 1 || e.drv.sent[0].To != "ada@example.com" {
		t.Errorf("driver saw %d sends", len(e.drv.sent))
	}

	// A provider failure is a normal response, so the page can show exactly
	// what the provider said.
	e.drv.err = errors.New("fake: API error 401: bad key")
	failed, err := sendTestHandler(e.deps)(bg, sendTestRequest{Channel: "email", Recipient: "ada@example.com", Body: "Hello"}, as(appA))
	if err != nil {
		t.Fatalf("a provider failure must not be a contract error: %v", err)
	}
	if failed.Status != "failed" || !strings.Contains(failed.Error, "401") {
		t.Errorf("failed = %+v", failed)
	}
}

func TestSendTestRefusals(t *testing.T) {
	e := newEnv(t)
	e.provider(t, appA, "primary")
	cases := []struct {
		name string
		in   sendTestRequest
		code string
	}{
		{"no recipient", sendTestRequest{Channel: "email", Body: "x"}, "BAD_REQUEST"},
		{"unknown channel", sendTestRequest{Channel: "fax", Recipient: "a", Body: "x"}, "BAD_REQUEST"},
		{"nothing to send", sendTestRequest{Channel: "email", Recipient: "a"}, "BAD_REQUEST"},
		{"no provider for the channel", sendTestRequest{Channel: "sms", Recipient: "+1", Body: "x"}, "BAD_REQUEST"},
		{"template that doesn't exist", sendTestRequest{Channel: "email", Recipient: "a", Template: "nope"}, "NOT_FOUND"},
	}
	for _, c := range cases {
		if _, err := sendTestHandler(e.deps)(bg, c.in, as(appA)); string(codeOf(err)) != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	if len(e.drv.sent) != 0 {
		t.Errorf("a refused test reached the driver %d times", len(e.drv.sent))
	}
}
