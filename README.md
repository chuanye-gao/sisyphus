# Sisyphus

A high-performance AI agent framework written in Go, following Linux standards.

> *Sisyphus, from Greek mythology, eternally pushes a boulder up a hill — never stopping, never giving up. This agent embodies the same relentless execution loop.*

## Quick Start

```bash
# Build
make build

# Set your API key
export OPENAI_API_KEY="sk-..."

# Run a task
./bin/sisyphus --instruction "List all Go files in the current directory"
```

## Architecture

```
  ┌──────────────────────────────────────────┐
  │              Agent Loop                    │
  │                                            │
  │  Task ─► Perceive ─► Think ─► Act ─► Loop │
  │              │          │        │         │
  │          Memory      LLM      Tools        │
  └──────────────────────────────────────────┘
```

- **Agent** (`internal/agent/`): The core execute loop — `perceive → think → act`
- **LLM** (`internal/llm/`): Pluggable LLM provider interface; ships with OpenAI-compatible backend
- **Tools** (`internal/tool/`): Extensible tool system with built-in `bash`, `read_file`, `write_file`, `web_search`
- **Memory** (`internal/memory/`): Token-aware conversation history with automatic trimming
- **Task** (`internal/task/`): Async task queue with worker pool and priority support

## Configuration

Sisyphus searches for configuration in XDG-compliant paths:
1. `$SISYPHUS_CONFIG` (explicit override)
2. `$XDG_CONFIG_HOME/sisyphus/config.yaml`
3. `~/.config/sisyphus/config.yaml`
4. `/etc/sisyphus/config.yaml`

Example `~/.config/sisyphus/config.yaml`:

```yaml
llm:
  provider: openai
  model: gpt-4o
  max_tokens: 4096
  temperature: 0.0
  # api_key: sk-...  # prefer $OPENAI_API_KEY

agent:
  max_steps: 50
  max_concurrent: 4

queue:
  size: 256
  workers: 2

memory:
  max_messages: 100
  max_tokens: 128000
```

## Environment Variables

| Variable | Description |
|---|---|
| `OPENAI_API_KEY` | LLM API key |
| `SISYPHUS_CONFIG` | Config file path override |
| `SISYPHUS_MODEL` | Model name override |
| `SISYPHUS_MAX_STEPS` | Max steps per task (default: 50) |
| `SISYPHUS_WORKERS` | Number of workers (default: 2) |

## Build

```makefile
make build    # Build binary → bin/sisyphus
make release  # Optimized build (stripped)
make test     # Run tests with race detection
make vet      # Run go vet
make lint     # vet + staticcheck
make install  # Install to /usr/local/bin
make clean    # Remove build artifacts
```

## Linux Standards

| Standard | Implementation |
|---|---|
| **FHS config** | `~/.config/sisyphus/`, `/etc/sisyphus/` |
| **FHS data** | `~/.local/share/sisyphus/` |
| **Signals** | SIGTERM/SIGINT → graceful shutdown |
| **Logging** | stdout/stderr (systemd journal compatible) |
| **Exit codes** | 0 on success, 1 on error |
| **Daemon-ready** | signal-driven lifecycle, no daemonize() needed with systemd |

## Performance

- Goroutine pool with bounded concurrency via channel-based worker pool
- `json.RawMessage` for zero-copy tool argument passing
- Context-based deadline propagation to all LLM and tool calls
- Token-count-based memory trimming to prevent context overflow
- `sync.Pool` ready (to be added for hot-path allocation optimization)

## License

MIT
