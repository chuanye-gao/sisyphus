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
	"net/http"
	"os"
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

const (
	defaultBashTimeoutSeconds       = 10
	defaultWebSearchTimeoutSeconds  = 15
	defaultWebSearchMaxResults      = 5
	defaultWebSearchMaxResultsLimit = 10
	defaultWebSearchEndpoint        = "https://api.tavily.com/search"
)

type BashTool struct {
	Timeout time.Duration
}

func NewBashTool(timeoutSeconds int) BashTool {
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultBashTimeoutSeconds
	}
	return BashTool{Timeout: time.Duration(timeoutSeconds) * time.Second}
}

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

func (t BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("bash: 参数解析失败: %w", err)
	}
	if params.Command == "" {
		return "", fmt.Errorf("bash: 命令为空")
	}

	timeout := t.Timeout
	if timeout <= 0 {
		timeout = defaultBashTimeoutSeconds * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
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

type ReadFileTool struct {
	Bash BashTool
}

func NewReadFileTool(bash BashTool) ReadFileTool {
	return ReadFileTool{Bash: bash}
}

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
	bt := t.Bash
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = fmt.Sprintf("type %s", params.Path)
	} else {
		cmd = fmt.Sprintf("cat %s", params.Path)
	}
	return bt.Execute(ctx, json.RawMessage(fmt.Sprintf(`{"command": "%s"}`, cmd)))
}

type WriteFileTool struct {
	Bash BashTool
}

func NewWriteFileTool(bash BashTool) WriteFileTool {
	return WriteFileTool{Bash: bash}
}

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

	bt := t.Bash
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

// WebSearchTool 使用 Tavily API 执行网络搜索。
// Tavily 是专为 AI agent 设计的搜索引擎，返回结构化结果。
// API 密钥通过环境变量 TAVILY_API_KEY 提供。
type WebSearchTool struct {
	APIKey            string
	Endpoint          string
	HTTPTimeout       time.Duration
	DefaultMaxResults int
	MaxResultsLimit   int
}

func NewWebSearchTool(apiKey, endpoint string, timeoutSeconds, defaultMaxResults, maxResultsLimit int) WebSearchTool {
	if endpoint == "" {
		endpoint = defaultWebSearchEndpoint
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultWebSearchTimeoutSeconds
	}
	if defaultMaxResults <= 0 {
		defaultMaxResults = defaultWebSearchMaxResults
	}
	if maxResultsLimit <= 0 {
		maxResultsLimit = defaultWebSearchMaxResultsLimit
	}
	if defaultMaxResults > maxResultsLimit {
		defaultMaxResults = maxResultsLimit
	}

	return WebSearchTool{
		APIKey:            apiKey,
		Endpoint:          endpoint,
		HTTPTimeout:       time.Duration(timeoutSeconds) * time.Second,
		DefaultMaxResults: defaultMaxResults,
		MaxResultsLimit:   maxResultsLimit,
	}
}

func (WebSearchTool) Name() string { return "web_search" }

func (WebSearchTool) Description() string {
	return "使用 Tavily 搜索引擎搜索网络，获取最新信息。返回结构化结果（标题、网址、摘要）。"
}

func (WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "搜索关键词"
			},
			"max_results": {
				"type": "integer",
				"description": "最大返回结果数（默认5，最多10）"
			}
		},
		"required": ["query"]
	}`)
}

func (t WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("web_search: 参数解析失败: %w", err)
	}
	if params.Query == "" {
		return "", fmt.Errorf("web_search: 搜索关键词为空")
	}
	defaultMax := t.DefaultMaxResults
	if defaultMax <= 0 {
		defaultMax = defaultWebSearchMaxResults
	}
	maxLimit := t.MaxResultsLimit
	if maxLimit <= 0 {
		maxLimit = defaultWebSearchMaxResultsLimit
	}
	if defaultMax > maxLimit {
		defaultMax = maxLimit
	}

	if params.MaxResults <= 0 {
		params.MaxResults = defaultMax
	}
	if params.MaxResults > maxLimit {
		params.MaxResults = maxLimit
	}

	apiKey := t.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("TAVILY_API_KEY")
	}
	if apiKey == "" {
		return "", fmt.Errorf("web_search: 未设置 TAVILY_API_KEY 环境变量，搜索不可用")
	}

	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = defaultWebSearchEndpoint
	}
	httpTimeout := t.HTTPTimeout
	if httpTimeout <= 0 {
		httpTimeout = defaultWebSearchTimeoutSeconds * time.Second
	}

	return tavilySearch(ctx, endpoint, httpTimeout, apiKey, params.Query, params.MaxResults)
}

// tavilySearch 调用 Tavily Search API 并返回结构化的搜索结果。
func tavilySearch(ctx context.Context, endpoint string, timeout time.Duration, apiKey, query string, maxResults int) (string, error) {
	reqBody := map[string]interface{}{
		"query":           query,
		"max_results":     maxResults,
		"search_depth":    "basic",
		"include_answer":  true,
		"include_domains": []string{},
		"exclude_domains": []string{},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("web_search: 序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("web_search: 创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web_search: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var errBody struct {
			Message string `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return "", fmt.Errorf("web_search: API 返回 %d: %s", resp.StatusCode, errBody.Message)
	}

	var result struct {
		Answer  string `json:"answer"`
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("web_search: 解析响应失败: %w", err)
	}

	var sb strings.Builder
	if result.Answer != "" {
		sb.WriteString(result.Answer)
		sb.WriteString("\n\n--- 搜索结果 ---\n")
	}
	for i, r := range result.Results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   网址: %s\n   摘要: %s\n", i+1, r.Title, r.URL, r.Content))
	}
	if sb.Len() == 0 {
		return "未找到相关结果。", nil
	}
	return strings.TrimSpace(sb.String()), nil
}
