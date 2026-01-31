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

	if err := db.StoreToken(hash, TokenAccess, "client-123", expiry, nil); err != nil {
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

	db.StoreToken(hash, TokenAccess, "client", expiry, nil)

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

	db.StoreToken(hash, TokenAccess, "client", expiry, nil)

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

	db.StoreToken(hash, TokenRefresh, "client", expiry, nil)

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
	db.StoreToken(expiredHash, TokenAccess, "client", time.Now().Add(-time.Hour).Unix(), nil)

	// Store new token - should trigger cleanup
	newHash := HashToken("new")
	db.StoreToken(newHash, TokenAccess, "client", time.Now().Add(time.Hour).Unix(), nil)

	// Expired token should be gone
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM tokens WHERE hash = ?", expiredHash).Scan(&count)
	if count != 0 {
		t.Error("Expired token should be cleaned up")
	}
}

func TestStoreTokenWithData(t *testing.T) {
	db := setupTestDB(t)

	hash := HashToken("token-with-data")
	expiry := time.Now().Add(time.Hour).Unix()
	data := map[string]string{"key": "value", "foo": "bar"}

	if err := db.StoreToken(hash, TokenCSRF, "client", expiry, data); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}

	token, err := db.ValidateToken(hash, TokenCSRF)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if token.Data["key"] != "value" {
		t.Errorf("Data[key] = %q, want %q", token.Data["key"], "value")
	}
	if token.Data["foo"] != "bar" {
		t.Errorf("Data[foo] = %q, want %q", token.Data["foo"], "bar")
	}
}

func TestConsumeToken(t *testing.T) {
	db := setupTestDB(t)

	hash := HashToken("single-use")
	expiry := time.Now().Add(time.Hour).Unix()
	data := map[string]string{"code": "abc123"}

	db.StoreToken(hash, TokenAuthCode, "client", expiry, data)

	// First consume succeeds
	token, err := db.ConsumeToken(hash, TokenAuthCode)
	if err != nil {
		t.Fatalf("ConsumeToken: %v", err)
	}
	if token.Data["code"] != "abc123" {
		t.Errorf("Data[code] = %q, want %q", token.Data["code"], "abc123")
	}

	// Second consume fails (already consumed)
	_, err = db.ConsumeToken(hash, TokenAuthCode)
	if err == nil {
		t.Error("Second ConsumeToken should fail")
	}
}

func TestConsumeTokenExpired(t *testing.T) {
	db := setupTestDB(t)

	hash := HashToken("expired-consume")
	expiry := time.Now().Add(-time.Hour).Unix() // expired

	db.StoreToken(hash, TokenCSRF, "client", expiry, nil)

	_, err := db.ConsumeToken(hash, TokenCSRF)
	if err == nil {
		t.Error("ConsumeToken should fail for expired token")
	}
}
