// Package cloudflare provides a Herald driver for the Cloudflare Email Service.
package cloudflare

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

// Driver delivers email via the Cloudflare Email Service REST API.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "cloudflare" }
func (d *Driver) Channel() string { return "email" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	if credentials["api_token"] == "" {
		return fmt.Errorf("cloudflare: missing required credential 'api_token'")
	}
	if credentials["account_id"] == "" {
		return fmt.Errorf("cloudflare: missing required credential 'account_id'")
	}
	return nil
}

type cfRequest struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	HTML    string `json:"html,omitempty"`
	Text    string `json:"text,omitempty"`
}

type cfResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Delivered        []string `json:"delivered"`
		PermanentBounces []string `json:"permanent_bounces"`
		Queued           []string `json:"queued"`
	} `json:"result"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	apiToken := msg.Data["api_token"]
	accountID := msg.Data["account_id"]
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}

	from := msg.From
	if msg.FromName != "" {
		from = msg.FromName + " <" + msg.From + ">"
	}

	body := cfRequest{
		To:      msg.To,
		From:    from,
		Subject: msg.Subject,
		HTML:    msg.HTML,
		Text:    msg.Text,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/accounts/%s/email/sending/send", baseURL, accountID)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("cloudflare: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("cloudflare: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result cfResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	if len(result.Result.PermanentBounces) > 0 {
		return nil, fmt.Errorf("cloudflare: permanent bounce for %v", result.Result.PermanentBounces)
	}

	return &driver.DeliveryResult{
		Status: message.StatusSent,
	}, nil
}
