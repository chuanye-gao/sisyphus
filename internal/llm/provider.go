// Package llm provides an abstraction layer over LLM providers.
// It defines a common Provider interface and ships with an OpenAI-compatible
// implementation that works with OpenAI, DeepSeek, vLLM, and similar APIs.
package llm

import (
	"context"
	"encoding/json"
	"time"
)

// Message represents a single chat message.
type Message struct {
	Role             string     `json:"role"` // "system", "user", "assistant", "tool"
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"` // DeepSeek thinking mode
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a request from the LLM to invoke a tool.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the function name and arguments.
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"` // zero-copy: parsed lazily by tool
}

// ToolDef defines a tool for the LLM to use.
type ToolDef struct {
	Type     string      `json:"type"` // "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef describes a function the LLM can call.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Usage contains token usage statistics from an LLM call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Response is the result of a Chat call.
type Response struct {
	Role             string        `json:"role"`
	Content          string        `json:"content"`
	ReasoningContent string        `json:"reasoning_content,omitempty"` // DeepSeek thinking mode
	ToolCalls        []ToolCall    `json:"tool_calls,omitempty"`
	Usage            Usage         `json:"usage,omitempty"`   // token usage stats
	Latency          time.Duration `json:"-"`                 // round-trip time of the API call
}

// Provider is the interface all LLM backends must implement.
type Provider interface {
	// Chat sends messages to the LLM and returns the response.
	// If tools are provided, the LLM may return tool_calls instead of content.
	Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error)
}
