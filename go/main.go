package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/neoden/mykb/mcp"
	"github.com/neoden/mykb/storage"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.SetOutput(os.Stderr) // Keep stdout clean for MCP

	// CLI flags
	dataDir := flag.String("data", ".", "Data directory for database")
	flag.Parse()

	// Open database
	dbPath := filepath.Join(*dataDir, "data.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	log.Printf("Database ready: %s", dbPath)

	// Run MCP server over stdio
	server := mcp.NewServer(db)
	if err := server.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
