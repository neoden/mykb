package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/neoden/mykb/httpd"
	"github.com/neoden/mykb/mcp"
	"github.com/neoden/mykb/storage"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var dataDir string

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.SetOutput(os.Stderr)

	// Global flags
	flag.StringVar(&dataDir, "data", defaultDataDir(), "Data directory")
	flag.Usage = usage
	flag.Parse()

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
			serveHTTP(args[2:])
		default:
			fmt.Fprintf(os.Stderr, "Unknown serve mode: %s\n", args[1])
			os.Exit(1)
		}
	case "set-password":
		setPassword()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `mykb - Personal knowledge base with full-text search

Usage:
  mykb serve stdio                  Run MCP server over stdio
  mykb serve http [--listen PORT]    Run HTTP on localhost (dev, default :8080)
  mykb serve http --domain DOMAIN   Run HTTPS with auto TLS (production)
  mykb set-password                 Set password for auth

Options:
  --data DIR       Data directory (default: ~/.local/share/mykb on Linux/macOS,
                   %%LOCALAPPDATA%%\mykb on Windows; env: MYKB_DATA)

HTTP transport options (mutually exclusive):
  --listen PORT    HTTP on localhost only (env: MYKB_LISTEN)
  --domain DOMAIN  HTTPS with Let's Encrypt (env: MYKB_DOMAIN)
  --behind-proxy   Trust X-Forwarded-For header (env: MYKB_BEHIND_PROXY=1)`)
}

func serveStdio() {
	db := openDB(dataDir)
	defer db.Close()

	server := mcp.NewServer(db)
	if err := server.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func serveHTTP(args []string) {
	fs := flag.NewFlagSet("serve http", flag.ExitOnError)
	listen := fs.String("listen", os.Getenv("MYKB_LISTEN"), "Listen address for HTTP (localhost only, dev mode)")
	domain := fs.String("domain", os.Getenv("MYKB_DOMAIN"), "Domain for HTTPS with auto TLS (production)")
	behindProxy := fs.Bool("behind-proxy", os.Getenv("MYKB_BEHIND_PROXY") == "1", "Trust X-Forwarded-For for client IP")
	fs.Parse(args)

	if *listen != "" && *domain != "" {
		fmt.Fprintln(os.Stderr, "Error: --listen and --domain are mutually exclusive")
		os.Exit(1)
	}
	if *listen == "" && *domain == "" {
		*listen = ":8080"
	}

	db := openDB(dataDir)
	defer db.Close()

	// Check password is set
	if _, err := db.GetPasswordHash(); err != nil {
		fmt.Fprintln(os.Stderr, "Password not set. Run: mykb set-password")
		os.Exit(1)
	}

	config := httpd.DefaultConfig()
	config.Domain = *domain
	config.CertCache = filepath.Join(dataDir, "certs")
	config.BehindProxy = *behindProxy

	if *domain != "" {
		config.BaseURL = "https://" + *domain
		log.Printf("Starting HTTPS server for %s", *domain)
	} else {
		// HTTP mode: force localhost only (no TLS = no public exposure)
		config.Listen, config.BaseURL = httpd.LocalhostAddr(*listen)
		log.Printf("Starting HTTP server on %s (dev mode)", config.Listen)
	}

	server := httpd.NewServer(db, config)
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func setPassword() {
	db := openDB(dataDir)
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

func defaultDataDir() string {
	if dir := os.Getenv("MYKB_DATA"); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "mykb")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "mykb")
}
