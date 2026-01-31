package httpd

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"sync"
	"time"
)

// MemoryTokenStore stores short-lived tokens in memory (auth codes, CSRF tokens).
type MemoryTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*memToken
}

type memToken struct {
	data      map[string]string
	expiresAt time.Time
}

// NewMemoryTokenStore creates a new in-memory token store.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{
		tokens: make(map[string]*memToken),
	}
}

// GenerateToken creates a cryptographically secure random token.
// Panics if the system's secure random number generator fails.
func GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken creates a SHA-256 hash of a token.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// HashPKCE creates S256 hash of PKCE code verifier.
func HashPKCE(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// Store saves a token with associated data and TTL.
func (s *MemoryTokenStore) Store(token string, data map[string]string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cleanup expired tokens
	now := time.Now()
	for k, v := range s.tokens {
		if v.expiresAt.Before(now) {
			delete(s.tokens, k)
		}
	}

	hash := HashToken(token)
	s.tokens[hash] = &memToken{
		data:      data,
		expiresAt: now.Add(ttl),
	}
}

// Get retrieves and deletes a token (single-use).
func (s *MemoryTokenStore) Get(token string) (map[string]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hash := HashToken(token)
	t, ok := s.tokens[hash]
	if !ok {
		return nil, false
	}

	delete(s.tokens, hash)

	if t.expiresAt.Before(time.Now()) {
		return nil, false
	}

	return t.data, true
}

// Validate checks if a token exists without consuming it.
func (s *MemoryTokenStore) Validate(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hash := HashToken(token)
	t, ok := s.tokens[hash]
	if !ok {
		return false
	}

	return t.expiresAt.After(time.Now())
}
