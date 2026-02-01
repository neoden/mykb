package httpd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/neoden/mykb/mcp"
)

const maxBodySize = 1 << 20 // 1 MB

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	// Validate Content-Type
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	// Read request body with size limit
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request too large")
		return
	}

	// Parse JSON-RPC request
	var req mcp.Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusOK, mcp.Response{
			JSONRPC: "2.0",
			Error: &mcp.Error{
				Code:    mcp.CodeParseError,
				Message: "Parse error",
			},
		})
		return
	}

	// Handle request (pass HTTP context for cancellation)
	resp := s.mcp.HandleRequest(r.Context(), &req)
	if resp == nil {
		// Notification - no response expected
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
