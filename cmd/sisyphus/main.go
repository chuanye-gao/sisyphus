// Sisyphus — A high-performance AI agent framework.
//
// Usage:
//
//	sisyphus [flags]
//	sisyphus web [--addr 127.0.0.1:7357]
//
// Flags:
//
//	--instruction  Task instruction to run (single-shot mode)
//	--session      Restore a saved interactive session
//	--config       Path to config file (overrides XDG search)
//	--debug        Print verbose debug output
//	--trace-raw    Include raw model reasoning in REPL trace output
//	--trace-json   Emit REPL trace events as JSONL
//
// Modes:
//
//	With --instruction: single-shot mode (run one task, exit)
//	Without --instruction: interactive REPL mode
//	With web: local browser UI mode
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
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/longway/sisyphus/internal/agent"
	"github.com/longway/sisyphus/internal/llm"
	"github.com/longway/sisyphus/internal/logger"
	"github.com/longway/sisyphus/internal/mcp"
	"github.com/longway/sisyphus/internal/repl"
	"github.com/longway/sisyphus/internal/task"
	"github.com/longway/sisyphus/internal/tool"
	"github.com/longway/sisyphus/internal/tool/builtin"
	webui "github.com/longway/sisyphus/internal/webui"
	"github.com/longway/sisyphus/pkg/config"
)

const defaultWebAddr = "127.0.0.1:7357"

func main() {
	var (
		instruction string
		session     string
		configPath  string
		debug       bool
		traceRaw    bool
		traceJSON   bool
		webMode     bool
		webAddr     = defaultWebAddr
	)

	if len(os.Args) > 1 && os.Args[1] == "web" {
		webMode = true
		webFlags := flag.NewFlagSet("web", flag.ExitOnError)
		webFlags.StringVar(&webAddr, "addr", defaultWebAddr, "Address for the local web UI")
		webFlags.StringVar(&session, "session", "", "Restore a saved interactive session by ID")
		webFlags.StringVar(&configPath, "config", "", "Config file path")
		webFlags.BoolVar(&debug, "debug", false, "Print tool call arguments, results, and model reasoning")
		webFlags.Parse(os.Args[2:])
	} else {
		flag.StringVar(&instruction, "instruction", "", "Task instruction to run (single-shot mode)")
		flag.StringVar(&session, "session", "", "Restore a saved interactive session by ID")
		flag.StringVar(&configPath, "config", "", "Config file path")
		flag.BoolVar(&debug, "debug", false, "Print tool call arguments, results, and model reasoning")
		flag.BoolVar(&traceRaw, "trace-raw", false, "Include raw model reasoning in REPL trace output")
		flag.BoolVar(&traceJSON, "trace-json", false, "Emit REPL trace events as JSONL")
		flag.Parse()
	}

	// Logger
	log := logger.New("main", debug)

	// Load configuration
	if configPath != "" {
		os.Setenv("SISYPHUS_CONFIG", configPath)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	logConfigSummary(log, cfg)

	log.Info("starting (model=%s, maxSteps=%d, workers=%d)",
		cfg.LLM.Model, cfg.Agent.MaxSteps, cfg.Queue.Workers)

	if cfg.LLM.APIKey == "" {
		log.Fatal("no API key configured. Set OPENAI_API_KEY or llm.api_key in config")
	}

	// Create signal-aware context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		log.Info("received %v, shutting down gracefully", sig)
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
		log.Info("using DeepSeek provider")
	default:
		provider = llm.NewOpenAI(providerCfg)
		log.Info("using OpenAI provider")
	}

	registry := tool.NewRegistry()
	registerBuiltins(registry, cfg)

	if webMode {
		logToolSummary(log, registry, debug)
		startMCPAsync(ctx, log, cfg, registry, debug)

		srv, err := webui.New(webui.Config{
			Provider:  provider,
			Registry:  registry,
			Cfg:       cfg,
			SessionID: session,
			Debug:     debug,
		})
		if err != nil {
			log.Fatalf("web: %v", err)
		}

		httpServer := &http.Server{
			Addr:              webAddr,
			Handler:           srv.Handler(),
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			<-ctx.Done()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				log.Warn("web shutdown: %v", err)
			}
		}()

		log.Info("web UI listening on http://%s", webAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("web: %v", err)
		}
		log.Info("shutdown complete")
		return
	}

	mcpManager, err := mcp.StartConfigured(ctx, cfg.MCP, registry)
	if err != nil {
		log.Warn("MCP startup: %v", err)
	}
	logToolSummary(log, registry, debug)
	defer func() {
		if err := mcpManager.Close(); err != nil {
			log.Warn("MCP shutdown: %v", err)
		}
	}()

	// Create task queue
	queue := task.NewQueue(cfg.Queue.Size)

	// Launch workers
	for i := 0; i < cfg.Queue.Workers; i++ {
		go worker(ctx, i, provider, registry, cfg.Agent.MaxSteps, cfg.Memory, debug, queue)
	}

	// Single-shot mode: submit the instruction as a task
	if instruction != "" {
		t := task.New(newTaskID(), instruction)
		if !queue.Submit(t) {
			log.Fatal("queue is full")
		}
		log.Info("submitted task %s: %s", t.ID, instruction)

		// Wait for task to complete
		for {
			if t.Status == task.StatusCompleted || t.Status == task.StatusFailed || t.Status == task.StatusCancelled {
				break
			}
			select {
			case <-ctx.Done():
				log.Warn("shutdown while waiting for task %s", t.ID)
				return
			case <-time.After(500 * time.Millisecond):
			}
		}

		if t.Status == task.StatusCompleted {
			log.Info("task %s completed (%d steps)", t.ID, t.Steps)
			fmt.Println(t.Result)
		} else {
			log.Error("task %s failed: %s", t.ID, t.Error)
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
		SessionID: session,
		Debug:     debug,
		TraceRaw:  traceRaw,
		TraceJSON: traceJSON,
	})
	if err != nil {
		log.Fatalf("repl: %v", err)
	}
	if err := r.Run(ctx); err != nil {
		log.Error("repl exited: %v", err)
	}
	queue.Close()
	log.Info("shutdown complete")
}

// worker processes tasks from the queue.
func worker(ctx context.Context, id int, provider llm.Provider, registry *tool.Registry, maxSteps int, memCfg config.MemoryConfig, debug bool, queue *task.Queue) {
	wlog := logger.New("worker", debug)
	wlog.Debug("worker %d started", id)
	for t := range queue.Chan() {
		queue.Track(t)
		agent.RunTask(ctx, provider, registry, t, maxSteps, memCfg, debug)
		queue.Untrack(t)
	}
	wlog.Debug("worker %d stopped", id)
}

func startMCPAsync(ctx context.Context, log *logger.Logger, cfg *config.Config, registry *tool.Registry, debug bool) {
	go func() {
		manager, err := mcp.StartConfigured(ctx, cfg.MCP, registry)
		if err != nil {
			log.Warn("MCP startup: %v", err)
		}
		logToolSummary(log, registry, debug)

		<-ctx.Done()
		if err := manager.Close(); err != nil {
			log.Warn("MCP shutdown: %v", err)
		}
	}()
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
		// Use a temporary logger here since we don't have one in scope.
		// This is called during startup from main() which has its own logger.
		l := logger.New("main", false)
		l.Warn("builtin tool %s skipped: %v", t.Name(), err)
	}
}

func logConfigSummary(l *logger.Logger, cfg *config.Config) {
	if cfg.Path == "" {
		l.Info("config loaded from defaults (no config file found)")
	} else {
		l.Info("config loaded from %s", cfg.Path)
	}

	if len(cfg.Tools.Enabled) == 0 {
		l.Info("builtin tools policy: enabled=all disabled=[%s]", strings.Join(cfg.Tools.Disabled, ", "))
	} else {
		l.Info("builtin tools policy: enabled=[%s] disabled=[%s]",
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
	l.Info("MCP servers configured=%d enabled=[%s] disabled=[%s]",
		len(cfg.MCP), strings.Join(enabled, ", "), strings.Join(disabled, ", "))
}

func logToolSummary(l *logger.Logger, r *tool.Registry, verbose bool) {
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
	l.Info("registered tools: %s", strings.Join(parts, ", "))

	if !verbose {
		return
	}
	for _, key := range keys {
		sort.Strings(names[key])
		l.Debug("registered tools [%s]: [%s]", key, strings.Join(names[key], ", "))
	}
}

var taskCounter int

func newTaskID() string {
	taskCounter++
	return fmt.Sprintf("task-%d-%d", os.Getpid(), taskCounter)
}
