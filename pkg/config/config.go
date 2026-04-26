// Package config 提供配置加载功能，自动适配 Windows 和 Linux 的配置路径。
//
// Linux/macOS 搜索路径（XDG 标准）：
//  1. $SISYPHUS_CONFIG（显式指定）
//  2. $XDG_CONFIG_HOME/sisyphus/config.yaml
//  3. ~/.config/sisyphus/config.yaml
//  4. /etc/sisyphus/config.yaml
//
// Windows 搜索路径：
//  1. %SISYPHUS_CONFIG%（显式指定）
//  2. %APPDATA%/sisyphus/config.yaml
//  3. %ProgramData%/sisyphus/config.yaml
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config 包含 Sisyphus 的所有配置项。
type Config struct {
	Agent    AgentConfig  `yaml:"agent"`
	LLM      LLMConfig    `yaml:"llm"`
	Memory   MemoryConfig `yaml:"memory"`
	Queue    QueueConfig  `yaml:"queue"`
	LogLevel string       `yaml:"log_level"`
}

// AgentConfig 控制 agent 执行循环。
type AgentConfig struct {
	MaxSteps      int `yaml:"max_steps"`      // 每任务最大步数（0 = 无限制）
	MaxConcurrent int `yaml:"max_concurrent"` // 最大并发 agent goroutine 数
}

// LLMConfig 配置大模型服务。
type LLMConfig struct {
	Provider    string  `yaml:"provider"` // "openai"
	APIKey      string  `yaml:"api_key"`  // 或使用环境变量 $OPENAI_API_KEY
	BaseURL     string  `yaml:"base_url"` // 可选，用于兼容 API（如 DeepSeek）
	Model       string  `yaml:"model"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
	Timeout     int     `yaml:"timeout"` // 秒
}

// MemoryConfig 控制 agent 记忆。
type MemoryConfig struct {
	MaxMessages int `yaml:"max_messages"` // 保留的最大消息数
	MaxTokens   int `yaml:"max_tokens"`   // 保留的最大 token 数
}

// QueueConfig 控制任务队列。
type QueueConfig struct {
	Size    int `yaml:"size"`    // 队列缓冲区大小
	Workers int `yaml:"workers"` // worker goroutine 数量
}

// DefaultConfig 返回一份带有合理默认值的配置。
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

// Load 按平台适配的搜索路径加载配置文件。
// 环境变量优先级高于配置文件。
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// 查找并解析配置文件
	path := findConfigPath()
	if path != "" {
		if err := loadFile(path, cfg); err != nil {
			return nil, fmt.Errorf("config: 解析 %s 失败: %w", path, err)
		}
	}

	// 环境变量覆盖
	applyEnvOverrides(cfg)

	return cfg, nil
}

// findConfigPath 按平台适配的路径搜索配置文件。
func findConfigPath() string {
	// 1. 显式指定
	if p := os.Getenv("SISYPHUS_CONFIG"); p != "" {
		return p
	}

	if runtime.GOOS == "windows" {
		return findConfigPathWindows()
	}
	return findConfigPathUnix()
}

func findConfigPathWindows() string {
	// %APPDATA%/sisyphus/config.yaml
	if appData := os.Getenv("APPDATA"); appData != "" {
		p := filepath.Join(appData, "sisyphus", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// %ProgramData%/sisyphus/config.yaml
	if progData := os.Getenv("ProgramData"); progData != "" {
		p := filepath.Join(progData, "sisyphus", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findConfigPathUnix() string {
	// XDG_CONFIG_HOME
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		p := filepath.Join(dir, "sisyphus", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// ~/.config
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".config", "sisyphus", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// /etc
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

// DataDir 返回平台适配的 Sisyphus 数据目录。
func DataDir() string {
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "sisyphus")
		}
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "sisyphus")
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Local", "sisyphus")
	}
	return filepath.Join(home, ".local", "share", "sisyphus")
}
