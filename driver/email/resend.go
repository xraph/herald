package email

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

// ResendDriver delivers email via the Resend HTTP API.
type ResendDriver struct{}

var _ driver.Driver = (*ResendDriver)(nil)

func (d *ResendDriver) Name() string    { return "resend" }
func (d *ResendDriver) Channel() string { return "email" }

func (d *ResendDriver) Validate(credentials, _ map[string]string) error {
	if credentials["api_key"] == "" {
		return fmt.Errorf("resend: missing required credential 'api_key'")
	}
	return nil
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
}

type resendResponse struct {
	ID string `json:"id"`
}

func (d *ResendDriver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	apiKey := msg.Data["api_key"]
	baseURL := msg.Data["base_url"]
	if baseURL == "" {
		baseURL = "https://api.resend.com"
	}

	from := msg.From
	if msg.FromName != "" {
		from = msg.FromName + " <" + msg.From + ">"
	}

	body := resendRequest{
		From:    from,
		To:      []string{msg.To},
		Subject: msg.Subject,
		HTML:    msg.HTML,
		Text:    msg.Text,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("resend: marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/emails", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("resend: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resend: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("resend: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result resendResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	return &driver.DeliveryResult{
		ProviderMessageID: result.ID,
		Status:            message.StatusSent,
	}, nil
}
