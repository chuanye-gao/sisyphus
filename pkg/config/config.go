// Package config 提供配置加载功能，自动适配 Windows 和 Linux 的配置路径。
//
// 所有平台均首先检查当前工作目录下的 config.yaml，方便开发和直接运行。
//
// Linux/macOS 搜索路径（XDG 标准）：
//  1. $SISYPHUS_CONFIG（显式指定）
//  2. ./config.yaml（当前工作目录）
//  3. $XDG_CONFIG_HOME/sisyphus/config.yaml
//  4. ~/.config/sisyphus/config.yaml
//  5. /etc/sisyphus/config.yaml
//
// Windows 搜索路径：
//  1. %SISYPHUS_CONFIG%（显式指定）
//  2. ./config.yaml（当前工作目录）
//  3. %APPDATA%/sisyphus/config.yaml
//  4. %ProgramData%/sisyphus/config.yaml
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config 包含 Sisyphus 的所有配置项。
type Config struct {
	Agent    AgentConfig       `yaml:"agent"`
	LLM      LLMConfig         `yaml:"llm"`
	Memory   MemoryConfig      `yaml:"memory"`
	Queue    QueueConfig       `yaml:"queue"`
	Tools    ToolsConfig       `yaml:"tools"`
	MCP      []MCPServerConfig `yaml:"mcp_servers"`
	LogLevel string            `yaml:"log_level"`
	Path     string            `yaml:"-"`
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

// ToolsConfig 控制内置工具行为，避免在代码里写死个人参数。
type ToolsConfig struct {
	Enabled   []string            `yaml:"enabled"`
	Disabled  []string            `yaml:"disabled"`
	Bash      BashToolConfig      `yaml:"bash"`
	WebSearch WebSearchToolConfig `yaml:"web_search"`
}

// BashToolConfig 控制 bash 工具执行超时。
type BashToolConfig struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// WebSearchToolConfig 控制网络搜索工具行为。
type WebSearchToolConfig struct {
	Provider          string `yaml:"provider"`            // 目前支持 "tavily"
	APIKey            string `yaml:"api_key"`             // 可选，推荐环境变量
	Endpoint          string `yaml:"endpoint"`            // Tavily API 地址
	TimeoutSeconds    int    `yaml:"timeout_seconds"`     // HTTP 超时（秒）
	DefaultMaxResults int    `yaml:"default_max_results"` // 默认返回数量
	MaxResultsLimit   int    `yaml:"max_results_limit"`   // 最大允许数量
}

// MCPServerConfig describes an external MCP server process. The current CLI
// stores this config for the next integration layer; built-in tools stay usable
// without MCP.
type MCPServerConfig struct {
	Name           string            `yaml:"name"`
	Enabled        *bool             `yaml:"enabled"`
	Command        string            `yaml:"command"`
	Args           []string          `yaml:"args"`
	Env            map[string]string `yaml:"env"`
	TimeoutSeconds int               `yaml:"timeout_seconds"`
	Tools          []string          `yaml:"tools"`
	DisabledTools  []string          `yaml:"disabled_tools"`
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
		Tools: ToolsConfig{
			Bash: BashToolConfig{
				TimeoutSeconds: 10,
			},
			WebSearch: WebSearchToolConfig{
				Provider:          "tavily",
				Endpoint:          "https://api.tavily.com/search",
				TimeoutSeconds:    15,
				DefaultMaxResults: 5,
				MaxResultsLimit:   10,
			},
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
		if abs, err := filepath.Abs(path); err == nil {
			cfg.Path = abs
		} else {
			cfg.Path = path
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

	// 2. 当前工作目录（所有平台通用，方便开发时直接运行）
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}

	// 3. 可执行文件所在目录（便携式部署：bin/ 下放 exe + config.yaml）
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
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

// envVarPattern 匹配 ${VAR_NAME} 格式的占位符。
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnvVars 将 YAML 文本中所有 ${VAR} 替换为对应的环境变量值。
// 未设置的变量保持为空字符串。
func expandEnvVars(data []byte) []byte {
	return envVarPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		name := envVarPattern.FindSubmatch(match)[1]
		return []byte(os.Getenv(string(name)))
	})
}

func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	data = expandEnvVars(data)
	return yaml.Unmarshal(data, cfg)
}

// BuiltinToolEnabled reports whether a built-in tool should be registered.
func (c ToolsConfig) BuiltinToolEnabled(name string) bool {
	if containsString(c.Disabled, name) {
		return false
	}
	if len(c.Enabled) == 0 {
		return true
	}
	return containsString(c.Enabled, name)
}

// IsEnabled reports whether an MCP server should be started. Missing enabled
// means true to preserve compatibility with existing configs.
func (c MCPServerConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// ToolEnabled reports whether a remote MCP tool should be adapted locally.
func (c MCPServerConfig) ToolEnabled(name string) bool {
	if containsString(c.DisabledTools, name) {
		return false
	}
	if len(c.Tools) == 0 {
		return true
	}
	return containsString(c.Tools, name)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SISYPHUS_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	// 环境变量直接覆盖（优先级高于配置文件）
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" && cfg.LLM.Provider == "deepseek" {
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
	if v := os.Getenv("SISYPHUS_BASH_TIMEOUT"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			cfg.Tools.Bash.TimeoutSeconds = n
		}
	}
	if v := os.Getenv("TAVILY_API_KEY"); v != "" {
		cfg.Tools.WebSearch.APIKey = v
	}
	if v := os.Getenv("SISYPHUS_WEB_SEARCH_ENDPOINT"); v != "" {
		cfg.Tools.WebSearch.Endpoint = v
	}
}

// DataDir 返回平台适配的 Sisyphus 数据目录。
func DataDir() string {
	if dir := os.Getenv("SISYPHUS_DATA_DIR"); dir != "" {
		return dir
	}
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
