package embedding

import (
	"context"
	"fmt"
)

// EmbeddingProvider generates vector embeddings for text.
type EmbeddingProvider interface {
	// Embed generates embeddings for the given texts.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dimensions returns the dimensionality of the embeddings.
	Dimensions() int
	// Model returns the model identifier.
	Model() string
}

// Config holds embedding provider configuration.
type Config struct {
	Provider string       `toml:"provider"`
	OpenAI   OpenAIConfig `toml:"openai"`
	Ollama   OllamaConfig `toml:"ollama"`
}

// OpenAIConfig holds OpenAI-specific settings.
type OpenAIConfig struct {
	APIKey string `toml:"api_key"`
	Model  string `toml:"model"`
}

// OllamaConfig holds Ollama-specific settings.
type OllamaConfig struct {
	URL   string `toml:"url"`
	Model string `toml:"model"`
}

// New creates an EmbeddingProvider based on the config.
// Supported providers: "openai", "ollama".
func New(cfg Config) (EmbeddingProvider, error) {
	switch cfg.Provider {
	case "openai":
		if cfg.OpenAI.APIKey == "" {
			return nil, fmt.Errorf("openai.api_key not set")
		}
		model := cfg.OpenAI.Model
		if model == "" {
			model = "text-embedding-3-small"
		}
		return NewOpenAIEmbeddingProvider(cfg.OpenAI.APIKey, model), nil

	case "ollama":
		url := cfg.Ollama.URL
		if url == "" {
			url = "http://localhost:11434"
		}
		model := cfg.Ollama.Model
		if model == "" {
			model = "nomic-embed-text"
		}
		return NewOllamaEmbeddingProvider(url, model), nil

	case "":
		return nil, fmt.Errorf("embedding provider not configured")

	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
}
