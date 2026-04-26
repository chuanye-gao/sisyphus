package llm

import (
	"context"
	"fmt"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// compile-time interface check
var _ Provider = (*OpenAIProvider)(nil)

// OpenAIProvider implements Provider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	client *openai.Client
	model  string
	config OpenAIConfig
}

// OpenAIConfig holds configuration for the OpenAI provider.
type OpenAIConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	MaxTokens   int
	Temperature float64
	Timeout     time.Duration
}

// NewOpenAI creates a new OpenAI-compatible provider.
func NewOpenAI(cfg OpenAIConfig) *OpenAIProvider {
	clientCfg := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		clientCfg.BaseURL = cfg.BaseURL
	}
	// Rely on context.Context for deadline control, not HTTP client timeout.
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}

	return &OpenAIProvider{
		client: openai.NewClientWithConfig(clientCfg),
		model:  cfg.Model,
		config: cfg,
	}
}

// Chat implements Provider.
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	// Apply per-call deadline
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model:       p.model,
		Messages:    toOpenAIMessages(messages),
		MaxTokens:   p.config.MaxTokens,
		Temperature: float32(p.config.Temperature),
	}

	if len(tools) > 0 {
		req.Tools = toOpenAITools(tools)
	}

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai chat: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	choice := resp.Choices[0]

	return &Response{
		Role:    choice.Message.Role,
		Content: choice.Message.Content,
		ToolCalls: fromOpenAIToolCalls(choice.Message.ToolCalls),
	}, nil
}

// toOpenAIMessages converts internal Message to OpenAI format.
func toOpenAIMessages(msgs []Message) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		oam := openai.ChatCompletionMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			oam.ToolCalls = append(oam.ToolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolType(tc.Type),
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: string(tc.Function.Arguments),
				},
			})
		}
		out = append(out, oam)
	}
	return out
}

// toOpenAITools converts internal ToolDef to OpenAI format.
func toOpenAITools(tools []ToolDef) []openai.Tool {
	out := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		out = append(out, openai.Tool{
			Type: openai.ToolType(t.Type),
			Function: &openai.FunctionDefinition{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}
	return out
}

// fromOpenAIToolCalls converts OpenAI tool calls back to internal format.
func fromOpenAIToolCalls(calls []openai.ToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(calls))
	for _, tc := range calls {
		out = append(out, ToolCall{
			ID:   tc.ID,
			Type: string(tc.Type),
			Function: FunctionCall{
				Name:      tc.Function.Name,
				Arguments: []byte(tc.Function.Arguments),
			},
		})
	}
	return out
}
