// Package repl provides the interactive REPL for Sisyphus.
package repl

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/longway/sisyphus/internal/llm"
	"github.com/longway/sisyphus/internal/trace"
)

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorGray   = "\033[90m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// Renderer handles terminal output for the REPL.
type Renderer struct {
	w          io.Writer
	useColor   bool
	verbose    bool // show full thinking content
	traceRaw   bool
	traceJSON  bool
	mu         sync.Mutex
	inThinking bool // currently streaming thinking content
	inContent  bool // currently streaming content
	thinking   strings.Builder
	turnID     int
	step       int
}

// NewRenderer creates a new Renderer.
// If useColor is true, ANSI escape codes are emitted.
func NewRenderer(w io.Writer, useColor bool, verbose bool, traceRaw bool, traceJSON bool) *Renderer {
	return &Renderer{
		w:         w,
		useColor:  useColor,
		verbose:   verbose,
		traceRaw:  traceRaw,
		traceJSON: traceJSON,
	}
}

// SetVerbose toggles verbose mode (full thinking output).
func (r *Renderer) SetVerbose(v bool) {
	r.mu.Lock()
	r.verbose = v
	r.mu.Unlock()
}

// Verbose returns the current verbose setting.
func (r *Renderer) Verbose() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.verbose
}

// --- StepHandler implementation ---

// OnTurnStart renders a structured trace event for a new user turn.
func (r *Renderer) OnTurnStart(input string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.turnID++
	r.step = 0
	r.inThinking = false
	r.inContent = false
	r.thinking.Reset()
	r.traceLine("INFO", "turn.start",
		trace.F("turn", r.turnID),
		trace.F("input_chars", len(input)),
		trace.F("input", trace.Truncate(input, 120)),
	)
}

// OnLLMRequest renders a structured trace event before a model request.
func (r *Renderer) OnLLMRequest(round int, messages int, tools int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.traceLine("DEBU", "llm.request",
		trace.F("turn", r.turnID),
		trace.F("round", round),
		trace.F("messages", messages),
		trace.F("tools", tools),
	)
}

// OnLLMResponse renders model response metadata.
func (r *Renderer) OnLLMResponse(round int, usage llm.Usage, latency time.Duration, toolCalls int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.traceLine("DEBU", "llm.response",
		trace.F("turn", r.turnID),
		trace.F("round", round),
		trace.F("tool_calls", toolCalls),
		trace.F("latency_ms", latency.Milliseconds()),
		trace.F("tokens_in", usage.PromptTokens),
		trace.F("tokens_out", usage.CompletionTokens),
		trace.F("tokens_total", usage.TotalTokens),
	)
}

// OnThinking renders incremental thinking/reasoning content.
func (r *Renderer) OnThinking(delta string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.thinking.WriteString(delta)
	if !r.inThinking {
		r.inThinking = true
		r.traceLine("DEBU", "llm.thinking.start",
			trace.F("turn", r.turnID),
			trace.F("raw", r.traceRaw || r.verbose),
		)
		return
	}
}

// OnContent renders incremental content from the assistant.
func (r *Renderer) OnContent(delta string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.finishThinking("before_content")

	if !r.inContent {
		r.inContent = true
		// No prefix — just start outputting.
	}
	r.write(delta)
}

// OnToolCall shows that a tool is being invoked.
func (r *Renderer) OnToolCall(name string, args string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.finishStreaming("before_tool_call")
	r.step++
	r.traceLine("INFO", "tool.call",
		trace.F("turn", r.turnID),
		trace.F("step", r.step),
		trace.F("name", name),
		trace.F("args", trace.Truncate(args, 300)),
	)
}

// OnToolResult shows the result of a tool invocation.
func (r *Renderer) OnToolResult(name string, result string, elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	lineCount := 0
	if result != "" {
		lineCount = len(strings.Split(result, "\n"))
	}
	r.traceLine("INFO", "tool.result",
		trace.F("turn", r.turnID),
		trace.F("step", r.step),
		trace.F("name", name),
		trace.F("status", statusFromResult(result)),
		trace.F("elapsed_ms", elapsed.Milliseconds()),
		trace.F("bytes", len(result)),
		trace.F("lines", lineCount),
		trace.F("preview", trace.Truncate(oneLine(result), 300)),
	)
}

// OnError shows an error message.
func (r *Renderer) OnError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.finishStreaming("before_error")
	r.traceLine("ERRO", "step.error",
		trace.F("turn", r.turnID),
		trace.F("error", err.Error()),
	)
}

// OnDone is called when a step completes.
func (r *Renderer) OnDone(usage llm.Usage, latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.finishStreaming("done")

	r.traceLine("DEBU", "turn.checkpoint",
		trace.F("turn", r.turnID),
		trace.F("latency_ms", latency.Milliseconds()),
		trace.F("tokens_in", usage.PromptTokens),
		trace.F("tokens_out", usage.CompletionTokens),
		trace.F("tokens_total", usage.TotalTokens),
	)
}

// Prompt prints the input prompt.
func (r *Renderer) Prompt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.write(r.color(colorCyan+colorBold, "\nsisyphus> "))
}

// Welcome prints the welcome banner.
func (r *Renderer) Welcome(model string, tools []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.write(r.color(colorBold, "Sisyphus"))
	r.write(r.color(colorGray, " — 永不言弃的 AI Agent\n"))
	r.write(r.color(colorGray, fmt.Sprintf("  model: %s | tools: [%s]\n", model, strings.Join(tools, ", "))))
	r.write(r.color(colorGray, "  输入 /help 查看命令，Ctrl+C 中断生成，输入 /exit 退出\n"))
}

// Info prints an info message.
func (r *Renderer) Info(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.write(r.color(colorGray, fmt.Sprintf("  %s\n", msg)))
}

// Success prints a success message.
func (r *Renderer) Success(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.write(r.color(colorGreen, fmt.Sprintf("  %s\n", msg)))
}

// Error prints an error message (public, for REPL use).
func (r *Renderer) Error(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.write(r.color(colorRed, fmt.Sprintf("  ✗ %s\n", msg)))
}

// --- internal helpers ---

// finishStreaming closes any open streaming indicators.
func (r *Renderer) finishStreaming(reason string) {
	r.finishThinking(reason)
	if r.inContent {
		r.inContent = false
		r.write("\n")
	}
}

func (r *Renderer) finishThinking(reason string) {
	if !r.inThinking && r.thinking.Len() == 0 {
		return
	}
	text := r.thinking.String()
	fields := []trace.Field{
		trace.F("turn", r.turnID),
		trace.F("reason", reason),
		trace.F("chars", len(text)),
	}
	if r.traceRaw || r.verbose {
		fields = append(fields, trace.F("text", trace.Truncate(text, rawTraceLimit(r.traceRaw))))
	}
	r.traceLine("DEBU", "llm.thinking.done", fields...)
	r.inThinking = false
	r.thinking.Reset()
}

func (r *Renderer) traceLine(level string, event string, fields ...trace.Field) {
	line := trace.Line(level, "trace", event, fields...)
	if r.traceJSON {
		line = trace.JSONLine(level, "trace", event, fields...)
	}
	switch level {
	case "ERRO", "ERROR":
		r.write(r.color(colorRed, "  "+line+"\n"))
	case "WARN":
		r.write(r.color(colorYellow, "  "+line+"\n"))
	case "DEBU", "DEBUG":
		if r.verbose || r.traceRaw || r.traceJSON {
			r.write(r.color(colorGray, "  "+line+"\n"))
		}
	default:
		r.write(r.color(colorGray, "  "+line+"\n"))
	}
}

func rawTraceLimit(raw bool) int {
	if raw {
		return 8000
	}
	return 800
}

func statusFromResult(result string) string {
	if strings.HasPrefix(strings.TrimSpace(strings.ToLower(result)), "error:") {
		return "error"
	}
	return "ok"
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func (r *Renderer) write(s string) {
	fmt.Fprint(r.w, s)
}

func (r *Renderer) color(code, text string) string {
	if !r.useColor {
		return text
	}
	return code + text + colorReset
}

// IsTerminal returns true if the given file is a terminal.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
