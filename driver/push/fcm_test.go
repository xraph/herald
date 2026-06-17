package push_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/drivertest"
	"github.com/xraph/herald/driver/push"
	"github.com/xraph/herald/message"
)

func TestFCMSend(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"name":"projects/p/messages/fcm-1"}`)
	d := &push.FCMDriver{}
	res, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "device-token", Title: "Hi", Text: "body",
		Data: map[string]string{"project_id": "p", "access_token": "tok", "base_url": srv.URL},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Status != message.StatusSent || res.ProviderMessageID != "projects/p/messages/fcm-1" {
		t.Errorf("res = %+v", res)
	}
	if srv.Captured.Path != "/v1/projects/p/messages:send" {
		t.Errorf("path = %q", srv.Captured.Path)
	}
	if got := srv.Captured.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("auth = %q", got)
	}
	var body struct {
		Message struct {
			Token        string `json:"token"`
			Notification struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			} `json:"notification"`
		} `json:"message"`
	}
	srv.Captured.DecodeJSON(t, &body)
	if body.Message.Token != "device-token" {
		t.Errorf("token = %q", body.Message.Token)
	}
	if body.Message.Notification.Title != "Hi" || body.Message.Notification.Body != "body" {
		t.Errorf("notification = %+v", body.Message.Notification)
	}
}

func TestFCMValidate(t *testing.T) {
	d := &push.FCMDriver{}
	if err := d.Validate(map[string]string{}, nil); err == nil {
		t.Error("expected error for missing server_key/access_token")
	}
	if err := d.Validate(map[string]string{"access_token": "t"}, nil); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}
