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
