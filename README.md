# Sisyphus（西西弗斯）

高性能 AI Agent 框架，Go 语言编写，跨平台支持 Windows 和 Linux。

> *西西弗斯，希腊神话中永不停歇推石上山的人物。这个 agent 继承了同样的精神——每一步都在推进任务，即使失败也会重试、适应，直至工作完成。*

## 快速开始

```bash
# 编译
go build -o bin/sisyphus ./cmd/sisyphus

# 设置 API 密钥
# Linux/macOS:
export OPENAI_API_KEY="sk-..."
# Windows (PowerShell):
$env:OPENAI_API_KEY="sk-..."

# 运行任务
./bin/sisyphus --instruction "列出当前目录下所有 Go 文件"
```

> **提示**：不使用 `make` 也可以直接 `go build`，因为 Makefile 在 Windows 上需要额外安装。项目提供 Makefile 给 Linux 用户带来便利，同时也提供等效的 PowerShell 脚本。

## 架构

```
  ┌──────────────────────────────────────────┐
  │              Agent 执行循环                │
  │                                            │
  │  任务 ─► 感知 ─► 思考 ─► 行动 ─► 循环     │
  │            │        │         │            │
  │          记忆库   大模型     工具集         │
  └──────────────────────────────────────────┘
```

- **Agent**（`internal/agent/`）：核心执行循环 —— 感知 → 思考 → 行动
- **LLM**（`internal/llm/`）：可插拔的大模型接口，首发 OpenAI 兼容后端（同时支持 DeepSeek、vLLM 等）
- **Tools**（`internal/tool/`）：可扩展的工具系统，内置 `bash`、`read_file`、`write_file`、`web_search`
- **Memory**（`internal/memory/`）：基于 token 计数的对话历史管理，自动裁剪超出的上下文
- **Task**（`internal/task/`）：异步任务队列，支持 worker pool 并发消费

## 配置

Sisyphus 按以下顺序搜索配置文件（自动适配操作系统）：

### Linux / macOS
1. `$SISYPHUS_CONFIG`（显式指定）
2. `$XDG_CONFIG_HOME/sisyphus/config.yaml`
3. `~/.config/sisyphus/config.yaml`
4. `/etc/sisyphus/config.yaml`

### Windows
1. `%SISYPHUS_CONFIG%`（显式指定）
2. `%APPDATA%/sisyphus/config.yaml`
3. `%ProgramData%/sisyphus/config.yaml`

配置示例 `config.yaml`：


仓库根目录提供了一个可维护的 `config.yaml` 模板（包含 `llm.base_url`、`llm.api_key`、`mcp_servers`，且不包含 `skills`）。可直接复制到系统配置路径，或通过 `SISYPHUS_CONFIG` 指向该文件。

```yaml
llm:
  provider: openai
  model: gpt-4o
  max_tokens: 4096
  temperature: 0.0
  # api_key: sk-...  # 推荐使用环境变量 OPENAI_API_KEY

agent:
  max_steps: 50        # 每个任务最大步数
  max_concurrent: 4    # 最大并发 agent 数

queue:
  size: 256            # 队列缓冲大小
  workers: 2           # worker goroutine 数量

memory:
  max_messages: 100    # 保留的最大消息数
  max_tokens: 128000   # 保留的最大 token 数

tools:
  bash:
    timeout_seconds: 10
  web_search:
    provider: tavily
    endpoint: https://api.tavily.com/search
    timeout_seconds: 15
    default_max_results: 5
    max_results_limit: 10
    # api_key: tvly-... # 推荐使用环境变量 TAVILY_API_KEY
```

## 环境变量

| 变量 | 说明 |
|---|---|
| `OPENAI_API_KEY` | LLM API 密钥 |
| `SISYPHUS_CONFIG` | 配置文件路径（覆盖默认搜索路径） |
| `SISYPHUS_MODEL` | 模型名称覆盖 |
| `SISYPHUS_MAX_STEPS` | 每任务最大步数（默认 50） |
| `SISYPHUS_WORKERS` | worker 数量（默认 2） |
| `SISYPHUS_BASH_TIMEOUT` | bash 工具超时（秒） |
| `SISYPHUS_WEB_SEARCH_ENDPOINT` | web_search API 地址覆盖 |
| `TAVILY_API_KEY` | web_search（Tavily）密钥 |

## 编译

Linux：

```bash
make build       # 编译 → bin/sisyphus
make release     # 生产优化编译（去除调试符号）
make test        # 运行测试（带竞态检测）
make vet         # 运行 go vet
make install     # 安装到 /usr/local/bin
make clean       # 清理编译产物
```

Windows（PowerShell）：

```powershell
# 编译
go build -o bin/sisyphus.exe ./cmd/sisyphus

# 运行测试
go test -race ./...

# 代码检查
go vet ./...
```

通用（Go 跨平台编译）：

```bash
# 编译 Linux 版本
GOOS=linux GOARCH=amd64 go build -o bin/sisyphus ./cmd/sisyphus

# 编译 Windows 版本
GOOS=windows GOARCH=amd64 go build -o bin/sisyphus.exe ./cmd/sisyphus
```

## 跨平台设计

| 特性 | Linux/macOS | Windows |
|---|---|---|
| Shell 执行 | `sh -c` | `cmd /c` |
| 配置路径 | XDG 标准 | `%APPDATA%` / `%ProgramData%` |
| 信号处理 | SIGTERM/SIGINT | os.Interrupt |
| 路径分隔 | `/` | `\`（Go `filepath` 自动处理） |

## 高性能设计

- 有界 channel 实现 goroutine pool，控制并发
- `json.RawMessage` 零拷贝传递工具参数
- context 全链路传递超时和取消信号
- 基于 tiktoken 精确计数的记忆裁剪，防止上下文溢出
- `sync.Pool` 预留接口（热路径内存分配优化待加入）

## License

MIT
