package agent

import (
	"context"
	"fmt"

	"JuanNiang-Neo/internal/adapter"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
)

// JuanNiangMiddleware 是 Eino ChatModelAgent 的自定义中间件，
// 实现动态指令注入、AgentLite 工具过滤和工具调用日志。
// DynamicInstruction 和 AgentLite 通过 context (MsgSessionCtx) 传递，
// 在 BeforeAgent 钩子中读取。
// 注：ACL 现仅管理聊天黑名单（在 handleMessage 阶段过滤消息），
// 不再对工具调用做 ACL 检查。
type JuanNiangMiddleware struct {
	*adk.BaseChatModelAgentMiddleware

	h   *HagoCenter
	msg *adapter.MessageEvent
}

// BeforeAgent 在 Agent 运行前注入动态 Instruction 和按需过滤工具列表（AgentLite）。
// DynamicInstruction 和 AgentLite 从 context 中读取（由 handleMessage 注入）。
// AgentLite 与正常模式一致（保留 ReAct 循环），仅过滤掉 MCP、沙箱与文生图工具。
func (m *JuanNiangMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
	// 从 context 读取 per-message 指令（由 handleMessage 通过 WithMsgSessionCtx 注入）
	if sc := GetMsgSessionCtx(ctx); sc != nil {
		if sc.DynamicInstruction != "" {
			runCtx.Instruction = sc.DynamicInstruction
		}
		if sc.AgentLite {
			runCtx.Tools = filterAgentLiteTools(ctx, m.h, runCtx.Tools)
		}
	}
	return ctx, runCtx, nil
}

// agentLiteDisabledToolNames 是 AgentLite 模式下禁用的内置工具名
// （沙箱相关 + 文生图）。MCP 工具通过 HagoCenter.MCP.HasTool 判定，全部禁用。
var agentLiteDisabledToolNames = map[string]bool{
	"create_sandbox": true,
	"list_sandboxes": true,
	"browser_search": true,
	"command_exec":   true,
	"code_exec":      true,
	"text_to_image":  true,
}

// filterAgentLiteTools 过滤 AgentLite 模式下不允许使用的工具：
// 保留 ReAct 循环与其余工具，仅禁用所有 MCP 工具、沙箱工具与文生图工具。
func filterAgentLiteTools(ctx context.Context, h *HagoCenter, tools []einotool.BaseTool) []einotool.BaseTool {
	kept := make([]einotool.BaseTool, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil || info == nil {
			kept = append(kept, t) // 无法获取元数据时保守保留
			continue
		}
		if agentLiteDisabledToolNames[info.Name] {
			continue
		}
		if h.MCP != nil && h.MCP.HasTool(ctx, info.Name) {
			continue
		}
		kept = append(kept, t)
	}
	return kept
}

// MsgSessionCtx 携带单条消息的 per-goroutine 状态。
// 通过 context 传递，避免 HagoCenter 共享字段导致的数据竞争。
type MsgSessionCtx struct {
	Msg                *adapter.MessageEvent
	SessionCtxStr      string // buildSessionContext 的输出
	RecentMsgsFn       func(ctx context.Context, msgType string, targetID int64, limit int) ([]string, error)
	DynamicInstruction string // 注入给 Eino Agent 的系统指令
	AgentLite          bool   // 精简模式：禁用所有工具
	StripMarkdown      bool   // 去除 Markdown 格式
	DisableSplit       bool   // 禁用分段回复
	LoopID             string // 当前消息对应的 Agent 循环 ID（LoopTracker 用）
}

type msgSessionKey struct{}

// WithMsgSessionCtx 将消息级状态注入 context。
func WithMsgSessionCtx(ctx context.Context, s *MsgSessionCtx) context.Context {
	return context.WithValue(ctx, msgSessionKey{}, s)
}

// GetMsgSessionCtx 从 context 中读取消息级状态。
func GetMsgSessionCtx(ctx context.Context) *MsgSessionCtx {
	v, _ := ctx.Value(msgSessionKey{}).(*MsgSessionCtx)
	return v
}

// WrapInvokableToolCall 包装每个工具的同步调用：
//   - 更新活跃循环的当前工具（供 Web 监控页展示）
//   - 所有工具前台同步执行并记录日志
// 注：ACL 只管理聊天黑名单，工具调用不再受 ACL 限制。
func (m *JuanNiangMiddleware) WrapInvokableToolCall(
	ctx context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	toolName := tCtx.Name

	wrapped := func(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error) {
		log.Info("Eino tool call", "tool", toolName, "call_id", tCtx.CallID, "args_len", len(argsJSON))

		// 更新活跃循环的当前工具（供 Web 监控页展示）
		if sc := GetMsgSessionCtx(ctx); sc != nil && m.h.Loops != nil {
			m.h.Loops.UpdateTool(sc.LoopID, toolName)
		}

		// --- 直接执行（所有工具前台同步执行）---
		result, err := endpoint(ctx, argsJSON, opts...)
		if err != nil {
			log.Error("工具执行失败", "tool", toolName, "err", err)
			return fmt.Sprintf("工具执行失败: %s", err.Error()), nil
		}

		log.Info("工具执行完成", "tool", toolName, "result_len", len(result))
		return result, nil
	}

	return wrapped, nil
}
