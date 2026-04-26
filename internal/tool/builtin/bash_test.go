package builtin

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBashToolName(t *testing.T) {
	bt := BashTool{}
	if bt.Name() != "bash" {
		t.Errorf("期望 bash，实际 %s", bt.Name())
	}
}

func TestBashToolDescription(t *testing.T) {
	bt := BashTool{}
	if bt.Description() == "" {
		t.Fatal("Description 不应为空")
	}
}

func TestBashToolParameters(t *testing.T) {
	bt := BashTool{}
	params := bt.Parameters()
	if len(params) == 0 {
		t.Fatal("Parameters 不应为空")
	}
	// 验证是有效的 JSON
	var m map[string]interface{}
	if err := json.Unmarshal(params, &m); err != nil {
		t.Fatalf("Parameters 不是有效 JSON: %v", err)
	}
}

func TestBashToolExecuteSuccess(t *testing.T) {
	bt := BashTool{}
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo hello"
	} else {
		cmd = "echo hello"
	}
	args, _ := json.Marshal(map[string]string{"command": cmd})
	result, err := bt.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if result != "hello" {
		t.Errorf("期望 'hello'，实际 '%s'", result)
	}
}

func TestBashToolExecuteEmptyCommand(t *testing.T) {
	bt := BashTool{}
	args, _ := json.Marshal(map[string]string{"command": ""})
	_, err := bt.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("空命令应返回错误")
	}
}

func TestBashToolExecuteBadArgs(t *testing.T) {
	bt := BashTool{}
	_, err := bt.Execute(context.Background(), json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("错误参数应返回错误")
	}
}

func TestBashToolExecuteFailedCommand(t *testing.T) {
	bt := BashTool{}
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "this_command_does_not_exist_xyz 2>nul"
	} else {
		cmd = "this_command_does_not_exist_xyz 2>/dev/null"
	}
	args, _ := json.Marshal(map[string]string{"command": cmd})
	result, err := bt.Execute(context.Background(), args)
	// 失败命令不应返回 error（会返回退出码）
	if err != nil {
		t.Errorf("失败命令不应返回 Go error: %v", err)
	}
	if !strings.Contains(result, "退出码") && !strings.Contains(result, "exit code") {
		t.Logf("结果: %s", result)
	}
}

func TestReadFileToolName(t *testing.T) {
	rt := ReadFileTool{}
	if rt.Name() != "read_file" {
		t.Errorf("期望 read_file，实际 %s", rt.Name())
	}
}

func TestWriteFileToolName(t *testing.T) {
	wt := WriteFileTool{}
	if wt.Name() != "write_file" {
		t.Errorf("期望 write_file，实际 %s", wt.Name())
	}
}

func TestWebSearchToolName(t *testing.T) {
	wst := WebSearchTool{}
	if wst.Name() != "web_search" {
		t.Errorf("期望 web_search，实际 %s", wst.Name())
	}
}

func TestWebSearchToolNoAPIKey(t *testing.T) {
	wst := WebSearchTool{}
	args, _ := json.Marshal(map[string]string{"query": "test"})
	_, err := wst.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("未设置 API key 应返回错误")
	}
	if !strings.Contains(err.Error(), "TAVILY_API_KEY") {
		t.Errorf("错误应提到 TAVILY_API_KEY: %v", err)
	}
}

func TestNewBashToolDefaultTimeout(t *testing.T) {
	bt := NewBashTool(0)
	if bt.Timeout != 10*time.Second {
		t.Fatalf("默认超时应为 10s，实际 %v", bt.Timeout)
	}
}

func TestNewWebSearchToolDefaults(t *testing.T) {
	wst := NewWebSearchTool("", "", 0, 0, 0)
	if wst.Endpoint == "" {
		t.Fatal("默认 endpoint 不应为空")
	}
	if wst.HTTPTimeout != 15*time.Second {
		t.Fatalf("默认 HTTP timeout 应为 15s，实际 %v", wst.HTTPTimeout)
	}
	if wst.DefaultMaxResults != 5 || wst.MaxResultsLimit != 10 {
		t.Fatalf("默认结果限制异常: default=%d limit=%d", wst.DefaultMaxResults, wst.MaxResultsLimit)
	}
}

// TestShellCommand 验证跨平台 shell 选择
func TestShellCommand(t *testing.T) {
	shell, flag := shellCommand()
	if shell == "" {
		t.Fatal("shell 不应为空")
	}
	if flag == "" {
		t.Fatal("flag 不应为空")
	}
	if runtime.GOOS == "windows" {
		if shell != "cmd" {
			t.Errorf("Windows 应用 cmd，实际 %s", shell)
		}
		if flag != "/c" {
			t.Errorf("Windows flag 应为 /c，实际 %s", flag)
		}
	} else {
		if shell != "sh" {
			t.Errorf("Unix 应用 sh，实际 %s", shell)
		}
		if flag != "-c" {
			t.Errorf("Unix flag 应为 -c，实际 %s", flag)
		}
	}
}
