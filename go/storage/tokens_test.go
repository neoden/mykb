package storage

import (
	"testing"
	"time"
)

func TestHashToken(t *testing.T) {
	hash1 := HashToken("test-token")
	hash2 := HashToken("test-token")
	hash3 := HashToken("different-token")

	if hash1 != hash2 {
		t.Error("Same token should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("Different tokens should produce different hashes")
	}
	if len(hash1) != 64 {
		t.Errorf("Hash length = %d, want 64 (SHA-256 hex)", len(hash1))
	}
}

func TestStoreAndValidateToken(t *testing.T) {
	db := setupTestDB(t)

	hash := HashToken("my-access-token")
	expiry := time.Now().Add(time.Hour).Unix()

	if err := db.StoreToken(hash, TokenAccess, "client-123", expiry); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}

	token, err := db.ValidateToken(hash, TokenAccess)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if token.Hash != hash {
		t.Errorf("Hash = %q, want %q", token.Hash, hash)
	}
	if token.Type != TokenAccess {
		t.Errorf("Type = %q, want %q", token.Type, TokenAccess)
	}
	if token.ClientID != "client-123" {
		t.Errorf("ClientID = %q, want %q", token.ClientID, "client-123")
	}
}

func TestValidateTokenWrongType(t *testing.T) {
	db := setupTestDB(t)

	hash := HashToken("token")
	expiry := time.Now().Add(time.Hour).Unix()

	db.StoreToken(hash, TokenAccess, "client", expiry)

	// Try to validate as refresh token
	_, err := db.ValidateToken(hash, TokenRefresh)
	if err == nil {
		t.Error("Expected error for wrong token type")
	}
}

func TestValidateTokenExpired(t *testing.T) {
	db := setupTestDB(t)

	hash := HashToken("expired-token")
	expiry := time.Now().Add(-time.Hour).Unix() // expired

	db.StoreToken(hash, TokenAccess, "client", expiry)

	_, err := db.ValidateToken(hash, TokenAccess)
	if err == nil {
		t.Error("Expected error for expired token")
	}
}

func TestValidateTokenNotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.ValidateToken("nonexistent", TokenAccess)
	if err == nil {
		t.Error("Expected error for nonexistent token")
	}
}

func TestDeleteToken(t *testing.T) {
	db := setupTestDB(t)

	hash := HashToken("to-delete")
	expiry := time.Now().Add(time.Hour).Unix()

	db.StoreToken(hash, TokenRefresh, "client", expiry)

	if err := db.DeleteToken(hash); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	_, err := db.ValidateToken(hash, TokenRefresh)
	if err == nil {
		t.Error("Token should be deleted")
	}
}

func TestStoreTokenCleansExpired(t *testing.T) {
	db := setupTestDB(t)

	// Store expired token
	expiredHash := HashToken("expired")
	db.StoreToken(expiredHash, TokenAccess, "client", time.Now().Add(-time.Hour).Unix())

	// Store new token - should trigger cleanup
	newHash := HashToken("new")
	db.StoreToken(newHash, TokenAccess, "client", time.Now().Add(time.Hour).Unix())

	// Expired token should be gone
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM tokens WHERE hash = ?", expiredHash).Scan(&count)
	if count != 0 {
		t.Error("Expired token should be cleaned up")
	}
}
