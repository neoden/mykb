package httpd

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/neoden/mykb/storage"
	"golang.org/x/crypto/bcrypt"
)

type oauthMetadata struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	RegistrationEndpoint             string   `json:"registration_endpoint"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	GrantTypesSupported              []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported"`
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

	// Generate CSRF token
	csrfToken := GenerateToken()
	s.csrfTokens.Store(csrfToken, nil, 5*time.Minute)

	// Render login form
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, authorizePage,
		html.EscapeString(clientID),
		html.EscapeString(redirectURI),
		html.EscapeString(codeChallenge),
		html.EscapeString(codeChallengeMethod),
		html.EscapeString(state),
		html.EscapeString(csrfToken),
	)
}

func (s *Server) handleAuthorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid form")
		return
	}

	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")
	state := r.FormValue("state")
	csrfToken := r.FormValue("csrf_token")
	password := r.FormValue("password")

	// Verify CSRF
	if !s.csrfTokens.Validate(csrfToken) {
		writeError(w, http.StatusBadRequest, "invalid or expired CSRF token")
		return
	}
	s.csrfTokens.Get(csrfToken) // consume it

	// Verify password
	storedHash, err := s.db.GetPasswordHash()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password not configured")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	// Generate authorization code
	code := GenerateToken()
	s.authCodes.Store(code, map[string]string{
		"client_id":             clientID,
		"redirect_uri":          redirectURI,
		"code_challenge":        codeChallenge,
		"code_challenge_method": codeChallengeMethod,
	}, s.config.CodeExpiry)

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
	codeData, ok := s.authCodes.Get(code)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid or expired code")
		return
	}

	// Validate code data
	if codeData["client_id"] != clientID {
		writeError(w, http.StatusBadRequest, "client_id mismatch")
		return
	}
	if codeData["redirect_uri"] != redirectURI {
		writeError(w, http.StatusBadRequest, "redirect_uri mismatch")
		return
	}

	// Verify PKCE
	expectedChallenge := HashPKCE(codeVerifier)
	if subtle.ConstantTimeCompare([]byte(expectedChallenge), []byte(codeData["code_challenge"])) != 1 {
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

	if err := s.db.StoreToken(storage.HashToken(accessToken), storage.TokenAccess, clientID, accessExpiry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}
	if err := s.db.StoreToken(storage.HashToken(refreshToken), storage.TokenRefresh, clientID, refreshExpiry); err != nil {
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
	if refreshToken == "" {
		writeError(w, http.StatusBadRequest, "missing refresh_token")
		return
	}

	// Validate refresh token
	hash := storage.HashToken(refreshToken)
	token, err := s.db.ValidateToken(hash, storage.TokenRefresh)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired refresh_token")
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

	if err := s.db.StoreToken(storage.HashToken(newAccessToken), storage.TokenAccess, token.ClientID, accessExpiry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}
	if err := s.db.StoreToken(storage.HashToken(newRefreshToken), storage.TokenRefresh, token.ClientID, refreshExpiry); err != nil {
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
        <input type="hidden" name="client_id" value="%s">
        <input type="hidden" name="redirect_uri" value="%s">
        <input type="hidden" name="code_challenge" value="%s">
        <input type="hidden" name="code_challenge_method" value="%s">
        <input type="hidden" name="state" value="%s">
        <input type="hidden" name="csrf_token" value="%s">
        <input type="password" name="password" placeholder="Enter password" required autofocus>
        <button type="submit">Authorize</button>
    </form>
</body>
</html>`
