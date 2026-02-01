package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/neoden/mykb/storage"
)

// Tool definitions for tools/list
var toolDefinitions = []Tool{
	{
		Name:        "store_chunk",
		Title:       "Store Chunk",
		Description: "Store a new text chunk with optional metadata.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"content": {
					Type:        "string",
					Description: "The text content to store",
				},
				"metadata": {
					Type:        "object",
					Description: "Optional metadata dict. Must be flat: only scalar values or arrays of scalars.",
				},
			},
			Required: []string{"content"},
		},
		Annotations: &ToolAnnotations{
			ReadOnlyHint: false,
		},
	},
	{
		Name:        "search_chunks",
		Title:       "Search Chunks",
		Description: "Full-text search across all stored chunks. Returns truncated content (first 80 chars). Use get_chunk(id) to retrieve full content.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"query": {
					Type:        "string",
					Description: "Search query (supports FTS5 syntax)",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum results to return",
					Default:     20,
				},
			},
			Required: []string{"query"},
		},
		Annotations: &ToolAnnotations{
			ReadOnlyHint: true,
		},
	},
	{
		Name:        "get_chunk",
		Title:       "Get Chunk",
		Description: "Get a specific chunk by ID with full content. Use this after search_chunks() to retrieve the complete content.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"chunk_id": {
					Type:        "string",
					Description: "The UUID of the chunk",
				},
			},
			Required: []string{"chunk_id"},
		},
		Annotations: &ToolAnnotations{
			ReadOnlyHint: true,
		},
	},
	{
		Name:        "update_chunk",
		Title:       "Update Chunk",
		Description: "Update an existing chunk.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"chunk_id": {
					Type:        "string",
					Description: "The UUID of the chunk to update",
				},
				"content": {
					Type:        "string",
					Description: "New content (optional)",
				},
				"metadata": {
					Type:        "object",
					Description: "New metadata (optional). Must be flat.",
				},
			},
			Required: []string{"chunk_id"},
		},
		Annotations: &ToolAnnotations{
			ReadOnlyHint: false,
		},
	},
	{
		Name:        "delete_chunk",
		Title:       "Delete Chunk",
		Description: "Delete a chunk by ID.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"chunk_id": {
					Type:        "string",
					Description: "The UUID of the chunk to delete",
				},
			},
			Required: []string{"chunk_id"},
		},
		Annotations: &ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: true,
		},
	},
	{
		Name:        "get_metadata_index",
		Title:       "Get Metadata Index",
		Description: "Get an overview of all metadata in the knowledge base. Returns aggregated metadata keys with their most common values and counts.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"top_n": {
					Type:        "integer",
					Description: "Maximum number of values to return per key",
					Default:     20,
				},
			},
		},
		Annotations: &ToolAnnotations{
			ReadOnlyHint: true,
		},
	},
	{
		Name:        "get_metadata_values",
		Title:       "Get Metadata Values",
		Description: "Get all values for a specific metadata key. Use this to drill down into a specific metadata field.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key": {
					Type:        "string",
					Description: "The metadata key to get values for",
				},
				"top_n": {
					Type:        "integer",
					Description: "Maximum number of values to return",
					Default:     50,
				},
			},
			Required: []string{"key"},
		},
		Annotations: &ToolAnnotations{
			ReadOnlyHint: true,
		},
	},
	{
		Name:        "semantic_search",
		Title:       "Semantic Search",
		Description: "Search chunks by semantic similarity using vector embeddings. Returns chunks most similar in meaning to the query.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"query": {
					Type:        "string",
					Description: "The search query",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum results to return",
					Default:     10,
				},
			},
			Required: []string{"query"},
		},
		Annotations: &ToolAnnotations{
			ReadOnlyHint: true,
		},
	},
}

// registerTools registers all tool handlers.
func (s *Server) registerTools() {
	s.tools["store_chunk"] = s.toolStoreChunk
	s.tools["search_chunks"] = s.toolSearchChunks
	s.tools["get_chunk"] = s.toolGetChunk
	s.tools["update_chunk"] = s.toolUpdateChunk
	s.tools["delete_chunk"] = s.toolDeleteChunk
	s.tools["get_metadata_index"] = s.toolGetMetadataIndex
	s.tools["get_metadata_values"] = s.toolGetMetadataValues
	s.tools["semantic_search"] = s.toolSemanticSearch
}

// Tool handlers

func (s *Server) toolStoreChunk(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Content  string          `json:"content"`
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if params.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	chunk, err := s.db.CreateChunk(params.Content, params.Metadata)
	if err != nil {
		return nil, err
	}

	// Generate and store embedding if provider is configured
	if s.embedder != nil {
		vecs, err := s.embedder.Embed(ctx, []string{params.Content})
		if err != nil {
			log.Printf("Failed to generate embedding for chunk %s: %v", chunk.ID, err)
		} else if len(vecs) > 0 {
			if err := s.db.SaveEmbedding(chunk.ID, s.embedder.Model(), vecs[0]); err != nil {
				log.Printf("Failed to save embedding for chunk %s: %v", chunk.ID, err)
			} else {
				s.index.Add(chunk.ID, vecs[0])
			}
		}
	}

	return chunk, nil
}

func (s *Server) toolSearchChunks(_ context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if params.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	results, err := s.db.SearchChunks(params.Query, params.Limit)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []storage.SearchResult{}
	}
	// Wrap in object for structuredContent (must be object, not array)
	return map[string]interface{}{
		"results": results,
		"query":   params.Query,
		"count":   len(results),
	}, nil
}

func (s *Server) toolGetChunk(_ context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		ChunkID string `json:"chunk_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if params.ChunkID == "" {
		return nil, fmt.Errorf("chunk_id is required")
	}

	chunk, err := s.db.GetChunk(params.ChunkID)
	if err != nil {
		return nil, err
	}
	if chunk == nil {
		return map[string]interface{}{"found": false}, nil
	}
	return chunk, nil
}

func (s *Server) toolUpdateChunk(ctx context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		ChunkID  string          `json:"chunk_id"`
		Content  *string         `json:"content"`
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if params.ChunkID == "" {
		return nil, fmt.Errorf("chunk_id is required")
	}

	chunk, err := s.db.UpdateChunk(params.ChunkID, params.Content, params.Metadata)
	if err != nil {
		return nil, err
	}
	if chunk == nil {
		return map[string]interface{}{"found": false}, nil
	}

	// Re-generate embedding if content changed
	if params.Content != nil && s.embedder != nil {
		vecs, err := s.embedder.Embed(ctx, []string{*params.Content})
		if err != nil {
			log.Printf("Failed to generate embedding for chunk %s: %v", chunk.ID, err)
		} else if len(vecs) > 0 {
			if err := s.db.SaveEmbedding(chunk.ID, s.embedder.Model(), vecs[0]); err != nil {
				log.Printf("Failed to save embedding for chunk %s: %v", chunk.ID, err)
			} else {
				s.index.Add(chunk.ID, vecs[0])
			}
		}
	}

	return chunk, nil
}

func (s *Server) toolDeleteChunk(_ context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		ChunkID string `json:"chunk_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if params.ChunkID == "" {
		return nil, fmt.Errorf("chunk_id is required")
	}

	deleted, err := s.db.DeleteChunk(params.ChunkID)
	if err != nil {
		return nil, err
	}

	// Remove from vector index (DB cascade deletes embedding)
	if deleted && s.index != nil {
		s.index.Remove(params.ChunkID)
	}

	return map[string]bool{"deleted": deleted}, nil
}

func (s *Server) toolGetMetadataIndex(_ context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		TopN int `json:"top_n"`
	}
	json.Unmarshal(args, &params) // ignore error, use defaults

	result, err := s.db.GetMetadataIndex(params.TopN)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Server) toolGetMetadataValues(_ context.Context, args json.RawMessage) (interface{}, error) {
	var params struct {
		Key  string `json:"key"`
		TopN int    `json:"top_n"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if params.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	result, err := s.db.GetMetadataValues(params.Key, params.TopN)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Server) toolSemanticSearch(ctx context.Context, args json.RawMessage) (interface{}, error) {
	if s.embedder == nil {
		return nil, fmt.Errorf("embedding provider not configured")
	}

	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if params.Query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if params.Limit <= 0 {
		params.Limit = 10
	}

	// Get query embedding
	vecs, err := s.embedder.Embed(ctx, []string{params.Query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	// Search vector index
	results := s.index.Search(vecs[0], params.Limit)

	// Fetch chunk details
	type resultWithChunk struct {
		ID       string          `json:"id"`
		Score    float32         `json:"score"`
		Content  string          `json:"content"`
		Metadata json.RawMessage `json:"metadata,omitempty"`
	}

	output := make([]resultWithChunk, 0, len(results))
	for _, r := range results {
		chunk, err := s.db.GetChunk(r.ID)
		if err != nil || chunk == nil {
			continue
		}
		content := chunk.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		output = append(output, resultWithChunk{
			ID:       r.ID,
			Score:    r.Score,
			Content:  content,
			Metadata: chunk.Metadata,
		})
	}

	return map[string]interface{}{
		"results": output,
		"query":   params.Query,
		"count":   len(output),
	}, nil
}
