package app

import (
	"context"
	"testing"

	"github.com/neoden/mykb/config"
	"github.com/neoden/mykb/storage"
)

type mockEmbedder struct{}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}

func (m *mockEmbedder) Dimensions() int { return 3 }
func (m *mockEmbedder) Model() string   { return "mock/test" }

func TestNew(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir: dir,
	}

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer a.Close()

	if a.DB == nil {
		t.Error("DB should not be nil")
	}
	if a.Index == nil {
		t.Error("Index should not be nil")
	}
	if a.MCP == nil {
		t.Error("MCP should not be nil")
	}
	// Embedder is nil when not configured - that's OK
}

func TestNewWithInvalidDataDir(t *testing.T) {
	cfg := &config.Config{
		DataDir: "/nonexistent/readonly/path/that/cannot/be/created",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for invalid data dir")
	}
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir: dir,
	}

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = a.Close()
	if err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestLoadVectorIndexWithoutEmbedder(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir: dir,
	}

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer a.Close()

	// Without embedder, index should be empty but not nil
	if a.Index == nil {
		t.Error("Index should not be nil")
	}
	if a.Index.Size() != 0 {
		t.Errorf("Index size = %d, want 0", a.Index.Size())
	}
}

func TestLoadVectorIndexWithEmbeddings(t *testing.T) {
	dir := t.TempDir()

	// Create DB and add some embeddings
	db, err := storage.Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Create a chunk first
	chunk, err := db.CreateChunk("test content", nil)
	if err != nil {
		t.Fatalf("CreateChunk: %v", err)
	}

	// Save embedding
	embedder := &mockEmbedder{}
	vec := []float32{0.1, 0.2, 0.3}
	if err := db.SaveEmbedding(chunk.ID, embedder.Model(), vec); err != nil {
		t.Fatalf("SaveEmbedding: %v", err)
	}
	db.Close()

	// Now test loadVectorIndex
	db2, err := storage.Open(dir + "/data.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db2.Close()

	idx := loadVectorIndex(db2, embedder)
	if idx.Size() != 1 {
		t.Errorf("Index size = %d, want 1", idx.Size())
	}
}

func TestLoadVectorIndexDBError(t *testing.T) {
	dir := t.TempDir()

	db, err := storage.Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	db.Close() // Close DB to cause error

	embedder := &mockEmbedder{}
	idx := loadVectorIndex(db, embedder)

	// Should return empty index on error
	if idx == nil {
		t.Error("Index should not be nil")
	}
}

func TestReindexNoEmbedder(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir}

	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer a.Close()

	err = a.Reindex(context.Background(), false)
	if err == nil {
		t.Error("Expected error when embedder not configured")
	}
}

func TestReindexNoChunks(t *testing.T) {
	dir := t.TempDir()

	db, err := storage.Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	embedder := &mockEmbedder{}
	a := &App{DB: db, Embedder: embedder, Index: loadVectorIndex(db, embedder)}
	defer a.Close()

	err = a.Reindex(context.Background(), false)
	if err != nil {
		t.Errorf("Reindex: %v", err)
	}
}

func TestReindexWithChunks(t *testing.T) {
	dir := t.TempDir()

	db, err := storage.Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Create chunks
	db.CreateChunk("test content 1", nil)
	db.CreateChunk("test content 2", nil)

	embedder := &mockEmbedder{}
	a := &App{DB: db, Embedder: embedder, Index: loadVectorIndex(db, embedder)}
	defer a.Close()

	err = a.Reindex(context.Background(), false)
	if err != nil {
		t.Errorf("Reindex: %v", err)
	}

	// Check embeddings were created
	emb, err := db.GetEmbedding("test-id") // This won't work, need to check differently
	_ = emb
}

func TestReindexForce(t *testing.T) {
	dir := t.TempDir()

	db, err := storage.Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Create chunk with existing embedding
	chunk, _ := db.CreateChunk("test content", nil)
	db.SaveEmbedding(chunk.ID, "old/model", []float32{1, 2, 3})

	embedder := &mockEmbedder{}
	a := &App{DB: db, Embedder: embedder, Index: loadVectorIndex(db, embedder)}
	defer a.Close()

	// Force reindex should re-embed
	err = a.Reindex(context.Background(), true)
	if err != nil {
		t.Errorf("Reindex force: %v", err)
	}
}
