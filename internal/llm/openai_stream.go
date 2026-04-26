package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// compile-time check: OpenAIProvider implements StreamProvider.
var _ StreamProvider = (*OpenAIProvider)(nil)

// oaiTCAccumulator accumulates incremental tool call fragments from OpenAI streaming.
type oaiTCAccumulator struct {
	id          string
	typ         string
	name        string
	argsBuilder strings.Builder
}

// StreamChat implements StreamProvider for OpenAI-compatible APIs.
func (p *OpenAIProvider) StreamChat(ctx context.Context, messages []Message, tools []ToolDef) (<-chan Chunk, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)

	req := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    toOpenAIMessages(messages),
		MaxTokens:   p.config.MaxTokens,
		Temperature: float32(p.config.Temperature),
		Stream:      true,
		StreamOptions: &openai.StreamOptions{
			IncludeUsage: true,
		},
	}

	if len(tools) > 0 {
		req.Tools = toOpenAITools(tools)
	}

	start := time.Now()
	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("openai stream: %w", err)
	}

	ch := make(chan Chunk, 32)

	go func() {
		defer cancel()
		defer stream.Close()
		defer close(ch)

		var accumulators []oaiTCAccumulator

		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				// Stream ended normally.
				finalChunk := Chunk{
					Done:    true,
					Latency: time.Since(start),
				}
				if len(accumulators) > 0 {
					finalChunk.ToolCalls = assembleOAIToolCalls(accumulators)
				}
				select {
				case ch <- finalChunk:
				case <-ctx.Done():
				}
				return
			}
			if err != nil {
				select {
				case ch <- Chunk{Done: true, Err: fmt.Errorf("openai stream: recv: %w", err), Latency: time.Since(start)}:
				case <-ctx.Done():
				}
				return
			}

			if len(resp.Choices) == 0 {
				// Usage-only chunk (when stream_options.include_usage is true).
				if resp.Usage != nil {
					select {
					case ch <- Chunk{
						Usage: Usage{
							PromptTokens:     resp.Usage.PromptTokens,
							CompletionTokens: resp.Usage.CompletionTokens,
							TotalTokens:      resp.Usage.TotalTokens,
						},
					}:
					case <-ctx.Done():
						return
					}
				}
				continue
			}

			delta := resp.Choices[0].Delta

			chunk := Chunk{
				Role:         delta.Role,
				ContentDelta: delta.Content,
			}

			// Accumulate tool call fragments.
			for _, tc := range delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				for len(accumulators) <= idx {
					accumulators = append(accumulators, oaiTCAccumulator{})
				}
				if tc.ID != "" {
					accumulators[idx].id = tc.ID
				}
				if tc.Type != "" {
					accumulators[idx].typ = string(tc.Type)
				}
				if tc.Function.Name != "" {
					accumulators[idx].name = tc.Function.Name
				}
				accumulators[idx].argsBuilder.WriteString(tc.Function.Arguments)
			}

			// Check finish reason.
			if resp.Choices[0].FinishReason != "" {
				chunk.Done = true
				chunk.Latency = time.Since(start)
				if len(accumulators) > 0 {
					chunk.ToolCalls = assembleOAIToolCalls(accumulators)
				}
			}

			select {
			case ch <- chunk:
			case <-ctx.Done():
				return
			}

			if chunk.Done {
				return
			}
		}
	}()

	return ch, nil
}

// assembleOAIToolCalls converts accumulated fragments into complete ToolCalls.
func assembleOAIToolCalls(accs []oaiTCAccumulator) []ToolCall {
	out := make([]ToolCall, 0, len(accs))
	for _, a := range accs {
		if a.name == "" {
			continue
		}
		out = append(out, ToolCall{
			ID:   a.id,
			Type: a.typ,
			Function: FunctionCall{
				Name:      a.name,
				Arguments: json.RawMessage(a.argsBuilder.String()),
			},
		})
	}
	return out
}
