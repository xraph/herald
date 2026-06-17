package messagebird_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/messagebird"
	"github.com/xraph/herald/message"
)

func TestMessageBirdSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusCreated, `{"id":"mb-1"}`)
	d := &messagebird.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "+3120", Text: "hi",
		Data: map[string]string{"access_key": "ak", "originator": "Acme", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "mb-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/messages" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "AccessKey ak" {
		t.Errorf("auth = %q", got)
	}
	var body struct {
		Originator string   `json:"originator"`
		Recipients []string `json:"recipients"`
		Body       string   `json:"body"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if body.Originator != "Acme" || len(body.Recipients) != 1 || body.Recipients[0] != "+3120" {
		t.Errorf("body = %+v", body)
	}
}

func TestMessageBirdValidate(t *testing.T) {
	d := &messagebird.Driver{}
	if err := d.Validate(map[string]string{"access_key": "k"}, nil); err == nil {
		t.Error("expected error for missing originator")
	}
}
