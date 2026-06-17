package postmark_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/postmark"
	"github.com/xraph/herald/message"
)

func TestPostmarkSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"MessageID":"pm-1"}`)
	d := &postmark.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hi", HTML: "<b>x</b>",
		Data: map[string]string{"server_token": "tok", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "pm-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/email" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("X-Postmark-Server-Token"); got != "tok" {
		t.Errorf("token header = %q", got)
	}
	var body struct {
		From     string
		To       string
		Subject  string
		HTMLBody string `json:"HtmlBody"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if body.To != "to@x.com" || body.Subject != "Hi" || body.HTMLBody != "<b>x</b>" {
		t.Errorf("body = %+v", body)
	}
}

func TestPostmarkValidate(t *testing.T) {
	d := &postmark.Driver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing server_token")
	}
}
