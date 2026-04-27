package repl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/longway/sisyphus/internal/agent"
	"github.com/longway/sisyphus/internal/llm"
	"github.com/longway/sisyphus/internal/memory"
	"github.com/longway/sisyphus/internal/tool"
	"github.com/longway/sisyphus/pkg/config"
)

// REPL is the interactive read-eval-print loop for Sisyphus.
type REPL struct {
	agent    *agent.Agent
	renderer *Renderer
	session  *SessionManager
	provider llm.Provider
	registry *tool.Registry
	cfg      *config.Config
	debug    bool

	sessionID string // current session ID
}

// Config holds REPL creation parameters.
type Config struct {
	Provider  llm.Provider
	Registry  *tool.Registry
	Cfg       *config.Config
	SessionID string // empty = new session
	Debug     bool
	TraceRaw  bool
	TraceJSON bool
}

// New creates a new REPL.
func New(rc Config) (*REPL, error) {
	dataDir := config.DataDir()
	sm := NewSessionManager(dataDir)

	mem, err := memory.New(rc.Cfg.Memory, sm.SessionDir())
	if err != nil {
		return nil, fmt.Errorf("repl: create memory: %w", err)
	}

	ag := agent.New(rc.Provider, mem, rc.Registry, rc.Cfg.Agent.MaxSteps, rc.Debug)

	useColor := IsTerminal(os.Stdout)
	renderer := NewRenderer(os.Stdout, useColor, rc.Debug, rc.TraceRaw, rc.TraceJSON)

	sessionID := rc.SessionID
	if sessionID != "" {
		// Restore existing session.
		if err := mem.Load(sessionID); err != nil {
			return nil, fmt.Errorf("repl: load session %s: %w", sessionID, err)
		}
	} else {
		// New session — inject system prompt.
		sessionID = sm.NewSessionID()
		ag.InitMemory()
	}

	return &REPL{
		agent:     ag,
		renderer:  renderer,
		session:   sm,
		provider:  rc.Provider,
		registry:  rc.Registry,
		cfg:       rc.Cfg,
		debug:     rc.Debug,
		sessionID: sessionID,
	}, nil
}

// Run starts the interactive loop. It blocks until the user exits or ctx is cancelled.
func (r *REPL) Run(ctx context.Context) error {
	// Welcome banner.
	toolNames := r.registry.List()
	r.renderer.Welcome(r.cfg.LLM.Model, toolNames)

	scanner := bufio.NewScanner(os.Stdin)
	// Allow long inputs.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		r.renderer.Prompt()

		if !scanner.Scan() {
			// EOF (Ctrl+D) or read error.
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Multi-line input: lines starting with """ enter multi-line mode.
		if strings.HasPrefix(line, `"""`) {
			line = r.readMultiLine(scanner, line)
		}

		// Slash command dispatch.
		if strings.HasPrefix(line, "/") {
			quit := r.handleCommand(ctx, line)
			if quit {
				return nil
			}
			continue
		}

		// Normal input — run a Step.
		// Create a cancellable sub-context so Ctrl+C can interrupt generation.
		stepCtx, stepCancel := context.WithCancel(ctx)

		err := r.agent.Step(stepCtx, line, r.renderer)
		stepCancel()

		if err != nil {
			if ctx.Err() != nil {
				// Parent context cancelled — full shutdown.
				return ctx.Err()
			}
			r.renderer.Error(fmt.Sprintf("step failed: %v", err))
		}

		// Auto-save after each turn.
		if saveErr := r.agent.Memory().Save(r.sessionID); saveErr != nil {
			r.renderer.Error(fmt.Sprintf("auto-save failed: %v", saveErr))
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("repl: scanner: %w", err)
	}

	return nil
}

// readMultiLine reads until a closing """ is found.
func (r *REPL) readMultiLine(scanner *bufio.Scanner, firstLine string) string {
	var sb strings.Builder
	// Strip the opening """ from the first line.
	rest := strings.TrimPrefix(firstLine, `"""`)
	if rest != "" {
		sb.WriteString(rest)
		sb.WriteByte('\n')
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == `"""` {
			break
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}
