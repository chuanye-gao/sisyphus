package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/longway/sisyphus/internal/llm"
)

type sseHandler struct {
	w       http.ResponseWriter
	flusher http.Flusher
	debug   bool
	mu      sync.Mutex
	turnID  int
	step    int
}

func newSSEHandler(w http.ResponseWriter, flusher http.Flusher, debug bool) *sseHandler {
	return &sseHandler{w: w, flusher: flusher, debug: debug}
}

func (h *sseHandler) OnTurnStart(input string) {
	h.mu.Lock()
	h.turnID++
	h.step = 0
	turnID := h.turnID
	h.mu.Unlock()

	h.send("turn_start", map[string]any{
		"turn":        turnID,
		"input_chars": len(input),
	})
}

func (h *sseHandler) OnLLMRequest(round int, messages int, tools int) {
	h.send("llm_request", map[string]any{
		"round":    round,
		"messages": messages,
		"tools":    tools,
	})
}

func (h *sseHandler) OnLLMResponse(round int, usage llm.Usage, latency time.Duration, toolCalls int) {
	h.send("llm_response", map[string]any{
		"round":        round,
		"tool_calls":   toolCalls,
		"latency_ms":   latency.Milliseconds(),
		"tokens_in":    usage.PromptTokens,
		"tokens_out":   usage.CompletionTokens,
		"tokens_total": usage.TotalTokens,
	})
}

func (h *sseHandler) OnThinking(delta string) {
	if !h.debug {
		h.send("thinking", map[string]any{"chars": len(delta)})
		return
	}
	h.send("thinking", map[string]string{"delta": delta})
}

func (h *sseHandler) OnContent(delta string) {
	h.send("content", map[string]string{"delta": delta})
}

func (h *sseHandler) OnToolCall(name string, args string) {
	h.mu.Lock()
	h.step++
	step := h.step
	h.mu.Unlock()

	h.send("tool_call", map[string]any{
		"step": step,
		"name": name,
		"args": args,
	})
}

func (h *sseHandler) OnToolResult(name string, result string, elapsed time.Duration) {
	h.send("tool_result", map[string]any{
		"name":       name,
		"result":     result,
		"elapsed_ms": elapsed.Milliseconds(),
	})
}

func (h *sseHandler) OnError(err error) {
	h.send("error", map[string]string{"message": err.Error()})
}

func (h *sseHandler) OnDone(usage llm.Usage, latency time.Duration) {
	h.send("checkpoint", map[string]any{
		"latency_ms":   latency.Milliseconds(),
		"tokens_in":    usage.PromptTokens,
		"tokens_out":   usage.CompletionTokens,
		"tokens_total": usage.TotalTokens,
	})
}

func (h *sseHandler) send(event string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(`{"message":"failed to encode event"}`)
		event = "error"
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintf(h.w, "event: %s\n", event)
	fmt.Fprintf(h.w, "data: %s\n\n", data)
	h.flusher.Flush()
}
