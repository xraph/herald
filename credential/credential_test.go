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
	i := len(enc) - 2
	flipped := enc[:i] + string(rune(enc[i]^1)) + enc[i+1:]
	if _, err := c.Decrypt("p", "k", flipped); err == nil {
		t.Error("a tampered value decrypted")
	}
	for _, bad := range []string{"enc:v1:", "enc:v1:k1", "enc:v1::abc", "enc:v1:k1:!!!"} {
		if _, err := c.Decrypt("p", "k", bad); err == nil {
			t.Errorf("Decrypt(%q) succeeded", bad)
		}
	}
}

func TestKeyValidation(t *testing.T) {
	for name, k := range map[string]Key{
		"short":    {ID: "k1", Bytes: make([]byte, 16)},
		"empty id": {ID: "", Bytes: make([]byte, KeySize)},
		"colon id": {ID: "k:1", Bytes: make([]byte, KeySize)},
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
