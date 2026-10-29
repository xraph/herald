// Package push provides push notification drivers.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

// FCMDriver delivers push notifications via Firebase Cloud Messaging HTTP v1 API.
type FCMDriver struct{}

var _ driver.Driver = (*FCMDriver)(nil)

func (d *FCMDriver) Name() string    { return "fcm" }
func (d *FCMDriver) Channel() string { return "push" }

func (d *FCMDriver) Validate(credentials, _ map[string]string) error {
	if credentials["server_key"] == "" && credentials["access_token"] == "" {
		return fmt.Errorf("fcm: missing required credential 'server_key' or 'access_token'")
	}
	return nil
}

type fcmMessage struct {
	Message fcmBody `json:"message"`
}

type fcmBody struct {
	Token        string            `json:"token"`
	Notification *fcmNotification  `json:"notification,omitempty"`
	Data         map[string]string `json:"data,omitempty"`
	Android      *fcmAndroid       `json:"android,omitempty"`
}

type fcmAndroid struct {
	Priority    string `json:"priority,omitempty"`
	CollapseKey string `json:"collapse_key,omitempty"`
	TTL         string `json:"ttl,omitempty"`
}

type fcmErrorBody struct {
	Error struct {
		Details []struct {
			ErrorCode       string `json:"errorCode"`
			FieldViolations []struct {
				Field string `json:"field"`
			} `json:"fieldViolations"`
		} `json:"details"`
	} `json:"error"`
}

// addressGone reports whether an FCM error body says the device token is dead.
// A bad field other than the token is a payload bug and never counts.
func addressGone(body []byte) bool {
	var e fcmErrorBody
	if json.Unmarshal(body, &e) != nil {
		return false
	}
	for _, d := range e.Error.Details {
		if d.ErrorCode == "UNREGISTERED" {
			return true
		}
		for _, v := range d.FieldViolations {
			if v.Field == "message.token" {
				return true
			}
		}
	}
	return false
}

type fcmNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type fcmResponse struct {
	Name string `json:"name"` // message resource name
}

func (d *FCMDriver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	projectID := msg.Data["project_id"]
	accessToken := msg.Data["access_token"]
	serverKey := msg.Data["server_key"]

	var fcmMsg fcmMessage
	fcmMsg.Message.Token = msg.To
	if msg.Push == nil || !msg.Push.DataOnly {
		fcmMsg.Message.Notification = &fcmNotification{
			Title: msg.Title,
			Body:  msg.Text,
		}
	}
	fcmMsg.Message.Data = driver.DataPayload(msg.Data)
	if msg.Push != nil {
		if len(msg.Push.Data) > 0 {
			if fcmMsg.Message.Data == nil {
				fcmMsg.Message.Data = map[string]string{}
			}
			for k, v := range msg.Push.Data {
				fcmMsg.Message.Data[k] = v
			}
		}
		android := &fcmAndroid{Priority: "NORMAL", CollapseKey: msg.Push.CollapseID}
		if u := msg.Push.EffectiveUrgency(); u == driver.UrgencyTimeSensitive || u == driver.UrgencyCritical {
			android.Priority = "HIGH"
		}
		if msg.Push.TTL > 0 {
			android.TTL = fmt.Sprintf("%ds", int64(msg.Push.TTL/time.Second))
		}
		fcmMsg.Message.Android = android
	}

	jsonBody, err := json.Marshal(fcmMsg)
	if err != nil {
		return nil, fmt.Errorf("fcm: marshal request: %w", err)
	}

	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://fcm.googleapis.com"
	}
	apiURL := fmt.Sprintf("%s/v1/projects/%s/messages:send", baseURL, projectID)

	httpClient := driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "fcm", URLTemplate: "/v1/projects/{project_id}/messages:send"})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("fcm: create request: %w", err)
	}

	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	} else if serverKey != "" {
		req.Header.Set("Authorization", "key="+serverKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fcm: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) //nolint:errcheck // best-effort error body read
		if addressGone(respBody) {
			return nil, fmt.Errorf("fcm: %w (%d)", driver.ErrAddressGone, resp.StatusCode)
		}
		return nil, fmt.Errorf("fcm: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result fcmResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	return &driver.DeliveryResult{
		ProviderMessageID: result.Name,
		Status:            message.StatusSent,
	}, nil
}
