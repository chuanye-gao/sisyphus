// Sisyphus — A high-performance AI agent framework.
//
// Usage:
//
//	sisyphus [flags]
//
// Flags:
//
//	--instruction  Task instruction to run (single-shot mode)
//	--session      Restore a saved interactive session
//	--config       Path to config file (overrides XDG search)
//	--debug        Print verbose debug output
//
// Modes:
//
//	With --instruction: single-shot mode (run one task, exit)
//	Without --instruction: interactive REPL mode
//
// Environment:
//
//	OPENAI_API_KEY        LLM API key (required)
//	TAVILY_API_KEY        Tavily web search API key (optional if tools.web_search.api_key is set)
//	SISYPHUS_CONFIG       Config file path
//	SISYPHUS_MODEL        Model name override
//	SISYPHUS_MAX_STEPS    Max steps per task
//	SISYPHUS_WORKERS      Number of worker goroutines
//	SISYPHUS_BASH_TIMEOUT Bash tool timeout (seconds)
//
// Signals:
//
//	SIGTERM, SIGINT — graceful shutdown: drain queue, cancel running tasks.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/longway/sisyphus/internal/agent"
	"github.com/longway/sisyphus/internal/llm"
	"github.com/longway/sisyphus/internal/mcp"
	"github.com/longway/sisyphus/internal/repl"
	"github.com/longway/sisyphus/internal/task"
	"github.com/longway/sisyphus/internal/tool"
	"github.com/longway/sisyphus/internal/tool/builtin"
	"github.com/longway/sisyphus/pkg/config"
)

func main() {
	// Parse flags
	instruction := flag.String("instruction", "", "Task instruction to run (single-shot mode)")
	session := flag.String("session", "", "Restore a saved interactive session by ID")
	configPath := flag.String("config", "", "Config file path")
	debug := flag.Bool("debug", false, "Print tool call arguments, results, and model reasoning")
	flag.Parse()

	// Load configuration
	if *configPath != "" {
		os.Setenv("SISYPHUS_CONFIG", *configPath)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("sisyphus: config: %v", err)
	}
	logConfigSummary(cfg)

	log.Printf("sisyphus: starting (model=%s, maxSteps=%d, workers=%d)",
		cfg.LLM.Model, cfg.Agent.MaxSteps, cfg.Queue.Workers)

	if cfg.LLM.APIKey == "" {
		log.Fatal("sisyphus: no API key configured. Set OPENAI_API_KEY or llm.api_key in config")
	}

	// Create signal-aware context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		log.Printf("sisyphus: received %v, shutting down gracefully", sig)
		cancel()
	}()

	// Assemble components
	providerCfg := llm.OpenAIConfig{
		APIKey:      cfg.LLM.APIKey,
		BaseURL:     cfg.LLM.BaseURL,
		Model:       cfg.LLM.Model,
		MaxTokens:   cfg.LLM.MaxTokens,
		Temperature: cfg.LLM.Temperature,
		Timeout:     time.Duration(cfg.LLM.Timeout) * time.Second,
	}
	var provider llm.Provider
	switch cfg.LLM.Provider {
	case "deepseek":
		provider = llm.NewDeepSeek(providerCfg)
		log.Printf("sisyphus: using DeepSeek provider")
	default:
		provider = llm.NewOpenAI(providerCfg)
		log.Printf("sisyphus: using OpenAI provider")
	}

	registry := tool.NewRegistry()
	registerBuiltins(registry, cfg)
	mcpManager, err := mcp.StartConfigured(ctx, cfg.MCP, registry)
	if err != nil {
		log.Printf("sisyphus: MCP startup warning: %v", err)
	}
	logToolSummary(registry, *debug)
	defer func() {
		if err := mcpManager.Close(); err != nil {
			log.Printf("sisyphus: MCP shutdown warning: %v", err)
		}
	}()

	// Create task queue
	queue := task.NewQueue(cfg.Queue.Size)

	// Launch workers
	for i := 0; i < cfg.Queue.Workers; i++ {
		go worker(ctx, i, provider, registry, cfg.Agent.MaxSteps, cfg.Memory, *debug, queue)
	}

	// Single-shot mode: submit the instruction as a task
	if *instruction != "" {
		t := task.New(newTaskID(), *instruction)
		if !queue.Submit(t) {
			log.Fatal("sisyphus: queue is full")
		}
		log.Printf("sisyphus: submitted task %s: %s", t.ID, *instruction)

		// Wait for task to complete
		for {
			if t.Status == task.StatusCompleted || t.Status == task.StatusFailed || t.Status == task.StatusCancelled {
				break
			}
			select {
			case <-ctx.Done():
				log.Printf("sisyphus: shutdown while waiting for task %s", t.ID)
				return
			case <-time.After(500 * time.Millisecond):
			}
		}

		if t.Status == task.StatusCompleted {
			log.Printf("sisyphus: task %s completed (%d steps)", t.ID, t.Steps)
			fmt.Println(t.Result)
		} else {
			log.Printf("sisyphus: task %s failed: %s", t.ID, t.Error)
			os.Exit(1)
		}

		// Shutdown
		cancel()
		queue.Close()
		return
	}

	// Interactive REPL mode
	r, err := repl.New(repl.Config{
		Provider:  provider,
		Registry:  registry,
		Cfg:       cfg,
		SessionID: *session,
		Debug:     *debug,
	})
	if err != nil {
		log.Fatalf("sisyphus: repl: %v", err)
	}
	if err := r.Run(ctx); err != nil {
		log.Printf("sisyphus: repl exited: %v", err)
	}
	queue.Close()
	log.Println("sisyphus: shutdown complete")
}

// worker processes tasks from the queue.
func worker(ctx context.Context, id int, provider llm.Provider, registry *tool.Registry, maxSteps int, memCfg config.MemoryConfig, debug bool, queue *task.Queue) {
	log.Printf("sisyphus: worker %d started", id)
	for t := range queue.Chan() {
		queue.Track(t)
		agent.RunTask(ctx, provider, registry, t, maxSteps, memCfg, debug)
		queue.Untrack(t)
	}
	log.Printf("sisyphus: worker %d stopped", id)
}

// registerBuiltins registers the standard built-in tools.
func registerBuiltins(r *tool.Registry, cfg *config.Config) {
	registerBuiltin(r, cfg, builtin.NewBashTool(cfg.Tools.Bash.TimeoutSeconds))
	registerBuiltin(r, cfg, builtin.SafeReadFileTool{})
	registerBuiltin(r, cfg, builtin.SafeWriteFileTool{})
	registerBuiltin(r, cfg, builtin.EditFileTool{})
	registerBuiltin(r, cfg, builtin.ListFilesTool{})
	registerBuiltin(r, cfg, builtin.SearchTool{})
	registerBuiltin(r, cfg, builtin.NewWebSearchTool(
		cfg.Tools.WebSearch.APIKey,
		cfg.Tools.WebSearch.Endpoint,
		cfg.Tools.WebSearch.TimeoutSeconds,
		cfg.Tools.WebSearch.DefaultMaxResults,
		cfg.Tools.WebSearch.MaxResultsLimit,
	))
}

func registerBuiltin(r *tool.Registry, cfg *config.Config, t tool.Tool) {
	if !cfg.Tools.BuiltinToolEnabled(t.Name()) {
		return
	}
	if err := r.RegisterWithSource(t, tool.Source{Kind: "builtin"}); err != nil {
		log.Printf("sisyphus: builtin tool %s skipped: %v", t.Name(), err)
	}
}

func logConfigSummary(cfg *config.Config) {
	if cfg.Path == "" {
		log.Printf("sisyphus: config loaded from defaults (no config file found)")
	} else {
		log.Printf("sisyphus: config loaded from %s", cfg.Path)
	}

	if len(cfg.Tools.Enabled) == 0 {
		log.Printf("sisyphus: builtin tools policy: enabled=all disabled=[%s]", strings.Join(cfg.Tools.Disabled, ", "))
	} else {
		log.Printf("sisyphus: builtin tools policy: enabled=[%s] disabled=[%s]",
			strings.Join(cfg.Tools.Enabled, ", "), strings.Join(cfg.Tools.Disabled, ", "))
	}

	var enabled []string
	var disabled []string
	for _, server := range cfg.MCP {
		if server.IsEnabled() {
			enabled = append(enabled, server.Name)
		} else {
			disabled = append(disabled, server.Name)
		}
	}
	sort.Strings(enabled)
	sort.Strings(disabled)
	log.Printf("sisyphus: MCP servers configured=%d enabled=[%s] disabled=[%s]",
		len(cfg.MCP), strings.Join(enabled, ", "), strings.Join(disabled, ", "))
}

func logToolSummary(r *tool.Registry, verbose bool) {
	counts := make(map[string]int)
	names := make(map[string][]string)
	for _, entry := range r.Entries() {
		key := entry.Source.Kind
		if key == "" {
			key = "unknown"
		}
		if entry.Source.Server != "" {
			key += ":" + entry.Source.Server
		}
		counts[key]++
		names[key] = append(names[key], entry.Tool.Name())
	}

	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	log.Printf("sisyphus: registered tools: %s", strings.Join(parts, ", "))

	if !verbose {
		return
	}
	for _, key := range keys {
		sort.Strings(names[key])
		log.Printf("sisyphus: registered tools [%s]: [%s]", key, strings.Join(names[key], ", "))
	}
}

var taskCounter int

func newTaskID() string {
	taskCounter++
	return fmt.Sprintf("task-%d-%d", os.Getpid(), taskCounter)
}
