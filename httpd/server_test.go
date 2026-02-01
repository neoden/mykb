package httpd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neoden/mykb/mcp"
	"github.com/neoden/mykb/storage"
	"github.com/neoden/mykb/vector"
	"golang.org/x/crypto/bcrypt"
)

func setupTestServer(t *testing.T) (*Server, *storage.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Set password
	hash, _ := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.MinCost)
	db.SetPasswordHash(string(hash))

	config := DefaultConfig()
	config.BaseURL = "http://localhost:8080"

	mcpServer := mcp.NewServer(db, nil, vector.NewIndex())
	return NewServer(db, mcpServer, config), db
}

func setupTestServerNoPassword(t *testing.T) (*Server, *storage.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// NO password set

	config := DefaultConfig()
	config.BaseURL = "http://localhost:8080"

	mcpServer := mcp.NewServer(db, nil, vector.NewIndex())
	return NewServer(db, mcpServer, config), db
}

func TestHealthEndpoint(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestOAuthMetadata(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var meta oauthMetadata
	json.NewDecoder(w.Body).Decode(&meta)

	if meta.Issuer != "http://localhost:8080" {
		t.Errorf("Issuer = %q", meta.Issuer)
	}
	if meta.AuthorizationEndpoint != "http://localhost:8080/authorize" {
		t.Errorf("AuthorizationEndpoint = %q", meta.AuthorizationEndpoint)
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/.well-known/oauth-protected-resource", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var meta protectedResourceMetadata
	json.NewDecoder(w.Body).Decode(&meta)

	if meta.Resource != "http://localhost:8080/mcp" {
		t.Errorf("Resource = %q", meta.Resource)
	}
}

func TestRegisterClient(t *testing.T) {
	server, _ := setupTestServer(t)

	body := `{"client_name":"Test App","redirect_uris":["http://localhost/callback"]}`
	req := httptest.NewRequest("POST", "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp clientRegistrationResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.ClientID == "" {
		t.Error("ClientID should not be empty")
	}
	if resp.ClientName != "Test App" {
		t.Errorf("ClientName = %q", resp.ClientName)
	}
}

func TestRegisterClientNoRedirectURIs(t *testing.T) {
	server, _ := setupTestServer(t)

	body := `{"client_name":"Test"}`
	req := httptest.NewRequest("POST", "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAuthorizeGetShowsForm(t *testing.T) {
	server, db := setupTestServer(t)

	// Register client first
	db.CreateClient("test-client", "Test", []string{"http://localhost/callback"})

	req := httptest.NewRequest("GET", "/authorize?client_id=test-client&redirect_uri=http://localhost/callback&response_type=code&code_challenge=abc&code_challenge_method=S256", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<form") {
		t.Error("Expected HTML form")
	}
	if !strings.Contains(body, "csrf_token") {
		t.Error("Expected CSRF token in form")
	}
}

func TestAuthorizeInvalidClient(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("GET", "/authorize?client_id=invalid&redirect_uri=http://localhost&response_type=code&code_challenge=abc", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMCPRequiresAuth(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestMCPWithValidToken(t *testing.T) {
	server, db := setupTestServer(t)

	// Create valid token
	token := GenerateToken()
	hash := storage.HashToken(token)
	expiry := time.Now().Add(time.Hour).Unix()
	db.StoreToken(hash, storage.TokenAccess, "client", expiry, nil)

	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["jsonrpc"] != "2.0" {
		t.Error("Expected JSON-RPC response")
	}
}

func TestMCPWithInvalidToken(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthorizeInvalidRedirectURI(t *testing.T) {
	server, db := setupTestServer(t)

	db.CreateClient("test-client", "Test", []string{"http://localhost/callback"})

	req := httptest.NewRequest("GET", "/authorize?client_id=test-client&redirect_uri=http://evil.com/callback&response_type=code&code_challenge=abc", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAuthorizeDefaultCodeChallengeMethod(t *testing.T) {
	server, db := setupTestServer(t)

	db.CreateClient("test-client", "Test", []string{"http://localhost/callback"})

	// No code_challenge_method - should default to S256
	req := httptest.NewRequest("GET", "/authorize?client_id=test-client&redirect_uri=http://localhost/callback&response_type=code&code_challenge=abc", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Should show form (S256 is default)
	if !strings.Contains(w.Body.String(), "csrf_token") {
		t.Error("Expected form with CSRF token")
	}
}

func TestAuthorizeUnsupportedCodeChallengeMethod(t *testing.T) {
	server, db := setupTestServer(t)

	db.CreateClient("test-client", "Test", []string{"http://localhost/callback"})

	// Use unsupported method "plain"
	req := httptest.NewRequest("GET", "/authorize?client_id=test-client&redirect_uri=http://localhost/callback&response_type=code&code_challenge=abc&code_challenge_method=plain", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d (unsupported method)", w.Code, http.StatusBadRequest)
	}
}

func TestAuthorizeUnsupportedResponseType(t *testing.T) {
	server, db := setupTestServer(t)

	db.CreateClient("test-client", "Test", []string{"http://localhost/callback"})

	req := httptest.NewRequest("GET", "/authorize?client_id=test-client&redirect_uri=http://localhost/callback&response_type=token&code_challenge=abc", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAuthorizePostInvalidCSRF(t *testing.T) {
	server, db := setupTestServer(t)

	db.CreateClient("test-client", "Test", []string{"http://localhost/callback"})

	form := url.Values{}
	form.Set("client_id", "test-client")
	form.Set("redirect_uri", "http://localhost/callback")
	form.Set("code_challenge", "abc")
	form.Set("code_challenge_method", "S256")
	form.Set("csrf_token", "invalid-csrf")
	form.Set("password", "testpass")

	req := httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAuthorizePostNoPasswordConfigured(t *testing.T) {
	server, db := setupTestServerNoPassword(t)

	db.CreateClient("test-client", "Test", []string{"http://localhost/callback"})

	// Store a CSRF token manually
	csrfToken := GenerateToken()
	csrfExpiry := time.Now().Add(time.Minute).Unix()
	db.StoreToken(storage.HashToken(csrfToken), storage.TokenCSRF, "test-client", csrfExpiry, map[string]string{
		"client_id":             "test-client",
		"redirect_uri":          "http://localhost/callback",
		"code_challenge":        "abc",
		"code_challenge_method": "S256",
		"state":                 "",
	})

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("password", "anypassword")

	req := httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Status = %d, want %d (password not configured)", w.Code, http.StatusInternalServerError)
	}
}

func TestAuthorizePostInvalidPassword(t *testing.T) {
	server, db := setupTestServer(t)

	db.CreateClient("test-client", "Test", []string{"http://localhost/callback"})

	// Get CSRF token
	authURL := "/authorize?client_id=test-client&redirect_uri=http://localhost/callback&response_type=code&code_challenge=abc&code_challenge_method=S256"
	req := httptest.NewRequest("GET", authURL, nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	body := w.Body.String()
	csrfStart := strings.Index(body, `name="csrf_token" value="`) + len(`name="csrf_token" value="`)
	csrfEnd := strings.Index(body[csrfStart:], `"`)
	csrfToken := body[csrfStart : csrfStart+csrfEnd]

	form := url.Values{}
	form.Set("client_id", "test-client")
	form.Set("redirect_uri", "http://localhost/callback")
	form.Set("code_challenge", "abc")
	form.Set("code_challenge_method", "S256")
	form.Set("csrf_token", csrfToken)
	form.Set("password", "wrongpassword")

	req = httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestTokenInvalidGrantType(t *testing.T) {
	server, _ := setupTestServer(t)

	form := url.Values{}
	form.Set("grant_type", "invalid")

	req := httptest.NewRequest("POST", "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTokenInvalidCode(t *testing.T) {
	server, _ := setupTestServer(t)

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "invalid-code")
	form.Set("redirect_uri", "http://localhost/callback")
	form.Set("code_verifier", "verifier")
	form.Set("client_id", "client")

	req := httptest.NewRequest("POST", "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTokenInvalidRefreshToken(t *testing.T) {
	server, _ := setupTestServer(t)

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", "invalid-refresh-token")

	req := httptest.NewRequest("POST", "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTokenMissingParams(t *testing.T) {
	server, _ := setupTestServer(t)

	// Missing code
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", "http://localhost/callback")

	req := httptest.NewRequest("POST", "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTokenMissingRefreshToken(t *testing.T) {
	server, _ := setupTestServer(t)

	form := url.Values{}
	form.Set("grant_type", "refresh_token")

	req := httptest.NewRequest("POST", "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTokenPKCEVerificationFailure(t *testing.T) {
	server, db := setupTestServer(t)

	db.CreateClient("pkce-client", "Test", []string{"http://localhost/callback"})

	// Get CSRF and submit authorization with one challenge
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := HashPKCE(verifier)

	authURL := "/authorize?client_id=pkce-client&redirect_uri=http://localhost/callback&response_type=code&code_challenge=" + challenge + "&code_challenge_method=S256"
	req := httptest.NewRequest("GET", authURL, nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	body := w.Body.String()
	csrfStart := strings.Index(body, `name="csrf_token" value="`) + len(`name="csrf_token" value="`)
	csrfEnd := strings.Index(body[csrfStart:], `"`)
	csrfToken := body[csrfStart : csrfStart+csrfEnd]

	form := url.Values{}
	form.Set("client_id", "pkce-client")
	form.Set("redirect_uri", "http://localhost/callback")
	form.Set("code_challenge", challenge)
	form.Set("code_challenge_method", "S256")
	form.Set("csrf_token", csrfToken)
	form.Set("password", "testpass")

	req = httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	location := w.Header().Get("Location")
	redirectURL, _ := url.Parse(location)
	code := redirectURL.Query().Get("code")

	// Try to exchange with WRONG verifier
	tokenForm := url.Values{}
	tokenForm.Set("grant_type", "authorization_code")
	tokenForm.Set("code", code)
	tokenForm.Set("redirect_uri", "http://localhost/callback")
	tokenForm.Set("code_verifier", "wrong-verifier-that-does-not-match")
	tokenForm.Set("client_id", "pkce-client")

	req = httptest.NewRequest("POST", "/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d (PKCE should fail)", w.Code, http.StatusBadRequest)
	}

	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["error"] != "invalid code_verifier" {
		t.Errorf("Error = %q, want 'invalid code_verifier'", errResp["error"])
	}
}

func TestTokenClientIDMismatch(t *testing.T) {
	server, db := setupTestServer(t)

	db.CreateClient("client-a", "Test A", []string{"http://localhost/callback"})
	db.CreateClient("client-b", "Test B", []string{"http://localhost/callback"})

	// Authorize as client-a
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := HashPKCE(verifier)

	authURL := "/authorize?client_id=client-a&redirect_uri=http://localhost/callback&response_type=code&code_challenge=" + challenge + "&code_challenge_method=S256"
	req := httptest.NewRequest("GET", authURL, nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	body := w.Body.String()
	csrfStart := strings.Index(body, `name="csrf_token" value="`) + len(`name="csrf_token" value="`)
	csrfEnd := strings.Index(body[csrfStart:], `"`)
	csrfToken := body[csrfStart : csrfStart+csrfEnd]

	form := url.Values{}
	form.Set("client_id", "client-a")
	form.Set("redirect_uri", "http://localhost/callback")
	form.Set("code_challenge", challenge)
	form.Set("code_challenge_method", "S256")
	form.Set("csrf_token", csrfToken)
	form.Set("password", "testpass")

	req = httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	location := w.Header().Get("Location")
	redirectURL, _ := url.Parse(location)
	code := redirectURL.Query().Get("code")

	// Try to exchange as client-b
	tokenForm := url.Values{}
	tokenForm.Set("grant_type", "authorization_code")
	tokenForm.Set("code", code)
	tokenForm.Set("redirect_uri", "http://localhost/callback")
	tokenForm.Set("code_verifier", verifier)
	tokenForm.Set("client_id", "client-b") // WRONG client

	req = httptest.NewRequest("POST", "/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTokenRedirectURIMismatch(t *testing.T) {
	server, db := setupTestServer(t)

	db.CreateClient("redirect-client", "Test", []string{"http://localhost/callback", "http://localhost/other"})

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := HashPKCE(verifier)

	// Authorize with /callback
	authURL := "/authorize?client_id=redirect-client&redirect_uri=http://localhost/callback&response_type=code&code_challenge=" + challenge + "&code_challenge_method=S256"
	req := httptest.NewRequest("GET", authURL, nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	body := w.Body.String()
	csrfStart := strings.Index(body, `name="csrf_token" value="`) + len(`name="csrf_token" value="`)
	csrfEnd := strings.Index(body[csrfStart:], `"`)
	csrfToken := body[csrfStart : csrfStart+csrfEnd]

	form := url.Values{}
	form.Set("client_id", "redirect-client")
	form.Set("redirect_uri", "http://localhost/callback")
	form.Set("code_challenge", challenge)
	form.Set("code_challenge_method", "S256")
	form.Set("csrf_token", csrfToken)
	form.Set("password", "testpass")

	req = httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	location := w.Header().Get("Location")
	redirectURL, _ := url.Parse(location)
	code := redirectURL.Query().Get("code")

	// Try to exchange with /other redirect_uri
	tokenForm := url.Values{}
	tokenForm.Set("grant_type", "authorization_code")
	tokenForm.Set("code", code)
	tokenForm.Set("redirect_uri", "http://localhost/other") // WRONG redirect
	tokenForm.Set("code_verifier", verifier)
	tokenForm.Set("client_id", "redirect-client")

	req = httptest.NewRequest("POST", "/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAuthorizePostInvalidForm(t *testing.T) {
	server, _ := setupTestServer(t)

	// Send a body that causes ParseForm to fail
	req := httptest.NewRequest("POST", "/authorize", strings.NewReader("%invalid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTokenInvalidForm(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest("POST", "/token", strings.NewReader("%invalid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMCPInvalidContentType(t *testing.T) {
	server, db := setupTestServer(t)

	token := GenerateToken()
	hash := storage.HashToken(token)
	expiry := time.Now().Add(time.Hour).Unix()
	db.StoreToken(hash, storage.TokenAccess, "client", expiry, nil)

	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestMCPParseError(t *testing.T) {
	server, db := setupTestServer(t)

	// Create valid token
	token := GenerateToken()
	hash := storage.HashToken(token)
	expiry := time.Now().Add(time.Hour).Unix()
	db.StoreToken(hash, storage.TokenAccess, "client", expiry, nil)

	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d (JSON-RPC error in body)", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == nil {
		t.Error("Expected JSON-RPC error")
	}
}

func TestFullOAuthFlow(t *testing.T) {
	server, db := setupTestServer(t)

	// 1. Register client
	db.CreateClient("flow-client", "Test", []string{"http://localhost/callback"})

	// 2. Get authorize page to get CSRF token
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := HashPKCE(verifier)

	authURL := "/authorize?client_id=flow-client&redirect_uri=http://localhost/callback&response_type=code&code_challenge=" + challenge + "&code_challenge_method=S256&state=xyz"
	req := httptest.NewRequest("GET", authURL, nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	// Extract CSRF token from form
	body := w.Body.String()
	csrfStart := strings.Index(body, `name="csrf_token" value="`) + len(`name="csrf_token" value="`)
	csrfEnd := strings.Index(body[csrfStart:], `"`)
	csrfToken := body[csrfStart : csrfStart+csrfEnd]

	// 3. Submit authorize form
	form := url.Values{}
	form.Set("client_id", "flow-client")
	form.Set("redirect_uri", "http://localhost/callback")
	form.Set("code_challenge", challenge)
	form.Set("code_challenge_method", "S256")
	form.Set("state", "xyz")
	form.Set("csrf_token", csrfToken)
	form.Set("password", "testpass")

	req = httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("Authorize POST status = %d, want %d", w.Code, http.StatusFound)
	}

	// Extract code from redirect
	location := w.Header().Get("Location")
	redirectURL, _ := url.Parse(location)
	code := redirectURL.Query().Get("code")
	state := redirectURL.Query().Get("state")

	if code == "" {
		t.Fatal("No code in redirect")
	}
	if state != "xyz" {
		t.Errorf("State = %q, want %q", state, "xyz")
	}

	// 4. Exchange code for token
	tokenForm := url.Values{}
	tokenForm.Set("grant_type", "authorization_code")
	tokenForm.Set("code", code)
	tokenForm.Set("redirect_uri", "http://localhost/callback")
	tokenForm.Set("code_verifier", verifier)
	tokenForm.Set("client_id", "flow-client")

	req = httptest.NewRequest("POST", "/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Token exchange status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var tokenResp tokenResponse
	json.NewDecoder(w.Body).Decode(&tokenResp)

	if tokenResp.AccessToken == "" {
		t.Error("No access token")
	}
	if tokenResp.RefreshToken == "" {
		t.Error("No refresh token")
	}
	if tokenResp.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want Bearer", tokenResp.TokenType)
	}

	// 5. Use access token for MCP
	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	w = httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("MCP status = %d, want %d", w.Code, http.StatusOK)
	}

	// 6. Refresh token
	refreshForm := url.Values{}
	refreshForm.Set("grant_type", "refresh_token")
	refreshForm.Set("refresh_token", tokenResp.RefreshToken)

	req = httptest.NewRequest("POST", "/token", strings.NewReader(refreshForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Refresh status = %d, want %d", w.Code, http.StatusOK)
	}

	var refreshResp tokenResponse
	json.NewDecoder(w.Body).Decode(&refreshResp)

	if refreshResp.AccessToken == "" {
		t.Error("No new access token")
	}
	if refreshResp.AccessToken == tokenResp.AccessToken {
		t.Error("New access token should be different")
	}
}

func TestLocalhostAddr(t *testing.T) {
	tests := []struct {
		addr        string
		wantListen  string
		wantBaseURL string
	}{
		{":8080", "127.0.0.1:8080", "http://localhost:8080"},
		{":9000", "127.0.0.1:9000", "http://localhost:9000"},
		{"8080", "127.0.0.1:8080", "http://localhost:8080"},
		{"0.0.0.0:8080", "127.0.0.1:8080", "http://localhost:8080"},
		{"192.168.1.1:3000", "127.0.0.1:3000", "http://localhost:3000"},
	}

	for _, tt := range tests {
		listen, baseURL := LocalhostAddr(tt.addr)
		if listen != tt.wantListen {
			t.Errorf("LocalhostAddr(%q) listen = %q, want %q", tt.addr, listen, tt.wantListen)
		}
		if baseURL != tt.wantBaseURL {
			t.Errorf("LocalhostAddr(%q) baseURL = %q, want %q", tt.addr, baseURL, tt.wantBaseURL)
		}
	}
}

func TestLocalhostAddrIgnoresExternalBindings(t *testing.T) {
	// Verify that any attempt to bind to external addresses is forced to localhost
	externalAddrs := []string{
		"0.0.0.0:8080",
		"192.168.1.100:8080",
		"10.0.0.1:8080",
		"[::]:8080",
	}

	for _, addr := range externalAddrs {
		listen, _ := LocalhostAddr(addr)
		if !strings.HasPrefix(listen, "127.0.0.1:") {
			t.Errorf("LocalhostAddr(%q) = %q, should bind to 127.0.0.1", addr, listen)
		}
	}
}
