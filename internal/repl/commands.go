package repl

import (
	"context"
	"fmt"
	"strings"
)

// handleCommand processes a slash command. Returns true if the REPL should exit.
func (r *REPL) handleCommand(ctx context.Context, line string) bool {
	parts := strings.Fields(line)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/exit", "/quit", "/q":
		r.renderer.Info("再见！")
		return true

	case "/help", "/h":
		r.showHelp()

	case "/clear":
		r.agent.Memory().Clear()
		r.agent.InitMemory()
		r.sessionID = r.session.NewSessionID()
		r.renderer.Success("记忆已清空，开始新对话。")

	case "/save":
		name := r.sessionID
		if len(args) > 0 {
			name = args[0]
		}
		if err := r.agent.Memory().Save(name); err != nil {
			r.renderer.Error(fmt.Sprintf("保存失败: %v", err))
		} else {
			r.renderer.Success(fmt.Sprintf("会话已保存: %s", name))
		}

	case "/load":
		if len(args) == 0 {
			r.renderer.Error("用法: /load <session-id>")
			return false
		}
		name := args[0]
		if err := r.agent.Memory().Load(name); err != nil {
			r.renderer.Error(fmt.Sprintf("加载失败: %v", err))
		} else {
			r.sessionID = name
			r.renderer.Success(fmt.Sprintf("已加载会话: %s (%d 条消息)", name, r.agent.Memory().Len()))
		}

	case "/sessions":
		sessions, err := r.session.List()
		if err != nil {
			r.renderer.Error(fmt.Sprintf("列出会话失败: %v", err))
			return false
		}
		if len(sessions) == 0 {
			r.renderer.Info("暂无保存的会话。")
			return false
		}
		r.renderer.Info(fmt.Sprintf("共 %d 个会话:", len(sessions)))
		for _, s := range sessions {
			marker := "  "
			if s.ID == r.sessionID {
				marker = "▶ "
			}
			r.renderer.Info(fmt.Sprintf("%s%s  (%s)", marker, s.ID, s.ModTime.Format("2006-01-02 15:04")))
		}

	case "/model":
		if len(args) == 0 {
			r.renderer.Info(fmt.Sprintf("当前模型: %s", r.cfg.LLM.Model))
		} else {
			r.renderer.Info(fmt.Sprintf("模型切换需要重新启动: sisyphus --model %s", args[0]))
		}

	case "/tools":
		names := r.registry.List()
		r.renderer.Info(fmt.Sprintf("已注册工具 (%d): [%s]", len(names), strings.Join(names, ", ")))

	case "/verbose":
		v := !r.renderer.Verbose()
		r.renderer.SetVerbose(v)
		if v {
			r.renderer.Success("详细���式已开启（显���完整 thinking）")
		} else {
			r.renderer.Success("详细模式已关闭")
		}

	case "/memory", "/mem":
		r.renderer.Info(fmt.Sprintf("消息数: %d | Token 数: ~%d | 会话: %s",
			r.agent.Memory().Len(), r.agent.Memory().TokenCount(), r.sessionID))

	default:
		r.renderer.Error(fmt.Sprintf("未知命令: %s（输入 /help 查看帮助）", cmd))
	}

	return false
}

func (r *REPL) showHelp() {
	help := `可用命令:
  /help, /h        显示此帮助
  /exit, /quit, /q 退出
  /clear           清空记忆，开始新对话
  /save [name]     保存当前会话
  /load <name>     加载已保存的会话
  /sessions        列出所有已保存的会话
  /model           查看当前模型
  /tools           列出已注册的工具
  /verbose         切换详细模式（显示完整 thinking）
  /memory, /mem    查看当��记忆状态

输入技巧:
  直接输入文字并回车���可对话
  用 """ 开始多行输入，再用 """ 结束
  Ctrl+C 中断当前生成
  Ctrl+D 退出`
	r.renderer.Info(help)
}
