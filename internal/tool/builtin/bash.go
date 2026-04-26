// Package builtin provides the standard set of tools available to the agent.
// These are the fundamental building blocks: shell execution, file I/O, and web search.
package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// BashTool executes shell commands.
type BashTool struct{}

// compile-time check
var _ interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
} = BashTool{}

func (BashTool) Name() string { return "bash" }

func (BashTool) Description() string {
	return "Execute a shell command and return its output. Use this to run commands, read files, manage processes, etc."
}

func (BashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The shell command to execute"
			}
		},
		"required": ["command"]
	}`)
}

func (BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("bash: parse args: %w", err)
	}
	if params.Command == "" {
		return "", fmt.Errorf("bash: command is empty")
	}

	// 10 second timeout per command
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", params.Command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("exit code: %d\n%s", cmd.ProcessState.ExitCode(), string(out)), nil
	}
	return strings.TrimSpace(string(out)), nil
}

// ReadFileTool reads file contents.
type ReadFileTool struct{}

func (ReadFileTool) Name() string { return "read_file" }

func (ReadFileTool) Description() string {
	return "Read the contents of a file at the given path."
}

func (ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Absolute path to the file to read"
			}
		},
		"required": ["path"]
	}`)
}

func (ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("read_file: parse args: %w", err)
	}
	// Delegate to bash with cat for simplicity
	// Avoids duplicating file I/O and path validation logic
	bt := BashTool{}
	return bt.Execute(ctx, json.RawMessage(
		fmt.Sprintf(`{"command": "cat %s"}`, params.Path),
	))
}

// WriteFileTool writes content to a file.
type WriteFileTool struct{}

func (WriteFileTool) Name() string { return "write_file" }

func (WriteFileTool) Description() string {
	return "Write content to a file at the given path. Creates the file if it doesn't exist, overwrites if it does."
}

func (WriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Absolute path to the file to write"
			},
			"content": {
				"type": "string",
				"description": "The content to write to the file"
			}
		},
		"required": ["path", "content"]
	}`)
}

func (WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("write_file: parse args: %w", err)
	}
	bt := BashTool{}
	escaped := strings.ReplaceAll(params.Content, "'", `'\''`)
	return bt.Execute(ctx, json.RawMessage(
		fmt.Sprintf(`{"command": "mkdir -p $(dirname '%s') && printf '%%s' '%s' > '%s'"}`, params.Path, escaped, params.Path),
	))
}

// WebSearchTool performs a web search. Currently a stub; to be backed by a real
// search API (Bing, SerpAPI, etc.) in a future version.
type WebSearchTool struct{}

func (WebSearchTool) Name() string { return "web_search" }

func (WebSearchTool) Description() string {
	return "Search the web for information."
}

func (WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query"
			}
		},
		"required": ["query"]
	}`)
}

func (WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return "", fmt.Errorf("web_search: not yet implemented — configure a search API backend")
}
