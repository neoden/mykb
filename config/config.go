package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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

// Validate checks the configuration for errors.
// Returns nil if the configuration is valid.
func (c *Config) Validate() error {
	// Validate data directory
	if err := validateDataDir(c.DataDir); err != nil {
		return fmt.Errorf("data_dir: %w", err)
	}

	// Validate server config
	if c.Server.Listen != "" && c.Server.Domain != "" {
		return fmt.Errorf("server: listen and domain are mutually exclusive")
	}

	// Validate embedding config
	if err := validateEmbedding(&c.Embedding); err != nil {
		return fmt.Errorf("embedding: %w", err)
	}

	return nil
}

// validateDataDir checks if the data directory is usable.
func validateDataDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("path is empty")
	}

	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// Try to create it
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("cannot create directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot access: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory")
	}

	// Check if writable by creating a temp file
	testFile := filepath.Join(dir, ".write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	f.Close()
	os.Remove(testFile)

	return nil
}

// validateEmbedding checks embedding configuration.
func validateEmbedding(cfg *embedding.Config) error {
	switch cfg.Provider {
	case "":
		// No provider configured - that's OK
		return nil

	case "openai":
		if cfg.OpenAI.APIKey == "" {
			return fmt.Errorf("openai.api_key is required")
		}
		// Basic format check for API key
		if !strings.HasPrefix(cfg.OpenAI.APIKey, "sk-") {
			return fmt.Errorf("openai.api_key should start with 'sk-'")
		}

	case "ollama":
		if cfg.Ollama.URL != "" {
			u, err := url.Parse(cfg.Ollama.URL)
			if err != nil {
				return fmt.Errorf("ollama.url is invalid: %w", err)
			}
			if u.Scheme != "http" && u.Scheme != "https" {
				return fmt.Errorf("ollama.url must use http or https scheme")
			}
		}

	default:
		return fmt.Errorf("unknown provider: %s (valid: openai, ollama)", cfg.Provider)
	}

	return nil
}
