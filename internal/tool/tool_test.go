package tool

import (
	"context"
	"encoding/json"
	"testing"
)

// fakeTool 是一个简单的测试用工具
type fakeTool struct {
	name  string
	desc  string
	value string
}

func (f fakeTool) Name() string                { return f.name }
func (f fakeTool) Description() string         { return f.desc }
func (f fakeTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f fakeTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	return f.value, nil
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()
	t1 := fakeTool{name: "tool1", desc: "first tool"}
	t2 := fakeTool{name: "tool2", desc: "second tool"}

	if err := r.Register(t1); err != nil {
		t.Fatalf("注册 tool1 失败: %v", err)
	}
	if err := r.Register(t2); err != nil {
		t.Fatalf("注册 tool2 失败: %v", err)
	}

	// 重复注册应报错
	if err := r.Register(t1); err == nil {
		t.Fatal("重复注册应返回错误")
	}
}

func TestRegistryGet(t *testing.T) {
	r := NewRegistry()
	t1 := fakeTool{name: "my_tool", value: "ok"}
	r.Register(t1)

	got := r.Get("my_tool")
	if got == nil {
		t.Fatal("Get 返回 nil")
	}
	if got.Name() != "my_tool" {
		t.Errorf("期望 my_tool，实际 %s", got.Name())
	}

	// 不存在的 tool
	if r.Get("nonexistent") != nil {
		t.Fatal("不存在的 tool 应返回 nil")
	}
}

func TestRegistryListAll(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeTool{name: "a"})
	r.Register(fakeTool{name: "b"})
	r.Register(fakeTool{name: "c"})

	list := r.List()
	if len(list) != 3 {
		t.Errorf("期望 3 个 tool，实际 %d", len(list))
	}

	all := r.All()
	if len(all) != 3 {
		t.Errorf("All() 期望 3 个 tool，实际 %d", len(all))
	}
}

func TestRegistryExecute(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeTool{name: "echo", value: "hello world"})

	tl := r.Get("echo")
	result, err := tl.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if result != "hello world" {
		t.Errorf("期望 'hello world'，实际 '%s'", result)
	}
}
