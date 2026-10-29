package push

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
)

var updateVectors = flag.Bool("update-vectors", false, "regenerate testdata/sealed_vectors.json")

const vectorsPath = "testdata/sealed_vectors.json"

func newKeyPair(t *testing.T) (priv, pub []byte) {
	t.Helper()
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k.Bytes(), k.PublicKey().Bytes()
}

func TestKeyIDKnownValue(t *testing.T) {
	if got := KeyID(make([]byte, 32)); got != "66687aadf862bd77" {
		t.Fatalf("KeyID = %q", got)
	}
}

func TestSealRoundTrip(t *testing.T) {
	priv, pub := newKeyPair(t)
	p := []byte(`{"v":1,"title":"t","body":"b"}`)
	sealed, err := Seal(pub, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) != len(p)+SealOverhead {
		t.Fatalf("len = %d, want %d", len(sealed), len(p)+SealOverhead)
	}
	got, err := Open(priv, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, p) {
		t.Fatalf("opened %q", got)
	}
}

func TestSealIsRandomized(t *testing.T) {
	_, pub := newKeyPair(t)
	a, err := Seal(pub, []byte("same"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Seal(pub, []byte("same"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a[:32], b[:32]) {
		t.Fatal("enc repeated across seals")
	}
}

func TestOpenRejectsTampering(t *testing.T) {
	priv, pub := newKeyPair(t)
	sealed, err := Seal(pub, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 32, len(sealed) - 1} {
		c := bytes.Clone(sealed)
		c[i] ^= 0x01
		if _, err := Open(priv, c); err == nil {
			t.Fatalf("flipping byte %d opened", i)
		}
	}
}

func TestSealNeedsAKey(t *testing.T) {
	for _, k := range [][]byte{nil, make([]byte, 31)} {
		if _, err := Seal(k, []byte("x")); !errors.Is(err, ErrMissingPublicKey) {
			t.Fatalf("len %d: err = %v", len(k), err)
		}
	}
}

// padded returns an envelope whose JSON is exactly size bytes.
func padded(t *testing.T, size int) Envelope {
	t.Helper()
	e := Envelope{V: 1, Title: "t", Body: "b", Data: map[string]string{"pad": ""}}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	e.Data["pad"] = strings.Repeat("x", size-len(b))
	return e
}

func TestMarshalEnvelopeLimit(t *testing.T) {
	b, err := MarshalEnvelope(padded(t, MaxEnvelopeBytes))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != MaxEnvelopeBytes {
		t.Fatalf("len = %d", len(b))
	}
	_, err = MarshalEnvelope(padded(t, MaxEnvelopeBytes+1))
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("err = %v", err)
	}
	b, err = MarshalEnvelope(Envelope{V: 0, Title: "t", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"v":1,"title":"t","body":"b"}` {
		t.Fatalf("got %s", b)
	}
}

type vector struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	Plaintext  string `json:"plaintext"`
	Sealed     string `json:"sealed"`
}

func writeVectors(t *testing.T) {
	t.Helper()
	approval, err := MarshalEnvelope(Envelope{
		Title:    "GitHub verification to Production",
		Body:     "Waiting for your approval.",
		Category: "approval",
		Thread:   "prj_01",
		Data:     map[string]string{"kind": "plan.waiting", "actions": "approve,review", "revision": "7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	unicode, err := MarshalEnvelope(Envelope{
		Title:    "Échec du déploiement 🚨",
		Body:     "Ünïcödé, 日本語 and emoji ✅",
		Category: "alert",
	})
	if err != nil {
		t.Fatal(err)
	}
	maxed, err := MarshalEnvelope(padded(t, MaxEnvelopeBytes))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]vector, 0, 3)
	for _, p := range [][]byte{approval, unicode, maxed} {
		priv, pub := newKeyPair(t)
		sealed, sealErr := Seal(pub, p)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		out = append(out, vector{
			PrivateKey: base64.StdEncoding.EncodeToString(priv),
			PublicKey:  base64.StdEncoding.EncodeToString(pub),
			Plaintext:  string(p),
			Sealed:     base64.StdEncoding.EncodeToString(sealed),
		})
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("testdata", 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vectorsPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSealedVectors(t *testing.T) {
	if *updateVectors {
		writeVectors(t)
	}
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	var vs []vector
	if err := json.Unmarshal(raw, &vs); err != nil {
		t.Fatal(err)
	}
	if len(vs) != 3 {
		t.Fatalf("%d vectors, want 3", len(vs))
	}
	for i, v := range vs {
		priv, err := base64.StdEncoding.DecodeString(v.PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		pub, err := base64.StdEncoding.DecodeString(v.PublicKey)
		if err != nil {
			t.Fatal(err)
		}
		sealed, err := base64.StdEncoding.DecodeString(v.Sealed)
		if err != nil {
			t.Fatal(err)
		}
		k, err := ecdh.X25519().NewPrivateKey(priv)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(k.PublicKey().Bytes(), pub) {
			t.Fatalf("vector %d: public key does not match private key", i)
		}
		got, err := Open(priv, sealed)
		if err != nil {
			t.Fatalf("vector %d: %v", i, err)
		}
		if string(got) != v.Plaintext {
			t.Fatalf("vector %d: plaintext mismatch", i)
		}
		var e Envelope
		if err := json.Unmarshal(got, &e); err != nil || e.V != 1 {
			t.Fatalf("vector %d: envelope v = %d, err = %v", i, e.V, err)
		}
	}
	if len(vs[2].Plaintext) != MaxEnvelopeBytes {
		t.Fatalf("vector 3 is %d bytes", len(vs[2].Plaintext))
	}
}
