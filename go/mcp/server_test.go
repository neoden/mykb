package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neoden/mykb/storage"
)

func setupTestServer(t *testing.T) *Server {
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
	return NewServer(db)
}

// Helper to call server with JSON-RPC request
func call(t *testing.T, s *Server, method string, params interface{}) json.RawMessage {
	t.Helper()

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			t.Fatalf("Marshal params: %v", err)
		}
	}

	req := &Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  method,
		Params:  paramsRaw,
	}

	// Use handleRequest directly
	resp := s.HandleRequest(req)
	if resp == nil {
		t.Fatal("Expected response, got nil")
	}
	if resp.Error != nil {
		t.Fatalf("RPC error: %d %s", resp.Error.Code, resp.Error.Message)
	}

	result, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}
	return result
}

func callExpectError(t *testing.T, s *Server, method string, params interface{}) *Error {
	t.Helper()

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			t.Fatalf("Marshal params: %v", err)
		}
	}

	req := &Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  method,
		Params:  paramsRaw,
	}

	resp := s.HandleRequest(req)
	if resp == nil {
		t.Fatal("Expected response, got nil")
	}
	return resp.Error
}

func TestInitialize(t *testing.T) {
	s := setupTestServer(t)

	result := call(t, s, "initialize", map[string]interface{}{
		"protocolVersion": "2025-11-25",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    "test",
			"version": "1.0",
		},
	})

	var init InitializeResult
	if err := json.Unmarshal(result, &init); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if init.ProtocolVersion != "2025-11-25" {
		t.Errorf("protocolVersion = %q, want %q", init.ProtocolVersion, "2025-11-25")
	}
	if init.ServerInfo.Name != "mykb" {
		t.Errorf("serverInfo.name = %q, want %q", init.ServerInfo.Name, "mykb")
	}
	if init.Capabilities.Tools == nil {
		t.Error("Expected tools capability")
	}
}

func TestToolsList(t *testing.T) {
	s := setupTestServer(t)

	result := call(t, s, "tools/list", nil)

	var list ToolsListResult
	if err := json.Unmarshal(result, &list); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(list.Tools) != 7 {
		t.Errorf("len(tools) = %d, want 7", len(list.Tools))
	}

	// Check tool names
	names := make(map[string]bool)
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}

	expected := []string{
		"store_chunk", "search_chunks", "get_chunk",
		"update_chunk", "delete_chunk",
		"get_metadata_index", "get_metadata_values",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("Missing tool: %s", name)
		}
	}
}

func TestToolsCallStoreChunk(t *testing.T) {
	s := setupTestServer(t)

	result := call(t, s, "tools/call", map[string]interface{}{
		"name": "store_chunk",
		"arguments": map[string]interface{}{
			"content":  "Test content",
			"metadata": map[string]interface{}{"key": "value"},
		},
	})

	var callResult CallToolResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if callResult.IsError {
		t.Error("Expected success, got error")
	}
	if callResult.StructuredContent == nil {
		t.Error("Expected structuredContent")
	}

	// Check structured content
	chunk, ok := callResult.StructuredContent.(*storage.Chunk)
	if !ok {
		// It might be a map after JSON round-trip
		data, _ := json.Marshal(callResult.StructuredContent)
		var c storage.Chunk
		json.Unmarshal(data, &c)
		if c.ID == "" {
			t.Error("Expected chunk with ID")
		}
	} else if chunk.ID == "" {
		t.Error("Expected chunk with ID")
	}
}

func TestToolsCallSearchChunks(t *testing.T) {
	s := setupTestServer(t)

	// Store some data first
	call(t, s, "tools/call", map[string]interface{}{
		"name": "store_chunk",
		"arguments": map[string]interface{}{
			"content": "The quick brown fox",
		},
	})

	result := call(t, s, "tools/call", map[string]interface{}{
		"name": "search_chunks",
		"arguments": map[string]interface{}{
			"query": "fox",
		},
	})

	var callResult CallToolResult
	json.Unmarshal(result, &callResult)

	if callResult.IsError {
		t.Error("Expected success")
	}

	// Check that result is wrapped in object
	data, _ := json.Marshal(callResult.StructuredContent)
	var wrapped map[string]interface{}
	json.Unmarshal(data, &wrapped)

	if wrapped["count"].(float64) != 1 {
		t.Errorf("count = %v, want 1", wrapped["count"])
	}
}

func TestToolsCallGetChunk(t *testing.T) {
	s := setupTestServer(t)

	// Store first
	storeResult := call(t, s, "tools/call", map[string]interface{}{
		"name": "store_chunk",
		"arguments": map[string]interface{}{
			"content": "Get me",
		},
	})

	var stored CallToolResult
	json.Unmarshal(storeResult, &stored)
	data, _ := json.Marshal(stored.StructuredContent)
	var chunk storage.Chunk
	json.Unmarshal(data, &chunk)

	// Get it back
	result := call(t, s, "tools/call", map[string]interface{}{
		"name": "get_chunk",
		"arguments": map[string]interface{}{
			"chunk_id": chunk.ID,
		},
	})

	var getResult CallToolResult
	json.Unmarshal(result, &getResult)

	if getResult.IsError {
		t.Error("Expected success")
	}
}

func TestToolsCallUpdateChunk(t *testing.T) {
	s := setupTestServer(t)

	// Store first
	storeResult := call(t, s, "tools/call", map[string]interface{}{
		"name": "store_chunk",
		"arguments": map[string]interface{}{
			"content": "Original",
		},
	})

	var stored CallToolResult
	json.Unmarshal(storeResult, &stored)
	data, _ := json.Marshal(stored.StructuredContent)
	var chunk storage.Chunk
	json.Unmarshal(data, &chunk)

	// Update
	result := call(t, s, "tools/call", map[string]interface{}{
		"name": "update_chunk",
		"arguments": map[string]interface{}{
			"chunk_id": chunk.ID,
			"content":  "Updated",
		},
	})

	var updateResult CallToolResult
	json.Unmarshal(result, &updateResult)

	if updateResult.IsError {
		t.Error("Expected success")
	}

	// Verify
	data, _ = json.Marshal(updateResult.StructuredContent)
	var updated storage.Chunk
	json.Unmarshal(data, &updated)

	if updated.Content != "Updated" {
		t.Errorf("Content = %q, want %q", updated.Content, "Updated")
	}
}

func TestToolsCallDeleteChunk(t *testing.T) {
	s := setupTestServer(t)

	// Store first
	storeResult := call(t, s, "tools/call", map[string]interface{}{
		"name": "store_chunk",
		"arguments": map[string]interface{}{
			"content": "Delete me",
		},
	})

	var stored CallToolResult
	json.Unmarshal(storeResult, &stored)
	data, _ := json.Marshal(stored.StructuredContent)
	var chunk storage.Chunk
	json.Unmarshal(data, &chunk)

	// Delete
	result := call(t, s, "tools/call", map[string]interface{}{
		"name": "delete_chunk",
		"arguments": map[string]interface{}{
			"chunk_id": chunk.ID,
		},
	})

	var deleteResult CallToolResult
	json.Unmarshal(result, &deleteResult)

	if deleteResult.IsError {
		t.Error("Expected success")
	}

	// Verify deleted
	getResult := call(t, s, "tools/call", map[string]interface{}{
		"name": "get_chunk",
		"arguments": map[string]interface{}{
			"chunk_id": chunk.ID,
		},
	})

	var got CallToolResult
	json.Unmarshal(getResult, &got)

	if got.StructuredContent != nil {
		t.Error("Expected nil for deleted chunk")
	}
}

func TestToolsCallGetMetadataIndex(t *testing.T) {
	s := setupTestServer(t)

	call(t, s, "tools/call", map[string]interface{}{
		"name": "store_chunk",
		"arguments": map[string]interface{}{
			"content":  "A",
			"metadata": map[string]interface{}{"type": "note"},
		},
	})

	result := call(t, s, "tools/call", map[string]interface{}{
		"name":      "get_metadata_index",
		"arguments": map[string]interface{}{},
	})

	var callResult CallToolResult
	json.Unmarshal(result, &callResult)

	if callResult.IsError {
		t.Error("Expected success")
	}

	data, _ := json.Marshal(callResult.StructuredContent)
	var index map[string]interface{}
	json.Unmarshal(data, &index)

	if index["total_chunks"].(float64) != 1 {
		t.Errorf("total_chunks = %v, want 1", index["total_chunks"])
	}
}

func TestToolsCallUnknownTool(t *testing.T) {
	s := setupTestServer(t)

	err := callExpectError(t, s, "tools/call", map[string]interface{}{
		"name":      "unknown_tool",
		"arguments": map[string]interface{}{},
	})

	if err == nil {
		t.Error("Expected error for unknown tool")
	}
}

func TestUnknownMethod(t *testing.T) {
	s := setupTestServer(t)

	err := callExpectError(t, s, "unknown/method", nil)

	if err == nil {
		t.Error("Expected error for unknown method")
	}
	if err.Code != CodeMethodNotFound {
		t.Errorf("Error code = %d, want %d", err.Code, CodeMethodNotFound)
	}
}

func TestNotificationNoResponse(t *testing.T) {
	s := setupTestServer(t)

	req := &Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		// No ID = notification
	}

	resp := s.HandleRequest(req)
	if resp != nil {
		t.Error("Notifications should not return response")
	}
}

func TestPing(t *testing.T) {
	s := setupTestServer(t)

	result := call(t, s, "ping", nil)

	var pong map[string]interface{}
	if err := json.Unmarshal(result, &pong); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	// Empty object is valid response
}

func TestToolsCallGetMetadataValues(t *testing.T) {
	s := setupTestServer(t)

	// Store chunks with metadata
	call(t, s, "tools/call", map[string]interface{}{
		"name": "store_chunk",
		"arguments": map[string]interface{}{
			"content":  "A",
			"metadata": map[string]interface{}{"lang": "go"},
		},
	})
	call(t, s, "tools/call", map[string]interface{}{
		"name": "store_chunk",
		"arguments": map[string]interface{}{
			"content":  "B",
			"metadata": map[string]interface{}{"lang": "python"},
		},
	})

	result := call(t, s, "tools/call", map[string]interface{}{
		"name": "get_metadata_values",
		"arguments": map[string]interface{}{
			"key": "lang",
		},
	})

	var callResult CallToolResult
	json.Unmarshal(result, &callResult)

	if callResult.IsError {
		t.Error("Expected success")
	}

	data, _ := json.Marshal(callResult.StructuredContent)
	var values map[string]interface{}
	json.Unmarshal(data, &values)

	if values["key"] != "lang" {
		t.Errorf("key = %v, want lang", values["key"])
	}
}

func TestToolsCallMissingRequiredParam(t *testing.T) {
	s := setupTestServer(t)

	// store_chunk without content
	result := call(t, s, "tools/call", map[string]interface{}{
		"name":      "store_chunk",
		"arguments": map[string]interface{}{},
	})

	var callResult CallToolResult
	json.Unmarshal(result, &callResult)

	if !callResult.IsError {
		t.Error("Expected error for missing content")
	}
}

func TestToolsCallGetChunkNotFound(t *testing.T) {
	s := setupTestServer(t)

	result := call(t, s, "tools/call", map[string]interface{}{
		"name": "get_chunk",
		"arguments": map[string]interface{}{
			"chunk_id": "nonexistent-id",
		},
	})

	var callResult CallToolResult
	json.Unmarshal(result, &callResult)

	// Should succeed but return null
	if callResult.IsError {
		t.Error("Should not be error, just null result")
	}
}

func TestServeStdio(t *testing.T) {
	s := setupTestServer(t)

	// Create pipes for stdin/stdout
	input := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}
`

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	// Create input pipe
	inR, inW, _ := os.Pipe()
	os.Stdin = inR

	// Create output pipe
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	// Write input and close
	go func() {
		inW.WriteString(input)
		inW.Close()
	}()

	// Run server (will exit on EOF)
	done := make(chan error)
	go func() {
		done <- s.ServeStdio()
	}()

	// Wait for completion
	err := <-done
	outW.Close()
	if err != nil {
		t.Fatalf("ServeStdio: %v", err)
	}

	// Read output
	var buf bytes.Buffer
	io.Copy(&buf, outR)

	if !strings.Contains(buf.String(), `"jsonrpc":"2.0"`) {
		t.Errorf("Expected JSON-RPC response, got: %s", buf.String())
	}
}
