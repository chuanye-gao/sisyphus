// Package llm — streaming extension.
//
// StreamProvider is an optional interface that LLM providers can implement
// to support token-by-token streaming. The REPL interactive mode uses this
// for real-time output; the single-shot mode continues to use Chat().
package llm

import (
	"context"
	"time"
)

// Chunk represents one incremental piece of a streaming response.
// Fields are additive: each chunk carries only the new delta since the last one.
type Chunk struct {
	// Role is set on the first chunk only (typically "assistant").
	Role string

	// ContentDelta is the incremental text content.
	ContentDelta string

	// ReasoningDelta is the incremental thinking/reasoning content (DeepSeek).
	ReasoningDelta string

	// ToolCalls carries fully-assembled tool calls.
	// They are delivered once the stream has finished accumulating all argument fragments.
	// Callers should check Done to know when tool calls are final.
	ToolCalls []ToolCall

	// Usage is populated on the final chunk (when Done == true).
	Usage Usage

	// Latency is populated on the final chunk — total time from request to stream end.
	Latency time.Duration

	// Done signals the end of the stream.
	Done bool

	// Err, if non-nil, indicates a stream-level error. Done will also be true.
	Err error
}

// StreamProvider extends Provider with streaming support.
// Implementations must also satisfy Provider (for non-streaming fallback).
type StreamProvider interface {
	Provider

	// StreamChat sends messages to the LLM and returns a channel of incremental chunks.
	// The channel is closed when the stream ends (the final chunk has Done == true).
	// Callers should drain the channel even after receiving Done to avoid goroutine leaks.
	StreamChat(ctx context.Context, messages []Message, tools []ToolDef) (<-chan Chunk, error)
}
