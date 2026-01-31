package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateChunk(t *testing.T) {
	db := setupTestDB(t)

	chunk, err := db.CreateChunk("Hello world", nil)
	if err != nil {
		t.Fatalf("CreateChunk: %v", err)
	}

	if chunk.ID == "" {
		t.Error("Expected non-empty ID")
	}
	if chunk.Content != "Hello world" {
		t.Errorf("Content = %q, want %q", chunk.Content, "Hello world")
	}
	if chunk.CreatedAt.IsZero() {
		t.Error("Expected non-zero CreatedAt")
	}
}

func TestCreateChunkWithMetadata(t *testing.T) {
	db := setupTestDB(t)

	meta := json.RawMessage(`{"tags":["test","go"],"priority":1}`)
	chunk, err := db.CreateChunk("Test content", meta)
	if err != nil {
		t.Fatalf("CreateChunk: %v", err)
	}

	// Verify metadata is stored
	got, err := db.GetChunk(chunk.ID)
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}

	var gotMeta map[string]interface{}
	if err := json.Unmarshal(got.Metadata, &gotMeta); err != nil {
		t.Fatalf("Unmarshal metadata: %v", err)
	}

	if gotMeta["priority"].(float64) != 1 {
		t.Errorf("priority = %v, want 1", gotMeta["priority"])
	}
}

func TestGetChunk(t *testing.T) {
	db := setupTestDB(t)

	created, _ := db.CreateChunk("Test", nil)

	got, err := db.GetChunk(created.ID)
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	if got.Content != "Test" {
		t.Errorf("Content = %q, want %q", got.Content, "Test")
	}
}

func TestGetChunkNotFound(t *testing.T) {
	db := setupTestDB(t)

	got, err := db.GetChunk("nonexistent-id")
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	if got != nil {
		t.Error("Expected nil for nonexistent chunk")
	}
}

func TestUpdateChunk(t *testing.T) {
	db := setupTestDB(t)

	created, _ := db.CreateChunk("Original", nil)

	newContent := "Updated"
	updated, err := db.UpdateChunk(created.ID, &newContent, nil)
	if err != nil {
		t.Fatalf("UpdateChunk: %v", err)
	}

	if updated.Content != "Updated" {
		t.Errorf("Content = %q, want %q", updated.Content, "Updated")
	}
	if !updated.UpdatedAt.After(created.CreatedAt) {
		t.Error("Expected UpdatedAt to be after CreatedAt")
	}
}

func TestUpdateChunkMetadataOnly(t *testing.T) {
	db := setupTestDB(t)

	created, _ := db.CreateChunk("Content", json.RawMessage(`{"v":1}`))

	newMeta := json.RawMessage(`{"v":2}`)
	updated, err := db.UpdateChunk(created.ID, nil, newMeta)
	if err != nil {
		t.Fatalf("UpdateChunk: %v", err)
	}

	if updated.Content != "Content" {
		t.Error("Content should not change")
	}

	var meta map[string]interface{}
	json.Unmarshal(updated.Metadata, &meta)
	if meta["v"].(float64) != 2 {
		t.Errorf("metadata v = %v, want 2", meta["v"])
	}
}

func TestUpdateChunkNotFound(t *testing.T) {
	db := setupTestDB(t)

	content := "test"
	updated, err := db.UpdateChunk("nonexistent", &content, nil)
	if err != nil {
		t.Fatalf("UpdateChunk: %v", err)
	}
	if updated != nil {
		t.Error("Expected nil for nonexistent chunk")
	}
}

func TestDeleteChunk(t *testing.T) {
	db := setupTestDB(t)

	created, _ := db.CreateChunk("To delete", nil)

	deleted, err := db.DeleteChunk(created.ID)
	if err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}
	if !deleted {
		t.Error("Expected deleted=true")
	}

	// Verify it's gone
	got, _ := db.GetChunk(created.ID)
	if got != nil {
		t.Error("Chunk should be deleted")
	}
}

func TestDeleteChunkNotFound(t *testing.T) {
	db := setupTestDB(t)

	deleted, err := db.DeleteChunk("nonexistent")
	if err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}
	if deleted {
		t.Error("Expected deleted=false for nonexistent chunk")
	}
}

func TestSearchChunks(t *testing.T) {
	db := setupTestDB(t)

	db.CreateChunk("The quick brown fox jumps over the lazy dog", nil)
	db.CreateChunk("Hello world from Go", nil)
	db.CreateChunk("Another test document", nil)

	results, err := db.SearchChunks("fox", 10)
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Snippet == "" {
		t.Error("Expected non-empty snippet")
	}
}

func TestSearchChunksMultipleTerms(t *testing.T) {
	db := setupTestDB(t)

	db.CreateChunk("Go is a programming language", nil)
	db.CreateChunk("Python is also a programming language", nil)
	db.CreateChunk("Something completely different", nil)

	results, err := db.SearchChunks("programming language", 10)
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
}

func TestSearchChunksNoResults(t *testing.T) {
	db := setupTestDB(t)

	db.CreateChunk("Hello world", nil)

	results, err := db.SearchChunks("nonexistent", 10)
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestSearchChunksLimit(t *testing.T) {
	db := setupTestDB(t)

	for i := 0; i < 10; i++ {
		db.CreateChunk("test document number", nil)
	}

	results, err := db.SearchChunks("test", 3)
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
}

func TestSearchChunksMaxLimit(t *testing.T) {
	db := setupTestDB(t)

	// Create 105 chunks
	for i := 0; i < 105; i++ {
		db.CreateChunk("searchable content here", nil)
	}

	// Request more than max limit
	results, err := db.SearchChunks("searchable", 500)
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	// Should be capped at 100
	if len(results) != 100 {
		t.Errorf("len(results) = %d, want 100 (max limit)", len(results))
	}
}

func TestSearchChunksMetadata(t *testing.T) {
	db := setupTestDB(t)

	db.CreateChunk("Some content", json.RawMessage(`{"author":"john"}`))

	// FTS5 should index metadata too
	results, err := db.SearchChunks("john", 10)
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1 (metadata should be searchable)", len(results))
	}
}

func TestGetMetadataIndex(t *testing.T) {
	db := setupTestDB(t)

	db.CreateChunk("A", json.RawMessage(`{"type":"note","tags":["go","test"]}`))
	db.CreateChunk("B", json.RawMessage(`{"type":"note","tags":["go"]}`))
	db.CreateChunk("C", json.RawMessage(`{"type":"doc"}`))

	result, err := db.GetMetadataIndex(20)
	if err != nil {
		t.Fatalf("GetMetadataIndex: %v", err)
	}

	total := result["total_chunks"].(int)
	if total != 3 {
		t.Errorf("total_chunks = %d, want 3", total)
	}

	keys := result["keys"].(map[string]map[string]int)

	// Check type counts
	if keys["type"]["note"] != 2 {
		t.Errorf("type.note = %d, want 2", keys["type"]["note"])
	}
	if keys["type"]["doc"] != 1 {
		t.Errorf("type.doc = %d, want 1", keys["type"]["doc"])
	}

	// Check tags (array values should be expanded)
	if keys["tags"]["go"] != 2 {
		t.Errorf("tags.go = %d, want 2", keys["tags"]["go"])
	}
	if keys["tags"]["test"] != 1 {
		t.Errorf("tags.test = %d, want 1", keys["tags"]["test"])
	}
}

func TestGetMetadataValues(t *testing.T) {
	db := setupTestDB(t)

	db.CreateChunk("A", json.RawMessage(`{"lang":"go"}`))
	db.CreateChunk("B", json.RawMessage(`{"lang":"go"}`))
	db.CreateChunk("C", json.RawMessage(`{"lang":"python"}`))

	result, err := db.GetMetadataValues("lang", 50)
	if err != nil {
		t.Fatalf("GetMetadataValues: %v", err)
	}

	if result["key"] != "lang" {
		t.Errorf("key = %v, want lang", result["key"])
	}

	values := result["values"].(map[string]int)
	if values["go"] != 2 {
		t.Errorf("values.go = %d, want 2", values["go"])
	}
	if values["python"] != 1 {
		t.Errorf("values.python = %d, want 1", values["python"])
	}
}

func TestDatabasePersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")

	// Create and write
	db1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db1.Migrate()
	created, _ := db1.CreateChunk("Persistent data", nil)
	db1.Close()

	// Reopen and read
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer db2.Close()

	got, err := db2.GetChunk(created.ID)
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	if got == nil || got.Content != "Persistent data" {
		t.Error("Data should persist across reopens")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
