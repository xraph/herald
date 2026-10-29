// Package apns provides a Herald driver for Apple Push Notification service (APNs).
package apns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

// Driver delivers push notifications via the Apple Push Notification service (APNs) HTTP/2 API.
type Driver struct {
	mu     sync.Mutex
	tokens map[string]cachedToken // keyed by team ID, key ID and public key fingerprint
}

type cachedToken struct {
	token string
	exp   time.Time
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

type apnsAlert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// criticalSound is the sound dictionary APNs needs for a critical alert.
type criticalSound struct {
	Critical int    `json:"critical"`
	Name     string `json:"name"`
	Volume   int    `json:"volume"`
}

// interruptionLevels maps a Herald urgency to the APNs interruption-level value.
var interruptionLevels = map[driver.Urgency]string{
	driver.UrgencyPassive:       "passive",
	driver.UrgencyActive:        "active",
	driver.UrgencyTimeSensitive: "time-sensitive",
	driver.UrgencyCritical:      "critical",
}

// buildPayload returns the request body, the apns-push-type and the
// apns-priority (empty when the old behaviour sends none).
func buildPayload(msg *driver.OutboundMessage) (body []byte, pushType, priority string, err error) {
	opts := msg.Push
	hasAlert := opts == nil || msg.Title != "" || msg.Text != ""
	background := !hasAlert && opts.ContentAvailable
	if !hasAlert && !background {
		return nil, "", "", errors.New("apns: nothing to send: no alert and not a background push")
	}

	aps := map[string]any{}
	if hasAlert {
		aps["alert"] = apnsAlert{Title: msg.Title, Body: msg.Text}
		aps["sound"] = "default"
	}
	pushType = "alert"
	if background {
		pushType = "background"
	}

	top := map[string]any{}
	if opts != nil {
		for k, v := range opts.Data {
			if k != "aps" {
				top[k] = v
			}
		}
	}
	switch {
	case background:
		// Apple wants a background push's aps to hold content-available only.
		aps["content-available"] = 1
		priority = "5"
	case opts != nil:
		if level, ok := interruptionLevels[opts.Urgency]; ok {
			aps["interruption-level"] = level
		}
		if opts.Urgency == driver.UrgencyCritical && hasAlert {
			aps["sound"] = criticalSound{Critical: 1, Name: "default", Volume: 1}
		}
		if opts.MutableContent {
			aps["mutable-content"] = 1
		}
		if opts.ContentAvailable {
			aps["content-available"] = 1
		}
		if opts.Category != "" {
			aps["category"] = opts.Category
		}
		if opts.ThreadID != "" {
			aps["thread-id"] = opts.ThreadID
		}
		priority = "10"
		if opts.Urgency == driver.UrgencyPassive {
			priority = "5"
		}
	}
	top["aps"] = aps

	body, err = json.Marshal(top)
	if err != nil {
		return nil, "", "", fmt.Errorf("apns: marshal payload: %w", err)
	}
	return body, pushType, priority, nil
}

// scrubURL drops the request URL a *url.Error carries. The URL holds the
// device token, which must never reach an error message.
func scrubURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}

// addressGone reports whether APNs says the device token is dead.
func addressGone(status int, respBody []byte) bool {
	if status == http.StatusGone {
		return true
	}
	if status != http.StatusBadRequest {
		return false
	}
	var r struct {
		Reason string `json:"reason"`
	}
	return json.Unmarshal(respBody, &r) == nil && r.Reason == "BadDeviceToken"
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

	host := resolveHost(msg.Data["base_url"], sandbox)

	body, pushType, priority, err := buildPayload(msg)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/3/device/%s", host, deviceToken), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("apns: create request: %w", scrubURL(err))
	}

	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("apns-topic", bundleID)
	req.Header.Set("apns-push-type", pushType)
	if priority != "" {
		req.Header.Set("apns-priority", priority)
	}
	if p := msg.Push; p != nil {
		if p.TTL > 0 {
			req.Header.Set("apns-expiration", strconv.FormatInt(time.Now().Add(p.TTL).Unix(), 10))
		}
		if p.CollapseID != "" {
			req.Header.Set("apns-collapse-id", p.CollapseID)
		}
	}

	httpClient := driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "apns", URLTemplate: "/3/device/{device_token}"})
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apns: send request: %w", scrubURL(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		if addressGone(resp.StatusCode, respBody) {
			return nil, fmt.Errorf("apns: API error %d: %s: %w", resp.StatusCode, string(respBody), driver.ErrAddressGone)
		}
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

	// APNs tokens are valid for up to 60 minutes; refresh at 50. One driver
	// serves every APNs provider, so the cache is per signing key, never
	// shared. Team ID and key ID are settings anyone editing a provider can
	// type, so they alone would hand one tenant another's cached token, and
	// keep a stale token after a corrected .p8. The public key fingerprint
	// ties the entry to the key that signed it.
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	fingerprint := sha256.Sum256(pub)
	cacheKey := teamID + "/" + keyID + "/" + hex.EncodeToString(fingerprint[:])
	if c, ok := d.tokens[cacheKey]; ok && time.Now().Before(c.exp) {
		return c.token, nil
	}

	token, err := generateJWT(keyID, teamID, key)
	if err != nil {
		return "", err
	}

	if d.tokens == nil {
		d.tokens = map[string]cachedToken{}
	}
	d.tokens[cacheKey] = cachedToken{token: token, exp: time.Now().Add(50 * time.Minute)}
	return token, nil
}

// resolveHost returns the APNs base host. An explicit base_url overrides the
// default; otherwise the sandbox or production host is chosen.
func resolveHost(baseURL string, sandbox bool) string {
	if baseURL != "" {
		return baseURL
	}
	if sandbox {
		return "https://api.sandbox.push.apple.com"
	}
	return "https://api.push.apple.com"
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
