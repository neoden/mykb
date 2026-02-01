package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"syscall"

	"github.com/neoden/mykb/config"
	"github.com/neoden/mykb/embedding"
	"github.com/neoden/mykb/httpd"
	"github.com/neoden/mykb/mcp"
	"github.com/neoden/mykb/storage"
	"github.com/neoden/mykb/vector"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

// App holds initialized application components.
type App struct {
	Config   *config.Config
	DB       *storage.DB
	Embedder embedding.EmbeddingProvider
	Index    *vector.Index
	MCP      *mcp.Server
}

// New creates and initializes all application components.
func New(cfg *config.Config) (*App, error) {
	db, err := storage.Init(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	log.Printf("Database ready: %s", cfg.DataDir)

	embedder, err := embedding.New(cfg.Embedding)
	if err != nil {
		log.Printf("Embedding not configured: %v", err)
		// Not fatal - embedder is optional
	}

	index := loadVectorIndex(db, embedder)
	mcpServer := mcp.NewServer(db, embedder, index)

	return &App{
		Config:   cfg,
		DB:       db,
		Embedder: embedder,
		Index:    index,
		MCP:      mcpServer,
	}, nil
}

// Close releases all resources.
func (a *App) Close() error {
	return a.DB.Close()
}

// ServeStdio runs the MCP server over stdio.
func (a *App) ServeStdio() error {
	return a.MCP.ServeStdio()
}

// ServeHTTP runs the HTTP server.
func (a *App) ServeHTTP() error {
	listen := a.Config.Server.Listen
	domain := a.Config.Server.Domain

	if listen != "" && domain != "" {
		return fmt.Errorf("listen and domain are mutually exclusive in config")
	}
	if listen == "" && domain == "" {
		listen = ":8080"
	}

	// Check password is set
	if _, err := a.DB.GetPasswordHash(); err != nil {
		return fmt.Errorf("password not set; run: mykb set-password")
	}

	httpConfig := httpd.DefaultConfig()
	httpConfig.Domain = domain
	httpConfig.CertCache = filepath.Join(a.Config.DataDir, "certs")
	httpConfig.BehindProxy = a.Config.Server.BehindProxy

	if domain != "" {
		httpConfig.BaseURL = "https://" + domain
		log.Printf("Starting HTTPS server for %s", domain)
	} else {
		httpConfig.Listen, httpConfig.BaseURL = httpd.LocalhostAddr(listen)
		log.Printf("Starting HTTP server on %s (dev mode)", httpConfig.Listen)
	}

	server := httpd.NewServer(a.DB, a.MCP, httpConfig)
	return server.ListenAndServe()
}

// SetPassword prompts for and sets the authentication password.
func (a *App) SetPassword() error {
	fmt.Print("Enter password: ")
	password1, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	fmt.Print("Confirm password: ")
	password2, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	if string(password1) != string(password2) {
		return fmt.Errorf("passwords do not match")
	}

	if len(password1) == 0 {
		return fmt.Errorf("password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword(password1, bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := a.DB.SetPasswordHash(string(hash)); err != nil {
		return fmt.Errorf("save password: %w", err)
	}

	fmt.Println("Password updated.")
	return nil
}

// Reindex generates embeddings for chunks.
// If force is true, re-indexes all chunks. Otherwise only chunks without embeddings.
func (a *App) Reindex(ctx context.Context, force bool) error {
	if a.Embedder == nil {
		return fmt.Errorf("embedding provider not configured")
	}

	var chunks []storage.Chunk
	var err error

	if force {
		chunks, err = a.DB.GetAllChunks()
		if err != nil {
			return fmt.Errorf("get chunks: %w", err)
		}
		if len(chunks) == 0 {
			log.Println("No chunks to index")
			return nil
		}
		log.Printf("Re-indexing all %d chunks with %s", len(chunks), a.Embedder.Model())
	} else {
		chunks, err = a.DB.GetChunksWithoutEmbeddings(a.Embedder.Model())
		if err != nil {
			return fmt.Errorf("get chunks: %w", err)
		}
		if len(chunks) == 0 {
			log.Println("All chunks already have embeddings for this model")
			return nil
		}
		log.Printf("Indexing %d chunks with %s", len(chunks), a.Embedder.Model())
	}

	const batchSize = 100

	for i := 0; i < len(chunks); i += batchSize {
		end := min(i+batchSize, len(chunks))
		batch := chunks[i:end]

		texts := make([]string, len(batch))
		for j, chunk := range batch {
			texts[j] = chunk.Content
		}

		vecs, err := a.Embedder.Embed(ctx, texts)
		if err != nil {
			log.Printf("Error embedding batch %d-%d: %v", i+1, end, err)
			continue
		}

		saved := 0
		for j, chunk := range batch {
			if j >= len(vecs) || vecs[j] == nil {
				log.Printf("No embedding returned for chunk %s", chunk.ID)
				continue
			}
			if err := a.DB.SaveEmbedding(chunk.ID, a.Embedder.Model(), vecs[j]); err != nil {
				log.Printf("Error saving embedding for chunk %s: %v", chunk.ID, err)
				continue
			}
			saved++
		}
		log.Printf("[%d-%d/%d] Indexed %d chunks", i+1, end, len(chunks), saved)
	}

	log.Println("Done")
	return nil
}

func loadVectorIndex(db *storage.DB, embedder embedding.EmbeddingProvider) *vector.Index {
	idx := vector.NewIndex()
	if embedder == nil {
		return idx
	}
	model := embedder.Model()
	vecs, err := db.LoadEmbeddingsByModel(model)
	if err != nil {
		log.Printf("Failed to load embeddings: %v", err)
		return idx
	}
	idx.Load(vecs)
	log.Printf("Loaded %d embeddings for model %s", len(vecs), model)
	return idx
}
