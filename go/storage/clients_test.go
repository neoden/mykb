package storage

import (
	"testing"
	"time"
)

func TestCreateAndGetClient(t *testing.T) {
	db := setupTestDB(t)

	err := db.CreateClient("client-id", "My App", []string{"http://localhost/callback"})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	client, err := db.GetClient("client-id")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}

	if client.ClientID != "client-id" {
		t.Errorf("ClientID = %q, want %q", client.ClientID, "client-id")
	}
	if client.ClientName != "My App" {
		t.Errorf("ClientName = %q, want %q", client.ClientName, "My App")
	}
	if len(client.RedirectURIs) != 1 || client.RedirectURIs[0] != "http://localhost/callback" {
		t.Errorf("RedirectURIs = %v, want [http://localhost/callback]", client.RedirectURIs)
	}
}

func TestGetClientNotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.GetClient("nonexistent")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateClientMultipleRedirectURIs(t *testing.T) {
	db := setupTestDB(t)

	uris := []string{
		"http://localhost:8080/callback",
		"http://localhost:3000/oauth",
		"myapp://callback",
	}
	db.CreateClient("multi-uri", "App", uris)

	client, _ := db.GetClient("multi-uri")
	if len(client.RedirectURIs) != 3 {
		t.Errorf("len(RedirectURIs) = %d, want 3", len(client.RedirectURIs))
	}
}

func TestTouchClient(t *testing.T) {
	db := setupTestDB(t)

	db.CreateClient("touch-me", "App", []string{"http://localhost"})

	// Manually set last_used_at to past
	past := time.Now().Unix() - 100
	db.conn.Exec("UPDATE oauth_clients SET last_used_at = ? WHERE client_id = ?", past, "touch-me")

	if err := db.TouchClient("touch-me"); err != nil {
		t.Fatalf("TouchClient: %v", err)
	}

	client, _ := db.GetClient("touch-me")
	if client.LastUsedAt <= past {
		t.Errorf("LastUsedAt = %d, should be > %d", client.LastUsedAt, past)
	}
}

func TestDeleteStaleClients(t *testing.T) {
	db := setupTestDB(t)

	// Create client
	db.CreateClient("stale-client", "Old App", []string{"http://localhost"})

	// Manually set last_used_at to 100 days ago
	db.conn.Exec(
		"UPDATE oauth_clients SET last_used_at = ? WHERE client_id = ?",
		time.Now().Unix()-100*24*60*60,
		"stale-client",
	)

	// Create fresh client
	db.CreateClient("fresh-client", "New App", []string{"http://localhost"})

	// Delete stale
	if err := db.DeleteStaleClients(); err != nil {
		t.Fatalf("DeleteStaleClients: %v", err)
	}

	// Stale should be gone
	_, err := db.GetClient("stale-client")
	if err != ErrNotFound {
		t.Error("Stale client should be deleted")
	}

	// Fresh should remain
	_, err = db.GetClient("fresh-client")
	if err != nil {
		t.Error("Fresh client should remain")
	}
}
