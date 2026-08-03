package agent

import (
	"context"
	"fmt"

	"JuanNiang-Neo/internal/adapter"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
)

// JuanNiangMiddleware 是 Eino ChatModelAgent 的自定义中间件，
// 实现 ACL 权限检查和工具调用日志。
// DynamicInstruction 和 AgentLite 通过 context (MsgSessionCtx) 传递，
// 在 BeforeAgent 钩子中读取。
type JuanNiangMiddleware struct {
	*adk.BaseChatModelAgentMiddleware

	h          *HagoCenter
	msg        *adapter.MessageEvent
	userID     int64
	chatAreaID string
	isAdmin    bool
	admins     []string
}

// BeforeAgent 在 Agent 运行前注入动态 Instruction 和按需清空工具列表（AgentLite）。
// DynamicInstruction 和 AgentLite 从 context 中读取（由 handleMessage 注入）。
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
			runCtx.Tools = nil
		}
	}
	return ctx, runCtx, nil
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
//   - Admin 用户绕过 ACL
//   - 非 Admin 进行 ACL 检查（内置工具走 CheckTool，MCP 工具走 CheckMCP）
//   - 所有工具前台同步执行并记录日志
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

		// --- ACL 检查 ---
		if !m.isAdmin {
			isMCP := m.h.MCP != nil && m.h.MCP.HasTool(ctx, toolName)
			var allowed bool
			var denial string
			if isMCP {
				allowed, denial = m.h.ACL.CheckMCP(ctx, m.userID, m.chatAreaID, toolName)
			} else {
				allowed, denial = m.h.ACL.CheckTool(ctx, m.userID, m.chatAreaID, toolName)
			}
			if !allowed {
				log.Info("ACL 拒绝工具调用", "user_id", m.userID, "tool", toolName, "reason", denial)
				return denial, nil
			}
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
