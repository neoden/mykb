package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaEmbeddingProvider implements EmbeddingProvider using a local Ollama server.
type OllamaEmbeddingProvider struct {
	url    string
	model  string
	client *http.Client
}

// NewOllamaEmbeddingProvider creates a new Ollama embedding provider.
func NewOllamaEmbeddingProvider(url, model string) *OllamaEmbeddingProvider {
	return &OllamaEmbeddingProvider{
		url:   url,
		model: model,
		client: &http.Client{
			Timeout: 60 * time.Second, // Ollama may need more time for local inference
		},
	}
}

type ollamaRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

func (p *OllamaEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody, err := json.Marshal(ollamaRequest{
		Model: p.model,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.url+"/api/embed", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("ollama api error: status %d: %s", resp.StatusCode, string(body))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	return result.Embeddings, nil
}

func (p *OllamaEmbeddingProvider) Dimensions() int {
	switch p.model {
	case "mxbai-embed-large":
		return 1024
	case "all-minilm":
		return 384
	default:
		return 768
	}
}

func (p *OllamaEmbeddingProvider) Model() string {
	return "ollama/" + p.model
}
