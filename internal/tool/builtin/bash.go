// Package builtin provides the standard set of tools available to the agent.
// These are the fundamental building blocks: shell execution, file I/O, and web search.
//
// Cross-platform: on Unix (Linux/macOS) shell commands run via "sh -c"; on Windows
// they run via "cmd /c". The agent adapts automatically.
package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// shellCommand returns the shell and its argument flag for the current platform.
func shellCommand() (shell string, flag string) {
	if runtime.GOOS == "windows" {
		return "cmd", "/c"
	}
	return "sh", "-c"
}

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
	if runtime.GOOS == "windows" {
		return "执行命令并返回输出。在 Windows 上使用 cmd /c 执行。"
	}
	return "执行 shell 命令并返回输出。用于运行命令、读写文件、管理进程等。"
}

func (BashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "要执行的命令"
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
		return "", fmt.Errorf("bash: 参数解析失败: %w", err)
	}
	if params.Command == "" {
		return "", fmt.Errorf("bash: 命令为空")
	}

	// 10 second timeout per command
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	shell, flag := shellCommand()
	cmd := exec.CommandContext(ctx, shell, flag, params.Command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		return fmt.Sprintf("退出码: %d\n%s", code, string(out)), nil
	}
	return strings.TrimSpace(string(out)), nil
}

// ReadFileTool 读取文件内容。
type ReadFileTool struct{}

func (ReadFileTool) Name() string { return "read_file" }

func (ReadFileTool) Description() string {
	return "读取指定路径的文件内容。"
}

func (ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "要读取的文件的绝对路径"
			}
		},
		"required": ["path"]
	}`)
}

func (t ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("read_file: 参数解析失败: %w", err)
	}
	// 通过 bash/cmd 执行，保证跨平台一致
	bt := BashTool{}
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = fmt.Sprintf("type %s", params.Path)
	} else {
		cmd = fmt.Sprintf("cat %s", params.Path)
	}
	return bt.Execute(ctx, json.RawMessage(fmt.Sprintf(`{"command": "%s"}`, cmd)))
}

// WriteFileTool 写入内容到文件。
type WriteFileTool struct{}

func (WriteFileTool) Name() string { return "write_file" }

func (WriteFileTool) Description() string {
	return "写入内容到指定路径的文件。文件不存在则创建，存在则覆盖。"
}

func (WriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "要写入的文件的绝对路径"
			},
			"content": {
				"type": "string",
				"description": "要写入的内容"
			}
		},
		"required": ["path", "content"]
	}`)
}

func (t WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("write_file: 参数解析失败: %w", err)
	}

	bt := BashTool{}
	var cmd string
	if runtime.GOOS == "windows" {
		// Windows: 创建目录 + 写入文件
		escaped := strings.ReplaceAll(params.Content, "%", "%%")
		cmd = fmt.Sprintf("mkdir %s 2>nul & echo %s > %s", params.Path, escaped, params.Path)
	} else {
		escaped := strings.ReplaceAll(params.Content, "'", `'\''`)
		cmd = fmt.Sprintf("mkdir -p $(dirname '%s') && printf '%%s' '%s' > '%s'", params.Path, escaped, params.Path)
	}
	return bt.Execute(ctx, json.RawMessage(fmt.Sprintf(`{"command": "%s"}`, cmd)))
}

// WebSearchTool 执行网络搜索。当前为桩实现，待对接搜索 API。
type WebSearchTool struct{}

func (WebSearchTool) Name() string { return "web_search" }

func (WebSearchTool) Description() string {
	return "搜索网络获取信息。"
}

func (WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "搜索关键词"
			}
		},
		"required": ["query"]
	}`)
}

func (WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return "", fmt.Errorf("web_search: 尚未实现，请配置搜索 API 后端")
}
