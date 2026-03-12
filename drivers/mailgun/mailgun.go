// Package mailgun provides a Herald driver for Mailgun email delivery.
package mailgun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

// Driver delivers email via the Mailgun REST API.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "mailgun" }
func (d *Driver) Channel() string { return "email" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	if credentials["api_key"] == "" {
		return fmt.Errorf("mailgun: missing required credential 'api_key'")
	}
	if credentials["domain"] == "" {
		return fmt.Errorf("mailgun: missing required credential 'domain'")
	}
	return nil
}

type mgResponse struct {
	ID string `json:"id"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	apiKey := msg.Data["api_key"]
	domain := msg.Data["domain"]
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://api.mailgun.net"
	}

	from := msg.From
	if msg.FromName != "" {
		from = msg.FromName + " <" + msg.From + ">"
	}

	data := url.Values{}
	data.Set("from", from)
	data.Set("to", msg.To)
	data.Set("subject", msg.Subject)
	if msg.HTML != "" {
		data.Set("html", msg.HTML)
	}
	if msg.Text != "" {
		data.Set("text", msg.Text)
	}

	apiURL := fmt.Sprintf("%s/v3/%s/messages", baseURL, domain)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("mailgun: create request: %w", err)
	}
	req.SetBasicAuth("api", apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mailgun: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("mailgun: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result mgResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	return &driver.DeliveryResult{
		ProviderMessageID: result.ID,
		Status:            message.StatusSent,
	}, nil
}
