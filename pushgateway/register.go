package pushgateway

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
)

const (
	maxRegistrationBody = 8 << 10
	maxTokenBytes       = 4096
	serverKeyBytes      = 32

	envProduction = "production"
	envSandbox    = "sandbox"
)

var (
	// iOS tokens are hex. FCM tokens use letters, digits, "_", "-" and ":".
	iosTokenPattern     = regexp.MustCompile(`^[0-9a-fA-F]+$`)
	androidTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_:-]+$`)
)

type registrationRequest struct {
	Platform  string `json:"platform"`
	Token     string `json:"token"`
	APNsEnv   string `json:"apns_env"`
	ServerKey string `json:"server_key"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) //nolint:errcheck // client went away, nothing to do
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// fingerprint is the first 32 hex chars of SHA-256 of the raw server key.
func fingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}

func (g *gateway) serveRegistration(w http.ResponseWriter, r *http.Request) {
	platform := "unknown"
	outcome := "invalid"
	defer func() {
		g.log.Info("push registration", "platform", platform, "outcome", outcome)
	}()
	fail := func(status int, msg string) {
		writeError(w, status, msg)
	}

	var req registrationRequest
	body := http.MaxBytesReader(w, r.Body, maxRegistrationBody)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(http.StatusBadRequest, "request body too large")
		} else {
			fail(http.StatusBadRequest, "request body is not valid JSON")
		}
		return
	}
	if req.Platform != platformAndroid && req.Platform != platformIOS {
		fail(http.StatusBadRequest, "platform must be android or ios")
		return
	}
	platform = req.Platform
	if n := len(req.Token); n == 0 || n > maxTokenBytes {
		fail(http.StatusBadRequest, "token must be 1 to 4096 bytes")
		return
	}
	if (platform == platformIOS && !iosTokenPattern.MatchString(req.Token)) ||
		(platform == platformAndroid && !androidTokenPattern.MatchString(req.Token)) {
		fail(http.StatusBadRequest, "token has characters a "+platform+" token never uses")
		return
	}
	switch {
	case platform == platformIOS && req.APNsEnv != envProduction && req.APNsEnv != envSandbox:
		fail(http.StatusBadRequest, "apns_env must be production or sandbox for ios")
		return
	case platform == platformAndroid && req.APNsEnv != "":
		fail(http.StatusBadRequest, "apns_env must be empty for android")
		return
	}
	serverKey, err := base64.StdEncoding.DecodeString(req.ServerKey)
	if err != nil || len(serverKey) != serverKeyBytes {
		fail(http.StatusBadRequest, "server_key must be base64 of 32 bytes")
		return
	}
	if g.platform(platform) == nil {
		outcome = "platform_off"
		fail(http.StatusBadRequest, "platform is not enabled on this gateway")
		return
	}
	handle, err := g.sealHandle(claims{
		V:   claimsV,
		P:   platform,
		T:   req.Token,
		E:   req.APNsEnv,
		F:   fingerprint(serverKey),
		IAT: g.now().Unix(),
	})
	if err != nil {
		outcome = "error"
		fail(http.StatusInternalServerError, "could not issue handle")
		return
	}
	outcome = "ok"
	writeJSON(w, http.StatusCreated, map[string]string{"handle": handle})
}
