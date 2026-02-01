package httpd

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/neoden/mykb/storage"
	"golang.org/x/crypto/bcrypt"
)

type oauthMetadata struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	RegistrationEndpoint          string   `json:"registration_endpoint"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
	GrantTypesSupported           []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
}

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

type clientRegistration struct {
	ClientName   string   `json:"client_name,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
}

type clientRegistrationResponse struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// validateRedirectURI checks that a redirect URI is safe.
// Allows https://* and http://localhost only.
func validateRedirectURI(uri string) error {
	parsed, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	switch parsed.Scheme {
	case "https":
		// OK
	case "http":
		host := parsed.Hostname()
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return fmt.Errorf("http only allowed for localhost")
		}
	default:
		return fmt.Errorf("scheme must be http or https")
	}

	if parsed.Fragment != "" {
		return fmt.Errorf("fragment not allowed")
	}

	return nil
}

func (s *Server) handleOAuthMetadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, oauthMetadata{
		Issuer:                        s.config.BaseURL,
		AuthorizationEndpoint:         s.config.BaseURL + "/authorize",
		TokenEndpoint:                 s.config.BaseURL + "/token",
		RegistrationEndpoint:          s.config.BaseURL + "/register",
		ResponseTypesSupported:        []string{"code"},
		GrantTypesSupported:           []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported: []string{"S256"},
	})
}

func (s *Server) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, protectedResourceMetadata{
		Resource:             s.config.BaseURL + "/mcp",
		AuthorizationServers: []string{s.config.BaseURL},
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req clientRegistration
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.RedirectURIs) == 0 {
		writeError(w, http.StatusBadRequest, "redirect_uris required")
		return
	}

	// Validate redirect URIs
	for _, uri := range req.RedirectURIs {
		if err := validateRedirectURI(uri); err != nil {
			writeError(w, http.StatusBadRequest, "invalid redirect_uri: "+err.Error())
			return
		}
	}

	// Cleanup stale clients
	s.db.DeleteStaleClients()

	clientID := uuid.New().String()
	if err := s.db.CreateClient(clientID, req.ClientName, req.RedirectURIs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create client")
		return
	}

	writeJSON(w, http.StatusCreated, clientRegistrationResponse{
		ClientID:     clientID,
		ClientName:   req.ClientName,
		RedirectURIs: req.RedirectURIs,
	})
}

func (s *Server) handleAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	state := q.Get("state")

	// Validate client
	client, err := s.db.GetClient(clientID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid client_id")
		return
	}

	// Validate redirect_uri
	validRedirect := false
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			validRedirect = true
			break
		}
	}
	if !validRedirect {
		writeError(w, http.StatusBadRequest, "invalid redirect_uri")
		return
	}

	if responseType != "code" {
		writeError(w, http.StatusBadRequest, "unsupported response_type")
		return
	}

	if codeChallengeMethod == "" {
		codeChallengeMethod = "S256"
	}
	if codeChallengeMethod != "S256" {
		writeError(w, http.StatusBadRequest, "unsupported code_challenge_method")
		return
	}

	// Generate CSRF token with bound parameters
	csrfToken := GenerateToken()
	csrfExpiry := time.Now().Add(5 * time.Minute).Unix()
	s.db.StoreToken(storage.HashToken(csrfToken), storage.TokenCSRF, clientID, csrfExpiry, map[string]string{
		"client_id":             clientID,
		"redirect_uri":          redirectURI,
		"code_challenge":        codeChallenge,
		"code_challenge_method": codeChallengeMethod,
		"state":                 state,
	})

	// Render login form
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, authorizePage, html.EscapeString(csrfToken))
}

func (s *Server) handleAuthorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid form")
		return
	}

	csrfToken := r.FormValue("csrf_token")
	password := r.FormValue("password")

	// Verify and consume CSRF token, extract bound parameters
	csrf, err := s.db.ConsumeToken(storage.HashToken(csrfToken), storage.TokenCSRF)
	if err != nil || csrf == nil {
		writeError(w, http.StatusBadRequest, "invalid or expired CSRF token")
		return
	}

	// Use parameters bound to CSRF token (not from form - prevents tampering)
	clientID := csrf.Data["client_id"]
	redirectURI := csrf.Data["redirect_uri"]
	codeChallenge := csrf.Data["code_challenge"]
	codeChallengeMethod := csrf.Data["code_challenge_method"]
	state := csrf.Data["state"]

	// Verify password
	storedHash, err := s.db.GetPasswordHash()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password not configured")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)); err != nil {
		log.Printf("AUTH FAILED: invalid password from %s for client %s", getIP(r), clientID)
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	// Generate authorization code
	code := GenerateToken()
	codeExpiry := time.Now().Add(s.config.CodeExpiry).Unix()
	s.db.StoreToken(storage.HashToken(code), storage.TokenAuthCode, clientID, codeExpiry, map[string]string{
		"client_id":             clientID,
		"redirect_uri":          redirectURI,
		"code_challenge":        codeChallenge,
		"code_challenge_method": codeChallengeMethod,
	})

	// Redirect back to client
	redirectURL, _ := url.Parse(redirectURI)
	q := redirectURL.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	redirectURL.RawQuery = q.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid form")
		return
	}

	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		s.handleTokenAuthCode(w, r)
	case "refresh_token":
		s.handleTokenRefresh(w, r)
	default:
		writeError(w, http.StatusBadRequest, "unsupported grant_type")
	}
}

func (s *Server) handleTokenAuthCode(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")
	clientID := r.FormValue("client_id")

	if code == "" || redirectURI == "" || codeVerifier == "" || clientID == "" {
		writeError(w, http.StatusBadRequest, "missing required parameters")
		return
	}

	// Get and consume authorization code
	authCode, err := s.db.ConsumeToken(storage.HashToken(code), storage.TokenAuthCode)
	if err != nil || authCode == nil {
		writeError(w, http.StatusBadRequest, "invalid or expired code")
		return
	}

	// Validate code data
	if authCode.Data["client_id"] != clientID {
		writeError(w, http.StatusBadRequest, "client_id mismatch")
		return
	}
	if authCode.Data["redirect_uri"] != redirectURI {
		writeError(w, http.StatusBadRequest, "redirect_uri mismatch")
		return
	}

	// Verify PKCE
	expectedChallenge := HashPKCE(codeVerifier)
	if subtle.ConstantTimeCompare([]byte(expectedChallenge), []byte(authCode.Data["code_challenge"])) != 1 {
		writeError(w, http.StatusBadRequest, "invalid code_verifier")
		return
	}

	// Generate tokens
	accessToken := GenerateToken()
	refreshToken := GenerateToken()

	// Store tokens in database
	now := time.Now()
	accessExpiry := now.Add(s.config.TokenExpiry).Unix()
	refreshExpiry := now.Add(s.config.RefreshTokenExpiry).Unix()

	if err := s.db.StoreToken(storage.HashToken(accessToken), storage.TokenAccess, clientID, accessExpiry, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}
	if err := s.db.StoreToken(storage.HashToken(refreshToken), storage.TokenRefresh, clientID, refreshExpiry, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}

	// Update client last used
	s.db.TouchClient(clientID)

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.config.TokenExpiry.Seconds()),
		RefreshToken: refreshToken,
	})
}

func (s *Server) handleTokenRefresh(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.FormValue("refresh_token")
	clientID := r.FormValue("client_id")
	if refreshToken == "" {
		writeError(w, http.StatusBadRequest, "missing refresh_token")
		return
	}
	if clientID == "" {
		writeError(w, http.StatusBadRequest, "missing client_id")
		return
	}

	// Validate refresh token
	hash := storage.HashToken(refreshToken)
	token, err := s.db.ValidateToken(hash, storage.TokenRefresh)
	if err != nil {
		log.Printf("AUTH FAILED: invalid refresh token from %s", getIP(r))
		writeError(w, http.StatusBadRequest, "invalid or expired refresh_token")
		return
	}

	// Verify token belongs to this client (RFC 6749 Section 6)
	if token.ClientID != clientID {
		log.Printf("AUTH FAILED: refresh token client mismatch from %s", getIP(r))
		writeError(w, http.StatusBadRequest, "invalid refresh_token")
		return
	}

	// Revoke old refresh token (rotation)
	s.db.DeleteToken(hash)

	// Generate new tokens
	newAccessToken := GenerateToken()
	newRefreshToken := GenerateToken()

	now := time.Now()
	accessExpiry := now.Add(s.config.TokenExpiry).Unix()
	refreshExpiry := now.Add(s.config.RefreshTokenExpiry).Unix()

	if err := s.db.StoreToken(storage.HashToken(newAccessToken), storage.TokenAccess, token.ClientID, accessExpiry, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}
	if err := s.db.StoreToken(storage.HashToken(newRefreshToken), storage.TokenRefresh, token.ClientID, refreshExpiry, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}

	// Update client last used
	s.db.TouchClient(token.ClientID)

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  newAccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.config.TokenExpiry.Seconds()),
		RefreshToken: newRefreshToken,
	})
}

const authorizePage = `<!DOCTYPE html>
<html>
<head>
    <title>Authorize - MyKB</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: system-ui, sans-serif; max-width: 400px; margin: 50px auto; padding: 20px; }
        h1 { font-size: 1.5em; }
        form { display: flex; flex-direction: column; gap: 15px; }
        input { padding: 10px; font-size: 16px; border: 1px solid #ccc; border-radius: 4px; }
        button { padding: 12px; font-size: 16px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer; }
        button:hover { background: #0056b3; }
        .info { color: #666; font-size: 0.9em; }
    </style>
</head>
<body>
    <h1>Authorize Access</h1>
    <p class="info">An application is requesting access to your MyKB data.</p>
    <form method="POST" action="/authorize">
        <input type="hidden" name="csrf_token" value="%s">
        <input type="password" name="password" placeholder="Enter password" required autofocus>
        <button type="submit">Authorize</button>
    </form>
</body>
</html>`
