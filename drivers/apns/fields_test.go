package apns

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/xraph/herald/driver"
)

func TestFieldsCoverValidate(t *testing.T) {
	d := &Driver{}
	merged := map[string]string{}
	seen := map[string]bool{}
	for _, f := range d.Fields() {
		if seen[f.Key] {
			t.Errorf("field %q declared twice", f.Key)
		}
		seen[f.Key] = true
		if f.Secret && f.Placement != driver.PlacementCredential {
			t.Errorf("secret field %q must be a credential", f.Key)
		}
		if f.Required {
			merged[f.Key] = "x"
		}
	}
	for k, v := range extra(t) {
		merged[k] = v
	}
	if err := d.Validate(merged, nil); err != nil {
		t.Fatalf("required fields filled, Validate still says: %v", err)
	}
}

func extra(t *testing.T) map[string]string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))}
}
