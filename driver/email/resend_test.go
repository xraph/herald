package email_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/driver/email"
	"github.com/xraph/herald/message"
)

func TestResendSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"id":"re_123"}`)
	d := &email.ResendDriver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hi", HTML: "<b>x</b>",
		Data: map[string]string{"api_key": "k", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "re_123" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/emails" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "Bearer k" {
		t.Errorf("auth = %q", got)
	}
	var body struct {
		To []string `json:"to"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if len(body.To) != 1 || body.To[0] != "to@x.com" {
		t.Errorf("to = %v", body.To)
	}
}

func TestResendValidate(t *testing.T) {
	d := &email.ResendDriver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing api_key")
	}
	if err := d.Validate(map[string]string{"api_key": "k"}, nil); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}

func TestResendAPIError(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusUnauthorized, `{"message":"bad key"}`)
	d := &email.ResendDriver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com",
		Data: map[string]string{"api_key": "k", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
