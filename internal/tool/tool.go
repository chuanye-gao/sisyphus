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

// Registry manages tool registration and lookup. It is safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool // name -> tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. It returns an error if a tool with
// the same name is already registered.
func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Name()
	if _, ok := r.tools[name]; ok {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// Get retrieves a tool by name, or nil if not found.
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
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
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}
