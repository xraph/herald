// Package ses provides a Herald driver for AWS SES v2 email delivery.
package ses

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
	"sort"
	"strings"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/message"
)

// Driver delivers email via the AWS SES v2 API with SigV4 signing.
type Driver struct{}

var _ driver.Driver = (*Driver)(nil)

func (d *Driver) Name() string    { return "ses" }
func (d *Driver) Channel() string { return "email" }

func (d *Driver) Validate(credentials, _ map[string]string) error {
	required := []string{"access_key_id", "secret_access_key", "region"}
	for _, key := range required {
		if credentials[key] == "" {
			return fmt.Errorf("ses: missing required credential %q", key)
		}
	}
	return nil
}

type sesDestination struct {
	ToAddresses []string `json:"ToAddresses"`
}

type sesBody struct {
	HTML *sesContent `json:"Html,omitempty"`
	Text *sesContent `json:"Text,omitempty"`
}

type sesContent struct {
	Data string `json:"Data"`
}

type sesMessage struct {
	Subject sesContent `json:"Subject"`
	Body    sesBody    `json:"Body"`
}

type sesEmailContent struct {
	Simple *sesSimpleContent `json:"Simple"`
}

type sesSimpleContent struct {
	Subject sesContent `json:"Subject"`
	Body    sesBody    `json:"Body"`
}

type sesRequest struct {
	FromEmailAddress string          `json:"FromEmailAddress"`
	Destination      sesDestination  `json:"Destination"`
	Content          sesEmailContent `json:"Content"`
}

type sesResponse struct {
	MessageId string `json:"MessageId"` //nolint:revive // AWS API uses this casing
}

func (d *Driver) Send(ctx context.Context, msg *driver.OutboundMessage) (*driver.DeliveryResult, error) {
	accessKeyID := msg.Data["access_key_id"]
	secretAccessKey := msg.Data["secret_access_key"]
	region := msg.Data["region"]

	from := msg.From
	if msg.FromName != "" {
		from = msg.FromName + " <" + msg.From + ">"
	}

	reqBody := sesRequest{
		FromEmailAddress: from,
		Destination:      sesDestination{ToAddresses: []string{msg.To}},
		Content: sesEmailContent{
			Simple: &sesSimpleContent{
				Subject: sesContent{Data: msg.Subject},
			},
		},
	}

	if msg.HTML != "" {
		reqBody.Content.Simple.Body.HTML = &sesContent{Data: msg.HTML}
	}
	if msg.Text != "" {
		reqBody.Content.Simple.Body.Text = &sesContent{Data: msg.Text}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ses: marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("https://email.%s.amazonaws.com/v2/email/outbound-emails", region)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("ses: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	signV4(httpReq, jsonBody, accessKeyID, secretAccessKey, region, "ses")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ses: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:errcheck // best-effort error body read
		return nil, fmt.Errorf("ses: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result sesResponse
	_ = json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck // best-effort response parse

	return &driver.DeliveryResult{
		ProviderMessageID: result.MessageId,
		Status:            message.StatusSent,
	}, nil
}

// signV4 adds AWS Signature Version 4 headers to an HTTP request.
func signV4(req *http.Request, payload []byte, accessKey, secretKey, region, service string) {
	now := time.Now().UTC()
	datestamp := now.Format("20060102")
	amzdate := now.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzdate)

	payloadHash := sha256Hex(payload)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Canonical headers
	signedHeaders := canonicalHeaders(req)

	// Canonical request
	canonicalReq := strings.Join([]string{
		req.Method,
		req.URL.Path,
		req.URL.RawQuery,
		canonicalHeaderString(req, signedHeaders),
		strings.Join(signedHeaders, ";"),
		payloadHash,
	}, "\n")

	// String to sign
	credentialScope := datestamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzdate + "\n" + credentialScope + "\n" + sha256Hex([]byte(canonicalReq))

	// Signing key
	signingKey := deriveSigningKey(secretKey, datestamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// Authorization header
	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, strings.Join(signedHeaders, ";"), signature)
	req.Header.Set("Authorization", auth)
}

func canonicalHeaders(req *http.Request) []string {
	headers := make([]string, 0, len(req.Header)+1)
	headers = append(headers, "host")
	for key := range req.Header {
		headers = append(headers, strings.ToLower(key))
	}
	sort.Strings(headers)
	return headers
}

func canonicalHeaderString(req *http.Request, signed []string) string {
	var b strings.Builder
	for _, h := range signed {
		if h == "host" {
			b.WriteString("host:" + req.Host + "\n")
		} else {
			vals := req.Header.Values(http.CanonicalHeaderKey(h))
			b.WriteString(h + ":" + strings.Join(vals, ",") + "\n")
		}
	}
	return b.String()
}

func deriveSigningKey(secret, datestamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
