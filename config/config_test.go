package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Embedding.OpenAI.Model != "text-embedding-3-small" {
		t.Errorf("OpenAI model = %q, want text-embedding-3-small", cfg.Embedding.OpenAI.Model)
	}
	if cfg.Embedding.Ollama.URL != "http://localhost:11434" {
		t.Errorf("Ollama URL = %q, want http://localhost:11434", cfg.Embedding.Ollama.URL)
	}
	if cfg.Embedding.Ollama.Model != "nomic-embed-text" {
		t.Errorf("Ollama model = %q, want nomic-embed-text", cfg.Embedding.Ollama.Model)
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("Load nonexistent: %v", err)
	}

	// Should return defaults
	if cfg.Embedding.OpenAI.Model != "text-embedding-3-small" {
		t.Error("Expected default values for nonexistent file")
	}

	// DataDir should be set to platform default
	if cfg.DataDir == "" {
		t.Error("DataDir should have platform default")
	}
}

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	content := `
data_dir = "/custom/data"

[server]
domain = "example.com"
behind_proxy = true

[embedding]
provider = "openai"

[embedding.openai]
api_key = "sk-test"
model = "text-embedding-3-large"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.DataDir != "/custom/data" {
		t.Errorf("DataDir = %q, want /custom/data", cfg.DataDir)
	}
	if cfg.Server.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", cfg.Server.Domain)
	}
	if !cfg.Server.BehindProxy {
		t.Error("BehindProxy should be true")
	}
	if cfg.Embedding.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", cfg.Embedding.Provider)
	}
	if cfg.Embedding.OpenAI.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want sk-test", cfg.Embedding.OpenAI.APIKey)
	}
	if cfg.Embedding.OpenAI.Model != "text-embedding-3-large" {
		t.Errorf("Model = %q, want text-embedding-3-large", cfg.Embedding.OpenAI.Model)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := os.WriteFile(path, []byte(`{invalid toml`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("Expected error for invalid TOML")
	}
}

func TestLoadReadError(t *testing.T) {
	dir := t.TempDir()
	// Use directory as file path - will cause read error
	_, err := Load(dir)
	if err == nil {
		t.Error("Expected error when reading directory as file")
	}
}

func TestLoadDataDirDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Config without data_dir
	content := `[server]
listen = ":8080"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// DataDir should be set to platform default
	if cfg.DataDir == "" {
		t.Error("DataDir should have platform default when not specified")
	}
}

func TestSearchPaths(t *testing.T) {
	paths := SearchPaths()

	if len(paths) == 0 {
		t.Fatal("SearchPaths returned empty")
	}

	for _, p := range paths {
		if p == "" {
			t.Error("SearchPaths contains empty path")
		}
		if !filepath.IsAbs(p) {
			t.Errorf("Path %q is not absolute", p)
		}
	}

	// Platform-specific checks
	if runtime.GOOS == "windows" {
		if len(paths) != 1 {
			t.Errorf("Windows should have 1 path, got %d", len(paths))
		}
	} else {
		if len(paths) != 2 {
			t.Errorf("Unix should have 2 paths, got %d", len(paths))
		}
		// Second path should be /etc/mykb/config.toml
		if paths[1] != "/etc/mykb/config.toml" {
			t.Errorf("Second path = %q, want /etc/mykb/config.toml", paths[1])
		}
	}
}

func TestSearchPathsWithXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG not used on Windows")
	}

	// Save and restore XDG_CONFIG_HOME
	old := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", old)

	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	paths := SearchPaths()

	if paths[0] != "/custom/config/mykb/config.toml" {
		t.Errorf("First path = %q, want /custom/config/mykb/config.toml", paths[0])
	}
}
