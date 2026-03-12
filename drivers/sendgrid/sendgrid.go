// Package sendgrid provides a Herald driver for SendGrid email delivery.
package sendgrid

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

// Driver delivers email via the SendGrid v3 Mail Send API.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "sendgrid" }
func (d *Driver) Channel() string { return "email" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	if credentials["api_key"] == "" {
		return fmt.Errorf("sendgrid: missing required credential 'api_key'")
	}
	return nil
}

type sgEmailAddr struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type sgPersonalization struct {
	To []sgEmailAddr `json:"to"`
}

type sgContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sgRequest struct {
	Personalizations []sgPersonalization `json:"personalizations"`
	From             sgEmailAddr         `json:"from"`
	Subject          string              `json:"subject"`
	Content          []sgContent         `json:"content"`
}

type sgResponse struct {
	// SendGrid returns the message ID in the X-Message-Id header, not the body.
	// We capture it from the response headers instead.
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	apiKey := msg.Data["api_key"]

	req := sgRequest{
		Personalizations: []sgPersonalization{
			{To: []sgEmailAddr{{Email: msg.To}}},
		},
		From:    sgEmailAddr{Email: msg.From, Name: msg.FromName},
		Subject: msg.Subject,
	}

	if msg.HTML != "" {
		req.Content = append(req.Content, sgContent{Type: "text/html", Value: msg.HTML})
	} else if msg.Text != "" {
		req.Content = append(req.Content, sgContent{Type: "text/plain", Value: msg.Text})
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("sendgrid: marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.sendgrid.com/v3/mail/send", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sendgrid: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sendgrid: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("sendgrid: API error %d: %s", resp.StatusCode, string(respBody))
	}

	return &driver.DeliveryResult{
		ProviderMessageID: resp.Header.Get("X-Message-Id"),
		Status:            message.StatusSent,
	}, nil
}
