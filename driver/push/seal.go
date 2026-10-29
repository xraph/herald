package push

import (
	"crypto/ecdh"
	"crypto/hpke"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// SealInfo is the HPKE info string every sealed push envelope is bound to.
const SealInfo = "herald push v1"

// MaxEnvelopeBytes is the largest envelope JSON, in bytes, accepted before sealing.
const MaxEnvelopeBytes = 2560

// SealOverhead is what sealing adds to a plaintext: a 32-byte enc and a 16-byte tag.
const SealOverhead = 48

const x25519KeyLen = 32

// Envelope is the plaintext a device opens after unsealing a push.
type Envelope struct {
	V        int               `json:"v"`
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Category string            `json:"category,omitempty"`
	Thread   string            `json:"thread,omitempty"`
	Data     map[string]string `json:"data,omitempty"`
}

var (
	// ErrPayloadTooLarge reports an envelope over MaxEnvelopeBytes.
	ErrPayloadTooLarge = errors.New("herald: push payload too large")
	// ErrMissingPublicKey reports a device without a usable X25519 public key.
	ErrMissingPublicKey = errors.New("herald: push device has no public key")
)

// KeyID returns the lowercase hex of the first 8 bytes of the SHA-256 of publicKey.
func KeyID(publicKey []byte) string {
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:8])
}

// MarshalEnvelope encodes e as JSON with V forced to 1.
func MarshalEnvelope(e Envelope) ([]byte, error) {
	e.V = 1
	b, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("herald: marshal push envelope: %w", err)
	}
	if len(b) > MaxEnvelopeBytes {
		return nil, ErrPayloadTooLarge
	}
	return b, nil
}

// Seal encrypts plaintext to a raw 32-byte X25519 public key with HPKE base
// mode. The result is the 32-byte enc followed by the ciphertext.
func Seal(publicKey, plaintext []byte) ([]byte, error) {
	if len(publicKey) != x25519KeyLen {
		return nil, ErrMissingPublicKey
	}
	pub, err := ecdh.X25519().NewPublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("herald: parse push public key: %w", err)
	}
	pk, err := hpke.NewDHKEMPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("herald: load push public key: %w", err)
	}
	return hpke.Seal(pk, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), []byte(SealInfo), plaintext)
}

// Open decrypts sealed with a raw 32-byte X25519 private key.
func Open(privateKey, sealed []byte) ([]byte, error) {
	priv, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("herald: parse push private key: %w", err)
	}
	sk, err := hpke.NewDHKEMPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("herald: load push private key: %w", err)
	}
	return hpke.Open(sk, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), []byte(SealInfo), sealed)
}
