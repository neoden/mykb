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

// OpenAIEmbeddingProvider implements EmbeddingProvider using the OpenAI API.
type OpenAIEmbeddingProvider struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIEmbeddingProvider creates a new OpenAI embedding provider.
func NewOpenAIEmbeddingProvider(apiKey, model string) *OpenAIEmbeddingProvider {
	return &OpenAIEmbeddingProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type openAIRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody, err := json.Marshal(openAIRequest{
		Input: texts,
		Model: p.model,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("openai api error: status %d: %s", resp.StatusCode, string(body))
	}

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("openai error: %s", result.Error.Message)
	}

	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		embeddings[d.Index] = d.Embedding
	}

	return embeddings, nil
}

func (p *OpenAIEmbeddingProvider) Dimensions() int {
	switch p.model {
	case "text-embedding-3-large":
		return 3072
	default:
		return 1536
	}
}

func (p *OpenAIEmbeddingProvider) Model() string {
	return "openai/" + p.model
}
