package webhook_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/webhook"
	"github.com/xraph/herald/message"
)

func TestWebhookDirectSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, ``)
	d := webhook.New(nil) // no relay -> direct HTTP POST
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", Subject: "Hi", Text: "body",
		Data: map[string]string{"url": srv.URL, "signing_secret": "shh", "event_type": "test.event"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q", res.Status)
	}
	sig := srv.Captured.Header.Get("X-Webhook-Signature")
	if !strings.HasPrefix(sig, "sha256=") {
		t.Errorf("signature = %q, want sha256= prefix", sig)
	}
	var body struct {
		Event   string `json:"event"`
		To      string `json:"to"`
		Subject string `json:"subject"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if body.Event != "test.event" || body.To != "to@x.com" || body.Subject != "Hi" {
		t.Errorf("body = %+v", body)
	}
}

func TestWebhookValidate(t *testing.T) {
	d := webhook.New(nil)
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing url")
	}
}

func TestWebhookForwardsOnlyDataSettings(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{}`)
	_, err := webhook.New(nil).Send(context.Background(), &driver.OutboundMessage{
		To: "u1", Text: "hi",
		Data: map[string]string{
			"url": srv.URL, "signing_secret": "s", "event_type": "order.paid",
			"from": "x@example.com", "api_key": "leaks-if-forwarded", "data.order": "1",
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var body struct {
		Data map[string]string `json:"data"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if len(body.Data) != 1 || body.Data["order"] != "1" {
		t.Errorf("payload data = %v, want only order=1", body.Data)
	}
}
