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
}

// New creates a new Agent.
func New(provider llm.Provider, mem *memory.Memory, registry *tool.Registry, maxSteps int) *Agent {
	if maxSteps <= 0 {
		maxSteps = 50
	}
	return &Agent{
		provider: provider,
		memory:   mem,
		registry: registry,
		maxSteps: maxSteps,
	}
}

// Run executes a task and returns the final result.
// The context controls the lifetime of the entire run.
func (a *Agent) Run(ctx context.Context, t *task.Task) {
	t.Start()
	log.Printf("[agent] task %s started: %s", t.ID, t.Instruction)

	// System prompt that instructs the LLM how to behave.
	systemMsg := llm.Message{
		Role:    "system",
		Content: buildSystemPrompt(t.Instruction),
	}
	a.memory.Add(systemMsg)

	// The user's instruction
	a.memory.Add(llm.Message{
		Role:    "user",
		Content: t.Instruction,
	})

	toolDefs := buildToolDefs(a.registry)

	for step := 1; step <= a.maxSteps; step++ {
		select {
		case <-ctx.Done():
			t.SetCancelled()
			log.Printf("[agent] task %s cancelled at step %d", t.ID, step)
			return
		default:
		}

		log.Printf("[agent] task %s step %d/%d", t.ID, step, a.maxSteps)

		resp, err := a.provider.Chat(ctx, a.memory.All(), toolDefs)
		if err != nil {
			t.SetError(fmt.Errorf("step %d: llm call: %w", step, err), step)
			log.Printf("[agent] task %s step %d: error: %v", t.ID, step, err)
			return
		}

		// LLM returned a text response (final or intermediate)
		if resp.Content != "" {
			a.memory.Add(llm.Message{
				Role:    resp.Role,
				Content: resp.Content,
			})

			// If no tool calls, the agent considers this the final answer
			if len(resp.ToolCalls) == 0 {
				t.SetResult(resp.Content, step)
				log.Printf("[agent] task %s completed in %d steps", t.ID, step)
				return
			}
		}

		// Execute tool calls
		if len(resp.ToolCalls) > 0 {
			// Save the assistant message with tool calls
			a.memory.Add(llm.Message{
				Role:      resp.Role,
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})

			for _, tc := range resp.ToolCalls {
				toolName := tc.Function.Name
				toolArgs := tc.Function.Arguments

				log.Printf("[agent] task %s step %d: calling tool %q", t.ID, step, toolName)

				tl := a.registry.Get(toolName)
				if tl == nil {
					a.memory.Add(llm.Message{
						Role:       "tool",
						Content:    fmt.Sprintf("error: tool %q not found", toolName),
						ToolCallID: tc.ID,
					})
					continue
				}

				result, err := tl.Execute(ctx, toolArgs)
				if err != nil {
					result = fmt.Sprintf("error: %v", err)
				}

				a.memory.Add(llm.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
		}
	}

	// Max steps exhausted: ask the LLM for a final summary
	t.SetError(fmt.Errorf("exceeded max steps (%d)", a.maxSteps), a.maxSteps)
	log.Printf("[agent] task %s exceeded max steps", t.ID)
}

// buildSystemPrompt 创建初始系统消息，根据当前操作系统自适应。
func buildSystemPrompt(instruction string) string {
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
func RunTask(ctx context.Context, provider llm.Provider, registry *tool.Registry, t *task.Task, maxSteps int) {
	cfg := config.MemoryConfig{MaxMessages: 100, MaxTokens: 128000}
	mem, err := memory.New(cfg, "")
	if err != nil {
		t.SetError(fmt.Errorf("agent: create memory: %w", err), 0)
		return
	}
	ag := New(provider, mem, registry, maxSteps)
	ag.Run(ctx, t)
}
