package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/neoden/mykb/app"
	"github.com/neoden/mykb/config"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.SetOutput(os.Stderr)

	var configPath string
	flag.StringVar(&configPath, "config", "", "Config file path")
	flag.Usage = usage
	flag.Parse()

	// Find config file
	if configPath == "" {
		for _, p := range config.SearchPaths() {
			if _, err := os.Stat(p); err == nil {
				configPath = p
				break
			}
		}
	}

	// Load config
	var cfg *config.Config
	if configPath != "" {
		var err error
		cfg, err = config.Load(configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
		cfg = config.Default()
	}

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	// Initialize app
	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	defer a.Close()

	switch args[0] {
	case "serve":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: mykb serve <stdio|http>")
			os.Exit(1)
		}
		switch args[1] {
		case "stdio":
			if err := a.ServeStdio(); err != nil {
				log.Fatalf("Server error: %v", err)
			}
		case "http":
			if err := a.ServeHTTP(); err != nil {
				log.Fatalf("Server error: %v", err)
			}
		default:
			fmt.Fprintf(os.Stderr, "Unknown serve mode: %s\n", args[1])
			os.Exit(1)
		}

	case "set-password":
		if err := a.SetPassword(); err != nil {
			log.Fatalf("Set password: %v", err)
		}

	case "reindex":
		fs := flag.NewFlagSet("reindex", flag.ExitOnError)
		force := fs.Bool("force", false, "Re-index all chunks, replacing existing embeddings")
		fs.Parse(args[1:])

		if err := a.Reindex(context.Background(), *force); err != nil {
			log.Fatalf("Reindex: %v", err)
		}

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
  --config PATH    Config file (searches: %s)
`, strings.Join(config.SearchPaths(), ", "))
}
