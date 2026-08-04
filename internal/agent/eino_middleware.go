package agent

import (
	"context"
	"fmt"

	"JuanNiang-Neo/internal/adapter"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
)

// JuanNiangMiddleware 是 Eino ChatModelAgent 的自定义中间件，
// 实现动态指令注入、AgentLite 工具过滤、高危工具管理员校验和工具调用日志。
// DynamicInstruction 和 AgentLite 通过 context (MsgSessionCtx) 传递，
// 在 BeforeAgent 钩子中读取。
// 注：ACL 现仅管理聊天黑名单（在 handleMessage 阶段过滤消息），不再对工具调用做
// ACL 检查；但高危工具（群管理/请求处理/撤回）在 WrapInvokableToolCall 中强制
// 校验调用者为 Admins 列表内，防止 Agent 被提示词注入诱导执行敏感操作。
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
	Admins             []string // 当前消息透传的管理员 QQ 列表（高危工具权限校验用）
	SessionCtxStr      string   // buildSessionContext 的输出
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

// adminOnlyToolNames 高危工具名单：仅管理员（Admins 列表内）可调用。
// 这些工具对应 OneBot11 群管理/请求处理/撤回等敏感 API，一旦被提示词注入
// 诱导调用会引发严重后果（踢人/禁言/全员禁言/改群名片/通过加群等）。
// 非管理员调用一律拒绝，坚决不执行。
var adminOnlyToolNames = map[string]bool{
	"kick_group_member":     true, // 踢出群成员
	"ban_group_member":      true, // 禁言群成员
	"set_group_whole_ban":   true, // 全员禁言
	"set_group_card":        true, // 修改群名片
	"handle_friend_request": true, // 处理好友申请
	"handle_group_request":  true, // 处理加群/邀请请求
	"delete_msg":            true, // 撤回消息（含他人消息）
}

// isAdminOnlyToolAllowed 高危工具权限判定：群管理/请求处理/撤回类工具
// 仅允许 Admins 列表内的用户调用，其余一律拒绝（防提示词注入诱导）。
// 非高危工具不受限制。
func isAdminOnlyToolAllowed(toolName string, userID int64, admins []string) bool {
	if !adminOnlyToolNames[toolName] {
		return true
	}
	return isAdmin(userID, admins)
}

// WrapInvokableToolCall 包装每个工具的同步调用：
//   - 高危工具（adminOnlyToolNames）执行前校验调用者是否为管理员，非管理员直接拒绝
//   - 更新活跃循环的当前工具（供 Web 监控页展示）
//   - 所有工具前台同步执行并记录日志
//
// 注：ACL 只管理聊天黑名单，工具调用不再受 ACL 限制；高危工具的权限由
// 本处基于当前消息发送者 + Admins 列表做强制校验（防提示词注入）。
func (m *JuanNiangMiddleware) WrapInvokableToolCall(
	ctx context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	toolName := tCtx.Name

	wrapped := func(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error) {
		log.Info("Eino tool call", "tool", toolName, "call_id", tCtx.CallID, "args_len", len(argsJSON))

		// 高危工具权限校验：仅管理员可调用（防提示词注入诱导执行群管理操作）
		if !isAdminOnlyToolAllowed(toolName, msgUserID(ctx), msgAdmins(ctx)) {
			log.Warn("高危工具被非管理员调用，已拒绝", "tool", toolName, "user_id", msgUserID(ctx))
			return "该操作仅限管理员执行，已拒绝执行。", nil
		}

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

// msgUserID 从 context 中读取当前消息发送者 QQ（无则 0）。
func msgUserID(ctx context.Context) int64 {
	if sc := GetMsgSessionCtx(ctx); sc != nil && sc.Msg != nil {
		return sc.Msg.UserID
	}
	return 0
}

// msgAdmins 从 context 中读取当前消息透传的管理员列表。
func msgAdmins(ctx context.Context) []string {
	if sc := GetMsgSessionCtx(ctx); sc != nil {
		return sc.Admins
	}
	return nil
}
