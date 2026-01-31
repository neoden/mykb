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

var version = "dev"

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.SetOutput(os.Stderr)

	// Handle global flags first
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "-h", "--help":
			usage()
			return
		case "-v", "--version":
			fmt.Println(version)
			return
		}
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: mykb serve <stdio|http>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "stdio":
			serveStdio(os.Args[3:])
		case "http":
			serveHTTP(os.Args[3:])
		default:
			fmt.Fprintf(os.Stderr, "Unknown serve mode: %s\n", os.Args[2])
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `mykb - Personal knowledge base with full-text search

Usage:
  mykb serve stdio [--data DIR]                  Run MCP server over stdio
  mykb serve http  [--data DIR] [--listen ADDR]  Run HTTP server (TODO)

Options:
  -h, --help       Show this help
  -v, --version    Print version`)
}

func serveStdio(args []string) {
	fs := flag.NewFlagSet("serve stdio", flag.ExitOnError)
	dataDir := fs.String("data", defaultDataDir(), "Data directory for database")
	fs.Parse(args)

	db := openDB(*dataDir)
	defer db.Close()

	server := mcp.NewServer(db)
	if err := server.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func serveHTTP(args []string) {
	fs := flag.NewFlagSet("serve http", flag.ExitOnError)
	dataDir := fs.String("data", defaultDataDir(), "Data directory for database")
	listen := fs.String("listen", ":8080", "Listen address")
	fs.Parse(args)

	_ = dataDir
	_ = listen

	fmt.Fprintln(os.Stderr, "HTTP server not implemented yet")
	os.Exit(1)
}

func openDB(dataDir string) *storage.DB {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
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

func defaultDataDir() string {
	if dir := os.Getenv("MYKB_DATA"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "mykb")
}
