package httpd

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/neoden/mykb/mcp"
)

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request")
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

	// Handle request
	resp := s.mcp.HandleRequest(&req)
	if resp == nil {
		// Notification - no response expected
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
