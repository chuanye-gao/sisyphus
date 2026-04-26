package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// compile-time interface check
var _ Provider = (*DeepSeekProvider)(nil)

// DeepSeekProvider implements Provider for DeepSeek's API.
// It uses a raw HTTP client to handle the reasoning_content field
// that DeepSeek thinking models require in both request and response,
// which the go-openai library does not support natively.
type DeepSeekProvider struct {
	config OpenAIConfig
	client *http.Client
}

// NewDeepSeek creates a new DeepSeek provider.
func NewDeepSeek(cfg OpenAIConfig) *DeepSeekProvider {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	// Normalize: strip trailing slash
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	return &DeepSeekProvider{
		config: cfg,
		client: &http.Client{},
	}
}

// dsMessage is the wire format for a single message in the DeepSeek API.
// reasoning_content must be passed back when it was present in a previous assistant response.
type dsMessage struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []dsReqToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
}

// dsReqToolCall is the outgoing wire format for tool calls.
// DeepSeek (like OpenAI) expects arguments as a JSON-encoded string, not an object.
type dsReqToolCall struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // must be a JSON string, not an object
	} `json:"function"`
}

type dsRequest struct {
	Model       string      `json:"model"`
	Messages    []dsMessage `json:"messages"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature float64     `json:"temperature"`
	Tools       []ToolDef   `json:"tools,omitempty"`
}

// dsRespToolCall is used only when parsing responses.
// The DeepSeek API (like OpenAI) returns arguments as a JSON-encoded string,
// not as a JSON object. Using string here lets encoding/json decode it
// correctly; we convert to json.RawMessage afterward.
type dsRespToolCall struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // arrives as a string, e.g. "{\"command\":\"dir\"}"
	} `json:"function"`
}

type dsResponse struct {
	Choices []struct {
		Message struct {
			Role             string           `json:"role"`
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			ToolCalls        []dsRespToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// convertRespToolCalls converts parsed response tool calls to the internal type.
func convertRespToolCalls(in []dsRespToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(in))
	for _, tc := range in {
		out = append(out, ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: FunctionCall{
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}
	return out
}

// convertToReqToolCalls converts internal ToolCall (json.RawMessage arguments)
// to the outgoing wire format (string arguments) that DeepSeek expects.
func convertToReqToolCalls(in []ToolCall) []dsReqToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]dsReqToolCall, len(in))
	for i, tc := range in {
		out[i].ID = tc.ID
		out[i].Type = tc.Type
		out[i].Function.Name = tc.Function.Name
		out[i].Function.Arguments = string(tc.Function.Arguments)
	}
	return out
}

// Chat implements Provider.
func (p *DeepSeekProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	// Convert internal messages to DeepSeek wire format,
	// carrying reasoning_content for assistant turns.
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

	req := dsRequest{
		Model:       p.config.Model,
		Messages:    dsMessages,
		MaxTokens:   p.config.MaxTokens,
		Temperature: p.config.Temperature,
	}
	if len(tools) > 0 {
		req.Tools = tools
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek: marshal request: %w", err)
	}

	endpoint := p.config.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("deepseek: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	start := time.Now()
	httpResp, err := p.client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("deepseek: http: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("deepseek: read response: %w", err)
	}

	var ds dsResponse
	if err := json.Unmarshal(respBody, &ds); err != nil {
		return nil, fmt.Errorf("deepseek: unmarshal response (status %d): %w", httpResp.StatusCode, err)
	}

	if ds.Error != nil {
		return nil, fmt.Errorf("deepseek: api error [%s]: %s", ds.Error.Code, ds.Error.Message)
	}

	if len(ds.Choices) == 0 {
		return nil, fmt.Errorf("deepseek: no choices in response (status %d)", httpResp.StatusCode)
	}

	msg := ds.Choices[0].Message
	resp := &Response{
		Role:             msg.Role,
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolCalls:        convertRespToolCalls(msg.ToolCalls),
		Latency:          latency,
	}
	if ds.Usage != nil {
		resp.Usage = Usage{
			PromptTokens:     ds.Usage.PromptTokens,
			CompletionTokens: ds.Usage.CompletionTokens,
			TotalTokens:      ds.Usage.TotalTokens,
		}
	}
	return resp, nil
}
