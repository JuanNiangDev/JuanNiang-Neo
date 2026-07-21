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
		}
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

	if ev.PostType != "message" || ev.Message == nil {
		return
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

	// ACL 检查：admin 自动绕过
	if !isAdmin(userID, ev.Admins) && !h.ACL.CheckChat(ctx, userID, chatArea.ID) {
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

	systemCtx, _ := h.Prompt.BuildFullContext(ctx, longTermMems, toolDescs)

	messages := []provider.ChatMessage{
		{Role: "system", Content: systemCtx},
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

	req := provider.ChatRequest{
		Messages:    messages,
		Tools:       toolList,
		Temperature: 0.7,
	}

	resp, err := llm.Chat(ctx, req)
	if err != nil {
		slog.Error("LLM 调用失败", "err", err)
		return
	}

	h.Session.UpdateTokenUsage(ctx, sess.ID, int64(resp.TokenUsage))

	if resp.Message.Content != "" {
		h.sendReply(msg, resp.Message.Content)
		h.recordChat(ctx, chatArea.ID, userID, "assistant", resp.Message.Content, resp.TokenUsage, marshalToolCalls(resp.Message.ToolCalls))
		if h.Memory != nil {
			h.Memory.AddShortTermMessage(ctx, chatArea.ID, shortterm.ChatMessage{Role: "assistant", Content: resp.Message.Content})
		}
	}

	if len(resp.Message.ToolCalls) > 0 {
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

		// 判断是注册工具还是 MCP 工具
		_, isRegistryTool := h.Tools.Get(toolName)

		if isRegistryTool {
			// ACL 检查：工具调用权限（admin 自动绕过）
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
					slog.Error("提交后台任务失败", "err", err)
					continue
				}

				h.sendReply(msg, fmt.Sprintf("任务 %s 已提交后台执行...", toolName))
				history = append(history, provider.ChatMessage{
					Role:       "tool",
					Content:    fmt.Sprintf("任务已提交后台执行，task_id: %s", taskID),
					ToolCallID: tc.ID,
				})
				h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("任务 %s -> 后台执行: %s", toolName, taskID), 0, nil)
				continue
			}

			result, err := h.Tools.Execute(ctx, toolName, args)
			if err != nil {
				result = fmt.Sprintf("工具执行失败: %s", err.Error())
				slog.Error("工具执行失败", "tool", toolName, "err", err)
			}

			history = append(history, provider.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				Name:       toolName,
			})
			h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("%s: %s", toolName, result), 0, nil)
	} else {
			// MCP 工具调用
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

			result, err := h.MCP.CallTool(ctx, toolName, args)
			if err != nil {
				result = fmt.Sprintf("MCP调用失败: %s", err.Error())
				slog.Error("MCP调用失败", "tool", toolName, "err", err)
			}

			history = append(history, provider.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				Name:       toolName,
			})
			h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("%s: %s", toolName, result), 0, nil)
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

func (h *HagoCenter) sendReply(msg *adapter.MessageEvent, content string) {
	var err error
	switch msg.MessageType {
	case "private":
		_, err = h.Adapter.SendPrivateMsg(msg.UserID, content)
	case "group":
		_, err = h.Adapter.SendGroupMsg(msg.GroupID, content)
	}
	if err != nil {
		slog.Error("发送消息失败", "err", err)
	}
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
