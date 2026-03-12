// Package discord provides a Herald driver for Discord chat delivery via webhooks.
package discord

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

// Driver delivers messages to Discord channels via webhooks.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "discord" }
func (d *Driver) Channel() string { return "chat" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	if credentials["webhook_url"] == "" {
		return fmt.Errorf("discord: missing required credential 'webhook_url'")
	}
	return nil
}

type discordPayload struct {
	Content string         `json:"content,omitempty"`
	Embeds  []discordEmbed `json:"embeds,omitempty"`
}

type discordEmbed struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	webhookURL := msg.Data["webhook_url"]

	payload := discordPayload{}

	// Use embeds for rich content, plain content for simple text.
	if msg.Subject != "" || msg.Title != "" {
		embed := discordEmbed{
			Title:       msg.Title,
			Description: msg.Text,
		}
		if embed.Title == "" {
			embed.Title = msg.Subject
		}
		payload.Embeds = append(payload.Embeds, embed)
	} else {
		payload.Content = msg.Text
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("discord: marshal payload: %w", err)
	}

	// Discord requires ?wait=true to get back the message object.
	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL+"?wait=true", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("discord: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("discord: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	return &driver.DeliveryResult{
		ProviderMessageID: result.ID,
		Status:            message.StatusSent,
	}, nil
}
