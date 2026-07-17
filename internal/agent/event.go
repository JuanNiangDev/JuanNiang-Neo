package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/memory/shortterm"
	"JuanNiang-Neo/internal/agent/prompt"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/pluggin"
)

// runEventLoop 是主事件循环，监听 OneBot11 事件并调用 Agent 处理。
func (h *HagoCenter) runEventLoop(ctx context.Context) {
	slog.Info("事件循环已启动")
	for ev := range h.Adapter.Events() {
		if ev.PostType != "message" || ev.Message == nil {
			continue
		}

		// Plugin 拦截
		if h.PluginEngine != nil {
			pluginEvent := pluggin.EventData{
				PostType:    "message",
				MessageType: ev.Message.MessageType,
				UserID:      ev.Message.UserID,
				GroupID:     ev.Message.GroupID,
				RawMessage:  ev.Message.RawMessage,
			}
			if h.PluginEngine.OnMessage(pluginEvent) {
				continue
			}
		}

		h.handleMessage(ctx, ev)
	}
	slog.Info("事件循环已停止")
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

	if !h.ACL.Check(ctx, userID, chatArea.ID, "chat") {
		slog.Info("ACL 拒绝", "user_id", userID, "chat_area_id", chatArea.ID)
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

	vars := prompt.GetDefaultVars(
		fmt.Sprintf("%d", userID),
		fmt.Sprintf("%d", msg.GroupID),
	)

	var longTermMems []string
	if h.Memory != nil {
		longTermMems, _ = h.Memory.GetLongTermMemory(ctx, "", 5)
	}
	toolList := h.Tools.GetOpenAITools()

	toolDescs := ""
	for _, t := range toolList {
		toolDescs += fmt.Sprintf("- %s: %s\n", t.Function.Name, t.Function.Description)
	}

	systemCtx, _ := h.Prompt.BuildFullContext(ctx, vars, longTermMems, toolDescs)

	messages := []provider.ChatMessage{
		{Role: "system", Content: systemCtx},
	}

	if skillMatched && matchedSkill.PromptRef != "" {
		skillPrompt, err := h.Prompt.GetByID(ctx, matchedSkill.PromptRef)
		if err == nil {
			rendered, _ := h.Prompt.RenderTemplate(skillPrompt.Content, vars)
			messages = append(messages, provider.ChatMessage{
				Role: "system", Content: "[Active Skill: " + matchedSkill.Name + "]\n" + rendered,
			})
		}
	}

	stMsgs, err := h.Memory.GetShortTermMessages(ctx)
	if err == nil {
		for _, m := range stMsgs {
			messages = append(messages, provider.ChatMessage{Role: m.Role, Content: m.Content, Name: m.Name})
		}
	}

	messages = append(messages, provider.ChatMessage{Role: "user", Content: userMsg})

	h.Memory.AddShortTermMessage(ctx, shortterm.ChatMessage{Role: "user", Content: userMsg})

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
		h.recordChat(ctx, chatArea.ID, userID, "assistant", resp.Message.Content, 0)
		h.Memory.AddShortTermMessage(ctx, shortterm.ChatMessage{Role: "assistant", Content: resp.Message.Content})
	}

	if len(resp.Message.ToolCalls) > 0 {
		h.handleToolCalls(ctx, msg, chatArea.ID, userID, sess.ID, messages, resp)
	}
}

func (h *HagoCenter) handleToolCalls(
	ctx context.Context, msg *adapter.MessageEvent,
	chatAreaID string, userID int64, sessionID string,
	history []provider.ChatMessage, resp *provider.ChatResponse,
) {
	llm := h.Providers.SelectModel(provider.ModelTypeText)
	if llm == nil {
		return
	}

	history = append(history, resp.Message)

	for _, tc := range resp.Message.ToolCalls {
		toolName := tc.Function.Name
		args := tc.Function.Arguments

		if h.Tools.IsLongRunning(toolName) {
			steps := []TaskStep{
				{ID: tc.ID, ToolName: toolName, Args: args},
			}
			taskID, err := h.BgTaskExecutor.Submit(ctx, chatAreaID, steps)
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
			h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("任务 %s -> 后台执行: %s", toolName, taskID), 0)
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
		h.recordChat(ctx, chatAreaID, userID, "tool", fmt.Sprintf("%s: %s", toolName, result), 0)
	}

	followUp, err := llm.Chat(ctx, provider.ChatRequest{
		Messages:    history,
		Tools:       h.Tools.GetOpenAITools(),
		Temperature: 0.7,
	})
	if err != nil {
		slog.Error("LLM followUp 调用失败", "err", err)
		return
	}

	h.Session.UpdateTokenUsage(ctx, sessionID, int64(followUp.TokenUsage))

	if followUp.Message.Content != "" {
		h.sendReply(msg, followUp.Message.Content)
		h.recordChat(ctx, chatAreaID, userID, "assistant", followUp.Message.Content, followUp.TokenUsage)
		h.Memory.AddShortTermMessage(ctx, shortterm.ChatMessage{Role: "assistant", Content: followUp.Message.Content})
	}

	// 递归处理可能的后续 tool calls
	if len(followUp.Message.ToolCalls) > 0 {
		h.handleToolCalls(ctx, msg, chatAreaID, userID, sessionID, history, followUp)
	}
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

func (h *HagoCenter) recordChat(ctx context.Context, chatAreaID string, userID int64, role, content string, tokens int) {
	record := &models.ChatRecord{
		ChatAreaID: chatAreaID,
		UserID:     userID,
		Role:       role,
		Content:    content,
		TokenCount: tokens,
	}
	if err := h.DAO.ChatRecord.Create(ctx, record); err != nil {
		slog.Error("记录聊天失败", "err", err)
	}
}
