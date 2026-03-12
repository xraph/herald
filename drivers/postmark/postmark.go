// Package postmark provides a Herald driver for Postmark email delivery.
package postmark

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

// Driver delivers email via the Postmark API.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "postmark" }
func (d *Driver) Channel() string { return "email" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	if credentials["server_token"] == "" {
		return fmt.Errorf("postmark: missing required credential 'server_token'")
	}
	return nil
}

type pmRequest struct {
	From     string `json:"From"`
	To       string `json:"To"`
	Subject  string `json:"Subject"`
	HTMLBody string `json:"HtmlBody,omitempty"`
	TextBody string `json:"TextBody,omitempty"`
}

type pmResponse struct {
	MessageID string `json:"MessageID"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	serverToken := msg.Data["server_token"]
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://api.postmarkapp.com"
	}

	from := msg.From
	if msg.FromName != "" {
		from = msg.FromName + " <" + msg.From + ">"
	}

	body := pmRequest{
		From:     from,
		To:       msg.To,
		Subject:  msg.Subject,
		HTMLBody: msg.HTML,
		TextBody: msg.Text,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("postmark: marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/email", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("postmark: create request: %w", err)
	}
	req.Header.Set("X-Postmark-Server-Token", serverToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("postmark: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("postmark: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result pmResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	return &driver.DeliveryResult{
		ProviderMessageID: result.MessageID,
		Status:            message.StatusSent,
	}, nil
}
