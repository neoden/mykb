package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/neoden/mykb/embedding"
	"github.com/pelletier/go-toml/v2"
)

// Config holds all application configuration.
type Config struct {
	DataDir   string           `toml:"data_dir"`
	Embedding embedding.Config `toml:"embedding"`
	Server    ServerConfig     `toml:"server"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Listen      string `toml:"listen"`
	Domain      string `toml:"domain"`
	BehindProxy bool   `toml:"behind_proxy"`
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		Embedding: embedding.Config{
			Provider: "",
			OpenAI: embedding.OpenAIConfig{
				Model: "text-embedding-3-small",
			},
			Ollama: embedding.OllamaConfig{
				URL:   "http://localhost:11434",
				Model: "nomic-embed-text",
			},
		},
		Server: ServerConfig{
			// No default for Listen/Domain - set in main.go if neither specified
		},
	}
}

// Load reads configuration from a TOML file.
// If the file doesn't exist, returns default config.
// Applies platform-specific defaults for empty values.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err == nil {
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	// Apply platform-specific defaults
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir()
	}

	return cfg, nil
}

func defaultDataDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "mykb")
	}
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "mykb")
}

// SearchPaths returns config file paths in search order.
// On Linux/macOS: user config ($XDG_CONFIG_HOME/mykb), then system config (/etc/mykb).
// On Windows: only user config (%APPDATA%\mykb).
func SearchPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(os.Getenv("APPDATA"), "mykb", "config.toml"),
		}
	}

	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}

	return []string{
		filepath.Join(configDir, "mykb", "config.toml"),
		"/etc/mykb/config.toml",
	}
}
