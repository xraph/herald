package vonage_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/vonage"
	"github.com/xraph/herald/message"
)

func TestVonageSendSuccess(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK,
		`{"messages":[{"message-id":"v-1","status":"0"}]}`)
	d := &vonage.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "+4477", Text: "hi",
		Data: map[string]string{"api_key": "k", "api_secret": "s", "from_number": "Acme", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "v-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/sms/json" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
}

func TestVonageSendFailureStatus(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK,
		`{"messages":[{"status":"2","error-text":"Missing api_key"}]}`)
	d := &vonage.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "+4477", Text: "hi",
		Data: map[string]string{"api_key": "k", "api_secret": "s", "from_number": "Acme", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error when status != 0")
	}
}

func TestVonageValidate(t *testing.T) {
	d := &vonage.Driver{}
	if err := d.Validate(map[string]string{"api_key": "k", "api_secret": "s"}, nil); err == nil {
		t.Error("expected error for missing from_number")
	}
}
