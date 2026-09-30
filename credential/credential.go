// Package credential encrypts provider credential values at rest.
//
// An encrypted value is self-describing:
//
//	enc:v1:<keyID>:<base64url(nonce || AES-256-GCM ciphertext)>
//
// so protection is a property of each stored value, never of the config.
// Anything without the prefix is plaintext. The additional authenticated data
// is the provider ID and the credential's key joined by a zero byte, so a
// ciphertext copied into another provider, or under another key name, fails
// to decrypt instead of quietly becoming someone else's secret.
package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const prefix = "enc:v1:"

// KeySize is the only key length accepted: AES-256.
const KeySize = 32

// Protection values reported per credential value.
const (
	ProtectionAESGCM    = "aes-256-gcm"
	ProtectionPlaintext = "plaintext"
)

var (
	// ErrKeyUnavailable means a value was encrypted under a key ID that isn't
	// configured, or no key is configured at all.
	ErrKeyUnavailable = errors.New("credential key is not configured")
	// ErrMalformed means a value has the encrypted prefix but can't be read.
	ErrMalformed = errors.New("credential value is malformed")
)

// Key is one AES-256 key and the ID stored beside every value it encrypts.
type Key struct {
	ID    string
	Bytes []byte
}

// Cipher encrypts with its primary key and decrypts with any configured key.
// A nil *Cipher is valid: it passes plaintext through and refuses ciphertext.
type Cipher struct {
	primary string
	aeads   map[string]cipher.AEAD
}

// NewCipher builds a Cipher that encrypts with primary and can also decrypt
// values written under any of previous.
func NewCipher(primary Key, previous ...Key) (*Cipher, error) {
	c := &Cipher{primary: primary.ID, aeads: map[string]cipher.AEAD{}}
	for _, k := range append([]Key{primary}, previous...) {
		if k.ID == "" || strings.Contains(k.ID, ":") {
			return nil, fmt.Errorf("credential: key ID %q must be non-empty and contain no colon", k.ID)
		}
		if len(k.Bytes) != KeySize {
			return nil, fmt.Errorf("credential: key %q is %d bytes, want %d", k.ID, len(k.Bytes), KeySize)
		}
		if _, dup := c.aeads[k.ID]; dup {
			return nil, fmt.Errorf("credential: key ID %q is configured twice", k.ID)
		}
		block, err := aes.NewCipher(k.Bytes)
		if err != nil {
			return nil, fmt.Errorf("credential: key %q: %w", k.ID, err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("credential: key %q: %w", k.ID, err)
		}
		c.aeads[k.ID] = aead
	}
	return c, nil
}

// ParseKey decodes a standard base64 key and checks its length.
func ParseKey(encoded string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("credential: key is not valid base64: %w", err)
	}
	if len(b) != KeySize {
		return nil, fmt.Errorf("credential: key is %d bytes, want %d", len(b), KeySize)
	}
	return b, nil
}

// KeyID is the ID of the key new values are encrypted under, or "" for a nil
// Cipher.
func (c *Cipher) KeyID() string {
	if c == nil {
		return ""
	}
	return c.primary
}

// Encrypt seals plaintext for one credential of one provider.
func (c *Cipher) Encrypt(providerID, name, plaintext string) (string, error) {
	if c == nil {
		return "", ErrKeyUnavailable
	}
	aead := c.aeads[c.primary]
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("credential: nonce: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), aad(providerID, name))
	return prefix + c.primary + ":" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a stored value. Plaintext passes through unchanged, so callers
// can decrypt a map without checking each value first.
func (c *Cipher) Decrypt(providerID, name, value string) (string, error) {
	if !IsEncrypted(value) {
		return value, nil
	}
	keyID, payload, ok := strings.Cut(strings.TrimPrefix(value, prefix), ":")
	if !ok || keyID == "" {
		return "", ErrMalformed
	}
	if c == nil {
		return "", fmt.Errorf("%w: %s", ErrKeyUnavailable, keyID)
	}
	aead, ok := c.aeads[keyID]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrKeyUnavailable, keyID)
	}
	sealed, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || len(sealed) < aead.NonceSize() {
		return "", ErrMalformed
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], aad(providerID, name))
	if err != nil {
		return "", fmt.Errorf("%w: authentication failed", ErrMalformed)
	}
	return string(plain), nil
}

// IsEncrypted reports whether a stored value carries the encrypted prefix.
func IsEncrypted(value string) bool { return strings.HasPrefix(value, prefix) }

// Describe reports how one stored value is protected, and under which key.
// An absent marker means plaintext, never "unknown".
func Describe(value string) (protection, keyID string) {
	if !IsEncrypted(value) {
		return ProtectionPlaintext, ""
	}
	keyID, _, _ = strings.Cut(strings.TrimPrefix(value, prefix), ":")
	return ProtectionAESGCM, keyID
}

func aad(providerID, name string) []byte {
	return []byte(providerID + "\x00" + name)
}
