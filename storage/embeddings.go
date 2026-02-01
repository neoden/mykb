package storage

import (
	"encoding/binary"
	"fmt"
	"math"
)

// SaveEmbedding saves an embedding for a chunk.
func (db *DB) SaveEmbedding(chunkID, model string, vec []float32) error {
	blob := float32ToBytes(vec)
	_, err := db.conn.Exec(`
		INSERT INTO embeddings (chunk_id, model, embedding)
		VALUES (?, ?, ?)
		ON CONFLICT(chunk_id) DO UPDATE SET
			model = excluded.model,
			embedding = excluded.embedding,
			created_at = unixepoch()
	`, chunkID, model, blob)
	if err != nil {
		return fmt.Errorf("save embedding: %w", err)
	}
	return nil
}

// GetEmbedding retrieves an embedding by chunk ID.
func (db *DB) GetEmbedding(chunkID string) ([]float32, error) {
	var blob []byte
	err := db.conn.QueryRow(`
		SELECT embedding FROM embeddings WHERE chunk_id = ?
	`, chunkID).Scan(&blob)
	if err != nil {
		return nil, fmt.Errorf("get embedding: %w", err)
	}
	return bytesToFloat32(blob), nil
}

// DeleteEmbedding deletes an embedding by chunk ID.
func (db *DB) DeleteEmbedding(chunkID string) error {
	_, err := db.conn.Exec(`DELETE FROM embeddings WHERE chunk_id = ?`, chunkID)
	if err != nil {
		return fmt.Errorf("delete embedding: %w", err)
	}
	return nil
}

// LoadEmbeddingsByModel loads embeddings for a specific model into a map.
// Only embeddings matching the given model are returned.
func (db *DB) LoadEmbeddingsByModel(model string) (map[string][]float32, error) {
	rows, err := db.conn.Query(`SELECT chunk_id, embedding FROM embeddings WHERE model = ?`, model)
	if err != nil {
		return nil, fmt.Errorf("load embeddings: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]float32)
	for rows.Next() {
		var chunkID string
		var blob []byte
		if err := rows.Scan(&chunkID, &blob); err != nil {
			return nil, fmt.Errorf("scan embedding: %w", err)
		}
		result[chunkID] = bytesToFloat32(blob)
	}
	return result, rows.Err()
}

// GetChunksWithoutEmbeddings returns chunks that don't have embeddings for the given model.
// This includes chunks with no embeddings at all and chunks with embeddings from a different model.
func (db *DB) GetChunksWithoutEmbeddings(model string) ([]Chunk, error) {
	rows, err := db.conn.Query(`
		SELECT c.id, c.content, c.metadata, c.created_at, c.updated_at
		FROM chunks c
		LEFT JOIN embeddings e ON c.id = e.chunk_id AND e.model = ?
		WHERE e.chunk_id IS NULL
	`, model)
	if err != nil {
		return nil, fmt.Errorf("get chunks without embeddings: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var chunk Chunk
		var metaStr *string
		if err := rows.Scan(&chunk.ID, &chunk.Content, &metaStr, &chunk.CreatedAt, &chunk.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		if metaStr != nil {
			chunk.Metadata = []byte(*metaStr)
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

// float32ToBytes converts a float32 slice to bytes (little-endian).
func float32ToBytes(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// bytesToFloat32 converts bytes to a float32 slice (little-endian).
func bytesToFloat32(buf []byte) []float32 {
	vec := make([]float32, len(buf)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return vec
}
