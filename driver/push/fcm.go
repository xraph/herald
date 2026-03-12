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
	Message struct {
		Token        string            `json:"token"`
		Notification *fcmNotification  `json:"notification,omitempty"`
		Data         map[string]string `json:"data,omitempty"`
	} `json:"message"`
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
	fcmMsg.Message.Notification = &fcmNotification{
		Title: msg.Title,
		Body:  msg.Text,
	}
	if len(msg.Data) > 0 {
		fcmMsg.Message.Data = msg.Data
	}

	jsonBody, err := json.Marshal(fcmMsg)
	if err != nil {
		return nil, fmt.Errorf("fcm: marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", projectID)

	httpClient := &http.Client{Timeout: 10 * time.Second}

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
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("fcm: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result fcmResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	return &driver.DeliveryResult{
		ProviderMessageID: result.Name,
		Status:            message.StatusSent,
	}, nil
}
