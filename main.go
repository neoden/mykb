package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
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

var (
	configPath string
	cfg        *config.Config
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.SetOutput(os.Stderr)

	// Global flags
	flag.StringVar(&configPath, "config", config.DefaultPath(), "Config file path")
	flag.Usage = usage
	flag.Parse()

	// Load config
	var err error
	cfg, err = config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	switch args[0] {
	case "serve":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: mykb serve <stdio|http>")
			os.Exit(1)
		}
		switch args[1] {
		case "stdio":
			serveStdio()
		case "http":
			serveHTTP()
		default:
			fmt.Fprintf(os.Stderr, "Unknown serve mode: %s\n", args[1])
			os.Exit(1)
		}
	case "set-password":
		setPassword()
	case "reindex":
		fs := flag.NewFlagSet("reindex", flag.ExitOnError)
		force := fs.Bool("force", false, "Re-index all chunks, replacing existing embeddings")
		fs.Parse(args[1:])
		reindex(*force)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `mykb - Personal knowledge base with full-text search

Usage:
  mykb serve stdio      Run MCP server over stdio
  mykb serve http       Run HTTP server
  mykb set-password     Set password for auth
  mykb reindex [--force]   Generate embeddings for chunks without them

Options:
  --config PATH    Config file (default: %s)
`, config.DefaultPath())
}

func serveStdio() {
	db := openDB(cfg.DataDir)
	defer db.Close()

	embedder := initEmbedder()
	index := initVectorIndex(db, embedderModel(embedder))

	server := mcp.NewServer(db, embedder, index)
	if err := server.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func serveHTTP() {
	listen := cfg.Server.Listen
	domain := cfg.Server.Domain

	if listen != "" && domain != "" {
		fmt.Fprintln(os.Stderr, "Error: listen and domain are mutually exclusive in config")
		os.Exit(1)
	}
	if listen == "" && domain == "" {
		listen = ":8080"
	}

	db := openDB(cfg.DataDir)
	defer db.Close()

	// Check password is set
	if _, err := db.GetPasswordHash(); err != nil {
		fmt.Fprintln(os.Stderr, "Password not set. Run: mykb set-password")
		os.Exit(1)
	}

	embedder := initEmbedder()
	index := initVectorIndex(db, embedderModel(embedder))
	mcpServer := mcp.NewServer(db, embedder, index)

	httpConfig := httpd.DefaultConfig()
	httpConfig.Domain = domain
	httpConfig.CertCache = filepath.Join(cfg.DataDir, "certs")
	httpConfig.BehindProxy = cfg.Server.BehindProxy

	if domain != "" {
		httpConfig.BaseURL = "https://" + domain
		log.Printf("Starting HTTPS server for %s", domain)
	} else {
		// HTTP mode: force localhost only (no TLS = no public exposure)
		httpConfig.Listen, httpConfig.BaseURL = httpd.LocalhostAddr(listen)
		log.Printf("Starting HTTP server on %s (dev mode)", httpConfig.Listen)
	}

	server := httpd.NewServer(db, mcpServer, httpConfig)
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func setPassword() {
	db := openDB(cfg.DataDir)
	defer db.Close()

	// Read password
	fmt.Print("Enter password: ")
	password1, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
		os.Exit(1)
	}

	// Confirm password
	fmt.Print("Confirm password: ")
	password2, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
		os.Exit(1)
	}

	// Check match
	if string(password1) != string(password2) {
		fmt.Fprintln(os.Stderr, "Passwords do not match")
		os.Exit(1)
	}

	if len(password1) == 0 {
		fmt.Fprintln(os.Stderr, "Password cannot be empty")
		os.Exit(1)
	}

	// Hash and store
	hash, err := bcrypt.GenerateFromPassword(password1, bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error hashing password: %v\n", err)
		os.Exit(1)
	}

	if err := db.SetPasswordHash(string(hash)); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving password: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Password updated.")
}

func openDB(dataDir string) *storage.DB {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	dbPath := filepath.Join(dataDir, "data.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	log.Printf("Database ready: %s", dbPath)
	return db
}

func reindex(force bool) {
	db := openDB(cfg.DataDir)
	defer db.Close()

	embedder := initEmbedder()
	if embedder == nil {
		log.Fatalf("Embedding provider not configured")
	}

	var chunks []storage.Chunk
	var err error

	if force {
		chunks, err = db.GetAllChunks()
		if err != nil {
			log.Fatalf("Failed to get chunks: %v", err)
		}
		if len(chunks) == 0 {
			log.Println("No chunks to index")
			return
		}
		log.Printf("Re-indexing all %d chunks with %s", len(chunks), embedder.Model())
	} else {
		chunks, err = db.GetChunksWithoutEmbeddings(embedder.Model())
		if err != nil {
			log.Fatalf("Failed to get chunks: %v", err)
		}
		if len(chunks) == 0 {
			log.Println("All chunks already have embeddings for this model")
			return
		}
		log.Printf("Indexing %d chunks with %s", len(chunks), embedder.Model())
	}

	const batchSize = 100

	for i := 0; i < len(chunks); i += batchSize {
		end := min(i+batchSize, len(chunks))
		batch := chunks[i:end]

		// Collect texts for batch embedding
		texts := make([]string, len(batch))
		for j, chunk := range batch {
			texts[j] = chunk.Content
		}

		vecs, err := embedder.Embed(context.Background(), texts)
		if err != nil {
			log.Printf("Error embedding batch %d-%d: %v", i+1, end, err)
			continue
		}

		// Save embeddings
		saved := 0
		for j, chunk := range batch {
			if j >= len(vecs) || vecs[j] == nil {
				log.Printf("No embedding returned for chunk %s", chunk.ID)
				continue
			}
			if err := db.SaveEmbedding(chunk.ID, embedder.Model(), vecs[j]); err != nil {
				log.Printf("Error saving embedding for chunk %s: %v", chunk.ID, err)
				continue
			}
			saved++
		}
		log.Printf("[%d-%d/%d] Indexed %d chunks", i+1, end, len(chunks), saved)
	}

	log.Println("Done")
}

func embedderModel(e embedding.EmbeddingProvider) string {
	if e == nil {
		return ""
	}
	return e.Model()
}

func initEmbedder() embedding.EmbeddingProvider {
	emb, err := embedding.New(cfg.Embedding)
	if err != nil {
		log.Printf("Embedding provider not configured: %v", err)
		return nil
	}
	log.Printf("Embedding provider: %s", emb.Model())
	return emb
}

func initVectorIndex(db *storage.DB, model string) *vector.Index {
	idx := vector.NewIndex()
	if model == "" {
		return idx
	}
	vecs, err := db.LoadEmbeddingsByModel(model)
	if err != nil {
		log.Printf("Failed to load embeddings: %v", err)
		return idx
	}
	idx.Load(vecs)
	log.Printf("Loaded %d embeddings for model %s", len(vecs), model)
	return idx
}
