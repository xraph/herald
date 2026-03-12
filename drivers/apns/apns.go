// Package apns provides a Herald driver for Apple Push Notification service (APNs).
package apns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

// Driver delivers push notifications via the Apple Push Notification service (APNs) HTTP/2 API.
type Driver struct {
	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "apns" }
func (d *Driver) Channel() string { return "push" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	required := []string{"key_id", "team_id", "bundle_id", "private_key"}
	for _, key := range required {
		if credentials[key] == "" {
			return fmt.Errorf("apns: missing required credential %q", key)
		}
	}
	// Validate the private key can be parsed.
	if _, err := parseECPrivateKey(credentials["private_key"]); err != nil {
		return fmt.Errorf("apns: invalid private_key: %w", err)
	}
	return nil
}

type apnsPayload struct {
	Aps apnsAps `json:"aps"`
}

type apnsAps struct {
	Alert apnsAlert `json:"alert"`
	Sound string    `json:"sound,omitempty"`
	Badge *int      `json:"badge,omitempty"`
}

type apnsAlert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	keyID := msg.Data["key_id"]
	teamID := msg.Data["team_id"]
	bundleID := msg.Data["bundle_id"]
	privateKeyPEM := msg.Data["private_key"]
	sandbox := msg.Data["sandbox"] == "true"

	deviceToken := msg.To

	privateKey, err := parseECPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("apns: parse private key: %w", err)
	}

	token, err := d.getOrRefreshToken(keyID, teamID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("apns: generate token: %w", err)
	}

	host := "https://api.push.apple.com"
	if sandbox {
		host = "https://api.sandbox.push.apple.com"
	}

	payload := apnsPayload{
		Aps: apnsAps{
			Alert: apnsAlert{
				Title: msg.Title,
				Body:  msg.Text,
			},
			Sound: "default",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("apns: marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/3/device/%s", host, deviceToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("apns: create request: %w", err)
	}

	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("apns-topic", bundleID)
	req.Header.Set("apns-push-type", "alert")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apns: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("apns: API error %d: %s", resp.StatusCode, string(respBody))
	}

	return &driver.DeliveryResult{
		ProviderMessageID: resp.Header.Get("apns-id"),
		Status:            message.StatusSent,
	}, nil
}

func (d *Driver) getOrRefreshToken(keyID, teamID string, key *ecdsa.PrivateKey) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// APNs tokens are valid for up to 60 minutes; refresh at 50 minutes.
	if d.token != "" && time.Now().Before(d.tokenExp) {
		return d.token, nil
	}

	token, err := generateJWT(keyID, teamID, key)
	if err != nil {
		return "", err
	}

	d.token = token
	d.tokenExp = time.Now().Add(50 * time.Minute)
	return token, nil
}

func parseECPrivateKey(pemStr string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not ECDSA")
	}
	return ecKey, nil
}
