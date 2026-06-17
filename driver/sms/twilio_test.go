package sms_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/driver/sms"
	"github.com/xraph/herald/message"
)

func TestTwilioSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusCreated, `{"sid":"SM123"}`)
	d := &sms.TwilioDriver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "+15005550006", Text: "hello",
		Data: map[string]string{
			"account_sid": "AC1", "auth_token": "tok", "from_number": "+15005550001",
			"base_url": srv.URL,
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "SM123" {
		t.Errorf("res = %+v", res)
	}
	if !strings.HasSuffix(srv.Captured.Path, "/Accounts/AC1/Messages.json") {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if srv.Captured.FormValue(t, "To") != "+15005550006" {
		t.Errorf("To = %q", srv.Captured.FormValue(t, "To"))
	}
	if srv.Captured.FormValue(t, "Body") != "hello" {
		t.Errorf("Body = %q", srv.Captured.FormValue(t, "Body"))
	}
}

func TestTwilioValidate(t *testing.T) {
	d := &sms.TwilioDriver{}
	if err := d.Validate(map[string]string{"account_sid": "a", "auth_token": "t"}, nil); err == nil {
		t.Error("expected error for missing from_number")
	}
}
