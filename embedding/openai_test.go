package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}

		var req openAIRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Model != "text-embedding-3-small" {
			t.Errorf("Model = %q", req.Model)
		}
		if len(req.Input) != 2 {
			t.Errorf("len(Input) = %d, want 2", len(req.Input))
		}

		resp := openAIResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1, 0.2}, Index: 0},
				{Embedding: []float32{0.3, 0.4}, Index: 1},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &OpenAIEmbeddingProvider{
		apiKey: "test-key",
		model:  "text-embedding-3-small",
		client: server.Client(),
	}
	// Override URL by using custom transport
	provider.client = &http.Client{
		Transport: &urlRewriteTransport{
			base: http.DefaultTransport,
			url:  server.URL,
		},
	}

	vecs, err := provider.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(vecs) != 2 {
		t.Fatalf("len(vecs) = %d, want 2", len(vecs))
	}
	if vecs[0][0] != 0.1 || vecs[0][1] != 0.2 {
		t.Errorf("vecs[0] = %v", vecs[0])
	}
	if vecs[1][0] != 0.3 || vecs[1][1] != 0.4 {
		t.Errorf("vecs[1] = %v", vecs[1])
	}
}

func TestOpenAIEmbedEmpty(t *testing.T) {
	provider := NewOpenAIEmbeddingProvider("key", "model")

	vecs, err := provider.Embed(context.Background(), []string{})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if vecs != nil {
		t.Errorf("vecs = %v, want nil", vecs)
	}
}

func TestOpenAIEmbedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIResponse{
			Error: &struct {
				Message string `json:"message"`
			}{
				Message: "invalid api key",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &OpenAIEmbeddingProvider{
		apiKey: "bad-key",
		model:  "model",
		client: &http.Client{
			Transport: &urlRewriteTransport{
				base: http.DefaultTransport,
				url:  server.URL,
			},
		},
	}

	_, err := provider.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Error("Expected error")
	}
}

func TestOpenAIDimensions(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"text-embedding-3-small", 1536},
		{"text-embedding-3-large", 3072},
		{"text-embedding-ada-002", 1536},
		{"unknown", 1536},
	}

	for _, tt := range tests {
		p := NewOpenAIEmbeddingProvider("key", tt.model)
		if got := p.Dimensions(); got != tt.want {
			t.Errorf("Dimensions(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestOpenAIModel(t *testing.T) {
	p := NewOpenAIEmbeddingProvider("key", "text-embedding-3-small")
	if got := p.Model(); got != "openai/text-embedding-3-small" {
		t.Errorf("Model() = %q, want %q", got, "openai/text-embedding-3-small")
	}
}

func TestOpenAIEmbedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	provider := &OpenAIEmbeddingProvider{
		apiKey: "key",
		model:  "model",
		client: &http.Client{
			Transport: &urlRewriteTransport{
				base: http.DefaultTransport,
				url:  server.URL,
			},
		},
	}

	_, err := provider.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("Expected error for 500 status")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("Error should mention status code: %v", err)
	}
}

func TestOpenAIEmbedHTTPErrorWithJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "rate limited"}`))
	}))
	defer server.Close()

	provider := &OpenAIEmbeddingProvider{
		apiKey: "key",
		model:  "model",
		client: &http.Client{
			Transport: &urlRewriteTransport{
				base: http.DefaultTransport,
				url:  server.URL,
			},
		},
	}

	_, err := provider.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("Expected error for 429 status")
	}
	if !strings.Contains(err.Error(), "status 429") {
		t.Errorf("Error should mention status code: %v", err)
	}
}

func TestOpenAIClientHasTimeout(t *testing.T) {
	p := NewOpenAIEmbeddingProvider("key", "model")
	if p.client.Timeout == 0 {
		t.Error("Client should have a timeout set")
	}
	if p.client.Timeout.Seconds() < 10 {
		t.Errorf("Timeout too short: %v", p.client.Timeout)
	}
}


// urlRewriteTransport rewrites request URLs to point to test server
type urlRewriteTransport struct {
	base http.RoundTripper
	url  string
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.url[7:] // strip "http://"
	return t.base.RoundTrip(req)
}
