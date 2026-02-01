package embedding

import (
	"testing"
)

func TestNewOpenAI(t *testing.T) {
	cfg := Config{
		Provider: "openai",
		OpenAI: OpenAIConfig{
			APIKey: "test-key",
		},
	}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, ok := p.(*OpenAIEmbeddingProvider); !ok {
		t.Error("Expected OpenAIEmbeddingProvider")
	}
}

func TestNewOpenAINoKey(t *testing.T) {
	cfg := Config{
		Provider: "openai",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error when api_key not set")
	}
}

func TestNewOllama(t *testing.T) {
	cfg := Config{
		Provider: "ollama",
	}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, ok := p.(*OllamaEmbeddingProvider); !ok {
		t.Error("Expected OllamaEmbeddingProvider")
	}
}

func TestNewOllamaCustomURL(t *testing.T) {
	cfg := Config{
		Provider: "ollama",
		Ollama: OllamaConfig{
			URL:   "http://custom:1234",
			Model: "custom-model",
		},
	}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ollama := p.(*OllamaEmbeddingProvider)
	if ollama.url != "http://custom:1234" {
		t.Errorf("url = %q, want http://custom:1234", ollama.url)
	}
	if ollama.model != "custom-model" {
		t.Errorf("model = %q, want custom-model", ollama.model)
	}
}

func TestNewUnknown(t *testing.T) {
	cfg := Config{
		Provider: "unknown",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for unknown provider")
	}
}

func TestNewOpenAICustomModel(t *testing.T) {
	cfg := Config{
		Provider: "openai",
		OpenAI: OpenAIConfig{
			APIKey: "test-key",
			Model:  "text-embedding-3-large",
		},
	}

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	openai := p.(*OpenAIEmbeddingProvider)
	if openai.model != "text-embedding-3-large" {
		t.Errorf("model = %q, want text-embedding-3-large", openai.model)
	}
}

func TestNewEmptyProvider(t *testing.T) {
	cfg := Config{
		Provider: "",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for empty provider")
	}
}
