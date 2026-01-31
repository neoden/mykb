package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/neoden/mykb/storage"
)

const (
	serverName    = "mykb"
	serverVersion = "0.1.0"
	mcpVersion    = "2025-11-25"
)

// Server is an MCP server.
type Server struct {
	db    *storage.DB
	tools map[string]ToolHandler
}

// ToolHandler handles a tool call.
type ToolHandler func(args json.RawMessage) (interface{}, error)

// NewServer creates a new MCP server.
func NewServer(db *storage.DB) *Server {
	s := &Server{
		db:    db,
		tools: make(map[string]ToolHandler),
	}
	s.registerTools()
	return s
}

// ServeStdio runs the server over stdin/stdout.
func (s *Server) ServeStdio() error {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}

		// Parse request
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("Parse error: %v", err)
			resp := Response{
				JSONRPC: "2.0",
				Error: &Error{
					Code:    CodeParseError,
					Message: "Parse error",
				},
			}
			encoder.Encode(resp)
			continue
		}

		// Handle request
		resp := s.HandleRequest(&req)
		if resp != nil {
			if err := encoder.Encode(resp); err != nil {
				log.Printf("Write error: %v", err)
			}
		}
	}
}

// HandleRequest processes a single MCP request and returns a response.
// Returns nil for notifications (requests without an ID).
func (s *Server) HandleRequest(req *Request) *Response {
	log.Printf("Request: %s", req.Method)

	// Notifications have no id and expect no response
	if req.ID == nil || string(req.ID) == "null" {
		s.handleNotification(req)
		return nil
	}

	var result interface{}
	var err *Error

	switch req.Method {
	case "initialize":
		result = s.handleInitialize(req.Params)
	case "ping":
		result = map[string]interface{}{}
	case "tools/list":
		result = s.handleToolsList()
	case "tools/call":
		result, err = s.handleToolsCall(req.Params)
	default:
		err = &Error{
			Code:    CodeMethodNotFound,
			Message: fmt.Sprintf("Method not found: %s", req.Method),
		}
	}

	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   err,
	}
}

func (s *Server) handleNotification(req *Request) {
	switch req.Method {
	case "notifications/initialized":
		log.Printf("Client initialized")
	case "notifications/cancelled":
		log.Printf("Request cancelled")
	default:
		log.Printf("Unknown notification: %s", req.Method)
	}
}

func (s *Server) handleInitialize(params json.RawMessage) *InitializeResult {
	return &InitializeResult{
		ProtocolVersion: mcpVersion,
		Capabilities: Capabilities{
			Tools: &ToolsCapability{},
		},
		ServerInfo: ServerInfo{
			Name:        serverName,
			Version:     serverVersion,
			Title:       "MyKB",
			Description: "Personal knowledge base with full-text search",
		},
		Instructions: `Personal knowledge base with full-text search.

Use get_metadata_index() for a high-level overview of what's stored.
Use get_metadata_values(key) to drill down into a specific metadata field.
Use search_chunks(query) to find chunks by content or metadata.`,
	}
}

func (s *Server) handleToolsList() *ToolsListResult {
	return &ToolsListResult{
		Tools: toolDefinitions,
	}
}

func (s *Server) handleToolsCall(params json.RawMessage) (*CallToolResult, *Error) {
	var p CallToolParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{
			Code:    CodeInvalidParams,
			Message: "Invalid params",
		}
	}

	handler, ok := s.tools[p.Name]
	if !ok {
		return nil, &Error{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("Unknown tool: %s", p.Name),
		}
	}

	result, err := handler(p.Arguments)
	if err != nil {
		return &CallToolResult{
			Content: []Content{TextContent(err.Error())},
			IsError: true,
		}, nil
	}

	// Convert result to JSON text
	data, err := json.Marshal(result)
	if err != nil {
		return &CallToolResult{
			Content: []Content{TextContent(fmt.Sprintf("Marshal error: %v", err))},
			IsError: true,
		}, nil
	}

	return &CallToolResult{
		Content:           []Content{TextContent(string(data))},
		StructuredContent: result,
	}, nil
}
