package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Chunk represents a stored text chunk.
type Chunk struct {
	ID        string          `json:"id"`
	Content   string          `json:"content"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// SearchResult represents a search hit.
type SearchResult struct {
	ID       string          `json:"id"`
	Content  string          `json:"content"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Snippet  string          `json:"snippet"`
}

// CreateChunk creates a new chunk.
func (db *DB) CreateChunk(content string, metadata json.RawMessage) (*Chunk, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	var metaStr *string
	if len(metadata) > 0 {
		s := string(metadata)
		metaStr = &s
	}

	_, err := db.conn.Exec(`
		INSERT INTO chunks (id, content, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, content, metaStr, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert chunk: %w", err)
	}

	return &Chunk{
		ID:        id,
		Content:   content,
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// GetChunk retrieves a chunk by ID.
func (db *DB) GetChunk(id string) (*Chunk, error) {
	var chunk Chunk
	var metaStr sql.NullString

	err := db.conn.QueryRow(`
		SELECT id, content, metadata, created_at, updated_at
		FROM chunks WHERE id = ?
	`, id).Scan(&chunk.ID, &chunk.Content, &metaStr, &chunk.CreatedAt, &chunk.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get chunk: %w", err)
	}

	if metaStr.Valid {
		chunk.Metadata = json.RawMessage(metaStr.String)
	}

	return &chunk, nil
}

// GetAllChunks returns all chunks.
func (db *DB) GetAllChunks() ([]Chunk, error) {
	rows, err := db.conn.Query(`
		SELECT id, content, metadata, created_at, updated_at
		FROM chunks ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("get all chunks: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var chunk Chunk
		var metaStr sql.NullString
		if err := rows.Scan(&chunk.ID, &chunk.Content, &metaStr, &chunk.CreatedAt, &chunk.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		if metaStr.Valid {
			chunk.Metadata = json.RawMessage(metaStr.String)
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

// UpdateChunk updates an existing chunk.
func (db *DB) UpdateChunk(id string, content *string, metadata json.RawMessage) (*Chunk, error) {
	existing, err := db.GetChunk(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := time.Now().UTC()

	newContent := existing.Content
	if content != nil {
		newContent = *content
	}

	newMeta := existing.Metadata
	if metadata != nil {
		newMeta = metadata
	}

	var metaStr *string
	if len(newMeta) > 0 {
		s := string(newMeta)
		metaStr = &s
	}

	_, err = db.conn.Exec(`
		UPDATE chunks SET content = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`, newContent, metaStr, now, id)
	if err != nil {
		return nil, fmt.Errorf("update chunk: %w", err)
	}

	return &Chunk{
		ID:        id,
		Content:   newContent,
		Metadata:  newMeta,
		CreatedAt: existing.CreatedAt,
		UpdatedAt: now,
	}, nil
}

// DeleteChunk deletes a chunk by ID.
func (db *DB) DeleteChunk(id string) (bool, error) {
	result, err := db.conn.Exec("DELETE FROM chunks WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("delete chunk: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}

	return rows > 0, nil
}

// SearchChunks performs full-text search.
func (db *DB) SearchChunks(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	// Wildcard: return recent chunks
	if query == "*" {
		return db.listChunks(limit)
	}

	rows, err := db.conn.Query(`
		SELECT c.id,
		       CASE WHEN length(c.content) > 80
		            THEN substr(c.content, 1, 80) || '...'
		            ELSE c.content
		       END as content,
		       c.metadata,
		       snippet(chunks_fts, 1, '<mark>', '</mark>', '...', 32) as snippet
		FROM chunks_fts fts
		JOIN chunks c ON fts.id = c.id
		WHERE chunks_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var metaStr sql.NullString

		if err := rows.Scan(&r.ID, &r.Content, &metaStr, &r.Snippet); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}

		if metaStr.Valid {
			r.Metadata = json.RawMessage(metaStr.String)
		}

		results = append(results, r)
	}

	return results, rows.Err()
}

// GetMetadataIndex returns aggregated metadata keys with top values.
func (db *DB) GetMetadataIndex(topN int) (map[string]interface{}, error) {
	if topN <= 0 {
		topN = 20
	}

	// Get total count
	var total int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&total); err != nil {
		return nil, fmt.Errorf("count chunks: %w", err)
	}

	// Aggregate metadata
	rows, err := db.conn.Query(`
		SELECT key, val, SUM(count) as count FROM (
			SELECT j.key as key, j.value as val, COUNT(*) as count
			FROM chunks c, json_each(c.metadata) j
			WHERE c.metadata IS NOT NULL AND j.type != 'array'
			GROUP BY j.key, j.value

			UNION ALL

			SELECT j.key as key, je.value as val, COUNT(*) as count
			FROM chunks c, json_each(c.metadata) j, json_each(j.value) je
			WHERE c.metadata IS NOT NULL AND j.type = 'array'
			GROUP BY j.key, je.value
		)
		GROUP BY key, val
		ORDER BY key, count DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query metadata: %w", err)
	}
	defer rows.Close()

	keys := make(map[string]map[string]int)
	for rows.Next() {
		var key, val string
		var count int
		if err := rows.Scan(&key, &val, &count); err != nil {
			return nil, fmt.Errorf("scan metadata: %w", err)
		}

		if _, ok := keys[key]; !ok {
			keys[key] = make(map[string]int)
		}
		if len(keys[key]) < topN {
			keys[key][val] = count
		}
	}

	return map[string]interface{}{
		"total_chunks": total,
		"keys":         keys,
	}, rows.Err()
}

// GetMetadataValues returns all values for a specific metadata key.
func (db *DB) GetMetadataValues(key string, topN int) (map[string]interface{}, error) {
	if topN <= 0 {
		topN = 50
	}

	rows, err := db.conn.Query(`
		SELECT val, SUM(count) as count FROM (
			SELECT j.value as val, COUNT(*) as count
			FROM chunks c, json_each(c.metadata) j
			WHERE c.metadata IS NOT NULL AND j.type != 'array' AND j.key = ?
			GROUP BY j.value

			UNION ALL

			SELECT je.value as val, COUNT(*) as count
			FROM chunks c, json_each(c.metadata) j, json_each(j.value) je
			WHERE c.metadata IS NOT NULL AND j.type = 'array' AND j.key = ?
			GROUP BY je.value
		)
		GROUP BY val
		ORDER BY count DESC
		LIMIT ?
	`, key, key, topN)
	if err != nil {
		return nil, fmt.Errorf("query values: %w", err)
	}
	defer rows.Close()

	values := make(map[string]int)
	for rows.Next() {
		var val string
		var count int
		if err := rows.Scan(&val, &count); err != nil {
			return nil, fmt.Errorf("scan value: %w", err)
		}
		values[val] = count
	}

	return map[string]interface{}{
		"key":    key,
		"values": values,
	}, rows.Err()
}

// listChunks returns recent chunks (for wildcard query).
func (db *DB) listChunks(limit int) ([]SearchResult, error) {
	rows, err := db.conn.Query(`
		SELECT id,
		       CASE WHEN length(content) > 80
		            THEN substr(content, 1, 80) || '...'
		            ELSE content
		       END,
		       metadata
		FROM chunks
		ORDER BY updated_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list chunks: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var metaStr sql.NullString
		if err := rows.Scan(&r.ID, &r.Content, &metaStr); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		if metaStr.Valid {
			r.Metadata = json.RawMessage(metaStr.String)
		}
		r.Snippet = r.Content
		results = append(results, r)
	}
	return results, rows.Err()
}
