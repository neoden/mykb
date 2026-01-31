package storage

import (
	"database/sql"
	"encoding/json"
	"time"
)

// OAuthClient represents a registered OAuth client.
type OAuthClient struct {
	ClientID     string
	ClientName   string
	RedirectURIs []string
	CreatedAt    int64
	LastUsedAt   int64
}

// CreateClient registers a new OAuth client.
func (db *DB) CreateClient(clientID, clientName string, redirectURIs []string) error {
	uris, err := json.Marshal(redirectURIs)
	if err != nil {
		return err
	}

	_, err = db.conn.Exec(
		"INSERT INTO oauth_clients (client_id, client_name, redirect_uris) VALUES (?, ?, ?)",
		clientID, clientName, string(uris),
	)
	return err
}

// GetClient retrieves an OAuth client by ID.
func (db *DB) GetClient(clientID string) (*OAuthClient, error) {
	var c OAuthClient
	var urisJSON string

	err := db.conn.QueryRow(
		"SELECT client_id, client_name, redirect_uris, created_at, last_used_at FROM oauth_clients WHERE client_id = ?",
		clientID,
	).Scan(&c.ClientID, &c.ClientName, &urisJSON, &c.CreatedAt, &c.LastUsedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(urisJSON), &c.RedirectURIs); err != nil {
		return nil, err
	}

	return &c, nil
}

// TouchClient updates the last_used_at timestamp.
func (db *DB) TouchClient(clientID string) error {
	_, err := db.conn.Exec(
		"UPDATE oauth_clients SET last_used_at = ? WHERE client_id = ?",
		time.Now().Unix(), clientID,
	)
	return err
}

// DeleteStaleClients removes clients unused for more than 90 days.
func (db *DB) DeleteStaleClients() error {
	staleTime := time.Now().Unix() - 90*24*60*60
	_, err := db.conn.Exec("DELETE FROM oauth_clients WHERE last_used_at < ?", staleTime)
	return err
}
