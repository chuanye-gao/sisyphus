// Package config provides configuration loading following XDG Base Directory
// Specification. It searches for config in:
//   1. $SISYPHUS_CONFIG (explicit override)
//   2. $XDG_CONFIG_HOME/sisyphus/config.yaml
//   3. ~/.config/sisyphus/config.yaml
//   4. /etc/sisyphus/config.yaml
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds all Sisyphus configuration.
type Config struct {
	Agent    AgentConfig    `yaml:"agent"`
	LLM      LLMConfig      `yaml:"llm"`
	Memory   MemoryConfig   `yaml:"memory"`
	Queue    QueueConfig    `yaml:"queue"`
	LogLevel string         `yaml:"log_level"`
}

// AgentConfig controls the agent execution loop.
type AgentConfig struct {
	MaxSteps     int  `yaml:"max_steps"`     // maximum steps per task (0 = no limit)
	MaxConcurrent int `yaml:"max_concurrent"` // max concurrent agent goroutines
}

// LLMConfig configures the LLM provider.
type LLMConfig struct {
	Provider    string  `yaml:"provider"`     // "openai"
	APIKey      string  `yaml:"api_key"`      // or $OPENAI_API_KEY
	BaseURL     string  `yaml:"base_url"`     // optional, for compatible APIs
	Model       string  `yaml:"model"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
	Timeout     int     `yaml:"timeout"`      // seconds
}

// MemoryConfig controls the agent memory.
type MemoryConfig struct {
	MaxMessages int `yaml:"max_messages"` // max conversation turns to keep
	MaxTokens   int `yaml:"max_tokens"`   // max tokens to keep in memory
}

// QueueConfig controls the task queue.
type QueueConfig struct {
	Size         int `yaml:"size"`          // buffer size of the queue
	Workers      int `yaml:"workers"`       // number of worker goroutines
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			MaxSteps:      50,
			MaxConcurrent: 4,
		},
		LLM: LLMConfig{
			Provider:    "openai",
			Model:       "gpt-4o",
			MaxTokens:   4096,
			Temperature: 0.0,
			Timeout:     120,
		},
		Memory: MemoryConfig{
			MaxMessages: 100,
			MaxTokens:   128000,
		},
		Queue: QueueConfig{
			Size:    256,
			Workers: 2,
		},
		LogLevel: "info",
	}
}

// Load finds and parses the config file according to XDG search order.
// Environment variables take precedence over config file values.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Find and parse config file
	path := findConfigPath()
	if path != "" {
		if err := loadFile(path, cfg); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}

	// Environment variable overrides
	applyEnvOverrides(cfg)

	return cfg, nil
}

// findConfigPath searches XDG paths for the config file.
func findConfigPath() string {
	// 1. Explicit override
	if p := os.Getenv("SISYPHUS_CONFIG"); p != "" {
		return p
	}
	// 2. XDG_CONFIG_HOME
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		p := filepath.Join(dir, "sisyphus", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 3. ~/.config
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".config", "sisyphus", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 4. /etc
	p := "/etc/sisyphus/config.yaml"
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SISYPHUS_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" && cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("SISYPHUS_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("SISYPHUS_MAX_STEPS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			cfg.Agent.MaxSteps = n
		}
	}
	if v := os.Getenv("SISYPHUS_WORKERS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			cfg.Queue.Workers = n
		}
	}
}

// DataDir returns the XDG data directory for Sisyphus.
func DataDir() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "sisyphus")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "sisyphus")
}
