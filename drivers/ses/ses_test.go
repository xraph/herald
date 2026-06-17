package ses_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/ses"
	"github.com/xraph/herald/message"
)

func TestSESSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"MessageId":"ses-1"}`)
	d := &ses.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@x.com", From: "from@x.com", Subject: "Hi", Text: "t",
		Data: map[string]string{
			"access_key_id": "AKIDEXAMPLE", "secret_access_key": "secret",
			"region": "us-east-1", "base_url": srv.URL,
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "ses-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/v2/email/outbound-emails" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	auth := srv.Captured.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/") {
		t.Errorf("auth = %q, want SigV4 with credential", auth)
	}
	if srv.Captured.Header.Get("X-Amz-Date") == "" {
		t.Error("missing X-Amz-Date")
	}
	var body struct {
		FromEmailAddress string
		Destination      struct{ ToAddresses []string }
	}
	srv.Captured.DecodeJSON(t, &body)
	if body.FromEmailAddress != "from@x.com" {
		t.Errorf("from = %q", body.FromEmailAddress)
	}
	if len(body.Destination.ToAddresses) != 1 || body.Destination.ToAddresses[0] != "to@x.com" {
		t.Errorf("to = %v", body.Destination.ToAddresses)
	}
}

func TestSESValidate(t *testing.T) {
	d := &ses.Driver{}
	if err := d.Validate(map[string]string{"access_key_id": "a", "secret_access_key": "s"}, nil); err == nil {
		t.Error("expected error for missing region")
	}
	if err := d.Validate(map[string]string{"access_key_id": "a", "secret_access_key": "s", "region": "us-east-1"}, nil); err != nil {
		t.Errorf("expected nil for valid config, got: %v", err)
	}
}
