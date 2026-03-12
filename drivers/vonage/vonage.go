// Package vonage provides a Herald driver for Vonage (Nexmo) SMS delivery.
package vonage

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

// Driver delivers SMS via the Vonage (Nexmo) REST API.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "vonage" }
func (d *Driver) Channel() string { return "sms" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	required := []string{"api_key", "api_secret", "from_number"}
	for _, key := range required {
		if credentials[key] == "" {
			return fmt.Errorf("vonage: missing required credential %q", key)
		}
	}
	return nil
}

type vonageRequest struct {
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
	From      string `json:"from"`
	To        string `json:"to"`
	Text      string `json:"text"`
}

type vonageResponse struct {
	Messages []struct {
		MessageID string `json:"message-id"`
		Status    string `json:"status"`
		ErrorText string `json:"error-text"`
	} `json:"messages"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	apiKey := msg.Data["api_key"]
	apiSecret := msg.Data["api_secret"]
	fromNumber := msg.Data["from_number"]

	if msg.From != "" {
		fromNumber = msg.From
	}

	reqBody := vonageRequest{
		APIKey:    apiKey,
		APISecret: apiSecret,
		From:      fromNumber,
		To:        msg.To,
		Text:      msg.Text,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("vonage: marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://rest.nexmo.com/sms/json", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("vonage: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vonage: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("vonage: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result vonageResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	if len(result.Messages) > 0 && result.Messages[0].Status != "0" {
		return nil, fmt.Errorf("vonage: send failed: %s", result.Messages[0].ErrorText)
	}

	var msgID string
	if len(result.Messages) > 0 {
		msgID = result.Messages[0].MessageID
	}

	return &driver.DeliveryResult{
		ProviderMessageID: msgID,
		Status:            message.StatusSent,
	}, nil
}
