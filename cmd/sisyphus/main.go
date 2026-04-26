// Sisyphus — A high-performance AI agent framework.
//
// Usage:
//
//	sisyphus [flags]
//
// Flags:
//
//	--instruction  Task instruction to run (single-shot mode)
//	--config       Path to config file (overrides XDG search)
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
	"syscall"
	"time"

	"github.com/longway/sisyphus/internal/agent"
	"github.com/longway/sisyphus/internal/llm"
	"github.com/longway/sisyphus/internal/task"
	"github.com/longway/sisyphus/internal/tool"
	"github.com/longway/sisyphus/internal/tool/builtin"
	"github.com/longway/sisyphus/pkg/config"
)

func main() {
	// Parse flags
	instruction := flag.String("instruction", "", "Task instruction to run")
	configPath := flag.String("config", "", "Config file path")
	flag.Parse()

	// Load configuration
	if *configPath != "" {
		os.Setenv("SISYPHUS_CONFIG", *configPath)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("sisyphus: config: %v", err)
	}

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
	provider := llm.NewOpenAI(llm.OpenAIConfig{
		APIKey:      cfg.LLM.APIKey,
		BaseURL:     cfg.LLM.BaseURL,
		Model:       cfg.LLM.Model,
		MaxTokens:   cfg.LLM.MaxTokens,
		Temperature: cfg.LLM.Temperature,
		Timeout:     time.Duration(cfg.LLM.Timeout) * time.Second,
	})

	registry := tool.NewRegistry()
	registerBuiltins(registry, cfg)

	// Create task queue
	queue := task.NewQueue(cfg.Queue.Size)

	// Launch workers
	for i := 0; i < cfg.Queue.Workers; i++ {
		go worker(ctx, i, provider, registry, cfg.Agent.MaxSteps, cfg.Memory, queue)
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

	// No instruction: wait for signal
	<-ctx.Done()
	queue.Close()
	log.Println("sisyphus: shutdown complete")
}

// worker processes tasks from the queue.
func worker(ctx context.Context, id int, provider llm.Provider, registry *tool.Registry, maxSteps int, memCfg config.MemoryConfig, queue *task.Queue) {
	log.Printf("sisyphus: worker %d started", id)
	for t := range queue.Chan() {
		queue.Track(t)
		agent.RunTask(ctx, provider, registry, t, maxSteps, memCfg)
		queue.Untrack(t)
	}
	log.Printf("sisyphus: worker %d stopped", id)
}

// registerBuiltins registers the standard built-in tools.
func registerBuiltins(r *tool.Registry, cfg *config.Config) {
	bashTool := builtin.NewBashTool(cfg.Tools.Bash.TimeoutSeconds)
	r.Register(bashTool)
	r.Register(builtin.NewReadFileTool(bashTool))
	r.Register(builtin.NewWriteFileTool(bashTool))
	r.Register(builtin.NewWebSearchTool(
		cfg.Tools.WebSearch.APIKey,
		cfg.Tools.WebSearch.Endpoint,
		cfg.Tools.WebSearch.TimeoutSeconds,
		cfg.Tools.WebSearch.DefaultMaxResults,
		cfg.Tools.WebSearch.MaxResultsLimit,
	))
}

var taskCounter int

func newTaskID() string {
	taskCounter++
	return fmt.Sprintf("task-%d-%d", os.Getpid(), taskCounter)
}
