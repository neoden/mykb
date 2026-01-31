package httpd

import (
	"testing"
	"time"
)

func TestGenerateToken(t *testing.T) {
	t1 := GenerateToken()
	t2 := GenerateToken()

	if t1 == t2 {
		t.Error("Tokens should be unique")
	}
	if len(t1) < 32 {
		t.Errorf("Token length = %d, expected >= 32", len(t1))
	}
}

func TestHashToken(t *testing.T) {
	h1 := HashToken("test")
	h2 := HashToken("test")
	h3 := HashToken("different")

	if h1 != h2 {
		t.Error("Same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("Different inputs should produce different hashes")
	}
}

func TestHashPKCE(t *testing.T) {
	// Test vector from RFC 7636
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	expected := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	got := HashPKCE(verifier)
	if got != expected {
		t.Errorf("HashPKCE = %q, want %q", got, expected)
	}
}

func TestMemoryTokenStoreBasic(t *testing.T) {
	store := NewMemoryTokenStore()

	token := "test-token"
	data := map[string]string{"key": "value"}

	store.Store(token, data, time.Minute)

	got, ok := store.Get(token)
	if !ok {
		t.Fatal("Token should exist")
	}
	if got["key"] != "value" {
		t.Errorf("data[key] = %q, want %q", got["key"], "value")
	}
}

func TestMemoryTokenStoreSingleUse(t *testing.T) {
	store := NewMemoryTokenStore()

	store.Store("single-use", nil, time.Minute)

	// First get succeeds
	_, ok1 := store.Get("single-use")
	if !ok1 {
		t.Error("First get should succeed")
	}

	// Second get fails (consumed)
	_, ok2 := store.Get("single-use")
	if ok2 {
		t.Error("Second get should fail (token consumed)")
	}
}

func TestMemoryTokenStoreExpiry(t *testing.T) {
	store := NewMemoryTokenStore()

	store.Store("short-lived", nil, time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	_, ok := store.Get("short-lived")
	if ok {
		t.Error("Expired token should not be returned")
	}
}

func TestMemoryTokenStoreValidate(t *testing.T) {
	store := NewMemoryTokenStore()

	store.Store("check-me", nil, time.Minute)

	// Validate doesn't consume
	if !store.Validate("check-me") {
		t.Error("Validate should return true")
	}
	if !store.Validate("check-me") {
		t.Error("Validate should not consume token")
	}

	// Get consumes
	store.Get("check-me")
	if store.Validate("check-me") {
		t.Error("Token should be consumed after Get")
	}
}

func TestMemoryTokenStoreCleanup(t *testing.T) {
	store := NewMemoryTokenStore()

	// Store expired token
	store.Store("expired", nil, -time.Second)

	// Store new token - triggers cleanup
	store.Store("fresh", nil, time.Minute)

	// Check internal map size
	store.mu.RLock()
	count := len(store.tokens)
	store.mu.RUnlock()

	if count != 1 {
		t.Errorf("Token count = %d, want 1 (expired should be cleaned)", count)
	}
}
