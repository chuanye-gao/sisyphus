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
		t.Fatal("cannot locate test file")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	cfgPath := filepath.Join(repoRoot, "config.yaml.example")

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read repository config template: %v", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse repository config template: %v", err)
	}

	llmAny, ok := raw["llm"]
	if !ok {
		t.Fatal("config template is missing llm config")
	}
	llm, ok := llmAny.(map[string]any)
	if !ok {
		t.Fatal("llm config has invalid shape")
	}
	for _, key := range []string{"provider", "model", "base_url", "api_key"} {
		if _, ok := llm[key]; !ok {
			t.Fatalf("llm config is missing field: %s", key)
		}
	}

	mcpAny, ok := raw["mcp_servers"]
	if !ok {
		t.Fatal("config template is missing mcp_servers config")
	}
	mcpServers, ok := mcpAny.([]any)
	if !ok || len(mcpServers) == 0 {
		t.Fatal("mcp_servers should be a non-empty list")
	}

	first, ok := mcpServers[0].(map[string]any)
	if !ok {
		t.Fatal("mcp server entry has invalid shape")
	}
	for _, key := range []string{"name", "command"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("mcp server config is missing field: %s", key)
		}
	}

	if _, hasSkills := raw["skills"]; hasSkills {
		t.Fatal("config template should not contain skills config")
	}
}
