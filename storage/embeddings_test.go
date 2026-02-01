package storage

import (
	"path/filepath"
	"testing"
)

func setupEmbeddingsTestDB(t *testing.T) *DB {
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

func TestSaveAndGetEmbedding(t *testing.T) {
	db := setupEmbeddingsTestDB(t)

	// Create a chunk first
	chunk, err := db.CreateChunk("test content", nil)
	if err != nil {
		t.Fatalf("CreateChunk: %v", err)
	}

	// Save embedding
	vec := []float32{0.1, 0.2, 0.3, 0.4}
	err = db.SaveEmbedding(chunk.ID, "openai/text-embedding-3-small", vec)
	if err != nil {
		t.Fatalf("SaveEmbedding: %v", err)
	}

	// Get embedding
	got, err := db.GetEmbedding(chunk.ID)
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}

	if len(got) != len(vec) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(vec))
	}
	for i := range vec {
		if got[i] != vec[i] {
			t.Errorf("got[%d] = %f, want %f", i, got[i], vec[i])
		}
	}
}

func TestSaveEmbeddingUpsert(t *testing.T) {
	db := setupEmbeddingsTestDB(t)

	chunk, _ := db.CreateChunk("test", nil)

	// Save first embedding
	vec1 := []float32{0.1, 0.2}
	db.SaveEmbedding(chunk.ID, "model1", vec1)

	// Upsert with new embedding
	vec2 := []float32{0.3, 0.4}
	err := db.SaveEmbedding(chunk.ID, "model2", vec2)
	if err != nil {
		t.Fatalf("SaveEmbedding upsert: %v", err)
	}

	// Should get new embedding
	got, _ := db.GetEmbedding(chunk.ID)
	if got[0] != vec2[0] || got[1] != vec2[1] {
		t.Errorf("got = %v, want %v", got, vec2)
	}
}

func TestDeleteEmbedding(t *testing.T) {
	db := setupEmbeddingsTestDB(t)

	chunk, _ := db.CreateChunk("test", nil)
	db.SaveEmbedding(chunk.ID, "model", []float32{0.1, 0.2})

	err := db.DeleteEmbedding(chunk.ID)
	if err != nil {
		t.Fatalf("DeleteEmbedding: %v", err)
	}

	_, err = db.GetEmbedding(chunk.ID)
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestLoadEmbeddingsByModel(t *testing.T) {
	db := setupEmbeddingsTestDB(t)

	// Create chunks and embeddings with different models
	c1, _ := db.CreateChunk("one", nil)
	c2, _ := db.CreateChunk("two", nil)
	c3, _ := db.CreateChunk("three", nil)

	db.SaveEmbedding(c1.ID, "openai/text-embedding-3-small", []float32{0.1, 0.2})
	db.SaveEmbedding(c2.ID, "openai/text-embedding-3-small", []float32{0.3, 0.4})
	db.SaveEmbedding(c3.ID, "ollama/nomic-embed-text", []float32{0.5, 0.6})

	// Load only OpenAI embeddings
	vecs, err := db.LoadEmbeddingsByModel("openai/text-embedding-3-small")
	if err != nil {
		t.Fatalf("LoadEmbeddingsByModel: %v", err)
	}

	if len(vecs) != 2 {
		t.Errorf("len(vecs) = %d, want 2", len(vecs))
	}

	if _, ok := vecs[c1.ID]; !ok {
		t.Error("Missing embedding for c1")
	}
	if _, ok := vecs[c2.ID]; !ok {
		t.Error("Missing embedding for c2")
	}
	if _, ok := vecs[c3.ID]; ok {
		t.Error("c3 should not be included (different model)")
	}

	// Load Ollama embeddings
	vecs2, _ := db.LoadEmbeddingsByModel("ollama/nomic-embed-text")
	if len(vecs2) != 1 {
		t.Errorf("len(vecs2) = %d, want 1", len(vecs2))
	}
}

func TestLoadEmbeddingsByModelEmpty(t *testing.T) {
	db := setupEmbeddingsTestDB(t)

	vecs, err := db.LoadEmbeddingsByModel("nonexistent/model")
	if err != nil {
		t.Fatalf("LoadEmbeddingsByModel: %v", err)
	}

	if len(vecs) != 0 {
		t.Errorf("len(vecs) = %d, want 0", len(vecs))
	}
}

func TestGetChunksWithoutEmbeddings(t *testing.T) {
	db := setupEmbeddingsTestDB(t)

	c1, _ := db.CreateChunk("has embedding", nil)
	c2, _ := db.CreateChunk("no embedding", nil)
	c3, _ := db.CreateChunk("also no embedding", nil)

	db.SaveEmbedding(c1.ID, "openai/model", []float32{0.1})

	chunks, err := db.GetChunksWithoutEmbeddings("openai/model")
	if err != nil {
		t.Fatalf("GetChunksWithoutEmbeddings: %v", err)
	}

	if len(chunks) != 2 {
		t.Errorf("len(chunks) = %d, want 2", len(chunks))
	}

	ids := make(map[string]bool)
	for _, c := range chunks {
		ids[c.ID] = true
	}

	if ids[c1.ID] {
		t.Error("c1 should not be in result (has embedding)")
	}
	if !ids[c2.ID] {
		t.Error("c2 should be in result")
	}
	if !ids[c3.ID] {
		t.Error("c3 should be in result")
	}
}

func TestGetChunksWithoutEmbeddingsAllHave(t *testing.T) {
	db := setupEmbeddingsTestDB(t)

	c1, _ := db.CreateChunk("one", nil)
	c2, _ := db.CreateChunk("two", nil)

	db.SaveEmbedding(c1.ID, "openai/model", []float32{0.1})
	db.SaveEmbedding(c2.ID, "openai/model", []float32{0.2})

	chunks, err := db.GetChunksWithoutEmbeddings("openai/model")
	if err != nil {
		t.Fatalf("GetChunksWithoutEmbeddings: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("len(chunks) = %d, want 0", len(chunks))
	}
}

func TestGetChunksWithoutEmbeddingsDifferentModel(t *testing.T) {
	db := setupEmbeddingsTestDB(t)

	c1, _ := db.CreateChunk("has openai embedding", nil)
	c2, _ := db.CreateChunk("has ollama embedding", nil)
	c3, _ := db.CreateChunk("no embedding", nil)

	db.SaveEmbedding(c1.ID, "openai/text-embedding-3-small", []float32{0.1})
	db.SaveEmbedding(c2.ID, "ollama/nomic-embed-text", []float32{0.2})

	// When querying for openai model, c2 should be included (has wrong model)
	chunks, err := db.GetChunksWithoutEmbeddings("openai/text-embedding-3-small")
	if err != nil {
		t.Fatalf("GetChunksWithoutEmbeddings: %v", err)
	}

	if len(chunks) != 2 {
		t.Errorf("len(chunks) = %d, want 2 (c2 has different model, c3 has none)", len(chunks))
	}

	ids := make(map[string]bool)
	for _, c := range chunks {
		ids[c.ID] = true
	}

	if ids[c1.ID] {
		t.Error("c1 should not be in result (has correct model)")
	}
	if !ids[c2.ID] {
		t.Error("c2 should be in result (has different model)")
	}
	if !ids[c3.ID] {
		t.Error("c3 should be in result (no embedding)")
	}
}

func TestCascadeDeleteEmbedding(t *testing.T) {
	db := setupEmbeddingsTestDB(t)

	chunk, _ := db.CreateChunk("test", nil)
	db.SaveEmbedding(chunk.ID, "model", []float32{0.1, 0.2})

	// Delete chunk should cascade to embedding
	db.DeleteChunk(chunk.ID)

	_, err := db.GetEmbedding(chunk.ID)
	if err == nil {
		t.Error("Embedding should be deleted with chunk")
	}
}

func TestFloat32BytesRoundtrip(t *testing.T) {
	original := []float32{0.0, 1.0, -1.0, 0.123456, 3.14159, -999.999}

	bytes := float32ToBytes(original)
	recovered := bytesToFloat32(bytes)

	if len(recovered) != len(original) {
		t.Fatalf("len(recovered) = %d, want %d", len(recovered), len(original))
	}

	for i := range original {
		if recovered[i] != original[i] {
			t.Errorf("recovered[%d] = %f, want %f", i, recovered[i], original[i])
		}
	}
}
