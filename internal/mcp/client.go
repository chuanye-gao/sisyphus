package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/longway/sisyphus/internal/tool"
	"github.com/longway/sisyphus/pkg/config"
)

const (
	defaultTimeout = 30 * time.Second
	protocolVer    = "2024-11-05"
)

// Manager owns MCP server processes and their registered tool adapters.
type Manager struct {
	clients []*Client
}

// StartConfigured starts MCP servers and registers their tools. A server that
// fails to start is returned as an error; callers can decide whether to abort.
func StartConfigured(ctx context.Context, servers []config.MCPServerConfig, registry *tool.Registry) (*Manager, error) {
	m := &Manager{}
	var errs []error

	for _, server := range servers {
		if !server.IsEnabled() {
			continue
		}

		client, err := Start(ctx, server)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", server.Name, err))
			continue
		}

		tools, err := client.ListTools(ctx)
		if err != nil {
			client.Close()
			errs = append(errs, fmt.Errorf("%s: list tools: %w", server.Name, err))
			continue
		}

		registered := true
		for _, remote := range tools {
			if !server.ToolEnabled(remote.RemoteName) {
				continue
			}
			adapter := NewToolAdapter(client, remote)
			if err := registry.RegisterWithSource(adapter, tool.Source{
				Kind:       "mcp",
				Server:     server.Name,
				RemoteName: remote.RemoteName,
			}); err != nil {
				errs = append(errs, fmt.Errorf("%s: register %s: %w", server.Name, adapter.Name(), err))
				registered = false
				continue
			}
		}
		if !registered {
			client.Close()
			continue
		}
		m.clients = append(m.clients, client)
	}

	if len(errs) > 0 {
		return m, errors.Join(errs...)
	}
	return m, nil
}

// Close stops all managed MCP server processes.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	var errs []error
	for _, client := range m.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Client is a minimal stdio MCP JSON-RPC client.
type Client struct {
	name    string
	command string
	args    []string
	timeout time.Duration

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr bytes.Buffer

	nextID  atomic.Int64
	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[int64]chan rpcResponse

	closeOnce sync.Once
	closed    chan struct{}
}

// Start launches one stdio MCP server and performs the initialize handshake.
func Start(ctx context.Context, cfg config.MCPServerConfig) (*Client, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("missing name")
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, fmt.Errorf("missing command")
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = mergeEnv(cfg.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	client := &Client{
		name:    cfg.Name,
		command: cfg.Command,
		args:    cfg.Args,
		timeout: timeout,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		pending: make(map[int64]chan rpcResponse),
		closed:  make(chan struct{}),
	}
	cmd.Stderr = &client.stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", cfg.Command, err)
	}
	go client.readLoop()

	initCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := client.initialize(initCtx); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": protocolVer,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "sisyphus",
			"version": "0.1.0",
		},
	}
	if _, err := c.request(ctx, "initialize", params); err != nil {
		return fmt.Errorf("initialize: %w%s", err, c.stderrSuffix())
	}
	if err := c.notify("notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}
	return nil
}

// ListTools returns the remote tools exposed by the server.
func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error) {
	resp, err := c.requestWithDefaultTimeout(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []RemoteTool `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	for i := range result.Tools {
		result.Tools[i].ServerName = c.name
		result.Tools[i].ToolName = safeToolName(c.name, result.Tools[i].RemoteName)
	}
	return result.Tools, nil
}

// CallTool invokes a remote MCP tool.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var decoded any
	if len(args) == 0 {
		decoded = map[string]any{}
	} else if err := json.Unmarshal(args, &decoded); err != nil {
		return "", fmt.Errorf("decode tool arguments: %w", err)
	}

	resp, err := c.requestWithDefaultTimeout(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": decoded,
	})
	if err != nil {
		return "", err
	}

	var result callToolResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("decode tools/call: %w", err)
	}
	text := result.String()
	if result.IsError {
		return text, fmt.Errorf("mcp tool error: %s", text)
	}
	return text, nil
}

// Close terminates the MCP process.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		if c.cmd == nil || c.cmd.Process == nil {
			return
		}

		done := make(chan error, 1)
		go func() {
			done <- c.cmd.Wait()
		}()

		select {
		case waitErr := <-done:
			err = waitErr
		case <-time.After(750 * time.Millisecond):
			_ = c.cmd.Process.Kill()
			<-done
		}
		if c.stdout != nil {
			_ = c.stdout.Close()
		}
	})
	return err
}

func (c *Client) requestWithDefaultTimeout(ctx context.Context, method string, params any) (json.RawMessage, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.request(reqCtx, method, params)
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	respCh := make(chan rpcResponse, 1)

	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	msg := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := c.write(msg); err != nil {
		return nil, err
	}

	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, fmt.Errorf("mcp server %q closed", c.name)
	}
}

func (c *Client) notify(method string, params any) error {
	return c.write(rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (c *Client) write(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write rpc: %w", err)
	}
	return nil
}

func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var resp rpcResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		if resp.ID == 0 {
			continue
		}

		c.pendingMu.Lock()
		ch := c.pending[resp.ID]
		c.pendingMu.Unlock()
		if ch == nil {
			continue
		}
		select {
		case ch <- resp:
		case <-c.closed:
			return
		}
	}
}

func (c *Client) stderrSuffix() string {
	s := strings.TrimSpace(c.stderr.String())
	if s == "" {
		return ""
	}
	if len(s) > 1000 {
		s = s[len(s)-1000:]
	}
	return ": " + s
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Data) == 0 {
		return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("rpc error %d: %s: %s", e.Code, e.Message, string(e.Data))
}

// RemoteTool is one tool exposed by an MCP server.
type RemoteTool struct {
	ServerName  string
	ToolName    string
	RemoteName  string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolAdapter adapts one MCP tool to the local tool.Tool interface.
type ToolAdapter struct {
	client *Client
	remote RemoteTool
}

func NewToolAdapter(client *Client, remote RemoteTool) ToolAdapter {
	return ToolAdapter{client: client, remote: remote}
}

func (t ToolAdapter) Name() string { return t.remote.ToolName }

func (t ToolAdapter) Description() string {
	desc := strings.TrimSpace(t.remote.Description)
	if desc == "" {
		desc = "MCP tool"
	}
	return fmt.Sprintf("[MCP:%s:%s] %s", t.remote.ServerName, t.remote.RemoteName, desc)
}

func (t ToolAdapter) Parameters() json.RawMessage {
	if len(t.remote.InputSchema) == 0 || string(t.remote.InputSchema) == "null" {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return t.remote.InputSchema
}

func (t ToolAdapter) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.client.CallTool(ctx, t.remote.RemoteName, args)
}

type callToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
		Data string `json:"data,omitempty"`
		Mime string `json:"mimeType,omitempty"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func (r callToolResult) String() string {
	if len(r.Content) == 0 {
		return ""
	}
	parts := make([]string, 0, len(r.Content))
	for _, item := range r.Content {
		switch item.Type {
		case "text":
			parts = append(parts, item.Text)
		case "image":
			parts = append(parts, fmt.Sprintf("[image %s, %d bytes base64]", item.Mime, len(item.Data)))
		default:
			if item.Text != "" {
				parts = append(parts, item.Text)
			} else {
				parts = append(parts, fmt.Sprintf("[%s content]", item.Type))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func mergeEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

var unsafeToolName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func safeToolName(serverName, remoteName string) string {
	server := sanitizeName(serverName)
	remote := sanitizeName(remoteName)
	if server == "" {
		server = "server"
	}
	if remote == "" {
		remote = "tool"
	}
	name := "mcp__" + server + "__" + remote
	if len(name) <= 64 {
		return name
	}
	sum := crc32.ChecksumIEEE([]byte(serverName + "\x00" + remoteName))
	suffix := fmt.Sprintf("__%08x", sum)
	keep := 64 - len(suffix)
	if keep < 1 {
		keep = 1
	}
	return strings.TrimRight(name[:keep], "_-") + suffix
}

func sanitizeName(name string) string {
	name = unsafeToolName.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_-")
	return name
}
