// Package tool defines the Tool interface and a registry for tool management.
// Tools are the actions an agent can take — shell commands, file I/O, web search, etc.
//
// Performance notes:
//   - Tool.Execute receives a context for deadline/cancellation control.
//   - Arguments are json.RawMessage for lazy parsing — the tool decides when to decode.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool is the interface every agent tool must implement.
type Tool interface {
	// Name returns the unique name of this tool (e.g., "bash", "read_file").
	Name() string

	// Description returns a human-readable description sent to the LLM.
	Description() string

	// Parameters returns the JSON Schema for the tool's parameters.
	Parameters() json.RawMessage

	// Execute runs the tool with the given arguments.
	// args is the raw JSON bytes of the parameters, as returned by the LLM.
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// Source describes where a tool came from. It is metadata for display,
// filtering, and policy; the agent loop still executes every tool uniformly.
type Source struct {
	Kind       string // builtin, mcp, plugin, etc.
	Server     string // MCP server or provider name, if any
	RemoteName string // provider-native tool name, if it differs from Name()
}

// Entry is a registered tool plus metadata.
type Entry struct {
	Tool   Tool
	Source Source
}

// Registry manages tool registration and lookup. It is safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Entry // name -> entry
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Entry),
	}
}

// Register adds a tool to the registry. It returns an error if a tool with
// the same name is already registered.
func (r *Registry) Register(t Tool) error {
	return r.RegisterWithSource(t, Source{Kind: "builtin"})
}

// RegisterWithSource adds a tool with source metadata.
func (r *Registry) RegisterWithSource(t Tool, source Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Name()
	if _, ok := r.tools[name]; ok {
		return fmt.Errorf("tool %q already registered", name)
	}
	if source.Kind == "" {
		source.Kind = "builtin"
	}
	r.tools[name] = Entry{Tool: t, Source: source}
	return nil
}

// Get retrieves a tool by name, or nil if not found.
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.tools[name]
	if !ok {
		return nil
	}
	return entry.Tool
}

// Source returns metadata for a registered tool.
func (r *Registry) Source(name string) (Source, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.tools[name]
	return entry.Source, ok
}

// List returns all registered tool names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// All returns all registered tools.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, entry := range r.tools {
		tools = append(tools, entry.Tool)
	}
	return tools
}

// Entries returns all registered tools with metadata.
func (r *Registry) Entries() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]Entry, 0, len(r.tools))
	for _, entry := range r.tools {
		entries = append(entries, entry)
	}
	return entries
}
