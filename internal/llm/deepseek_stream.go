package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// compile-time check: DeepSeekProvider implements StreamProvider.
var _ StreamProvider = (*DeepSeekProvider)(nil)

// dsStreamDelta is the delta object inside a streaming choice.
type dsStreamDelta struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content"`
	ToolCalls        []dsStreamTCDelta `json:"tool_calls"`
}

// dsStreamTCDelta represents an incremental tool call fragment in a streaming response.
// The index field tells us which tool call this fragment belongs to.
type dsStreamTCDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`       // present only on first fragment
	Type     string `json:"type,omitempty"`      // present only on first fragment
	Function struct {
		Name      string `json:"name,omitempty"`      // present only on first fragment
		Arguments string `json:"arguments,omitempty"` // incremental JSON string fragment
	} `json:"function"`
}

// dsStreamChunk is one SSE data payload from the DeepSeek streaming API.
type dsStreamChunk struct {
	Choices []struct {
		Index        int           `json:"index"`
		Delta        dsStreamDelta `json:"delta"`
		FinishReason *string       `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// toolCallAccumulator accumulates incremental tool call fragments.
type toolCallAccumulator struct {
	id       string
	typ      string
	name     string
	argsBuilder strings.Builder
}

// StreamChat implements StreamProvider for DeepSeek.
func (p *DeepSeekProvider) StreamChat(ctx context.Context, messages []Message, tools []ToolDef) (<-chan Chunk, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)

	// Build request — same as Chat but with stream: true.
	dsMessages := make([]dsMessage, len(messages))
	for i, m := range messages {
		dsMessages[i] = dsMessage{
			Role:             m.Role,
			Content:          m.Content,
			ReasoningContent: m.ReasoningContent,
			ToolCalls:        convertToReqToolCalls(m.ToolCalls),
			ToolCallID:       m.ToolCallID,
		}
	}

	type dsStreamRequest struct {
		Model       string      `json:"model"`
		Messages    []dsMessage `json:"messages"`
		MaxTokens   int         `json:"max_tokens,omitempty"`
		Temperature float64     `json:"temperature"`
		Tools       []ToolDef   `json:"tools,omitempty"`
		Stream      bool        `json:"stream"`
		StreamOptions *struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options,omitempty"`
	}

	req := dsStreamRequest{
		Model:       p.config.Model,
		Messages:    dsMessages,
		MaxTokens:   p.config.MaxTokens,
		Temperature: p.config.Temperature,
		Stream:      true,
		StreamOptions: &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true},
	}
	if len(tools) > 0 {
		req.Tools = tools
	}

	body, err := json.Marshal(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("deepseek stream: marshal: %w", err)
	}

	endpoint := p.config.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("deepseek stream: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("deepseek stream: http: %w", err)
	}

	if httpResp.StatusCode != 200 {
		defer httpResp.Body.Close()
		cancel()
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("deepseek stream: http %d: %s", httpResp.StatusCode, string(respBody))
	}

	ch := make(chan Chunk, 32)

	go func() {
		defer cancel()
		defer httpResp.Body.Close()
		defer close(ch)

		var accumulators []toolCallAccumulator
		scanner := bufio.NewScanner(httpResp.Body)
		// Increase buffer for potentially large chunks.
		scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

		for scanner.Scan() {
			line := scanner.Text()

			// SSE format: "data: {...}" or "data: [DONE]"
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				// Assemble any pending tool calls.
				finalChunk := Chunk{
					Done:    true,
					Latency: time.Since(start),
				}
				if len(accumulators) > 0 {
					finalChunk.ToolCalls = assembleToolCalls(accumulators)
				}
				select {
				case ch <- finalChunk:
				case <-ctx.Done():
				}
				return
			}

			var sc dsStreamChunk
			if err := json.Unmarshal([]byte(data), &sc); err != nil {
				select {
				case ch <- Chunk{Done: true, Err: fmt.Errorf("deepseek stream: parse chunk: %w", err)}:
				case <-ctx.Done():
				}
				return
			}

			if len(sc.Choices) == 0 {
				// Usage-only chunk (sent after [DONE] in some configurations, or as separate chunk).
				if sc.Usage != nil {
					select {
					case ch <- Chunk{
						Usage: Usage{
							PromptTokens:     sc.Usage.PromptTokens,
							CompletionTokens: sc.Usage.CompletionTokens,
							TotalTokens:      sc.Usage.TotalTokens,
						},
					}:
					case <-ctx.Done():
						return
					}
				}
				continue
			}

			delta := sc.Choices[0].Delta

			chunk := Chunk{
				Role:           delta.Role,
				ContentDelta:   delta.Content,
				ReasoningDelta: delta.ReasoningContent,
			}

			// Accumulate tool call fragments.
			for _, tcDelta := range delta.ToolCalls {
				idx := tcDelta.Index
				// Grow the accumulator slice if needed.
				for len(accumulators) <= idx {
					accumulators = append(accumulators, toolCallAccumulator{})
				}
				if tcDelta.ID != "" {
					accumulators[idx].id = tcDelta.ID
				}
				if tcDelta.Type != "" {
					accumulators[idx].typ = tcDelta.Type
				}
				if tcDelta.Function.Name != "" {
					accumulators[idx].name = tcDelta.Function.Name
				}
				accumulators[idx].argsBuilder.WriteString(tcDelta.Function.Arguments)
			}

			// Populate usage if present on this chunk.
			if sc.Usage != nil {
				chunk.Usage = Usage{
					PromptTokens:     sc.Usage.PromptTokens,
					CompletionTokens: sc.Usage.CompletionTokens,
					TotalTokens:      sc.Usage.TotalTokens,
				}
			}

			// Check finish_reason.
			if sc.Choices[0].FinishReason != nil {
				chunk.Done = true
				chunk.Latency = time.Since(start)
				if len(accumulators) > 0 {
					chunk.ToolCalls = assembleToolCalls(accumulators)
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

		if err := scanner.Err(); err != nil {
			select {
			case ch <- Chunk{Done: true, Err: fmt.Errorf("deepseek stream: read: %w", err)}:
			case <-ctx.Done():
			}
		}
	}()

	return ch, nil
}

// assembleToolCalls converts accumulated fragments into complete ToolCalls.
func assembleToolCalls(accs []toolCallAccumulator) []ToolCall {
	out := make([]ToolCall, 0, len(accs))
	for _, a := range accs {
		if a.name == "" {
			continue // skip empty/incomplete entries
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
