package push_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestFCMSendFiltersCredentials(t *testing.T) {
	srv := drivertest.NewServer(t, http.StatusOK, `{"name":"projects/p/messages/fcm-1"}`)
	d := &push.FCMDriver{}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{
		To: "device-token", Title: "Hi", Text: "body",
		Data: map[string]string{
			"project_id":     "p",
			"access_token":   "tok",
			"server_key":     "sk",
			"base_url":       srv.URL,
			"from":           "no-reply@example.com",
			"username":       "leaks-if-forwarded",
			"data.order_id":  "42",
			"data.deep_link": "app://orders/42",
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var body struct {
		Message struct {
			Data map[string]string `json:"data"`
		} `json:"message"`
	}
	srv.Captured.DecodeJSON(t, &body)

	want := map[string]string{"order_id": "42", "deep_link": "app://orders/42"}
	if !reflect.DeepEqual(body.Message.Data, want) {
		t.Errorf("message.data = %v, want only the data. settings %v", body.Message.Data, want)
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
	if err := d.Validate(map[string]string{"service_account": "not json"}, nil); err == nil {
		t.Error("expected error for a service_account that is not JSON")
	}
	if err := d.Validate(map[string]string{"service_account": serviceAccountJSON(t, "http://127.0.0.1/token", "p")}, nil); err != nil {
		t.Errorf("valid service account: %v", err)
	}
}

func fcmSend(t *testing.T, status int, resp string, opts *driver.PushOptions, data map[string]string) (map[string]any, error) {
	t.Helper()
	srv := drivertest.NewServer(t, status, resp)
	d := &push.FCMDriver{}
	all := map[string]string{"project_id": "p", "access_token": "tok", "base_url": srv.URL}
	for k, v := range data {
		all[k] = v
	}
	_, err := d.Send(context.Background(), &driver.OutboundMessage{To: "device-token", Title: "Hi", Text: "body", Data: all, Push: opts})
	var body map[string]any
	if len(srv.Captured.Body) > 0 {
		srv.Captured.DecodeJSON(t, &body)
	}
	return body, err
}

func TestFCMPushOptions(t *testing.T) {
	sealed := map[string]string{"sealed": "QUJD", "kid": "0011223344556677"}
	t.Run("data only critical", func(t *testing.T) {
		body, err := fcmSend(t, http.StatusOK, `{}`, &driver.PushOptions{
			DataOnly: true, Urgency: driver.UrgencyCritical, CollapseID: "c0ffee0123456789", TTL: time.Hour, Data: sealed,
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		m := body["message"].(map[string]any)
		if _, ok := m["notification"]; ok {
			t.Error("notification present")
		}
		if !reflect.DeepEqual(m["data"], map[string]any{"sealed": "QUJD", "kid": "0011223344556677"}) {
			t.Errorf("data = %v", m["data"])
		}
		want := map[string]any{"priority": "HIGH", "collapse_key": "c0ffee0123456789", "ttl": "3600s"}
		if !reflect.DeepEqual(m["android"], want) {
			t.Errorf("android = %v", m["android"])
		}
	})
	for urgency, want := range map[driver.Urgency]string{
		driver.UrgencyTimeSensitive: "HIGH", driver.UrgencyActive: "NORMAL", driver.UrgencyPassive: "NORMAL", "": "NORMAL",
	} {
		t.Run("priority "+string(urgency), func(t *testing.T) {
			body, err := fcmSend(t, http.StatusOK, `{}`, &driver.PushOptions{Urgency: urgency}, nil)
			if err != nil {
				t.Fatal(err)
			}
			a := body["message"].(map[string]any)["android"].(map[string]any)
			if !reflect.DeepEqual(a, map[string]any{"priority": want}) {
				t.Errorf("android = %v", a)
			}
		})
	}
	t.Run("per message data wins", func(t *testing.T) {
		body, err := fcmSend(t, http.StatusOK, `{}`, &driver.PushOptions{Data: map[string]string{"kind": "show"}},
			map[string]string{"data.kind": "x", "data.campaign": "c1"})
		if err != nil {
			t.Fatal(err)
		}
		got := body["message"].(map[string]any)["data"]
		if !reflect.DeepEqual(got, map[string]any{"kind": "show", "campaign": "c1"}) {
			t.Errorf("data = %v", got)
		}
	})
	t.Run("nil push", func(t *testing.T) {
		body, err := fcmSend(t, http.StatusOK, `{}`, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		m := body["message"].(map[string]any)
		if _, ok := m["android"]; ok {
			t.Error("android present")
		}
		if _, ok := m["notification"]; !ok {
			t.Error("notification missing")
		}
	})
}

func TestFCMAddressGone(t *testing.T) {
	const violation = `{"error":{"code":400,"status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"INVALID_ARGUMENT"},{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":%q,"description":"Invalid registration token"}]}]}}`
	tests := []struct {
		name   string
		status int
		body   string
		gone   bool
	}{
		{"unregistered", 404, `{"error":{"code":404,"status":"NOT_FOUND","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`, true},
		{"bad token", 400, fmt.Sprintf(violation, "message.token"), true},
		{"bad ttl", 400, fmt.Sprintf(violation, "message.android.ttl"), false},
		{"server error", 500, `{"error":{"code":500,"status":"INTERNAL"}}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fcmSend(t, tc.status, tc.body, nil, nil)
			if err == nil {
				t.Fatal("want error")
			}
			if got := errors.Is(err, driver.ErrAddressGone); got != tc.gone {
				t.Errorf("ErrAddressGone = %v, want %v (%v)", got, tc.gone, err)
			}
		})
	}
}

func TestFCMMaxSealedDataFits(t *testing.T) {
	body, err := fcmSend(t, http.StatusOK, `{}`, &driver.PushOptions{DataOnly: true, Data: map[string]string{
		"sealed": strings.Repeat("A", 3480), "kid": strings.Repeat("a", 16), "kind": "withdraw", "collapse_id": strings.Repeat("b", 16),
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for k, v := range body["message"].(map[string]any)["data"].(map[string]any) {
		n += len(k) + len(v.(string))
	}
	if n > 4096 {
		t.Errorf("data is %d bytes", n)
	}
}
