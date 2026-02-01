package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/embed" {
			t.Errorf("Path = %s, want /api/embed", r.URL.Path)
		}

		var req ollamaRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Model != "nomic-embed-text" {
			t.Errorf("Model = %q", req.Model)
		}
		if len(req.Input) != 2 {
			t.Errorf("len(Input) = %d, want 2", len(req.Input))
		}

		resp := ollamaResponse{
			Embeddings: [][]float32{
				{0.1, 0.2, 0.3},
				{0.4, 0.5, 0.6},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaEmbeddingProvider(server.URL, "nomic-embed-text")

	vecs, err := provider.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(vecs) != 2 {
		t.Fatalf("len(vecs) = %d, want 2", len(vecs))
	}
	if vecs[0][0] != 0.1 {
		t.Errorf("vecs[0][0] = %f, want 0.1", vecs[0][0])
	}
	if vecs[1][0] != 0.4 {
		t.Errorf("vecs[1][0] = %f, want 0.4", vecs[1][0])
	}
}

func TestOllamaEmbedEmpty(t *testing.T) {
	provider := NewOllamaEmbeddingProvider("http://localhost:11434", "model")

	vecs, err := provider.Embed(context.Background(), []string{})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if vecs != nil {
		t.Errorf("vecs = %v, want nil", vecs)
	}
}

func TestOllamaEmbedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{
			Error: "model not found",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaEmbeddingProvider(server.URL, "bad-model")

	_, err := provider.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Error("Expected error")
	}
}

func TestOllamaDimensions(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"nomic-embed-text", 768},
		{"mxbai-embed-large", 1024},
		{"all-minilm", 384},
		{"unknown", 768},
	}

	for _, tt := range tests {
		p := NewOllamaEmbeddingProvider("url", tt.model)
		if got := p.Dimensions(); got != tt.want {
			t.Errorf("Dimensions(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestOllamaModel(t *testing.T) {
	p := NewOllamaEmbeddingProvider("url", "nomic-embed-text")
	if got := p.Model(); got != "ollama/nomic-embed-text" {
		t.Errorf("Model() = %q, want %q", got, "ollama/nomic-embed-text")
	}
}

func TestOllamaEmbedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	provider := NewOllamaEmbeddingProvider(server.URL, "model")

	_, err := provider.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("Expected error for 500 status")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("Error should mention status code: %v", err)
	}
}

func TestOllamaEmbedHTTPErrorServiceUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("service unavailable"))
	}))
	defer server.Close()

	provider := NewOllamaEmbeddingProvider(server.URL, "model")

	_, err := provider.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("Expected error for 503 status")
	}
	if !strings.Contains(err.Error(), "status 503") {
		t.Errorf("Error should mention status code: %v", err)
	}
}

func TestOllamaClientHasTimeout(t *testing.T) {
	p := NewOllamaEmbeddingProvider("http://localhost:11434", "model")
	if p.client.Timeout == 0 {
		t.Error("Client should have a timeout set")
	}
	if p.client.Timeout.Seconds() < 30 {
		t.Errorf("Timeout too short for local inference: %v", p.client.Timeout)
	}
}
