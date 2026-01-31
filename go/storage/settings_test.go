package storage

import "testing"

func TestSetAndGetSetting(t *testing.T) {
	db := setupTestDB(t)

	if err := db.SetSetting("foo", "bar"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	got, err := db.GetSetting("foo")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if got != "bar" {
		t.Errorf("got %q, want %q", got, "bar")
	}
}

func TestGetSettingNotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.GetSetting("nonexistent")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSetSettingOverwrite(t *testing.T) {
	db := setupTestDB(t)

	db.SetSetting("key", "value1")
	db.SetSetting("key", "value2")

	got, _ := db.GetSetting("key")
	if got != "value2" {
		t.Errorf("got %q, want %q", got, "value2")
	}
}

func TestPasswordHash(t *testing.T) {
	db := setupTestDB(t)

	// Not set initially
	_, err := db.GetPasswordHash()
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}

	// Set it
	if err := db.SetPasswordHash("$2a$10$hash"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	// Get it back
	hash, err := db.GetPasswordHash()
	if err != nil {
		t.Fatalf("GetPasswordHash: %v", err)
	}
	if hash != "$2a$10$hash" {
		t.Errorf("hash = %q, want %q", hash, "$2a$10$hash")
	}
}
