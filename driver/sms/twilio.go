// Package sms provides SMS notification drivers.
package sms

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

// TwilioDriver delivers SMS via the Twilio REST API.
type TwilioDriver struct{}

var _ driver.Driver = (*TwilioDriver)(nil)

func (d *TwilioDriver) Name() string    { return "twilio" }
func (d *TwilioDriver) Channel() string { return "sms" }

func (d *TwilioDriver) Validate(credentials, _ map[string]string) error {
	required := []string{"account_sid", "auth_token", "from_number"}
	for _, key := range required {
		if credentials[key] == "" {
			return fmt.Errorf("twilio: missing required credential %q", key)
		}
	}
	return nil
}

type twilioResponse struct {
	SID string `json:"sid"`
}

func (d *TwilioDriver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	accountSID := msg.Data["account_sid"]
	authToken := msg.Data["auth_token"]
	fromNumber := msg.Data["from_number"]

	if msg.From != "" {
		fromNumber = msg.From
	}

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", accountSID)

	data := url.Values{}
	data.Set("To", msg.To)
	data.Set("From", fromNumber)
	data.Set("Body", msg.Text)

	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("twilio: create request: %w", err)
	}

	req.SetBasicAuth(accountSID, authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twilio: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("twilio: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result twilioResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	return &driver.DeliveryResult{
		ProviderMessageID: result.SID,
		Status:            message.StatusSent,
	}, nil
}
