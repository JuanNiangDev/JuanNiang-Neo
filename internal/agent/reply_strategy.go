package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/models"
)

// RelevanceCheckResult 相关性检查结果。
type RelevanceCheckResult struct {
	Relevance float64 `json:"relevance"`
	Reason    string  `json:"reason"`
}

// relevanceAgentEvaluate 调用 LLM 评估消息相关性。
// 返回 0-1 之间的相关性分数，以及原因。
func (h *HagoCenter) relevanceAgentEvaluate(ctx context.Context, msg *adapter.MessageEvent, recentMessages []string) (float64, string) {
	// 获取机器人 QQ，优先 Adapter 实时值（防止缓存为 0）
	selfQQ := h.SelfQQ
	if h.Adapter != nil {
		if id := h.Adapter.SelfID(); id != 0 {
			selfQQ = id
		}
	}

	// 检查是否有图片消息但无视觉模型
	hasImage := false
	for _, seg := range msg.Message {
		if seg.Type == "image" {
			hasImage = true
			break
		}
	}
	if hasImage {
		visionModel := h.Providers.SelectModel(provider.ModelTypeImage)
		if visionModel == nil {
			return 0, "消息包含图片但没有配置视觉模型"
		}
		// 有视觉模型，用 Vision 分析
		for _, seg := range msg.Message {
			if seg.Type == "image" {
				url := extractImageURL(seg)
				if url != "" {
					prompt := fmt.Sprintf(`你是一个群聊相关度判断助手。请判断以下由用户发送的图片和文字是否与你（机器人）相关，是否需要你回复。

你的身份:
- 你的昵称: %s
- 你的QQ: %d

当前消息:
- 发送者昵称: %s
- 发送者QQ: %d

回复规则:
- 如果图片/文字明确指向你（@你、叫你名字"%s"、询问你的能力、请求你操作），相关度应该高（>0.7）
- 如果图片/文字是群友之间的闲聊、互怼、讨论与你无关的话题，相关度应该低（<0.3）
- 如果图片/文字是群管理相关需求，相关度应该高
- 如果图片是纯表情包、风景照、美食照等与任务无关的内容，相关度应该低

请以 JSON 格式回复: {"relevance": 0.0-1.0, "reason": "简短原因"}`, h.SelfNickname, selfQQ, msg.Sender.Nickname, msg.UserID, h.SelfNickname)
					// 下载图片并调用 Vision
					resp, err := visionModel.Vision(ctx, nil, prompt) // Vision expects raw image bytes
					if err != nil {
						slog.Warn("Vision 调用失败", "err", err)
						return 0, "Vision 调用失败"
					}
					// 解析 JSON
					var result RelevanceCheckResult
					if err := json.Unmarshal([]byte(resp), &result); err != nil {
						// 尝试从响应中提取 JSON
						result = extractRelevanceJSON(resp)
					}
					return result.Relevance, result.Reason
				}
			}
		}
		return 0, "无法获取图片内容"
	}

	// 文本消息，调用文本模型
	llm := h.Providers.SelectModel(provider.ModelTypeText)
	if llm == nil {
		return 0, "无可用 Text 模型"
	}

	// 构建上下文：最近消息 + 当前消息
	contextStr := "最近群聊消息:\n"
	for i, m := range recentMessages {
		contextStr += fmt.Sprintf("[%d] %s\n", i+1, m)
	}
	contextStr += fmt.Sprintf("\n当前消息 (发送者: %s, QQ: %d):\n%s",
		msg.Sender.Nickname, msg.UserID, msg.RawMessage)

	prompt := fmt.Sprintf(`你是一个群聊相关度判断助手。请根据以下群聊上下文判断当前消息是否与你（机器人）相关，是否需要你回复。

你的身份:
- 你的昵称: %s
- 你的QQ: %d
- 群聊中 at 你的格式: [CQ:at,qq=%d]

当前消息:
- 发送者昵称: %s
- 发送者QQ: %d

回复规则:
- 如果消息明确指向你（@你、叫你名字"%s"、称呼"机器人"/"bot"、询问你的能力、请求你操作），相关度应该高（>0.7）
- 如果消息是群友之间的闲聊、互怼、讨论与你无关的话题，相关度应该低（<0.3）
- 如果消息是群管理相关需求（踢人、禁言等），即使没有@你，相关度也应该高
- 如果消息是群友之间互相求助且没有@你，相关度应该低
- 如果消息是纯表情、无意义内容，相关度应该低
- 如果群聊处于热聊状态（多人连续发言讨论同一话题），可以适当提高相关度（0.3-0.5）

请以 JSON 格式回复，只包含 JSON 对象，不要有其他内容:
{"relevance": 0.0-1.0, "reason": "简短原因"}

上下文:
%s`, h.SelfNickname, selfQQ, selfQQ, msg.Sender.Nickname, msg.UserID, h.SelfNickname, contextStr)

	messages := []provider.ChatMessage{
		{Role: "system", Content: "你是一个群聊相关性判断助手。请以 JSON 格式回复。"},
		{Role: "user", Content: prompt},
	}

	req := provider.ChatRequest{
		Messages:    messages,
		Temperature: 0.3,
	}

	resp, err := llm.Chat(ctx, req)
	if err != nil {
		slog.Warn("相关性检查 LLM 调用失败", "err", err)
		return 0, "LLM 调用失败"
	}

	content := strings.TrimSpace(resp.Message.Content)
	var result RelevanceCheckResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// 尝试从响应中提取 JSON
		result = extractRelevanceJSON(content)
	}

	slog.Info("相关性检查结果", "relevance", result.Relevance, "reason", result.Reason, "raw", content)
	return result.Relevance, result.Reason
}

// extractRelevanceJSON 从 LLM 响应中提取 JSON
func extractRelevanceJSON(content string) RelevanceCheckResult {
	// 尝试找到 JSON 对象
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		jsonStr := content[start : end+1]
		var result RelevanceCheckResult
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
			return result
		}
	}
	return RelevanceCheckResult{Relevance: 0, Reason: "无法解析相关性结果"}
}

// extractImageURL 从消息段中提取图片 URL
func extractImageURL(seg adapter.Segment) string {
	if u, ok := seg.Data["url"]; ok {
		if s, ok := u.(string); ok && s != "" {
			return s
		}
	}
	if f, ok := seg.Data["file"]; ok {
		if s, ok := f.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// getRecentMessages 获取 ChatArea 中最近 N 条消息（不含当前消息）。
// 同时用于相关性检查和 Agent 工具。
func (h *HagoCenter) getRecentMessages(ctx context.Context, chatAreaID string, limit int) ([]string, error) {
	if h.Memory == nil {
		return nil, nil
	}
	stMsgs, err := h.Memory.GetShortTermMessages(ctx, chatAreaID)
	if err != nil {
		return nil, err
	}
	// 最近 N 条（倒序取）
	if len(stMsgs) > limit {
		stMsgs = stMsgs[len(stMsgs)-limit:]
	}
	var result []string
	for _, m := range stMsgs {
		result = append(result, fmt.Sprintf("[%s] %s", m.Role, m.Content))
	}
	return result, nil
}

// getRecentMessagesByMsgType 根据消息类型和目标 ID 获取最近消息（供工具使用）。
func (h *HagoCenter) getRecentMessagesByMsgType(ctx context.Context, msgType string, targetID int64, limit int) ([]string, error) {
	var chatAreaType models.AreaType
	switch msgType {
	case "private":
		chatAreaType = models.AreaTypePrivate
	case "group":
		chatAreaType = models.AreaTypeGroup
	default:
		return nil, nil
	}
	area, err := h.DAO.ChatArea.GetOrCreate(ctx, chatAreaType, targetID)
	if err != nil {
		return nil, err
	}
	return h.getRecentMessages(ctx, area.ID, limit)
}

// isAtSelf 检查消息是否 @ 了机器人自己。
// 优先使用 Adapter 实时 SelfID（防止 Init 时 QQ bot 尚未连接导致缓存为 0），
// 回退到启动时缓存的 SelfQQ。
func (h *HagoCenter) isAtSelf(rawMsg string) bool {
	selfQQ := h.SelfQQ
	if h.Adapter != nil {
		if id := h.Adapter.SelfID(); id != 0 {
			selfQQ = id
		}
	}
	if selfQQ == 0 {
		return false
	}
	return strings.Contains(rawMsg, fmt.Sprintf("[CQ:at,qq=%d]", selfQQ))
}