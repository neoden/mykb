package mcp

import "encoding/json"

// JSON-RPC 2.0 types

// Request represents a JSON-RPC request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // can be string, number, or null
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error represents a JSON-RPC error.
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// MCP protocol types (2025-11-25)

// Icon describes an icon for display.
type Icon struct {
	Src      string   `json:"src"`
	MimeType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
}

// ServerInfo describes the MCP server.
type ServerInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	WebsiteURL  string `json:"websiteUrl,omitempty"`
	Icons       []Icon `json:"icons,omitempty"`
}

// InitializeResult is returned from initialize.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Instructions    string       `json:"instructions,omitempty"`
}

// Capabilities describes server capabilities.
type Capabilities struct {
	Tools   *ToolsCapability `json:"tools,omitempty"`
	Logging *struct{}        `json:"logging,omitempty"`
}

// ToolsCapability describes tool support.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// Tool describes an MCP tool.
type Tool struct {
	Name         string           `json:"name"`
	Title        string           `json:"title,omitempty"`
	Description  string           `json:"description,omitempty"`
	InputSchema  InputSchema      `json:"inputSchema"`
	OutputSchema *InputSchema     `json:"outputSchema,omitempty"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	Icons        []Icon           `json:"icons,omitempty"`
}

// ToolAnnotations provides metadata about tool behavior.
type ToolAnnotations struct {
	// Title for display (deprecated, use Tool.Title)
	Title string `json:"title,omitempty"`
	// Whether the tool has side effects (default true for safety)
	ReadOnlyHint bool `json:"readOnlyHint,omitempty"`
	// Whether the tool is destructive
	DestructiveHint bool `json:"destructiveHint,omitempty"`
	// Whether the tool is idempotent
	IdempotentHint bool `json:"idempotentHint,omitempty"`
	// Whether the tool may run for a long time
	OpenWorldHint bool `json:"openWorldHint,omitempty"`
}

// InputSchema is a JSON Schema for tool input.
type InputSchema struct {
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties,omitempty"`
	Required             []string            `json:"required,omitempty"`
	AdditionalProperties *bool               `json:"additionalProperties,omitempty"`
}

// Property describes a schema property.
type Property struct {
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Items       *Property   `json:"items,omitempty"` // for arrays
	AnyOf       []Property  `json:"anyOf,omitempty"` // for unions
}

// ToolsListResult is returned from tools/list.
type ToolsListResult struct {
	Tools      []Tool  `json:"tools"`
	NextCursor *string `json:"nextCursor,omitempty"`
}

// CallToolParams are params for tools/call.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// CallToolResult is returned from tools/call.
type CallToolResult struct {
	Content           []Content   `json:"content"`
	StructuredContent interface{} `json:"structuredContent,omitempty"`
	IsError           bool        `json:"isError,omitempty"`
}

// Content is a content block in tool results.
type Content struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	Data     string            `json:"data,omitempty"`     // base64 for image/audio
	MimeType string            `json:"mimeType,omitempty"` // for image/audio
	Annotations *ContentAnnotations `json:"annotations,omitempty"`
}

// ContentAnnotations provides metadata about content.
type ContentAnnotations struct {
	Audience     []string `json:"audience,omitempty"` // "user", "assistant"
	Priority     *float64 `json:"priority,omitempty"` // 0.0-1.0
	LastModified string   `json:"lastModified,omitempty"`
}

// TextContent creates a text content block.
func TextContent(text string) Content {
	return Content{Type: "text", Text: text}
}

// Notification is a JSON-RPC notification (no id).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}
