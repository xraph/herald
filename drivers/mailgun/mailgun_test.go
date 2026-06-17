package mailgun_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/mailgun"
	"github.com/xraph/herald/message"
)

func TestMailgunSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"id":"<msg@mg>"}`)
	d := &mailgun.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hi", Text: "t",
		Data: map[string]string{"api_key": "key", "domain": "mg.example.com", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "<msg@mg>" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/v3/mg.example.com/messages" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	user, pass, ok := parseBasicAuth(srv.Captured.Header.Get("Authorization"))
	if !ok || user != "api" || pass != "key" {
		t.Errorf("basic auth = %q/%q ok=%v", user, pass, ok)
	}
	if got := srv.Captured.FormValue(t, "to"); got != "to@x.com" {
		t.Errorf("form to = %q", got)
	}
}

func TestMailgunValidate(t *testing.T) {
	d := &mailgun.Driver{}
	if err := d.Validate(map[string]string{"api_key": "k"}, nil); err == nil {
		t.Error("expected error for missing domain")
	}
	if err := d.Validate(map[string]string{"api_key": "k", "domain": "d"}, nil); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}

func parseBasicAuth(header string) (user, pass string, ok bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) {
		return "", "", false
	}
	r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	if err != nil {
		return "", "", false
	}
	r.Header.Set("Authorization", header)
	return r.BasicAuth()
}
