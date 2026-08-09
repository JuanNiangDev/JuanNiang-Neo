package tool

import (
	"testing"

	"github.com/openai/openai-go/v3"
)

// TestBuildEinoToolsSkipsUnavailable 验证服务不可用的工具（如 T2I/Sandbox 停用）
// 不会注册进 Eino 工具列表，实现自动卸载。
func TestBuildEinoToolsSkipsUnavailable(t *testing.T) {
	registry := NewToolRegistry()

	ok := &onebotTool{BaseTool: NewTool(NewToolInput{id: "", name: "ok_tool", desc: "可用工具", params: openai.FunctionParameters{"type": "object"}, builtin: true})}
	bad := &onebotTool{BaseTool: NewTool(NewToolInput{id: "", name: "bad_tool", desc: "服务已停用", params: openai.FunctionParameters{"type": "object"}, builtin: true})}
	bad.SetAvailable(func() bool { return false }) // 模拟 T2I/Sandbox 未启用

	registry.Register(ok)
	registry.Register(bad)

	tools := BuildEinoTools(registry, nil, nil)

	if _, found := GetEinoToolByName(tools, "bad_tool"); found {
		t.Error("bad_tool 不应注册进 Eino 工具列表（服务不可用）")
	}
	if _, found := GetEinoToolByName(tools, "ok_tool"); !found {
		t.Error("ok_tool 应正常注册")
	}
}

// TestIsAvailableDefault 验证未设置可用性回调的工具默认始终可用。
func TestIsAvailableDefault(t *testing.T) {
	bt := NewTool(NewToolInput{id: "", name: "plain", desc: "普通工具", params: openai.FunctionParameters{"type": "object"}, builtin: true})
	if !bt.IsAvailable() {
		t.Error("未设置 available 的工具应始终可用")
	}

	bt2 := bt
	bt2.SetAvailable(func() bool { return true })
	if !bt2.IsAvailable() {
		t.Error("available=true 时应可用")
	}
	bt2.SetAvailable(func() bool { return false })
	if bt2.IsAvailable() {
		t.Error("available=false 时应不可用")
	}
}
