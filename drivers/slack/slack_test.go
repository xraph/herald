package slack_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/slack"
	"github.com/xraph/herald/message"
)

func TestSlackWebhookMode(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `ok`)
	d := &slack.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		Text: "hello",
		Data: map[string]string{"webhook_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q", res.Status)
	}
	var body struct{ Text string }
	srv.Captured.DecodeJSON(t, &body)
	if body.Text != "hello" {
		t.Errorf("text = %q", body.Text)
	}
}

func TestSlackAPIMode(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"ok":true,"ts":"123.456"}`)
	d := &slack.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "#general", Text: "hello",
		Data: map[string]string{"bot_token": "xoxb", "channel": "#default", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "123.456" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/api/chat.postMessage" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "Bearer xoxb" {
		t.Errorf("auth = %q", got)
	}
	var body struct{ Channel, Text string }
	srv.Captured.DecodeJSON(t, &body)
	if body.Channel != "#general" { // msg.To overrides channel
		t.Errorf("channel = %q", body.Channel)
	}
	if body.Text != "hello" {
		t.Errorf("text = %q", body.Text)
	}
}

func TestSlackAPIError(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"ok":false,"error":"channel_not_found"}`)
	d := &slack.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "#x", Text: "hi",
		Data: map[string]string{"bot_token": "xoxb", "channel": "#x", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error when ok=false")
	}
}

func TestSlackValidate(t *testing.T) {
	d := &slack.Driver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error with neither webhook_url nor bot_token+channel")
	}
}
