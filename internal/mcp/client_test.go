package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/longway/sisyphus/internal/tool"
	"github.com/longway/sisyphus/pkg/config"
)

func TestSafeToolName(t *testing.T) {
	name := safeToolName("filesystem.server", "read-file/path")
	if name != "mcp__filesystem_server__read-file_path" {
		t.Fatalf("unexpected tool name: %s", name)
	}

	long := safeToolName(strings.Repeat("server", 20), strings.Repeat("tool", 20))
	if len(long) > 64 {
		t.Fatalf("tool name too long: %d", len(long))
	}
	if !strings.HasPrefix(long, "mcp__") {
		t.Fatalf("missing MCP prefix: %s", long)
	}
}

func TestCallToolResultString(t *testing.T) {
	result := callToolResult{
		Content: []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
			Data string `json:"data,omitempty"`
			Mime string `json:"mimeType,omitempty"`
		}{
			{Type: "text", Text: "hello"},
			{Type: "image", Data: "abcdef", Mime: "image/png"},
		},
	}

	text := result.String()
	if !strings.Contains(text, "hello") {
		t.Fatalf("missing text content: %s", text)
	}
	if !strings.Contains(text, "[image image/png") {
		t.Fatalf("missing image placeholder: %s", text)
	}
}

func TestStartConfiguredWithHelperProcess(t *testing.T) {
	if os.Getenv("SISYPHUS_MCP_HELPER") == "1" {
		runMCPHelper()
		return
	}

	registry := tool.NewRegistry()
	manager, err := StartConfigured(context.Background(), []config.MCPServerConfig{
		{
			Name:    "helper",
			Command: os.Args[0],
			Args:    []string{"-test.run=TestStartConfiguredWithHelperProcess"},
			Env: map[string]string{
				"SISYPHUS_MCP_HELPER": "1",
			},
			TimeoutSeconds: 5,
		},
	}, registry)
	if err != nil {
		t.Fatalf("start configured: %v", err)
	}
	defer manager.Close()

	localTool := registry.Get("mcp__helper__echo")
	if localTool == nil {
		t.Fatalf("MCP tool was not registered")
	}

	got, err := localTool.Execute(context.Background(), json.RawMessage(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("execute MCP tool: %v", err)
	}
	if got != `{"message":"hello"}` {
		t.Fatalf("unexpected tool result: %s", got)
	}

	source, ok := registry.Source("mcp__helper__echo")
	if !ok {
		t.Fatalf("missing source metadata")
	}
	if source.Kind != "mcp" || source.Server != "helper" || source.RemoteName != "echo" {
		t.Fatalf("unexpected source: %+v", source)
	}
}

func runMCPHelper() {
	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	for scanner.Scan() {
		shouldExit := false
		var req struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil || req.ID == 0 {
			continue
		}

		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": protocolVer,
				"capabilities":    map[string]any{},
				"serverInfo": map[string]string{
					"name":    "helper",
					"version": "test",
				},
			}
		case "tools/list":
			result = map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echo arguments as JSON.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"message": map[string]string{"type": "string"},
							},
						},
					},
				},
			}
		case "tools/call":
			var params struct {
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)
			result = map[string]any{
				"content": []map[string]string{
					{"type": "text", "text": string(params.Arguments)},
				},
			}
			shouldExit = true
		default:
			writeHelperResponse(writer, req.ID, nil, map[string]any{
				"code":    -32601,
				"message": fmt.Sprintf("unknown method %s", req.Method),
			})
			continue
		}

		writeHelperResponse(writer, req.ID, result, nil)
		if shouldExit {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func writeHelperResponse(writer *bufio.Writer, id int64, result any, errObj any) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
	}
	if errObj != nil {
		msg["error"] = errObj
	} else {
		msg["result"] = result
	}
	data, _ := json.Marshal(msg)
	writer.Write(data)
	writer.WriteByte('\n')
	writer.Flush()
}
