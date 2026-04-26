package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRepositoryConfigTemplateHasLLMAndMCPWithoutSkills(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件路径")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	cfgPath := filepath.Join(repoRoot, "config.yaml")

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("读取仓库 config.yaml 失败: %v", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("解析仓库 config.yaml 失败: %v", err)
	}

	llmAny, ok := raw["llm"]
	if !ok {
		t.Fatal("config.yaml 缺少 llm 配置")
	}
	llm, ok := llmAny.(map[string]any)
	if !ok {
		t.Fatal("llm 配置格式错误")
	}
	for _, key := range []string{"provider", "model", "base_url", "api_key"} {
		if _, ok := llm[key]; !ok {
			t.Fatalf("llm 配置缺少字段: %s", key)
		}
	}

	mcpAny, ok := raw["mcp_servers"]
	if !ok {
		t.Fatal("config.yaml 缺少 mcp_servers 配置")
	}
	mcpServers, ok := mcpAny.([]any)
	if !ok || len(mcpServers) == 0 {
		t.Fatal("mcp_servers 应为非空列表")
	}

	first, ok := mcpServers[0].(map[string]any)
	if !ok {
		t.Fatal("mcp_servers 的元素格式错误")
	}
	for _, key := range []string{"name", "command"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("mcp server 配置缺少字段: %s", key)
		}
	}

	if _, hasSkills := raw["skills"]; hasSkills {
		t.Fatal("config.yaml 不应包含 skills 配置")
	}
}
