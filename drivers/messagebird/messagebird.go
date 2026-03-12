// Package messagebird provides a Herald driver for MessageBird SMS delivery.
package messagebird

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

// Driver delivers SMS via the MessageBird REST API.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "messagebird" }
func (d *Driver) Channel() string { return "sms" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	if credentials["access_key"] == "" {
		return fmt.Errorf("messagebird: missing required credential 'access_key'")
	}
	if credentials["originator"] == "" {
		return fmt.Errorf("messagebird: missing required credential 'originator'")
	}
	return nil
}

type mbRequest struct {
	Originator string   `json:"originator"`
	Recipients []string `json:"recipients"`
	Body       string   `json:"body"`
}

type mbResponse struct {
	ID string `json:"id"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	accessKey := msg.Data["access_key"]
	originator := msg.Data["originator"]

	if msg.From != "" {
		originator = msg.From
	}

	reqBody := mbRequest{
		Originator: originator,
		Recipients: []string{msg.To},
		Body:       msg.Text,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("messagebird: marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://rest.messagebird.com/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("messagebird: create request: %w", err)
	}
	req.Header.Set("Authorization", "AccessKey "+accessKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("messagebird: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("messagebird: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result mbResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	return &driver.DeliveryResult{
		ProviderMessageID: result.ID,
		Status:            message.StatusSent,
	}, nil
}
