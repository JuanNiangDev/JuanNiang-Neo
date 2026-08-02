package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"JuanNiang-Neo/internal/adapter"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
)

// JuanNiangMiddleware 是 Eino ChatModelAgent 的自定义中间件，
// 实现 ACL 权限检查、长耗时工具的后台任务提交、以及工具调用日志。
type JuanNiangMiddleware struct {
	*adk.BaseChatModelAgentMiddleware

	h          *HagoCenter
	msg        *adapter.MessageEvent
	userID     int64
	chatAreaID string
	isAdmin    bool
	admins     []string

	// dynamicInstruction 每条消息动态注入的系统指令（由 handleMessage 在 Run 前设置）。
	dynamicInstruction string
	// agentLite 为 true 时清空工具列表，禁止 Agent 使用任何工具。
	agentLite bool
}

// BeforeAgent 在 Agent 运行前注入动态 Instruction 和按需清空工具列表（AgentLite）。
func (m *JuanNiangMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
	if m.dynamicInstruction != "" {
		runCtx.Instruction = m.dynamicInstruction
	}
	if m.agentLite {
		runCtx.Tools = nil
	}
	return ctx, runCtx, nil
}

// JuanNiangSessionCtx 用于在 context 中传递请求级状态。
type JuanNiangSessionCtx struct {
	Msg        *adapter.MessageEvent
	UserID     int64
	ChatAreaID string
	IsAdmin    bool
	Admins     []string
	SessionID  string
	UserMsg    string
}

type sessionCtxKey struct{}

// WithSessionCtx 将请求级状态注入 context。
func WithSessionCtx(ctx context.Context, s *JuanNiangSessionCtx) context.Context {
	return context.WithValue(ctx, sessionCtxKey{}, s)
}

// GetSessionCtx 从 context 中读取请求级状态。
func GetSessionCtx(ctx context.Context) *JuanNiangSessionCtx {
	v, _ := ctx.Value(sessionCtxKey{}).(*JuanNiangSessionCtx)
	return v
}

// WrapInvokableToolCall 包装每个工具的同步调用：
//   - Admin 用户绕过 ACL
//   - 非 Admin 进行 ACL 检查（内置工具走 CheckTool，MCP 工具走 CheckMCP）
//   - 长耗时工具提交后台任务（BgTaskExecutor），不直接执行
//   - 其余工具正常执行并记录日志
func (m *JuanNiangMiddleware) WrapInvokableToolCall(
	ctx context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	toolName := tCtx.Name

	wrapped := func(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error) {
		slog.Info("Eino tool call", "tool", toolName, "call_id", tCtx.CallID, "args_len", len(argsJSON))

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
				slog.Info("ACL 拒绝工具调用", "user_id", m.userID, "tool", toolName, "reason", denial)
				return denial, nil
			}
		}

		// --- 长耗时工具 → 提交后台任务 ---
		if m.h.Tools.IsLongRunning(toolName) {
			steps := []TaskStep{
				{ID: tCtx.CallID, ToolName: toolName, Args: json.RawMessage(argsJSON)},
			}
			taskID, err := m.h.BgTaskExecutor.Submit(
				ctx, m.chatAreaID, m.msg.MessageType,
				getTargetID(m.msg), "", steps,
			)
			if err != nil {
				slog.Error("提交后台任务失败", "tool", toolName, "err", err)
				return fmt.Sprintf("提交后台任务失败: %s", err.Error()), nil
			}
			slog.Info("长耗时工具已提交后台", "tool", toolName, "task_id", taskID)
			return fmt.Sprintf("[系统] 任务 %s 已提交后台执行 (task_id: %s)。你不需要做任何后续处理——禁止编造或猜测执行结果。只需告知用户任务已提交。", toolName, taskID), nil
		}

		// --- 正常执行 ---
		result, err := endpoint(ctx, argsJSON, opts...)
		if err != nil {
			slog.Error("工具执行失败", "tool", toolName, "err", err)
			return fmt.Sprintf("工具执行失败: %s", err.Error()), nil
		}

		slog.Info("工具执行完成", "tool", toolName, "result_len", len(result))
		return result, nil
	}

	return wrapped, nil
}
