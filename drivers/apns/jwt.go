package apns

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"time"
)

// generateJWT creates a minimal ES256 JWT for APNs authentication.
func generateJWT(keyID, teamID string, key *ecdsa.PrivateKey) (string, error) {
	header := map[string]string{
		"alg": "ES256",
		"kid": keyID,
	}
	claims := map[string]any{
		"iss": teamID,
		"iat": time.Now().Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64

	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}

	// ES256 signature is r || s, each 32 bytes (P-256).
	curveBits := key.Curve.Params().BitSize
	keyBytes := curveBits / 8
	if curveBits%8 > 0 {
		keyBytes++
	}

	rBytes := r.Bytes()
	sBytes := s.Bytes()
	sig := make([]byte, 2*keyBytes)
	copy(sig[keyBytes-len(rBytes):keyBytes], rBytes)
	copy(sig[2*keyBytes-len(sBytes):], sBytes)

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64, nil
}

// verifyJWT is unexported and used only for testing. It verifies the ES256 signature.
func verifyJWT(token string, key *ecdsa.PublicKey) bool {
	parts := splitJWT(token)
	if len(parts) != 3 {
		return false
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	curveBits := key.Curve.Params().BitSize
	keyBytes := curveBits / 8
	if curveBits%8 > 0 {
		keyBytes++
	}

	if len(sigBytes) != 2*keyBytes {
		return false
	}

	r := new(big.Int).SetBytes(sigBytes[:keyBytes])
	s := new(big.Int).SetBytes(sigBytes[keyBytes:])

	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	return ecdsa.Verify(key, hash[:], r, s)
}

func splitJWT(token string) []string {
	var parts []string
	start := 0
	for i := range token {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}
