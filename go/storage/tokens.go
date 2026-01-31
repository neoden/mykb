package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// TokenType represents the type of token.
type TokenType string

const (
	TokenAccess  TokenType = "access"
	TokenRefresh TokenType = "refresh"
)

// Token represents a stored token.
type Token struct {
	Hash      string
	Type      TokenType
	ClientID  string
	ExpiresAt int64
}

// HashToken creates a SHA-256 hash of a token for storage.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// StoreToken stores a token in the database.
// It also cleans up expired tokens.
func (db *DB) StoreToken(hash string, typ TokenType, clientID string, expiresAt int64) error {
	// Cleanup expired tokens first
	db.conn.Exec("DELETE FROM tokens WHERE expires_at < ?", time.Now().Unix())

	_, err := db.conn.Exec(
		"INSERT OR REPLACE INTO tokens (hash, type, client_id, expires_at) VALUES (?, ?, ?, ?)",
		hash, string(typ), clientID, expiresAt,
	)
	return err
}

// ValidateToken checks if a token is valid and returns its data.
func (db *DB) ValidateToken(hash string, typ TokenType) (*Token, error) {
	var t Token
	err := db.conn.QueryRow(
		"SELECT hash, type, client_id, expires_at FROM tokens WHERE hash = ? AND type = ? AND expires_at > ?",
		hash, string(typ), time.Now().Unix(),
	).Scan(&t.Hash, &t.Type, &t.ClientID, &t.ExpiresAt)

	if err != nil {
		return nil, err
	}
	return &t, nil
}

// DeleteToken removes a token from the database.
func (db *DB) DeleteToken(hash string) error {
	_, err := db.conn.Exec("DELETE FROM tokens WHERE hash = ?", hash)
	return err
}
