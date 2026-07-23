package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/memory/shortterm"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/pluggin"
)

// SilenceToken 是 LLM 在不回复时输出的固定标记。
// prompt 会告知 LLM："判定不回复时输出 __NO_REPLY__，系统检测到后自动丢弃"。
const SilenceToken = "__NO_REPLY__"

// runEventLoop 是主事件循环，监听 OneBot11 事件并调用 Agent 处理。
// 当 adapter 的 events channel 关闭时（如重启），会尝试等待后重新获取新的 channel，
// 而不是直接退出事件循环。
func (h *HagoCenter) runEventLoop(ctx context.Context) {
	slog.Info("事件循环已启动")

	adapterEvents := h.Adapter.Events()

	var webhookEvents <-chan adapter.Event
	if h.WebhookAdapter != nil {
		webhookEvents = h.WebhookAdapter.Events()
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("事件循环已停止")
			return
		case ev, ok := <-adapterEvents:
			if !ok {
				slog.Warn("Adapter events channel 已关闭，尝试重新获取...")
				// 等待 adapter 重启后重新获取新的 channel
				select {
				case <-ctx.Done():
					slog.Info("事件循环已停止")
					return
				case <-time.After(time.Second):
				}
				adapterEvents = h.Adapter.Events()
				slog.Info("已重新获取 Adapter events channel")
				continue
			}
			// 透传 Admins 列表（来自 adapter 配置）
			ev.Admins = h.Adapter.Admins()
			h.processEvent(ctx, ev)
		case ev, ok := <-webhookEvents:
			if !ok {
				webhookEvents = nil
				continue
			}
			h.processEvent(ctx, ev)
		case output, ok := <-h.BgTaskResultChan:
			if !ok {
				continue
			}
			slog.Info("收到后台任务结果，注入主 Agent", "task_id", output.TaskID, "chat_area_id", output.ChatAreaID, "media", len(output.MediaPayloads))
			// 先发送媒体负载（图片等 CQ 码）直接到 QQ
			if len(output.MediaPayloads) > 0 {
				// 构造临时 MessageEvent 用于 sendReply
				tmpMsg := &adapter.MessageEvent{
					MessageType: output.MessageType,
				}
				if output.MessageType == "private" {
					tmpMsg.UserID = output.TargetID
				} else {
					tmpMsg.GroupID = output.TargetID
				}
				for _, media := range output.MediaPayloads {
					slog.Info("BgTaskResult 直接发送媒体", "cq_len", len(media))
					h.sendReply(tmpMsg, media)
				}
			}
			// 将 DrainerOutput 转换为合成 Event，触发主 Agent 处理（LLM 只需生成文字回复）
			syntheticEvent := h.bgTaskOutputToEvent(output)
			h.processEvent(ctx, syntheticEvent)
		case ev, ok := <-h.CronJobEvents:
			if !ok {
				continue
			}
			slog.Info("收到 CronJob 事件，注入 Agent", "post_type", ev.PostType)
			ev.Admins = h.Adapter.Admins()
			h.processEvent(ctx, ev)
		}
	}
}

// bgTaskOutputToEvent 将 Drainer 汇总结果转换为合成 Event，供主 Agent 处理。
func (h *HagoCenter) bgTaskOutputToEvent(output DrainerOutput) adapter.Event {
	// 构造用户消息：包含原始请求上下文 + 任务结果
	rawMsg := output.Result
	if output.UserPrompt != "" {
		rawMsg = fmt.Sprintf("用户曾请求：%s\n\n%s", output.UserPrompt, rawMsg)
	}

	msg := &adapter.MessageEvent{
		MessageType: output.MessageType,
		RawMessage:  rawMsg,
	}

	if output.MessageType == "group" {
		msg.GroupID = output.TargetID
		msg.UserID = 0 // 系统消息
	} else {
		msg.UserID = output.TargetID
	}

	return adapter.Event{
		PostType:       "message",
		IsBgTaskResult: true,
		Admins:         h.Adapter.Admins(),
		Message:        msg,
	}
}

// processEvent 派发事件：webhook 走 on_webhook，message 走 handleMessage。
func (h *HagoCenter) processEvent(ctx context.Context, ev adapter.Event) {
	// Webhook 事件交给插件 on_webhook 处理
	if ev.PostType == "webhook" && ev.Webhook != nil {
		if h.PluginEngine != nil {
			pluginEvent := pluggin.EventData{
				PostType: "webhook",
				Admins:   ev.Admins,
				Webhook: map[string]any{
					"path":    ev.Webhook.Path,
					"method":  ev.Webhook.Method,
					"payload": ev.Webhook.Payload,
				},
			}
			h.PluginEngine.OnWebhook(pluginEvent)
		}
		return
	}

	if (ev.PostType != "message" && ev.PostType != "cronjob") || ev.Message == nil {
		return
	}

	// CronJob 事件跳过所有策略检查
	if ev.PostType == "cronjob" {
		h.handleMessage(ctx, ev)
		return
	}

	// 后台任务结果事件（Drainer 合成）跳过策略检查，图片等媒体已由 Drainer 直接发送
	if ev.IsBgTaskResult {
		if cfg, err := h.DAO.ReplyStrategy.GetOrCreate(ctx); err == nil {
			h.StripMarkdown = cfg.StripMarkdown
		}
		h.handleMessage(ctx, ev)
		return
	}

	// === 回复策略过滤 ===
	msg := ev.Message

	// 私聊消息：除"完全不回复"外一律正常处理（无需@、无需相关性判断）
	if msg.MessageType == "private" {
		cfg, err := h.DAO.ReplyStrategy.GetOrCreate(ctx)
		if err != nil {
			slog.Warn("获取回复策略失败，跳过过滤", "err", err)
		} else {
			h.StripMarkdown = cfg.StripMarkdown
			if cfg.Strategy == models.StrategyNeverReply {
				slog.Debug("回复策略: 完全不回复，跳过私聊", "user_id", msg.UserID)
				return
			}
		}
		// 非完全不回复策略，私聊直接放行
		h.handleMessage(ctx, ev)
		return
	}

	cfg, err := h.DAO.ReplyStrategy.GetOrCreate(ctx)
	if err != nil {
		slog.Warn("获取回复策略失败，跳过过滤", "err", err)
	} else {
		h.StripMarkdown = cfg.StripMarkdown
		switch cfg.Strategy {
		case models.StrategyNeverReply:
			// 完全不处理
			slog.Debug("回复策略: 完全不回复，跳过", "group_id", msg.GroupID, "user_id", msg.UserID)
			return
		case models.StrategyAtOnly:
			// 仅@我时回复，未被@则跳过
			if !h.isAtSelf(msg.RawMessage) {
				slog.Debug("回复策略: 仅@我时回复，跳过", "group_id", msg.GroupID, "user_id", msg.UserID)
				return
			}
		case models.StrategyPluginOnly:
			// 仅 Plugin，不调用 Agent
			h.runPluginOnly(ctx, ev)
			return
		case models.StrategyRelevance:
			// 相关性判断：@我 或 插件命令 直接通过，否则调用相关性 Agent
			if !h.isAtSelf(msg.RawMessage) && !h.isPluginCommand(msg.RawMessage) {
				// 获取 ChatArea 用于短期记忆查询
				chatAreaID := ""
				if msg.MessageType == "group" {
					area, err := h.DAO.ChatArea.GetOrCreate(ctx, models.AreaTypeGroup, msg.GroupID)
					if err == nil {
						chatAreaID = area.ID
					}
				}
				recentMsgs, _ := h.getRecentMessages(ctx, chatAreaID, 10)
				score, reason := h.relevanceAgentEvaluate(ctx, msg, recentMsgs, cfg.BotName)
				if score < cfg.RelevanceThreshold {
					slog.Debug("回复策略: 相关性不足，跳过",
						"score", score, "threshold", cfg.RelevanceThreshold, "reason", reason,
						"group_id", msg.GroupID, "user_id", msg.UserID)
					return
				}
				slog.Debug("回复策略: 相关性通过",
					"score", score, "threshold", cfg.RelevanceThreshold, "reason", reason)
			}
		}
		// StrategyAlways: 正常处理，无需额外操作
	}

	// Plugin 拦截
	if h.PluginEngine != nil {
		pluginEvent := pluggin.EventData{
			PostType:    "message",
			MessageType: ev.Message.MessageType,
			UserID:      ev.Message.UserID,
			GroupID:     ev.Message.GroupID,
			RawMessage:  ev.Message.RawMessage,
			Admins:      ev.Admins,
		}
		if h.PluginEngine.OnMessage(pluginEvent) {
			return
		}
	}

	h.handleMessage(ctx, ev)
}

// runPluginOnly 仅 Plugin 模式：运行插件但不调用 Agent。
func (h *HagoCenter) runPluginOnly(ctx context.Context, ev adapter.Event) {
	if h.PluginEngine != nil {
		pluginEvent := pluggin.EventData{
			PostType:    "message",
			MessageType: ev.Message.MessageType,
			UserID:      ev.Message.UserID,
			GroupID:     ev.Message.GroupID,
			RawMessage:  ev.Message.RawMessage,
			Admins:      ev.Admins,
		}
		h.PluginEngine.OnMessage(pluginEvent)
	}
	slog.Debug("回复策略: 仅 Plugin，跳过 Agent", "group_id", ev.Message.GroupID, "user_id", ev.Message.UserID)
}

func (h *HagoCenter) handleMessage(ctx context.Context, ev adapter.Event) {
	msg := ev.Message
	userID := msg.UserID
	var chatAreaType models.AreaType
	var targetID int64

	switch msg.MessageType {
	case "private":
		chatAreaType = models.AreaTypePrivate
		targetID = userID
	case "group":
		chatAreaType = models.AreaTypeGroup
		targetID = msg.GroupID
	default:
		return
	}

	chatArea, err := h.DAO.ChatArea.GetOrCreate(ctx, chatAreaType, targetID)
	if err != nil {
		slog.Error("获取 ChatArea 失败", "err", err)
		return
	}

	// ACL 检查：admin 自动绕过；后台任务/定时任务事件也跳过
	if !ev.IsBgTaskResult && !ev.IsCronJob && !isAdmin(userID, ev.Admins) && !h.ACL.CheckChat(ctx, userID, chatArea.ID) {
		slog.Info("ACL 拒绝", "user_id", userID, "chat_area_id", chatArea.ID, "scope", "chat")
		return
	}

	sess, err := h.Session.GetOrCreate(ctx, chatArea.ID)
	if err != nil {
		slog.Error("获取 Session 失败", "err", err)
		return
	}

	userMsg := strings.TrimSpace(msg.RawMessage)

	matchedSkill, skillMatched := h.Skills.Match(userMsg)

	llm := h.Providers.SelectModel(provider.ModelTypeText)
	if llm == nil {
		slog.Error("无可用 Text 模型")
		return
	}

	var longTermMems []string
	if h.Memory != nil {
		longTermMems, _ = h.Memory.GetLongTermMemory(ctx, chatArea.ID, "", 5)
	}
	toolList := h.buildToolList(ctx)

	toolDescs := ""
	for _, t := range toolList {
		toolDescs += fmt.Sprintf("- %s: %s\n", t.Function.Name, t.Function.Description)
	}

	// 构建会话上下文（透传发送者QQ/名字/权限 + 环境信息）
	h.CurrentMsg = msg
	h.CurrentSessionCtx = h.buildSessionContext(ctx, msg, ev.Admins)

	systemCtx, _ := h.Prompt.BuildFullContext(ctx, longTermMems, toolDescs)

	messages := []provider.ChatMessage{
		{Role: "system", Content: systemCtx},
	}

	// 注入当前会话上下文（让 Agent 感知聊天环境：谁在说话、什么群、自己的身份等）
	if h.CurrentSessionCtx != "" {
		messages = append(messages, provider.ChatMessage{
			Role: "system", Content: h.CurrentSessionCtx,
		})
	}

	if skillMatched && matchedSkill.PromptRef != "" {
		skillPrompt, err := h.Prompt.GetByID(ctx, matchedSkill.PromptRef)
		if err == nil {
			messages = append(messages, provider.ChatMessage{
				Role: "system", Content: "[Active Skill: " + matchedSkill.Name + "]\n" + skillPrompt.Content,
			})
		}
	}

	if h.Memory != nil {
		stMsgs, err := h.Memory.GetShortTermMessages(ctx, chatArea.ID)
		if err == nil {
			for _, m := range stMsgs {
				messages = append(messages, provider.ChatMessage{Role: m.Role, Content: m.Content, Name: m.Name})
			}
		}
	}

	messages = append(messages, provider.ChatMessage{Role: "user", Content: userMsg})

	if h.Memory != nil {
		h.Memory.AddShortTermMessage(ctx, chatArea.ID, shortterm.ChatMessage{Role: "user", Content: userMsg})
	}
	// 持久化原始聊天记录到 DB (与短期记忆解耦, 不受 Redis 重启或 Compact 影响)
	h.Session.AppendRecord(ctx, chatArea.ID, userID, "user", userMsg, 0, nil)

	// 读取回复策略：always 模式跳过静默检测；AgentLite 模式不传工具、跳过工具循环
	replyCfg, _ := h.DAO.ReplyStrategy.GetOrCreate(ctx)
	skipSilenceCheck := replyCfg != nil && replyCfg.Strategy == models.StrategyAlways
	agentLite := replyCfg != nil && replyCfg.AgentLite

	req := provider.ChatRequest{
		Messages:    messages,
		Tools:       toolList,
		Temperature: 0.7,
	}
	// AgentLite 模式：不向 LLM 暴露工具
	if agentLite {
		req.Tools = nil
	}

	resp, err := llm.Chat(ctx, req)
	if err != nil {
		slog.Error("LLM 调用失败", "err", err)
		return
	}

	h.Session.UpdateTokenUsage(ctx, sess.ID, int64(resp.TokenUsage))

	if resp.Message.Content != "" {
		silenced := !skipSilenceCheck && msg.MessageType == "group" && isSilenceResponse(resp.Message.Content)
		if silenced {
			slog.Info("群聊静默响应已丢弃", "content", resp.Message.Content, "group_id", msg.GroupID)
		} else {
			h.sendReply(msg, resp.Message.Content)
			h.recordChat(ctx, chatArea.ID, userID, "assistant", resp.Message.Content, resp.TokenUsage, marshalToolCalls(resp.Message.ToolCalls))
			if h.Memory != nil {
				h.Memory.AddShortTermMessage(ctx, chatArea.ID, shortterm.ChatMessage{Role: "assistant", Content: resp.Message.Content})
			}
		}
	}

	// AgentLite 模式：跳过所有工具/MCP 调用和 Agent 循环
	if agentLite {
		return
	}

	// 静默响应时也跳过工具调用（防止 LLM 绕过文本输出直接调 send_group_msg 发废话）
	// always 模式下不跳过工具调用
	toolSilenced := !skipSilenceCheck && msg.MessageType == "group" && resp.Message.Content != "" && isSilenceResponse(resp.Message.Content)
	if !toolSilenced && len(resp.Message.ToolCalls) > 0 {
		h.handleToolCalls(ctx, msg, chatArea.ID, userID, userMsg, sess.ID, messages, resp, ev.Admins)
	}
}

func (h *HagoCenter) handleToolCalls(
	ctx context.Context, msg *adapter.MessageEvent,
	chatAreaID string, userID int64, userMsg string, sessionID string,
	history []provider.ChatMessage, resp *provider.ChatResponse,
	admins []string,
) {
	llm := h.Providers.SelectModel(provider.ModelTypeText)
	if llm == nil {
		return
	}

	userIsAdmin := isAdmin(userID, admins)
	history = append(history, resp.Message)

	for _, tc := range resp.Message.ToolCalls {
		toolName := tc.Function.Name
		args := tc.Function.Arguments

		// 判定工具来源：优先 MCP（MCP 工具可覆盖同名内置工具），再查 ToolRegistry
		isMCPTool := h.MCP != nil && h.MCP.HasTool(ctx, toolName)
		_, isRegistryTool := h.Tools.Get(toolName)

		slog.Info("Tool 调用开始", "tool", toolName, "is_mcp", isMCPTool, "is_builtin", isRegistryTool,
			"chat_area_id", chatAreaID, "user_id", userID)

		if isMCPTool {
			// --- MCP 工具调用 ---
			if !userIsAdmin {
				allowed, denialMsg := h.ACL.CheckMCP(ctx, userID, chatAreaID, toolName)
				if !allowed {
					history = append(history, provider.ChatMessage{
						Role:       "tool",
						Content:    denialMsg,
						ToolCallID: tc.ID,
						Name:       toolName,
					})
					h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("%s: %s", toolName, denialMsg), 0, nil)
					slog.Info("ACL 拒绝 MCP 调用", "user_id", userID, "tool", toolName)
					continue
				}
			}

			// MCP 长耗时工具也走后台任务
			if h.Tools.IsLongRunning(toolName) {
				steps := []TaskStep{
					{ID: tc.ID, ToolName: toolName, Args: args},
				}
				taskID, err := h.BgTaskExecutor.Submit(ctx, chatAreaID, msg.MessageType, getTargetID(msg), userMsg, steps)
				if err != nil {
					slog.Error("提交后台任务失败(MCP)", "tool", toolName, "err", err)
					continue
				}
				slog.Info("MCP 长耗时工具已提交后台", "tool", toolName, "task_id", taskID)
				h.sendReply(msg, fmt.Sprintf("MCP 任务 %s 已提交后台执行...", toolName))
				history = append(history, provider.ChatMessage{
					Role:       "tool",
					Content:    fmt.Sprintf("[系统] MCP 任务 %s 已提交后台执行 (task_id: %s)。你不需要做任何后续处理——禁止编造或猜测执行结果。只需告知用户任务已提交。", toolName, taskID),
					ToolCallID: tc.ID,
				})
				h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("MCP 任务 %s -> 后台执行: %s", toolName, taskID), 0, nil)
				continue
			}

			slog.Info("MCP Tool 执行中", "tool", toolName)
			result, err := h.MCP.CallTool(ctx, toolName, args)
			if err != nil {
				result = fmt.Sprintf("MCP调用失败: %s", err.Error())
				slog.Error("MCP调用失败", "tool", toolName, "err", err)
			} else {
				slog.Info("MCP Tool 执行完成", "tool", toolName, "result_len", len(result))
			}

			history = append(history, provider.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				Name:       toolName,
			})
			h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("MCP %s: %s", toolName, result), 0, nil)
		} else if isRegistryTool {
			// --- 内置工具调用 ---
			if !userIsAdmin {
				allowed, denialMsg := h.ACL.CheckTool(ctx, userID, chatAreaID, toolName)
				if !allowed {
					history = append(history, provider.ChatMessage{
						Role:       "tool",
						Content:    denialMsg,
						ToolCallID: tc.ID,
						Name:       toolName,
					})
					h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("%s: %s", toolName, denialMsg), 0, nil)
					slog.Info("ACL 拒绝工具调用", "user_id", userID, "tool", toolName)
					continue
				}
			}

			if h.Tools.IsLongRunning(toolName) {
				steps := []TaskStep{
					{ID: tc.ID, ToolName: toolName, Args: args},
				}
				taskID, err := h.BgTaskExecutor.Submit(ctx, chatAreaID, msg.MessageType, getTargetID(msg), userMsg, steps)
				if err != nil {
					slog.Error("提交后台任务失败", "tool", toolName, "err", err)
					continue
				}

				slog.Info("内置长耗时工具已提交后台", "tool", toolName, "task_id", taskID)
				h.sendReply(msg, fmt.Sprintf("任务 %s 已提交后台执行...", toolName))
				history = append(history, provider.ChatMessage{
					Role:       "tool",
					Content:    fmt.Sprintf("[系统] 任务 %s 已提交后台执行 (task_id: %s)。你不需要做任何后续处理——禁止编造或猜测执行结果。只需告知用户任务已提交。", toolName, taskID),
					ToolCallID: tc.ID,
				})
				h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("任务 %s -> 后台执行: %s", toolName, taskID), 0, nil)
				continue
			}

			slog.Info("内置 Tool 执行中", "tool", toolName)
			result, err := h.Tools.Execute(ctx, toolName, args)
			if err != nil {
				result = fmt.Sprintf("工具执行失败: %s", err.Error())
				slog.Error("内置工具执行失败", "tool", toolName, "err", err)
			} else {
				slog.Info("内置 Tool 执行完成", "tool", toolName, "result_len", len(result))
			}

			history = append(history, provider.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				Name:       toolName,
			})
			h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("%s: %s", toolName, result), 0, nil)
		} else {
			// 未找到工具
			errMsg := fmt.Sprintf("工具 %q 未找到 (非内置工具也非 MCP 工具)", toolName)
			slog.Error("工具未找到", "tool", toolName)
			history = append(history, provider.ChatMessage{
				Role:       "tool",
				Content:    errMsg,
				ToolCallID: tc.ID,
				Name:       toolName,
			})
			h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("%s: %s", toolName, errMsg), 0, nil)
		}
	}

followUp, err := llm.Chat(ctx, provider.ChatRequest{
		Messages:    history,
		Tools:       h.buildToolList(ctx),
		Temperature: 0.7,
	})
	if err != nil {
		slog.Error("LLM followUp 调用失败", "err", err)
		return
	}

	h.Session.UpdateTokenUsage(ctx, sessionID, int64(followUp.TokenUsage))

	if followUp.Message.Content != "" {
		h.sendReply(msg, followUp.Message.Content)
		h.recordChat(ctx, chatAreaID, userID, "assistant", followUp.Message.Content, followUp.TokenUsage, marshalToolCalls(followUp.Message.ToolCalls))
		if h.Memory != nil {
			h.Memory.AddShortTermMessage(ctx, chatAreaID, shortterm.ChatMessage{Role: "assistant", Content: followUp.Message.Content})
		}
	}

	// 递归处理可能的后续 tool calls
	if len(followUp.Message.ToolCalls) > 0 {
		h.handleToolCalls(ctx, msg, chatAreaID, userID, userMsg, sessionID, history, followUp, admins)
	}
}

// isAdmin 检查 userID 是否在 admins 列表中（admins 元素为字符串形式的 QQ 号）。
func isAdmin(userID int64, admins []string) bool {
	if len(admins) == 0 {
		return false
	}
	uidStr := strconv.FormatInt(userID, 10)
	for _, a := range admins {
		if a == uidStr {
			return true
		}
	}
	return false
}

// splitMessages 将 LLM 输出拆分为多条独立消息。
// 优先使用 <|msg|> 显式分隔符；未找到时回退到 \n\n（双换行）拆分。
// 不尝试拆分超过 5 段的输出（避免破坏多段落长消息）。
func splitMessages(content string) []string {
	// 优先：显式 <|msg|> 分隔符
	if strings.Contains(content, "<|msg|>") {
		var parts []string
		for _, p := range strings.Split(content, "<|msg|>") {
			p = strings.TrimSpace(p)
			if p != "" {
				parts = append(parts, p)
			}
		}
		return parts
	}
	// 回退：\n\n 拆分（DeepSeek 习惯用空行分隔多条独立消息）
	// 限制 2~5 段，避免拆分多段落长消息
	parts := strings.Split(content, "\n\n")
	if len(parts) >= 2 && len(parts) <= 5 {
		var result []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		if len(result) >= 2 {
			return result
		}
	}
	return []string{content}
}

func (h *HagoCenter) sendReply(msg *adapter.MessageEvent, content string) {
	for _, part := range splitMessages(content) {
		if h.StripMarkdown {
			part = stripMarkdown(part)
		}
		var err error
		switch msg.MessageType {
		case "private":
			_, err = h.Adapter.SendPrivateMsg(msg.UserID, part)
		case "group":
			_, err = h.Adapter.SendGroupMsg(msg.GroupID, part)
		}
		if err != nil {
			slog.Error("发送消息失败", "err", err)
		}
	}
}

// isSilenceResponse 检测 LLM 是否输出了静默标记或静默声明类废话。
// 主路径：LLM 按 prompt 要求输出 __NO_REPLY__ 标记。
// 兜底：匹配已知静默短语/表情，防止 LLM 不遵守标记约定。
func isSilenceResponse(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}

	// 主路径：静默标记（LLM 被 prompt 告知"不回复时输出 __NO_REPLY__"）
	if strings.Contains(trimmed, SilenceToken) {
		return true
	}

	// 兜底：已知静默短语匹配（仅限短消息，避免误伤）
	if len([]rune(trimmed)) > 15 {
		return false
	}
	lower := strings.ToLower(trimmed)
	silencePhrases := []string{
		"保持静默", "保持沉默", "静默观察", "静默",
		"不回复", "我不回复", "我不回",
		"不插话", "我不插话",
		"不说话", "我不说话",
		"不发言", "我不发言",
		"不参与", "我不参与",
		"与我无关", "不关我的事",
		"我不说",
		"不响", "不响，做空气", "不响，做空气。",
		"做空气", "装死", "当没看到", "没看到",
		"路过", "潜水", "暗中观察",
		"😶", "🤐", "🙈", "🫥",
	}
	for _, m := range silencePhrases {
		if lower == m {
			return true
		}
	}
	if strings.Contains(lower, "静默") || strings.Contains(lower, "不回复") || strings.Contains(lower, "不插话") ||
		strings.Contains(lower, "不响") || strings.Contains(lower, "做空气") || strings.Contains(lower, "装死") {
		return true
	}

	return false
}

// getTargetID 根据消息类型返回对应的 QQ 目标 ID。
func getTargetID(msg *adapter.MessageEvent) int64 {
	switch msg.MessageType {
	case "private":
		return msg.UserID
	case "group":
		return msg.GroupID
	default:
		return 0
	}
}

func (h *HagoCenter) recordChat(ctx context.Context, chatAreaID string, userID int64, role, content string, tokens int, toolCalls models.JSONMap) {
	if err := h.Session.AppendRecord(ctx, chatAreaID, userID, role, content, tokens, toolCalls); err != nil {
		slog.Error("记录聊天失败", "err", err)
	}
}

// marshalToolCalls 将 ToolCall 列表转为 JSONMap 存入 DB
func marshalToolCalls(tcs []provider.ToolCall) models.JSONMap {
	if len(tcs) == 0 {
		return nil
	}
	b, _ := json.Marshal(tcs)
	var raw []any
	json.Unmarshal(b, &raw)
	return models.JSONMap{"tool_calls": raw}
}

// buildSessionContext 构建当前会话上下文，包含发送者、群信息、机器人身份等。
// 这些信息以 system prompt 形式注入，让 Agent 感知聊天环境。
func (h *HagoCenter) buildSessionContext(ctx context.Context, msg *adapter.MessageEvent, admins []string) string {
	var parts []string

	switch msg.MessageType {
	case "private":
		parts = append(parts, "消息类型: 私聊")
		parts = append(parts, fmt.Sprintf("对方QQ: %d", msg.UserID))
		if msg.Sender.Nickname != "" {
			parts = append(parts, fmt.Sprintf("对方昵称: %s", msg.Sender.Nickname))
		}
	case "group":
		parts = append(parts, "消息类型: 群聊")
		parts = append(parts, fmt.Sprintf("群号: %d", msg.GroupID))
		// 获取发送者群内信息
		senderName := msg.Sender.Card
		if senderName == "" {
			senderName = msg.Sender.Nickname
		}
		parts = append(parts, fmt.Sprintf("发送者QQ: %d", msg.UserID))
		parts = append(parts, fmt.Sprintf("发送者名称: %s", senderName))

		// 获取群内权限（owner/admin/member）
		if h.Adapter != nil {
			memberInfo, err := h.Adapter.GetGroupMemberInfo(msg.GroupID, msg.UserID)
			if err == nil && memberInfo != nil {
				roleLabel := memberInfo.Role
				switch memberInfo.Role {
				case "owner":
					roleLabel = "群主"
				case "admin":
					roleLabel = "管理员"
				case "member":
					roleLabel = "普通成员"
				}
				parts = append(parts, fmt.Sprintf("发送者权限: %s", roleLabel))
			}
		}
	}

	// 当前消息 ID（供 Agent 使用 get_recent_messages 工具）
	if msg.MessageID != 0 {
		parts = append(parts, fmt.Sprintf("当前消息ID: %d", msg.MessageID))
	}

	// 机器人自身信息（优先 Adapter 实时 SelfID，防止 Init 时 QQ bot 未连接导致缓存为 0）
	selfQQ := h.SelfQQ
	if h.Adapter != nil {
		if id := h.Adapter.SelfID(); id != 0 {
			selfQQ = id
		}
	}
	if selfQQ != 0 {
		parts = append(parts, fmt.Sprintf("你的QQ: %d", selfQQ))
	}
	if h.SelfNickname != "" {
		parts = append(parts, fmt.Sprintf("你的昵称: %s", h.SelfNickname))
	}

	// 管理员列表
	if len(admins) > 0 {
		parts = append(parts, fmt.Sprintf("管理员QQ列表: [%s]", strings.Join(admins, ", ")))
	}

	return "[当前聊天环境]\n" + strings.Join(parts, "\n")
}
