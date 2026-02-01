package storage

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Storage defines the interface for data persistence.
// Implementations must be safe for concurrent use.
type Storage interface {
	ChunkStore
	EmbeddingStore
	TokenStore
	ClientStore
	SettingsStore

	// Close closes the storage connection.
	Close() error
}

// ChunkStore handles chunk operations.
type ChunkStore interface {
	CreateChunk(content string, metadata json.RawMessage) (*Chunk, error)
	GetChunk(id string) (*Chunk, error)
	GetAllChunks() ([]Chunk, error)
	UpdateChunk(id string, content *string, metadata json.RawMessage) (*Chunk, error)
	DeleteChunk(id string) (bool, error)
	SearchChunks(query string, limit int) ([]SearchResult, error)
	GetMetadataIndex(topN int) (map[string]any, error)
	GetMetadataValues(key string, topN int) (map[string]any, error)
}

// EmbeddingStore handles embedding operations.
type EmbeddingStore interface {
	SaveEmbedding(chunkID, model string, vec []float32) error
	GetEmbedding(chunkID string) ([]float32, error)
	DeleteEmbedding(chunkID string) error
	LoadEmbeddingsByModel(model string) (map[string][]float32, error)
	GetChunksWithoutEmbeddings(model string) ([]Chunk, error)
}

// TokenStore handles OAuth token operations.
type TokenStore interface {
	StoreToken(hash string, typ TokenType, clientID string, expiresAt int64, data map[string]string) error
	ValidateToken(hash string, typ TokenType) (*Token, error)
	ConsumeToken(hash string, typ TokenType) (*Token, error)
	DeleteToken(hash string) error
}

// ClientStore handles OAuth client operations.
type ClientStore interface {
	CreateClient(clientID, clientName string, redirectURIs []string) error
	GetClient(clientID string) (*OAuthClient, error)
	TouchClient(clientID string) error
	DeleteStaleClients() error
}

// SettingsStore handles application settings.
type SettingsStore interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
	GetPasswordHash() (string, error)
	SetPasswordHash(hash string) error
}

// TxStorage extends Storage with transaction support.
type TxStorage interface {
	Storage

	// BeginTx starts a new transaction.
	BeginTx(ctx context.Context) (Tx, error)
}

// Tx represents a database transaction.
// Only includes methods needed for atomic chunk+embedding creation.
type Tx interface {
	// CreateChunk creates a new chunk within the transaction.
	CreateChunk(content string, metadata json.RawMessage) (*Chunk, error)

	// UpdateChunk updates a chunk within the transaction.
	UpdateChunk(id string, content *string, metadata json.RawMessage) (*Chunk, error)

	// SaveEmbedding saves an embedding within the transaction.
	SaveEmbedding(chunkID, model string, vec []float32) error

	// Commit commits the transaction.
	Commit() error

	// Rollback aborts the transaction.
	Rollback() error
}

// Verify DB implements TxStorage at compile time.
var _ TxStorage = (*DB)(nil)

// sqlExecutor abstracts sql.DB and sql.Tx for shared query execution.
type sqlExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// txWrapper wraps sql.Tx to implement Tx interface.
type txWrapper struct {
	tx *sql.Tx
}

// BeginTx starts a new transaction.
func (db *DB) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &txWrapper{tx: tx}, nil
}

func (t *txWrapper) Commit() error {
	return t.tx.Commit()
}

func (t *txWrapper) Rollback() error {
	return t.tx.Rollback()
}

func (t *txWrapper) CreateChunk(content string, metadata json.RawMessage) (*Chunk, error) {
	return createChunk(t.tx, content, metadata)
}

func (t *txWrapper) UpdateChunk(id string, content *string, metadata json.RawMessage) (*Chunk, error) {
	return updateChunk(t.tx, id, content, metadata)
}

func (t *txWrapper) SaveEmbedding(chunkID, model string, vec []float32) error {
	return saveEmbedding(t.tx, chunkID, model, vec)
}
