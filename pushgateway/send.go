package pushgateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/xraph/herald/driver"
	"github.com/xraph/herald/driver/push"
)

const (
	maxSendBody    = 16 << 10
	timeWindow     = 300 // seconds either side of the gateway clock
	maxTTLSeconds  = 2419200
	maxCollapseLen = 64
	maxSealedBytes = push.MaxEnvelopeBytes + push.SealOverhead
)

var sendKeyIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// sendResult is what the log line reports about one request.
type sendResult struct {
	server, platform, urgency, kind string
}

func (g *gateway) serveSend(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	res := sendResult{platform: "unknown", urgency: "unknown", kind: "unknown"}
	status := http.StatusBadRequest
	defer func() {
		attrs := make([]any, 0, 12)
		if res.server != "" {
			attrs = append(attrs, "server", res.server)
		}
		attrs = append(attrs,
			"platform", res.platform,
			"urgency", res.urgency,
			"kind", res.kind,
			"outcome", status,
			"latency_ms", time.Since(start).Milliseconds(),
		)
		g.log.Info("push send", attrs...)
	}()
	fail := func(code int, msg string) {
		status = code
		writeError(w, code, msg)
	}

	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSendBody))
	if err != nil {
		fail(http.StatusBadRequest, "request body too large or unreadable")
		return
	}

	key, code, msg := g.authenticate(r, raw)
	if msg != "" {
		fail(code, msg)
		return
	}
	fp := fingerprint(key)
	res.server = fp[:16]

	var body push.SendBody
	if err = json.Unmarshal(raw, &body); err != nil {
		fail(http.StatusBadRequest, "request body is not valid JSON")
		return
	}
	if msg = validateSend(&body); msg != "" {
		fail(http.StatusBadRequest, msg)
		return
	}
	urgency := driver.Urgency(body.Urgency)
	if urgency == "" {
		urgency = driver.UrgencyActive
	}
	kind := body.Kind
	if kind == "" {
		kind = driver.PushKindShow
	}
	res.urgency, res.kind = string(urgency), kind

	c, err := g.openHandle(body.Handle)
	switch {
	case errors.Is(err, errRetiredKey):
		fail(http.StatusGone, "address gone")
		return
	case err != nil:
		fail(http.StatusBadRequest, "invalid handle")
		return
	}
	res.platform = c.P
	if subtle.ConstantTimeCompare([]byte(c.F), []byte(fp)) != 1 {
		fail(http.StatusUnauthorized, "key does not match the handle")
		return
	}

	// Task 12: per-server-key and per-handle rate limits go here.

	p := g.platform(c.P)
	if p == nil {
		// Not 410: that would make the sender delete the device for good.
		fail(http.StatusBadGateway, "delivery failed")
		return
	}
	result, err := g.deliver(r.Context(), p, c, &body, urgency, kind)
	switch {
	case errors.Is(err, driver.ErrAddressGone):
		fail(http.StatusGone, "address gone")
		return
	case err != nil:
		fail(http.StatusBadGateway, "delivery failed")
		return
	}
	id := ""
	if result != nil {
		id = result.ProviderMessageID
	}
	if id == "" {
		id = gatewayMessageID()
	}
	status = http.StatusAccepted
	writeJSON(w, status, map[string]string{"id": id})
}

// authenticate checks the key, signature and timestamp headers. On failure it
// returns the status and message to send.
func (g *gateway) authenticate(r *http.Request, raw []byte) (key ed25519.PublicKey, code int, msg string) {
	const badSig = "bad signature"
	keyHdr := r.Header.Get(push.HeaderKey)
	sigHdr := r.Header.Get(push.HeaderSignature)
	tsHdr := r.Header.Get(push.HeaderTimestamp)
	if keyHdr == "" || sigHdr == "" || tsHdr == "" {
		return nil, http.StatusUnauthorized, badSig
	}
	key, err := base64.StdEncoding.DecodeString(keyHdr)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, http.StatusUnauthorized, badSig
	}
	sig, err := base64.StdEncoding.DecodeString(sigHdr)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, http.StatusUnauthorized, badSig
	}
	ts, err := strconv.ParseInt(tsHdr, 10, 64)
	if err != nil {
		return nil, http.StatusUnauthorized, badSig
	}
	if !ed25519.Verify(key, push.SignatureBase(tsHdr, raw), sig) {
		return nil, http.StatusUnauthorized, badSig
	}
	if d := g.now().Unix() - ts; d > timeWindow || d < -timeWindow {
		return nil, http.StatusUnauthorized, "timestamp outside the allowed window"
	}
	return key, 0, ""
}

// validateSend returns a message for the first invalid field, or "".
func validateSend(b *push.SendBody) string {
	if b.Urgency != "" && !driver.Urgency(b.Urgency).Valid() {
		return "urgency is not valid"
	}
	if b.Kind != "" && b.Kind != driver.PushKindShow && b.Kind != driver.PushKindWithdraw {
		return "kind must be show or withdraw"
	}
	if !sendKeyIDPattern.MatchString(b.KeyID) {
		return "key_id must be 16 lowercase hex characters"
	}
	if b.Kind != driver.PushKindWithdraw && b.Sealed == "" {
		return "sealed is required for show"
	}
	if b.Sealed != "" {
		sealed, err := base64.StdEncoding.DecodeString(b.Sealed)
		if err != nil {
			return "sealed must be standard base64"
		}
		if len(sealed) > maxSealedBytes {
			return "sealed is too large"
		}
	}
	if b.TTLSeconds < 0 || b.TTLSeconds > maxTTLSeconds {
		return "ttl_seconds must be 0 to 2419200"
	}
	if len(b.CollapseID) > maxCollapseLen {
		return "collapse_id must be at most 64 bytes"
	}
	return ""
}

func gatewayMessageID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "gw_" + hex.EncodeToString(buf)
}
