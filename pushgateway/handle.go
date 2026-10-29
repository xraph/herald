package pushgateway

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

const (
	platformAndroid = "android"
	platformIOS     = "ios"

	handlePrefix = "h1"
	nonceSize    = 12
	claimsV      = 1
)

var (
	// errRetiredKey means the handle names a sealing key the gateway no longer holds.
	errRetiredKey = errors.New("pushgateway: handle sealed under a retired key")
	// errBadHandle means the handle is malformed or fails authentication.
	errBadHandle = errors.New("pushgateway: bad handle")
)

// claims is the plaintext a handle seals.
type claims struct {
	V   int    `json:"v"`
	P   string `json:"p"`   // platform
	T   string `json:"t"`   // device token
	E   string `json:"e"`   // apns environment, iOS only
	F   string `json:"f"`   // server key fingerprint
	IAT int64  `json:"iat"` // issued at, unix seconds
}

// sealHandle seals c under the first sealing key.
func (g *gateway) sealHandle(c claims) (string, error) {
	k := g.keys[0]
	plain, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	prefix := handlePrefix + "." + k.ID
	nonce := make([]byte, nonceSize)
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	out := g.aeads[k.ID].Seal(nonce, nonce, plain, []byte(prefix))
	return prefix + "." + base64.RawURLEncoding.EncodeToString(out), nil
}

// openHandle opens a handle with the key its ID segment names.
func (g *gateway) openHandle(h string) (claims, error) {
	var c claims
	parts := strings.Split(h, ".")
	if len(parts) != 3 || parts[0] != handlePrefix {
		return c, errBadHandle
	}
	aead, ok := g.aeads[parts[1]]
	if !ok {
		return c, errRetiredKey
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(raw) < nonceSize+aead.Overhead() {
		return c, errBadHandle
	}
	plain, err := aead.Open(nil, raw[:nonceSize], raw[nonceSize:], []byte(parts[0]+"."+parts[1]))
	if err != nil {
		return c, errBadHandle
	}
	if err = json.Unmarshal(plain, &c); err != nil || c.V != claimsV {
		return claims{}, errBadHandle
	}
	return c, nil
}
