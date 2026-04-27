# Sisyphus（西西弗斯）

Sisyphus 是一个用 Go 编写的跨平台 AI Agent 框架，支持单次任务执行和交互式 REPL。它把大模型、记忆、任务队列、内置工具和 MCP 服务组合在一起，用来持续推进一个明确的任务。

## 快速开始

```bash
go build -o bin/sisyphus ./cmd/sisyphus
cp config.yaml.example config.yaml
```

设置模型密钥：

```bash
# Linux/macOS
export DEEPSEEK_API_KEY="..."

# Windows PowerShell
$env:DEEPSEEK_API_KEY="..."
```

运行一次性任务：

```bash
./bin/sisyphus --instruction "列出当前目录下所有 Go 文件"
```

不传 `--instruction` 时会进入交互式 REPL：

```bash
./bin/sisyphus
```

## 配置文件

仓库只提交脱敏后的 [config.yaml.example](./config.yaml.example)。本地真实配置文件使用 `config.yaml`，该文件已被 `.gitignore` 忽略，避免把 API key、本机路径等敏感信息提交到 git。

初始化本地配置：

```bash
# Linux/macOS
cp config.yaml.example config.yaml

# Windows PowerShell
Copy-Item config.yaml.example config.yaml
```

配置文件搜索顺序：

1. `SISYPHUS_CONFIG` 指定的路径
2. 当前工作目录下的 `config.yaml`
3. 可执行文件所在目录下的 `config.yaml`
4. Linux/macOS: `$XDG_CONFIG_HOME/sisyphus/config.yaml`
5. Linux/macOS: `~/.config/sisyphus/config.yaml`
6. Linux/macOS: `/etc/sisyphus/config.yaml`
7. Windows: `%APPDATA%/sisyphus/config.yaml`
8. Windows: `%ProgramData%/sisyphus/config.yaml`

示例配置默认使用 DeepSeek：

```yaml
llm:
  provider: deepseek
  model: deepseek-v4-pro
  base_url: https://api.deepseek.com
  api_key: ${DEEPSEEK_API_KEY}
```

如果要使用 OpenAI 兼容服务，可以把 `provider`、`model`、`base_url` 和对应 API key 调整为你的服务配置。

## 环境变量

| 变量 | 说明 |
|---|---|
| `DEEPSEEK_API_KEY` | DeepSeek provider 的 API key |
| `OPENAI_API_KEY` | OpenAI provider 的 API key，也会覆盖配置中的 `llm.api_key` |
| `TAVILY_API_KEY` | Tavily web search API key |
| `SISYPHUS_CONFIG` | 显式指定配置文件路径 |
| `SISYPHUS_MODEL` | 覆盖配置中的模型名 |
| `SISYPHUS_MAX_STEPS` | 覆盖单任务最大步数 |
| `SISYPHUS_WORKERS` | 覆盖任务队列 worker 数量 |
| `SISYPHUS_BASH_TIMEOUT` | 覆盖 bash 工具超时时间，单位秒 |
| `SISYPHUS_WEB_SEARCH_ENDPOINT` | 覆盖 web search endpoint |
| `SISYPHUS_WORKSPACE` | 示例 MCP filesystem server 使用的工作目录占位变量 |

## 命令行参数

```text
--instruction  Task instruction to run (single-shot mode)
--session      Restore a saved interactive session
--config       Path to config file
--debug        Print verbose debug output
--trace-raw    Include raw model reasoning in REPL trace output
--trace-json   Emit REPL trace events as JSONL
```

## 项目结构

```text
cmd/sisyphus        CLI 入口
internal/agent     Agent 执行循环
internal/llm       OpenAI/DeepSeek 兼容 provider
internal/mcp       MCP client 与工具注册
internal/memory    对话记忆与裁剪
internal/repl      交互式命令行界面
internal/task      任务队列
internal/tool      工具接口与内置工具
pkg/config         配置加载与环境变量覆盖
```

## 构建与测试

Linux/macOS：

```bash
make build
make test
make vet
```

Windows PowerShell：

```powershell
go build -o bin/sisyphus.exe ./cmd/sisyphus
go test ./...
go vet ./...
```

跨平台编译：

```bash
GOOS=linux GOARCH=amd64 go build -o bin/sisyphus ./cmd/sisyphus
GOOS=windows GOARCH=amd64 go build -o bin/sisyphus.exe ./cmd/sisyphus
```

## 设计要点

- 通过有限 worker pool 控制并发任务执行。
- 使用 `context.Context` 贯穿超时、取消和信号处理。
- 内置文件读写、搜索、bash、web search 等工具。
- 支持通过 MCP server 动态注册外部工具。
- 支持单次任务模式和交互式 REPL 模式。

## License

MIT
