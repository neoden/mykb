package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"
)

// TokenType represents the type of token.
type TokenType string

const (
	TokenAccess   TokenType = "access"
	TokenRefresh  TokenType = "refresh"
	TokenAuthCode TokenType = "auth_code"
	TokenCSRF     TokenType = "csrf"
)

// Token represents a stored token.
type Token struct {
	Hash      string
	Type      TokenType
	ClientID  string
	ExpiresAt int64
	Data      map[string]string
}

// HashToken creates a SHA-256 hash of a token for storage.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// StoreToken stores a token in the database.
// data is optional (pass nil for access/refresh tokens).
func (db *DB) StoreToken(hash string, typ TokenType, clientID string, expiresAt int64, data map[string]string) error {
	// Cleanup expired tokens first
	db.conn.Exec("DELETE FROM tokens WHERE expires_at < ?", time.Now().Unix())

	var dataJSON *string
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		s := string(b)
		dataJSON = &s
	}

	_, err := db.conn.Exec(
		"INSERT OR REPLACE INTO tokens (hash, type, client_id, expires_at, data) VALUES (?, ?, ?, ?, ?)",
		hash, string(typ), clientID, expiresAt, dataJSON,
	)
	return err
}

// ValidateToken checks if a token is valid and returns its data.
func (db *DB) ValidateToken(hash string, typ TokenType) (*Token, error) {
	var t Token
	var dataStr sql.NullString
	err := db.conn.QueryRow(
		"SELECT hash, type, client_id, expires_at, data FROM tokens WHERE hash = ? AND type = ? AND expires_at > ?",
		hash, string(typ), time.Now().Unix(),
	).Scan(&t.Hash, &t.Type, &t.ClientID, &t.ExpiresAt, &dataStr)

	if err != nil {
		return nil, err
	}

	if dataStr.Valid && dataStr.String != "" {
		if err := json.Unmarshal([]byte(dataStr.String), &t.Data); err != nil {
			return nil, err
		}
	}

	return &t, nil
}

// ConsumeToken atomically validates and deletes a token (single-use).
func (db *DB) ConsumeToken(hash string, typ TokenType) (*Token, error) {
	var t Token
	var dataStr sql.NullString
	err := db.conn.QueryRow(
		"DELETE FROM tokens WHERE hash = ? AND type = ? AND expires_at > ? RETURNING hash, type, client_id, expires_at, data",
		hash, string(typ), time.Now().Unix(),
	).Scan(&t.Hash, &t.Type, &t.ClientID, &t.ExpiresAt, &dataStr)

	if err != nil {
		return nil, err
	}

	if dataStr.Valid && dataStr.String != "" {
		if err := json.Unmarshal([]byte(dataStr.String), &t.Data); err != nil {
			return nil, err
		}
	}

	return &t, nil
}

// DeleteToken removes a token from the database.
func (db *DB) DeleteToken(hash string) error {
	_, err := db.conn.Exec("DELETE FROM tokens WHERE hash = ?", hash)
	return err
}
