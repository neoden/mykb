package storage

import (
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	dir := t.TempDir()

	db, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer db.Close()

	// Should be able to use the database
	chunk, err := db.CreateChunk("test", nil)
	if err != nil {
		t.Fatalf("CreateChunk: %v", err)
	}
	if chunk.ID == "" {
		t.Error("Expected non-empty ID")
	}
}

func TestInitCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	db, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	db.Close()
}

func TestInitInvalidPath(t *testing.T) {
	// Try to create in a read-only location
	_, err := Init("/nonexistent/readonly/path/that/cannot/be/created")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestInitIdempotent(t *testing.T) {
	dir := t.TempDir()

	// First init
	db1, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	db1.CreateChunk("test", nil)
	db1.Close()

	// Second init should work and preserve data
	db2, err := Init(dir)
	if err != nil {
		t.Fatalf("Init second time: %v", err)
	}
	defer db2.Close()

	chunks, _ := db2.GetAllChunks()
	if len(chunks) != 1 {
		t.Errorf("len(chunks) = %d, want 1", len(chunks))
	}
}

func TestOpenInvalidPath(t *testing.T) {
	// Directory as file path
	dir := t.TempDir()
	_, err := Open(dir)
	if err == nil {
		t.Error("Expected error when opening directory as database")
	}
}
