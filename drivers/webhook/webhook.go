// Package webhook provides a Herald driver for webhook delivery.
//
// The webhook driver sends notification payloads via HTTP POST to a
// configured URL. It optionally integrates with Relay for built-in
// retries, signing, and dead-letter queue support.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/xraph/relay"
	"github.com/xraph/relay/event"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

// Driver delivers notifications via HTTP webhooks.
// If a Relay instance is provided, events are published through Relay
// (which provides retries, signatures, and DLQ). Otherwise, a direct
// HTTP POST is performed with optional HMAC-SHA256 signing.
type Driver struct {
	relay *relay.Relay
}

// New creates a new webhook driver. If r is non-nil, events are
// published through Relay instead of direct HTTP POST.
func New(r *relay.Relay) *Driver {
	return &Driver{relay: r}
}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "webhook" }
func (d *Driver) Channel() string { return "webhook" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	if credentials["url"] == "" {
		return fmt.Errorf("webhook: missing required credential 'url'")
	}
	return nil
}

type webhookPayload struct {
	Event     string            `json:"event"`
	To        string            `json:"to"`
	From      string            `json:"from,omitempty"`
	Subject   string            `json:"subject,omitempty"`
	Text      string            `json:"text,omitempty"`
	HTML      string            `json:"html,omitempty"`
	Title     string            `json:"title,omitempty"`
	Data      map[string]string `json:"data,omitempty"`
	Timestamp string            `json:"timestamp"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	webhookURL := msg.Data["url"]
	signingSecret := msg.Data["signing_secret"]

	payload := webhookPayload{
		Event:     msg.Data["event_type"],
		To:        msg.To,
		From:      msg.From,
		Subject:   msg.Subject,
		Text:      msg.Text,
		HTML:      msg.HTML,
		Title:     msg.Title,
		Data:      filterData(msg.Data),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if payload.Event == "" {
		payload.Event = "notification"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("webhook: marshal payload: %w", err)
	}

	// If Relay is available, publish through it for retries + DLQ.
	if d.relay != nil {
		return d.sendViaRelay(ctx, webhookURL, body, payload.Event)
	}

	return d.sendDirect(ctx, webhookURL, body, signingSecret)
}

func (d *Driver) sendViaRelay(ctx context.Context, _ string, body []byte, eventType string) (*driver.DeliveryResult, error) {
	evt := &event.Event{
		Type: eventType,
		Data: json.RawMessage(body),
	}
	if err := d.relay.Send(ctx, evt); err != nil {
		return nil, fmt.Errorf("webhook: relay send: %w", err)
	}
	return &driver.DeliveryResult{
		ProviderMessageID: evt.ID.String(),
		Status:            message.StatusSent,
	}, nil
}

func (d *Driver) sendDirect(ctx context.Context, url string, body []byte, signingSecret string) (*driver.DeliveryResult, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("webhook: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if signingSecret != "" {
		sig := computeHMAC(body, signingSecret)
		req.Header.Set("X-Webhook-Signature", sig)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("webhook: HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	return &driver.DeliveryResult{
		Status: message.StatusSent,
	}, nil
}

func computeHMAC(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// filterData returns a copy of data excluding internal credential keys.
func filterData(data map[string]string) map[string]string {
	exclude := map[string]bool{
		"url":            true,
		"signing_secret": true,
		"event_type":     true,
	}
	filtered := make(map[string]string)
	for k, v := range data {
		if !exclude[k] {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}
