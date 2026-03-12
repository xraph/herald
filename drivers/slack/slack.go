// Package slack provides a Herald driver for Slack chat delivery.
//
// It supports two modes:
//   - Incoming webhook: set "webhook_url" credential
//   - Web API (chat.postMessage): set "bot_token" and "channel" credentials
package slack

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

// Driver delivers messages to Slack channels.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "slack" }
func (d *Driver) Channel() string { return "chat" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	hasWebhook := credentials["webhook_url"] != ""
	hasBot := credentials["bot_token"] != "" && credentials["channel"] != ""
	if !hasWebhook && !hasBot {
		return fmt.Errorf("slack: requires either 'webhook_url' or both 'bot_token' and 'channel'")
	}
	return nil
}

type slackWebhookPayload struct {
	Text string `json:"text"`
}

type slackAPIPayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

type slackAPIResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	TS    string `json:"ts,omitempty"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	webhookURL := msg.Data["webhook_url"]
	if webhookURL != "" {
		return d.sendWebhook(ctx, webhookURL, msg)
	}
	return d.sendAPI(ctx, msg)
}

func (d *Driver) sendWebhook(ctx context.Context, url string, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	text := formatMessage(msg)
	payload := slackWebhookPayload{Text: text}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("slack: marshal payload: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("slack: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("slack: webhook error %d: %s", resp.StatusCode, string(respBody))
	}

	return &driver.DeliveryResult{
		Status: message.StatusSent,
	}, nil
}

func (d *Driver) sendAPI(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	botToken := msg.Data["bot_token"]
	channel := msg.Data["channel"]

	// msg.To overrides channel if set (allows per-message targeting).
	if msg.To != "" {
		channel = msg.To
	}

	text := formatMessage(msg)
	payload := slackAPIPayload{Channel: channel, Text: text}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("slack: marshal payload: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("slack: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("slack: API HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	var result slackAPIResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	if !result.OK {
		return nil, fmt.Errorf("slack: API error: %s", result.Error)
	}

	return &driver.DeliveryResult{
		ProviderMessageID: result.TS,
		Status:            message.StatusSent,
	}, nil
}

func formatMessage(msg *driver.OutboundMessage) string {
	if msg.Text != "" {
		return msg.Text
	}
	if msg.Subject != "" {
		return "*" + msg.Subject + "*"
	}
	if msg.Title != "" {
		return msg.Title
	}
	return "New notification"
}
