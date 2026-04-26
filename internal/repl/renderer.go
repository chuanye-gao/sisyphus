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
	mu         sync.Mutex
	inThinking bool // currently streaming thinking content
	inContent  bool // currently streaming content
}

// NewRenderer creates a new Renderer.
// If useColor is true, ANSI escape codes are emitted.
func NewRenderer(w io.Writer, useColor bool, verbose bool) *Renderer {
	return &Renderer{
		w:        w,
		useColor: useColor,
		verbose:  verbose,
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

// OnThinking renders incremental thinking/reasoning content.
func (r *Renderer) OnThinking(delta string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.verbose {
		// In non-verbose mode, show a minimal indicator on the first chunk.
		if !r.inThinking {
			r.inThinking = true
			r.write(r.color(colorGray, "  [thinking...] "))
		}
		return
	}

	// Verbose: stream thinking content in gray.
	if !r.inThinking {
		r.inThinking = true
		r.write(r.color(colorGray, "  💭 "))
	}
	r.write(r.color(colorGray, delta))
}

// OnContent renders incremental content from the assistant.
func (r *Renderer) OnContent(delta string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Close thinking line if needed.
	if r.inThinking {
		r.inThinking = false
		r.write("\n")
	}

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

	r.finishStreaming()
	r.write(r.color(colorYellow, fmt.Sprintf("  ⟶ %s", name)))
	// Show args on the same line, truncated.
	display := args
	if len(display) > 120 {
		display = display[:120] + "..."
	}
	r.write(r.color(colorGray, fmt.Sprintf(" %s\n", display)))
}

// OnToolResult shows the result of a tool invocation.
func (r *Renderer) OnToolResult(name string, result string, elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Truncate long results for display.
	display := result
	lines := strings.Split(display, "\n")
	if len(lines) > 15 {
		display = strings.Join(lines[:15], "\n") + fmt.Sprintf("\n    ... (%d more lines)", len(lines)-15)
	}

	r.write(r.color(colorGray, fmt.Sprintf("    (%s) %s\n", elapsed.Round(time.Millisecond), display)))
}

// OnError shows an error message.
func (r *Renderer) OnError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.finishStreaming()
	r.write(r.color(colorRed, fmt.Sprintf("  ✗ error: %v\n", err)))
}

// OnDone is called when a step completes.
func (r *Renderer) OnDone(usage llm.Usage, latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.finishStreaming()

	if r.verbose && usage.TotalTokens > 0 {
		r.write(r.color(colorGray, fmt.Sprintf(
			"\n  [tokens: %d in / %d out / %d total | %s]\n",
			usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
			latency.Round(time.Millisecond),
		)))
	}
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
func (r *Renderer) finishStreaming() {
	if r.inThinking || r.inContent {
		r.inThinking = false
		r.inContent = false
		r.write("\n")
	}
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
