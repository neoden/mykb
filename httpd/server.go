package httpd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/neoden/mykb/mcp"
	"github.com/neoden/mykb/storage"
	"golang.org/x/crypto/acme/autocert"
)

// Config holds HTTP server configuration.
type Config struct {
	Listen      string // HTTP listen address (used when Domain is empty)
	Domain      string // Domain for HTTPS with auto TLS
	CertCache   string // Directory to cache TLS certificates
	BaseURL     string // Base URL for OAuth endpoints
	BehindProxy bool   // Trust X-Forwarded-For header for client IP

	TokenExpiry        time.Duration
	RefreshTokenExpiry time.Duration
	CodeExpiry         time.Duration
}

// DefaultConfig returns configuration with default values.
func DefaultConfig() *Config {
	return &Config{
		Listen:             ":8080",
		TokenExpiry:        time.Hour,
		RefreshTokenExpiry: 30 * 24 * time.Hour,
		CodeExpiry:         5 * time.Minute,
	}
}

// Server is the HTTP server.
type Server struct {
	db          *storage.DB
	mcp         *mcp.Server
	config      *Config
	rateLimiter *IPRateLimiter
	mux         *http.ServeMux
}

// NewServer creates a new HTTP server.
func NewServer(db *storage.DB, mcpServer *mcp.Server, config *Config) *Server {
	s := &Server{
		db:          db,
		mcp:         mcpServer,
		config:      config,
		rateLimiter: NewIPRateLimiter(0.1, 3, config.BehindProxy), // 1 req/10sec, burst 3
		mux:         http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// OAuth discovery
	s.mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleOAuthMetadata)
	s.mux.HandleFunc("GET /.well-known/oauth-authorization-server/mcp", s.handleOAuthMetadata)
	s.mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handleProtectedResourceMetadata)
	s.mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", s.handleProtectedResourceMetadata)

	// OAuth endpoints (rate limited)
	s.mux.HandleFunc("POST /register", s.rateLimiter.RateLimit(s.handleRegister))
	s.mux.HandleFunc("GET /authorize", s.handleAuthorizeGet)
	s.mux.HandleFunc("POST /authorize", s.rateLimiter.RateLimit(s.handleAuthorizePost))
	s.mux.HandleFunc("POST /token", s.rateLimiter.RateLimit(s.handleToken))

	// MCP endpoint
	s.mux.HandleFunc("POST /mcp", s.requireAuth(s.handleMCP))

	// Health check
	s.mux.HandleFunc("GET /health", s.handleHealth)
}

// ListenAndServe starts the HTTP or HTTPS server.
func (s *Server) ListenAndServe() error {
	if s.config.Domain != "" {
		return s.listenAndServeTLS()
	}
	log.Printf("HTTP server listening on %s", s.config.Listen)
	log.Printf("Base URL: %s", s.config.BaseURL)

	server := &http.Server{
		Addr:              s.config.Listen,
		Handler:           s.mux,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return server.ListenAndServe()
}

func (s *Server) listenAndServeTLS() error {
	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(s.config.Domain),
		Cache:      autocert.DirCache(s.config.CertCache),
	}

	// HTTPS server
	server := &http.Server{
		Addr:              ":443",
		Handler:           s.mux,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig: &tls.Config{
			GetCertificate: manager.GetCertificate,
			MinVersion:     tls.VersionTLS13,
		},
	}

	// HTTP server for ACME challenges and redirect
	go func() {
		log.Printf("HTTP server listening on :80 (ACME + redirect)")
		h := manager.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := "https://" + s.config.Domain + r.URL.Path
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}))
		if err := http.ListenAndServe(":80", h); err != nil {
			log.Printf("HTTP redirect server error: %v", err)
		}
	}()

	log.Printf("HTTPS server listening on :443")
	log.Printf("Domain: %s", s.config.Domain)
	log.Printf("Base URL: %s", s.config.BaseURL)
	return server.ListenAndServeTLS("", "")
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// requireAuth wraps a handler with Bearer token authentication.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			log.Printf("AUTH FAILED: missing token from %s", getIP(r))
			w.Header().Set("WWW-Authenticate", `Bearer realm="mykb"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing token"})
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		hash := storage.HashToken(token)

		_, err := s.db.ValidateToken(hash, storage.TokenAccess)
		if err != nil {
			log.Printf("AUTH FAILED: invalid token from %s", getIP(r))
			w.Header().Set("WWW-Authenticate", `Bearer realm="mykb", error="invalid_token"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}

		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// LocalhostAddr extracts port from addr and returns localhost binding.
// Used to force HTTP mode to bind only to localhost for security.
func LocalhostAddr(addr string) (listen, baseURL string) {
	port := addr
	if len(port) > 0 && port[0] == ':' {
		port = port[1:]
	} else if idx := strings.LastIndex(port, ":"); idx != -1 {
		port = port[idx+1:]
	}
	return "127.0.0.1:" + port, "http://localhost:" + port
}
