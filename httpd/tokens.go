package httpd

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// GenerateToken creates a cryptographically secure random token.
// Panics if the system's secure random number generator fails.
func GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashPKCE creates S256 hash of PKCE code verifier.
func HashPKCE(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
