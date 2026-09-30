package apns

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
)

func TestTokenCacheIsPerKey(t *testing.T) {
	d := &Driver{}
	k1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	k2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	first, err := d.getOrRefreshToken("KEY1", "TEAM", k1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := d.getOrRefreshToken("KEY2", "TEAM", k2)
	if err != nil {
		t.Fatal(err)
	}
	if !verifyJWT(second, &k2.PublicKey) {
		t.Error("the second provider got a token signed with the first provider's key")
	}
	again, _ := d.getOrRefreshToken("KEY1", "TEAM", k1)
	if again != first {
		t.Error("the same key should reuse its cached token")
	}
}

func TestTokenCacheIsPerSigningKey(t *testing.T) {
	d := &Driver{}
	owner, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	first, err := d.getOrRefreshToken("KEY1", "TEAM", owner)
	if err != nil {
		t.Fatal(err)
	}
	// Same team ID and key ID, different private key: another tenant naming
	// these IDs, or a corrected .p8 under the same key ID.
	second, err := d.getOrRefreshToken("KEY1", "TEAM", other)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("a different private key was handed the cached token of the first")
	}
	if !verifyJWT(second, &other.PublicKey) {
		t.Error("the token for the second key doesn't verify against the second key")
	}
}
