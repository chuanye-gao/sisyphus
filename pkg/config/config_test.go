package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig 返回 nil")
	}
	if cfg.Agent.MaxSteps != 50 {
		t.Errorf("默认 MaxSteps 应为 50，实际 %d", cfg.Agent.MaxSteps)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("默认 Provider 应为 openai，实际 %s", cfg.LLM.Provider)
	}
	if cfg.Queue.Workers != 2 {
		t.Errorf("默认 Workers 应为 2，实际 %d", cfg.Queue.Workers)
	}
	if cfg.Tools.Bash.TimeoutSeconds != 10 {
		t.Errorf("默认 Bash timeout 应为 10，实际 %d", cfg.Tools.Bash.TimeoutSeconds)
	}
	if cfg.Tools.WebSearch.Endpoint == "" {
		t.Error("默认 WebSearch endpoint 不应为空")
	}
}

func TestLoadDefaults(t *testing.T) {
	// 确保没有配置文件干扰
	os.Unsetenv("SISYPHUS_CONFIG")
	os.Unsetenv("XDG_CONFIG_HOME")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("期望 openai，实际 %s", cfg.LLM.Provider)
	}
}

func TestEnvOverrides(t *testing.T) {
	os.Setenv("SISYPHUS_MAX_STEPS", "10")
	os.Setenv("SISYPHUS_WORKERS", "4")
	os.Setenv("SISYPHUS_BASH_TIMEOUT", "20")
	os.Setenv("SISYPHUS_WEB_SEARCH_ENDPOINT", "https://example.com/search")
	defer func() {
		os.Unsetenv("SISYPHUS_MAX_STEPS")
		os.Unsetenv("SISYPHUS_WORKERS")
		os.Unsetenv("SISYPHUS_BASH_TIMEOUT")
		os.Unsetenv("SISYPHUS_WEB_SEARCH_ENDPOINT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.Agent.MaxSteps != 10 {
		t.Errorf("env 覆盖 MaxSteps 应为 10，实际 %d", cfg.Agent.MaxSteps)
	}
	if cfg.Queue.Workers != 4 {
		t.Errorf("env 覆盖 Workers 应为 4，实际 %d", cfg.Queue.Workers)
	}
	if cfg.Tools.Bash.TimeoutSeconds != 20 {
		t.Errorf("env 覆盖 bash timeout 应为 20，实际 %d", cfg.Tools.Bash.TimeoutSeconds)
	}
	if cfg.Tools.WebSearch.Endpoint != "https://example.com/search" {
		t.Errorf("env 覆盖 web_search endpoint 失败，实际 %s", cfg.Tools.WebSearch.Endpoint)
	}
}

func TestLoadRecordsConfigPath(t *testing.T) {
	dir := t.TempDir()
	path := dir + string(os.PathSeparator) + "config.yaml"
	if err := os.WriteFile(path, []byte("llm:\n  provider: deepseek\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	os.Setenv("SISYPHUS_CONFIG", path)
	defer os.Unsetenv("SISYPHUS_CONFIG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Path == "" {
		t.Fatal("expected config path to be recorded")
	}
	if cfg.LLM.Provider != "deepseek" {
		t.Fatalf("expected config file to be loaded, got provider %s", cfg.LLM.Provider)
	}
}

func TestDataDir(t *testing.T) {
	dir := DataDir()
	if dir == "" {
		t.Fatal("DataDir 返回空字符串")
	}
	// Windows 和 Linux 路径不一样，但都应该包含 "sisyphus"
	if !contains(dir, "sisyphus") {
		t.Errorf("DataDir 应包含 'sisyphus'，实际 %s", dir)
	}
}

func TestToolEnableFilters(t *testing.T) {
	cfg := ToolsConfig{
		Enabled:  []string{"bash", "read_file"},
		Disabled: []string{"read_file"},
	}
	if !cfg.BuiltinToolEnabled("bash") {
		t.Fatal("bash should be enabled")
	}
	if cfg.BuiltinToolEnabled("read_file") {
		t.Fatal("disabled should override enabled")
	}
	if cfg.BuiltinToolEnabled("search") {
		t.Fatal("search should not be enabled when allow list is set")
	}

	defaultCfg := ToolsConfig{}
	if !defaultCfg.BuiltinToolEnabled("search") {
		t.Fatal("empty allow list should enable built-ins by default")
	}
}

func TestMCPServerEnableFilters(t *testing.T) {
	disabled := false
	server := MCPServerConfig{Enabled: &disabled}
	if server.IsEnabled() {
		t.Fatal("server should be disabled")
	}

	enabled := true
	server = MCPServerConfig{
		Enabled:       &enabled,
		Tools:         []string{"create_issue", "push_files"},
		DisabledTools: []string{"push_files"},
	}
	if !server.IsEnabled() {
		t.Fatal("server should be enabled")
	}
	if !server.ToolEnabled("create_issue") {
		t.Fatal("create_issue should be enabled")
	}
	if server.ToolEnabled("push_files") {
		t.Fatal("disabled_tools should override tools")
	}
	if server.ToolEnabled("search_code") {
		t.Fatal("search_code should not be enabled when tool allow list is set")
	}
}

func TestLoadWithAPIKey(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test123")
	os.Setenv("TAVILY_API_KEY", "tv-test123")
	defer os.Unsetenv("OPENAI_API_KEY")
	defer os.Unsetenv("TAVILY_API_KEY")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.LLM.APIKey != "sk-test123" {
		t.Errorf("期望 APIKey 为 sk-test123，实际 '%s'", cfg.LLM.APIKey)
	}
	if cfg.Tools.WebSearch.APIKey != "tv-test123" {
		t.Errorf("期望 Tavily APIKey 为 tv-test123，实际 '%s'", cfg.Tools.WebSearch.APIKey)
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
