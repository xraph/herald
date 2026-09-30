package credential

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func key(id string, b byte) Key { return Key{ID: id, Bytes: bytes.Repeat([]byte{b}, KeySize)} }

func TestRoundTrip(t *testing.T) {
	c, err := NewCipher(key("k1", 1))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := c.Encrypt("hpvd_1", "api_key", "sk_live_canary")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "enc:v1:k1:") || strings.Contains(enc, "canary") {
		t.Fatalf("unexpected ciphertext %q", enc)
	}
	got, err := c.Decrypt("hpvd_1", "api_key", enc)
	if err != nil || got != "sk_live_canary" {
		t.Fatalf("Decrypt = %q, %v", got, err)
	}
	if p, k := Describe(enc); p != ProtectionAESGCM || k != "k1" {
		t.Errorf("Describe = %q, %q", p, k)
	}
}

func TestNonceIsFreshEachTime(t *testing.T) {
	c, _ := NewCipher(key("k1", 1))
	a, _ := c.Encrypt("p", "k", "same")
	b, _ := c.Encrypt("p", "k", "same")
	if a == b {
		t.Error("two encryptions of the same value produced the same ciphertext")
	}
}

func TestAADBindsProviderAndName(t *testing.T) {
	c, _ := NewCipher(key("k1", 1))
	enc, _ := c.Encrypt("hpvd_1", "api_key", "secret")
	if _, err := c.Decrypt("hpvd_2", "api_key", enc); !errors.Is(err, ErrMalformed) {
		t.Errorf("copied to another provider: err = %v, want ErrMalformed", err)
	}
	if _, err := c.Decrypt("hpvd_1", "password", enc); !errors.Is(err, ErrMalformed) {
		t.Errorf("moved to another key name: err = %v, want ErrMalformed", err)
	}
}

func TestPlaintextPassesThrough(t *testing.T) {
	var c *Cipher
	got, err := c.Decrypt("p", "k", "plain-value")
	if err != nil || got != "plain-value" {
		t.Fatalf("nil cipher on plaintext = %q, %v", got, err)
	}
	if p, k := Describe("plain-value"); p != ProtectionPlaintext || k != "" {
		t.Errorf("Describe(plaintext) = %q, %q", p, k)
	}
}

func TestUnknownKeyIsNamed(t *testing.T) {
	old, _ := NewCipher(key("k0", 9))
	enc, _ := old.Encrypt("p", "k", "secret")

	current, _ := NewCipher(key("k1", 1))
	_, err := current.Decrypt("p", "k", enc)
	if !errors.Is(err, ErrKeyUnavailable) || !strings.Contains(err.Error(), "k0") {
		t.Errorf("err = %v, want ErrKeyUnavailable naming k0", err)
	}
	var none *Cipher
	if _, err := none.Decrypt("p", "k", enc); !errors.Is(err, ErrKeyUnavailable) {
		t.Errorf("nil cipher on ciphertext: err = %v", err)
	}
}

func TestPreviousKeysDecryptOnly(t *testing.T) {
	old, _ := NewCipher(key("k0", 9))
	enc, _ := old.Encrypt("p", "k", "secret")
	rotated, err := NewCipher(key("k1", 1), key("k0", 9))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := rotated.Decrypt("p", "k", enc); err != nil || got != "secret" {
		t.Fatalf("previous key decrypt = %q, %v", got, err)
	}
	fresh, _ := rotated.Encrypt("p", "k", "secret")
	if _, k := Describe(fresh); k != "k1" {
		t.Errorf("new values must use the primary key, got %q", k)
	}
}

func TestTamperedPayloadIsRefused(t *testing.T) {
	c, _ := NewCipher(key("k1", 1))
	enc, _ := c.Encrypt("p", "k", "secret")

	// Flip a byte in the actual ciphertext, not the base64 encoding
	sealed, _ := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(strings.TrimPrefix(enc, "enc:v1:k1:"), ""))
	if len(sealed) > 16 {
		sealed[16] ^= 1 // flip a byte after the nonce (in the ciphertext/tag)
	}
	flipped := "enc:v1:k1:" + base64.RawURLEncoding.EncodeToString(sealed)
	if _, err := c.Decrypt("p", "k", flipped); !errors.Is(err, ErrMalformed) {
		t.Errorf("tampered value decrypted or wrong error: %v", err)
	}

	for _, bad := range []string{"enc:v1:", "enc:v1:k1", "enc:v1::abc", "enc:v1:k1:!!!"} {
		if _, err := c.Decrypt("p", "k", bad); !errors.Is(err, ErrMalformed) {
			t.Errorf("Decrypt(%q) should return ErrMalformed, got %v", bad, err)
		}
	}
}

func TestKeyValidation(t *testing.T) {
	for name, k := range map[string]Key{
		"short":       {ID: "k1", Bytes: make([]byte, 16)},
		"empty id":    {ID: "", Bytes: make([]byte, KeySize)},
		"colon id":    {ID: "k:1", Bytes: make([]byte, KeySize)},
		"has space":   {ID: "k 1", Bytes: make([]byte, KeySize)},
		"has newline": {ID: "k\n1", Bytes: make([]byte, KeySize)},
		"65 chars":    {ID: strings.Repeat("a", 65), Bytes: make([]byte, KeySize)},
	} {
		if _, err := NewCipher(k); err == nil {
			t.Errorf("%s: NewCipher accepted it", name)
		}
	}
	if _, err := NewCipher(key("k1", 1), key("k1", 2)); err == nil {
		t.Error("duplicate key IDs accepted")
	}
	if _, err := ParseKey(base64.StdEncoding.EncodeToString(make([]byte, 16))); err == nil {
		t.Error("ParseKey accepted a 16-byte key")
	}
	if b, err := ParseKey(base64.StdEncoding.EncodeToString(make([]byte, KeySize))); err != nil || len(b) != KeySize {
		t.Errorf("ParseKey(valid) = %d bytes, %v", len(b), err)
	}
}

func TestZeroCipherPanicsInEncrypt(t *testing.T) {
	var c *Cipher
	_, err := c.Encrypt("p", "k", "plaintext")
	if !errors.Is(err, ErrKeyUnavailable) {
		t.Errorf("nil Cipher.Encrypt: got %v, want ErrKeyUnavailable", err)
	}

	empty := &Cipher{}
	_, err = empty.Encrypt("p", "k", "plaintext")
	if !errors.Is(err, ErrKeyUnavailable) {
		t.Errorf("zero-value Cipher.Encrypt: got %v, want ErrKeyUnavailable", err)
	}
}

func TestMalformedKeyIDIsRefused(t *testing.T) {
	c, _ := NewCipher(key("k1", 1))

	malformed := "enc:v1:bad id:xxxx"
	_, err := c.Decrypt("p", "k", malformed)
	if !errors.Is(err, ErrMalformed) {
		t.Errorf("Decrypt(%q): got %v, want ErrMalformed", malformed, err)
	}
	if strings.Contains(err.Error(), "bad id") {
		t.Errorf("error message leaked key ID: %v", err)
	}

	p, kid := Describe(malformed)
	if p != ProtectionAESGCM || kid != "" {
		t.Errorf("Describe(%q) = %q, %q; want ProtectionAESGCM, empty", malformed, p, kid)
	}
}

func TestNoCredentialLeaksInErrors(t *testing.T) {
	plaintext := "sk_live_canary"
	c, _ := NewCipher(key("k1", 1))
	enc, _ := c.Encrypt("p", "k", plaintext)

	// Test that encrypted value doesn't leak plaintext
	if strings.Contains(enc, "canary") {
		t.Error("plaintext leaked in ciphertext")
	}

	// Test that errors don't leak plaintext
	_, err := c.Decrypt("p", "k", "enc:v1:unknown_key:abc123")
	if err != nil && strings.Contains(err.Error(), plaintext) {
		t.Errorf("plaintext leaked in error: %v", err)
	}

	// Test that NewCipher errors don't leak key bytes
	k := key("k1", 5)
	_, err = NewCipher(k, k) // duplicate key ID
	if err != nil && strings.Contains(err.Error(), string(k.Bytes)) {
		t.Errorf("key bytes leaked in error: %v", err)
	}
}
