package cloudflare_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/drivers/cloudflare"
	"github.com/xraph/herald/message"
)

func TestSendHappyPath(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK,
		`{"success":true,"errors":[],"result":{"delivered":["to@example.com"],"permanent_bounces":[],"queued":[]}}`)

	d := &cloudflare.Driver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To:      "to@example.com",
		From:    "from@example.com",
		Subject: "Hi",
		HTML:    "<b>hi</b>",
		Text:    "hi",
		Data: map[string]string{
			"api_token":  "tok",
			"account_id": "acct123",
			"base_url":   srv.URL,
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent {
		t.Errorf("status = %q, want sent", res.Status)
	}
	if srv.Captured.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", srv.Captured.Method)
	}
	if srv.Captured.Path != "/accounts/acct123/email/sending/send" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("auth = %q, want Bearer tok", got)
	}
	var body map[string]string
	srv.Captured.DecodeJSON(t, &body)
	if body["to"] != "to@example.com" || body["from"] != "from@example.com" || body["subject"] != "Hi" {
		t.Errorf("body = %+v", body)
	}
	if body["html"] != "<b>hi</b>" {
		t.Errorf("html = %q", body["html"])
	}
}

func TestSendFromNameFormatted(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"success":true,"result":{"delivered":["to@example.com"]}}`)
	d := &cloudflare.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@example.com", From: "from@example.com", FromName: "Acme",
		Data: map[string]string{"api_token": "t", "account_id": "a", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var body map[string]string
	srv.Captured.DecodeJSON(t, &body)
	if body["from"] != "Acme <from@example.com>" {
		t.Errorf("from = %q, want \"Acme <from@example.com>\"", body["from"])
	}
}

func TestSendAPIError(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusBadRequest, `{"success":false,"errors":[{"message":"bad domain"}]}`)
	d := &cloudflare.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@example.com", From: "from@example.com",
		Data: map[string]string{"api_token": "t", "account_id": "a", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error on 400, got nil")
	}
}

func TestSendPermanentBounce(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK,
		`{"success":true,"result":{"delivered":[],"permanent_bounces":["to@example.com"],"queued":[]}}`)
	d := &cloudflare.Driver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "to@example.com", From: "from@example.com",
		Data: map[string]string{"api_token": "t", "account_id": "a", "base_url": srv.URL},
	})
	if err == nil {
		t.Fatal("expected error on permanent bounce, got nil")
	}
}

func TestValidate(t *testing.T) {
	d := &cloudflare.Driver{}
	if err := d.Validate(map[string]string{"account_id": "a"}, nil); err == nil {
		t.Error("expected error for missing api_token")
	}
	if err := d.Validate(map[string]string{"api_token": "t"}, nil); err == nil {
		t.Error("expected error for missing account_id")
	}
	if err := d.Validate(map[string]string{"api_token": "t", "account_id": "a"}, nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
