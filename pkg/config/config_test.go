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
	defer func() {
		os.Unsetenv("SISYPHUS_MAX_STEPS")
		os.Unsetenv("SISYPHUS_WORKERS")
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

func TestLoadWithAPIKey(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test123")
	defer os.Unsetenv("OPENAI_API_KEY")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if cfg.LLM.APIKey != "sk-test123" {
		t.Errorf("期望 APIKey 为 sk-test123，实际 '%s'", cfg.LLM.APIKey)
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
