package storage

import (
	"database/sql"
	"errors"
)

var ErrNotFound = errors.New("not found")

// GetSetting retrieves a setting value by key.
func (db *DB) GetSetting(key string) (string, error) {
	var value string
	err := db.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return value, err
}

// SetSetting stores a setting value.
func (db *DB) SetSetting(key, value string) error {
	_, err := db.conn.Exec(
		"INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)",
		key, value,
	)
	return err
}

// GetPasswordHash retrieves the stored password hash.
func (db *DB) GetPasswordHash() (string, error) {
	return db.GetSetting("password_hash")
}

// SetPasswordHash stores the password hash.
func (db *DB) SetPasswordHash(hash string) error {
	return db.SetSetting("password_hash", hash)
}
