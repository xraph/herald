package push

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

const (
	x25519PublicKeyLen = 32
	maxGatewayErrBytes = 200
)

// SealedDriver delivers a push through a gateway without letting it read the
// content. It seals the notification to the device's public key, then asks the
// gateway to relay the sealed bytes, signing the request with an Ed25519 key.
//
// The zero value is ready to use.
type SealedDriver struct {
	// Now supplies the signing time. Nil means time.Now.
	Now func() time.Time
}

var _ driver.Driver = (*SealedDriver)(nil)

// Name returns the driver name.
func (d *SealedDriver) Name() string { return "sealed" }

// Channel returns the channel the driver serves.
func (d *SealedDriver) Channel() string { return "push" }

// Validate checks the signing key and gateway URL.
func (d *SealedDriver) Validate(credentials, settings map[string]string) error {
	if _, err := signingKey(credentials["signing_key"]); err != nil {
		return err
	}
	raw := settings["gateway_url"]
	if raw == "" {
		raw = credentials["gateway_url"]
	}
	return validateGatewayURL(raw)
}

func signingKey(b64 string) (ed25519.PrivateKey, error) {
	if b64 == "" {
		return nil, fmt.Errorf("sealed: missing required credential 'signing_key'")
	}
	seed, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("sealed: credential 'signing_key' must be standard base64 of a 32-byte seed")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func validateGatewayURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("sealed: missing required setting 'gateway_url'")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("sealed: setting 'gateway_url' is not a valid URL")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return nil
		}
	}
	return fmt.Errorf("sealed: setting 'gateway_url' must use https (http only for localhost)")
}

// Send seals msg to the device key and posts it to the gateway.
func (d *SealedDriver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	if len(msg.PublicKey) != x25519PublicKeyLen {
		return nil, ErrMissingPublicKey
	}
	priv, err := signingKey(msg.Data["signing_key"])
	if err != nil {
		return nil, err
	}
	gateway := msg.Data["gateway_url"]
	if gwErr := validateGatewayURL(gateway); gwErr != nil {
		return nil, gwErr
	}

	body := SendBody{
		Handle:     msg.To,
		KeyID:      KeyID(msg.PublicKey),
		Urgency:    string(msg.Push.EffectiveUrgency()),
		Kind:       msg.Push.EffectiveKind(),
		CollapseID: pushCollapseID(msg.Push),
	}
	if msg.Push != nil && msg.Push.TTL > 0 {
		body.TTLSeconds = int64(msg.Push.TTL / time.Second)
	}
	if body.Kind == driver.PushKindWithdraw && body.CollapseID == "" {
		return nil, ErrMissingCollapseID
	}
	if body.Kind != driver.PushKindWithdraw {
		env := Envelope{Title: msg.Title, Body: msg.Text}
		if msg.Push != nil {
			env.Category, env.Thread, env.Data = msg.Push.Category, msg.Push.ThreadID, msg.Push.Data
		}
		var plain, sealed []byte
		if plain, err = MarshalEnvelope(env); err != nil {
			return nil, err
		}
		if sealed, err = Seal(msg.PublicKey, plain); err != nil {
			return nil, err
		}
		body.Sealed = base64.StdEncoding.EncodeToString(sealed)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("sealed: marshal request: %w", err)
	}

	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	ts := strconv.FormatInt(now().Unix(), 10)
	sig := ed25519.Sign(priv, SignatureBase(ts, raw))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(gateway, "/")+"/v1/send", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("sealed: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderKey, base64.StdEncoding.EncodeToString(priv[ed25519.SeedSize:]))
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, base64.StdEncoding.EncodeToString(sig))

	client := driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "sealed", URLTemplate: "/v1/send"})
	resp, err := client.Do(req)
	if err != nil {
		// The transport error carries the gateway URL, which is configuration, not a secret.
		return nil, fmt.Errorf("sealed: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) //nolint:errcheck // best-effort body read

	switch resp.StatusCode {
	case http.StatusAccepted:
		var out struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(respBody, &out) //nolint:errcheck // best-effort response parse
		return &driver.DeliveryResult{ProviderMessageID: out.ID, Status: message.StatusSent}, nil
	case http.StatusGone:
		return nil, fmt.Errorf("sealed: %w (%d)", driver.ErrAddressGone, resp.StatusCode)
	}
	return nil, fmt.Errorf("sealed: gateway answered %d: %s", resp.StatusCode, gatewayError(respBody))
}

func pushCollapseID(p *driver.PushOptions) string {
	if p == nil {
		return ""
	}
	return p.CollapseID
}

// gatewayError returns the gateway's "error" string, cut to 200 bytes.
func gatewayError(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	if len(e.Error) > maxGatewayErrBytes {
		return strings.ToValidUTF8(e.Error[:maxGatewayErrBytes], "")
	}
	return e.Error
}
