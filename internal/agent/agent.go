// Package agent implements the core Sisyphus agent loop: Perceive → Think → Act.
//
// The agent is the heart of Sisyphus. It takes a task instruction, engages
// the LLM in a multi-step reasoning loop, executes tools on the agent's behalf,
// and returns a final result. Each agent runs in its own goroutine and is
// controlled via context.Context.
//
// The name "Sisyphus" reflects the agent's nature: it pushes its task forward
// step by step, even in the face of failures, retrying and adapting until the
// work is done — or until it runs out of allotted steps.
package agent

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/longway/sisyphus/internal/llm"
	"github.com/longway/sisyphus/internal/memory"
	"github.com/longway/sisyphus/internal/task"
	"github.com/longway/sisyphus/internal/tool"
	"github.com/longway/sisyphus/pkg/config"
)

// Agent executes tasks using an LLM and a set of tools.
// Each Agent is self-contained and safe for concurrent use.
type Agent struct {
	provider llm.Provider
	memory   *memory.Memory
	registry *tool.Registry
	maxSteps int
	debug    bool
}

// New creates a new Agent.
func New(provider llm.Provider, mem *memory.Memory, registry *tool.Registry, maxSteps int, debug bool) *Agent {
	if maxSteps <= 0 {
		maxSteps = 50
	}
	return &Agent{
		provider: provider,
		memory:   mem,
		registry: registry,
		maxSteps: maxSteps,
		debug:    debug,
	}
}

// Run executes a task and returns the final result.
// The context controls the lifetime of the entire run.
func (a *Agent) Run(ctx context.Context, t *task.Task) {
	taskStart := time.Now()
	t.Start()
	log.Printf("[agent] task %s started: %s", t.ID, t.Instruction)

	// System prompt that instructs the LLM how to behave.
	sysPrompt := buildSystemPromptV2()
	a.memory.Add(llm.Message{
		Role:    "system",
		Content: sysPrompt,
	})

	// The user's instruction
	a.memory.Add(llm.Message{
		Role:    "user",
		Content: t.Instruction,
	})

	toolDefs := buildToolDefs(a.registry)

	// Debug: print initial setup
	if a.debug {
		log.Printf("[debug] ========== TASK INIT ==========")
		log.Printf("[debug] system prompt (%d chars):\n%s", len(sysPrompt), sysPrompt)
		var names []string
		for _, td := range toolDefs {
			names = append(names, td.Function.Name)
		}
		log.Printf("[debug] tools registered: [%s]", strings.Join(names, ", "))
		log.Printf("[debug] max steps: %d", a.maxSteps)
		log.Printf("[debug] ================================")
	}

	for step := 1; step <= a.maxSteps; step++ {
		select {
		case <-ctx.Done():
			t.SetCancelled()
			log.Printf("[agent] task %s cancelled at step %d", t.ID, step)
			return
		default:
		}

		log.Printf("[agent] task %s step %d/%d", t.ID, step, a.maxSteps)

		// Debug: memory state before LLM call
		if a.debug {
			log.Printf("[debug] memory: %d messages, ~%d tokens", a.memory.Len(), a.memory.TokenCount())
		}

		stepStart := time.Now()
		resp, err := a.provider.Chat(ctx, a.memory.All(), toolDefs)
		if err != nil {
			t.SetError(fmt.Errorf("step %d: llm call: %w", step, err), step)
			log.Printf("[agent] task %s step %d: error: %v", t.ID, step, err)
			return
		}

		// Debug: API response metadata
		if a.debug {
			log.Printf("[debug] --- LLM response ---")
			log.Printf("[debug] latency: %s", resp.Latency)
			log.Printf("[debug] usage: prompt=%d, completion=%d, total=%d",
				resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
			if resp.ReasoningContent != "" {
				log.Printf("[debug] thinking:\n%s", resp.ReasoningContent)
			}
			if resp.Content != "" {
				log.Printf("[debug] content:\n%s", resp.Content)
			}
			log.Printf("[debug] tool_calls: %d", len(resp.ToolCalls))
		}

		// Store the assistant message once, with both content and tool calls.
		a.memory.Add(llm.Message{
			Role:             resp.Role,
			Content:          resp.Content,
			ReasoningContent: resp.ReasoningContent,
			ToolCalls:        resp.ToolCalls,
		})

		// No tool calls: this is the final answer.
		if len(resp.ToolCalls) == 0 {
			t.SetResult(resp.Content, step)
			elapsed := time.Since(taskStart)
			log.Printf("[agent] task %s completed in %d steps (%s)", t.ID, step, elapsed)
			return
		}

		// Execute each tool call and feed results back into memory.
		for _, tc := range resp.ToolCalls {
			toolName := tc.Function.Name
			toolArgs := tc.Function.Arguments

			log.Printf("[agent] task %s step %d: calling tool %q", t.ID, step, toolName)
			if a.debug {
				log.Printf("[debug] call:   %s %s", toolName, string(toolArgs))
			}

			tl := a.registry.Get(toolName)
			if tl == nil {
				errMsg := fmt.Sprintf("error: tool %q not found", toolName)
				if a.debug {
					log.Printf("[debug] result: %s", errMsg)
				}
				a.memory.Add(llm.Message{
					Role:       "tool",
					Content:    errMsg,
					ToolCallID: tc.ID,
				})
				continue
			}

			toolStart := time.Now()
			result, err := tl.Execute(ctx, toolArgs)
			toolElapsed := time.Since(toolStart)
			if err != nil {
				result = fmt.Sprintf("error: %v", err)
			}

			if a.debug {
				log.Printf("[debug] result (%s): %s", toolElapsed, result)
			}

			a.memory.Add(llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}

		// Debug: step summary
		if a.debug {
			log.Printf("[debug] step %d total: %s", step, time.Since(stepStart))
		}
	}

	elapsed := time.Since(taskStart)
	t.SetError(fmt.Errorf("exceeded max steps (%d)", a.maxSteps), a.maxSteps)
	log.Printf("[agent] task %s exceeded max steps (%s elapsed)", t.ID, elapsed)
}

func buildSystemPromptV2() string {
	base := `You are Sisyphus, a small CLI coding agent for everyday programmer work.

Mission:
- Understand the user's goal, inspect the workspace, make focused changes, and verify them.
- Prefer narrow, reversible edits over broad rewrites.
- Keep going until the task is done or a real blocker is found.

Operating loop:
1. Inspect first. Use list_files, search, and read_file before editing.
2. Edit safely. Use edit_file for small changes and write_file only for new files or full replacements.
3. Run the smallest useful verification command after changes.
4. If a tool fails, read the error, adjust, and try a better next step.
5. Final answers should be concise: say what changed, how it was verified, and any remaining risk.

Built-in skills:
- codebase_explore: list top-level files, read entry points, search for relevant symbols, then summarize the shape of the code.
- bug_fix: reproduce or inspect the failure, find the narrow cause, patch it, then run the relevant tests.
- test_failure_debug: read the failing output, locate the code and tests involved, patch the smallest cause, rerun the failing test first.

Tool discipline:
- Do not invent files or APIs. Inspect them.
- Do not use bash for file reads/writes when read_file, write_file, search, list_files, or edit_file can do it.
- Use bash for build, test, git, and project commands.
- For destructive commands, explain the risk and avoid them unless explicitly requested.`

	if runtime.GOOS == "windows" {
		return base + "\n\nRuntime: Windows. When using bash, commands run through cmd /c, so prefer Windows-compatible commands."
	}
	return base + "\n\nRuntime: Unix-like. When using bash, prefer portable POSIX shell commands."
}

// buildSystemPrompt creates the system prompt, adapting to the current OS.
func buildSystemPrompt() string {
	base := `你是 Sisyphus，一个永不言弃的 AI agent。你的职责是逐步完成交给你的任务。

规则：
1. 将复杂任务拆分为小而可操作的步骤。
2. 使用可用工具与系统交互。
3. 每次工具调用后，评估结果并决定下一步。
4. 任务完成后，给出清晰的总结。
5. 如果工具调用失败，尝试替代方案。保持韧性。
6. 保持简洁——只输出当前步骤所需的内容。`

	if runtime.GOOS == "windows" {
		return base + "\n\n你运行在 Windows 系统上。使用 cmd 命令（dir 而非 ls，type 而非 cat 等）。"
	}
	return base + "\n\n你运行在 Linux 系统上。尽量使用标准 POSIX 命令。"
}

// buildToolDefs converts registered tools to LLM-compatible definitions.
func buildToolDefs(registry *tool.Registry) []llm.ToolDef {
	tools := registry.All()
	defs := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}

// RunTask is a convenience function that runs a single task synchronously.
// It creates an ephemeral memory that is discarded after execution.
func RunTask(ctx context.Context, provider llm.Provider, registry *tool.Registry, t *task.Task, maxSteps int, memCfg config.MemoryConfig, debug bool) {
	mem, err := memory.New(memCfg, "")
	if err != nil {
		t.SetError(fmt.Errorf("agent: create memory: %w", err), 0)
		return
	}
	ag := New(provider, mem, registry, maxSteps, debug)
	ag.Run(ctx, t)
}

// ---------------------------------------------------------------------------
// Interactive (REPL) support
// ---------------------------------------------------------------------------

// StepHandler receives streaming events during an interactive Step().
// All methods are called from the agent goroutine; implementations must be
// safe for that call pattern (typically writing to stdout sequentially).
type StepHandler interface {
	OnThinking(delta string)
	OnContent(delta string)
	OnToolCall(name string, args string)
	OnToolResult(name string, result string, elapsed time.Duration)
	OnError(err error)
	OnDone(usage llm.Usage, latency time.Duration)
}

// InitMemory injects the system prompt into memory.
// Call once when starting a new REPL session, or skip if loading a saved session.
func (a *Agent) InitMemory() {
	a.memory.Add(llm.Message{
		Role:    "system",
		Content: buildSystemPromptV2(),
	})
}

// Memory returns the agent's memory, for save/load operations.
func (a *Agent) Memory() *memory.Memory {
	return a.memory
}

// ToolDefs returns LLM tool definitions for the registered tools.
func (a *Agent) ToolDefs() []llm.ToolDef {
	return buildToolDefs(a.registry)
}

// Step processes one round of user input in interactive mode.
// It adds the user message, calls the LLM (streaming if available),
// executes any tool calls in a loop, and returns when the LLM produces
// a final text response (no tool calls).
//
// The handler receives streaming events in real time.
// Step may run multiple LLM calls internally (when tools are invoked).
func (a *Agent) Step(ctx context.Context, userInput string, handler StepHandler) error {
	a.memory.Add(llm.Message{
		Role:    "user",
		Content: userInput,
	})

	toolDefs := a.ToolDefs()

	// Check if provider supports streaming.
	sp, canStream := a.provider.(llm.StreamProvider)

	for round := 0; round < a.maxSteps; round++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if canStream {
			if err := a.stepStream(ctx, sp, toolDefs, handler); err != nil {
				return err
			}
		} else {
			if err := a.stepSync(ctx, toolDefs, handler); err != nil {
				return err
			}
		}

		// Check if the last assistant message has tool calls.
		msgs := a.memory.All()
		lastAssistant := msgs[len(msgs)-1]
		// If the last message is a tool result, keep looking back for the assistant.
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "assistant" {
				lastAssistant = msgs[i]
				break
			}
		}

		if len(lastAssistant.ToolCalls) == 0 {
			// Final answer delivered — done.
			return nil
		}

		// Tool calls were made and results are in memory.
		// Continue the loop to send them back to the LLM.
	}

	return fmt.Errorf("exceeded max steps (%d) in interactive mode", a.maxSteps)
}

// stepStream performs one LLM call using streaming, accumulates the full
// response, executes any tool calls, and writes everything into memory.
func (a *Agent) stepStream(ctx context.Context, sp llm.StreamProvider, toolDefs []llm.ToolDef, handler StepHandler) error {
	ch, err := sp.StreamChat(ctx, a.memory.All(), toolDefs)
	if err != nil {
		handler.OnError(err)
		return err
	}

	var (
		fullContent   strings.Builder
		fullReasoning strings.Builder
		finalCalls    []llm.ToolCall
		role          string
		usage         llm.Usage
		latency       time.Duration
	)

	for chunk := range ch {
		if chunk.Err != nil {
			handler.OnError(chunk.Err)
			return chunk.Err
		}
		if chunk.Role != "" {
			role = chunk.Role
		}
		if chunk.ReasoningDelta != "" {
			fullReasoning.WriteString(chunk.ReasoningDelta)
			handler.OnThinking(chunk.ReasoningDelta)
		}
		if chunk.ContentDelta != "" {
			fullContent.WriteString(chunk.ContentDelta)
			handler.OnContent(chunk.ContentDelta)
		}
		if len(chunk.ToolCalls) > 0 {
			finalCalls = chunk.ToolCalls
		}
		if chunk.Usage.TotalTokens > 0 {
			usage = chunk.Usage
		}
		if chunk.Latency > 0 {
			latency = chunk.Latency
		}
	}

	if role == "" {
		role = "assistant"
	}

	// Store the assistant message.
	a.memory.Add(llm.Message{
		Role:             role,
		Content:          fullContent.String(),
		ReasoningContent: fullReasoning.String(),
		ToolCalls:        finalCalls,
	})

	// Execute tool calls if any.
	if len(finalCalls) > 0 {
		a.executeToolCalls(ctx, finalCalls, handler)
	}

	handler.OnDone(usage, latency)
	return nil
}

// stepSync performs one LLM call without streaming (fallback path).
func (a *Agent) stepSync(ctx context.Context, toolDefs []llm.ToolDef, handler StepHandler) error {
	resp, err := a.provider.Chat(ctx, a.memory.All(), toolDefs)
	if err != nil {
		handler.OnError(err)
		return err
	}

	if resp.ReasoningContent != "" {
		handler.OnThinking(resp.ReasoningContent)
	}
	if resp.Content != "" {
		handler.OnContent(resp.Content)
	}

	a.memory.Add(llm.Message{
		Role:             resp.Role,
		Content:          resp.Content,
		ReasoningContent: resp.ReasoningContent,
		ToolCalls:        resp.ToolCalls,
	})

	if len(resp.ToolCalls) > 0 {
		a.executeToolCalls(ctx, resp.ToolCalls, handler)
	}

	handler.OnDone(resp.Usage, resp.Latency)
	return nil
}

// executeToolCalls runs each tool and feeds results back into memory.
func (a *Agent) executeToolCalls(ctx context.Context, calls []llm.ToolCall, handler StepHandler) {
	for _, tc := range calls {
		toolName := tc.Function.Name
		toolArgs := tc.Function.Arguments

		handler.OnToolCall(toolName, string(toolArgs))

		tl := a.registry.Get(toolName)
		if tl == nil {
			errMsg := fmt.Sprintf("error: tool %q not found", toolName)
			handler.OnToolResult(toolName, errMsg, 0)
			a.memory.Add(llm.Message{
				Role:       "tool",
				Content:    errMsg,
				ToolCallID: tc.ID,
			})
			continue
		}

		toolStart := time.Now()
		result, err := tl.Execute(ctx, toolArgs)
		toolElapsed := time.Since(toolStart)
		if err != nil {
			result = fmt.Sprintf("error: %v", err)
		}

		handler.OnToolResult(toolName, result, toolElapsed)
		a.memory.Add(llm.Message{
			Role:       "tool",
			Content:    result,
			ToolCallID: tc.ID,
		})
	}
}
