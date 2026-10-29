package push

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/xraph/herald/driver"
)

const (
	fcmScope = "https://www.googleapis.com/auth/firebase.messaging"
	// fcmTokenMargin is how long before expiry a cached token is replaced.
	fcmTokenMargin = 5 * time.Minute
)

// serviceAccountToken returns a bearer token minted from a Google service
// account JSON key. Tokens are cached per key and replaced once they are
// within five minutes of expiry. Errors never carry the key, the client email
// or the token endpoint's response body.
func (d *FCMDriver) serviceAccountToken(ctx context.Context, keyJSON string) (string, error) {
	sum := sha256.Sum256([]byte(keyJSON))
	cacheKey := hex.EncodeToString(sum[:])

	d.mu.Lock()
	defer d.mu.Unlock()

	if tok, ok := d.tokens[cacheKey]; ok && time.Until(tok.Expiry) > fcmTokenMargin {
		return tok.AccessToken, nil
	}

	cfg, err := google.JWTConfigFromJSON([]byte(keyJSON), fcmScope)
	if err != nil {
		return "", errors.New("fcm: service_account is not a valid service account key")
	}

	client := driver.HTTPClient(10*time.Second, driver.Endpoint{Driver: "fcm", URLTemplate: "/token"})
	tctx := context.WithValue(ctx, oauth2.HTTPClient, client)
	// A fresh TokenSource per mint bypasses oauth2's own reuse cache, which
	// only refreshes 10 seconds before expiry.
	tok, err := cfg.TokenSource(tctx).Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.Response != nil {
			return "", fmt.Errorf("fcm: mint access token failed (status %d)", re.Response.StatusCode)
		}
		return "", errors.New("fcm: mint access token failed")
	}

	if d.tokens == nil {
		d.tokens = map[string]*oauth2.Token{}
	}
	d.tokens[cacheKey] = tok
	return tok.AccessToken, nil
}

// serviceAccountProject returns the project_id of a service account JSON key,
// or "" when it can't be read.
func serviceAccountProject(keyJSON string) string {
	var k struct {
		ProjectID string `json:"project_id"`
	}
	if json.Unmarshal([]byte(keyJSON), &k) != nil {
		return ""
	}
	return k.ProjectID
}
