package agent

import (
	"context"
	"fmt"
	"strings"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/core/models"
)

// isDefinitelyRelevant 规则快路径（L1）：消息明显指向机器人，无需攒窗直接视为必回。
// 调用方已先处理 @ 自己 / 插件命令 / 私聊；这里覆盖"提及名字/常见称呼"。
func isDefinitelyRelevant(msg *adapter.MessageEvent, rs ReplySettings) bool {
	text := strings.TrimSpace(msg.RawMessage)
	if text == "" {
		return false
	}
	if rs.BotName != "" && strings.Contains(text, rs.BotName) {
		return true
	}
	for _, kw := range []string{"机器人", "bot", "Bot", "BOT"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// isDefinitelyIrrelevant 规则快路径（L1）：明显无关的噪音消息，直接丢弃不进参与窗口。
// 覆盖：剥离 CQ 码/URL 后无有效文字（纯表情包/纯图片）、过短消息（≤2 字）、
// 纯 emoji/符号（无 CJK/字母数字）且较短。
func isDefinitelyIrrelevant(msg *adapter.MessageEvent) bool {
	text := strings.TrimSpace(msg.RawMessage)
	// 剥离 CQ 码与 URL 后取纯文本
	plain := cqCodeRegexp.ReplaceAllString(text, "")
	plain = urlRegexp.ReplaceAllString(plain, "")
	plain = strings.TrimSpace(plain)

	runes := []rune(plain)
	// 过短（≤2 个有效字符）→ 噪音（"哈哈"、"666"、"1"、纯图片无文字等）
	if len(runes) <= 2 {
		return true
	}
	// 纯 emoji/符号（无 CJK/字母数字）且较短 → 噪音（"😂😂😂"）
	if len(runes) <= 6 && !containsMeaningfulChars(plain) {
		return true
	}
	return false
}

// containsMeaningfulChars 判断文本是否包含有意义字符（ASCII 字母数字或 CJK）。
func containsMeaningfulChars(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return true
		}
		if r >= 0x4E00 && r <= 0x9FFF { // CJK 统一表意文字
			return true
		}
	}
	return false
}

// getRecentMessages 获取 ChatArea 中最近 N 条消息（不含当前消息）。
// 同时用于 Agent 工具（get_recent_messages）的上下文召回。
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
// SelfID 优先级：事件自带的 self_id（多连接时确定性归属，不依赖 map 遍历顺序）
// > Adapter 实时 SelfID > 启动时缓存的 SelfQQ。
func (h *HagoCenter) isAtSelf(rawMsg string, eventSelfID int64) bool {
	selfQQ := h.SelfQQ
	if eventSelfID != 0 {
		selfQQ = eventSelfID
	}
	if selfQQ == 0 && h.Adapter != nil {
		if id := h.Adapter.SelfID(); id != 0 {
			selfQQ = id
		}
	}
	if selfQQ == 0 {
		return false
	}
	return strings.Contains(rawMsg, fmt.Sprintf("[CQ:at,qq=%d]", selfQQ))
}

// isPluginCommand 检查消息是否是已注册的插件命令（以 / 开头且在命令注册表中存在）。
func (h *HagoCenter) isPluginCommand(rawMsg string) bool {
	if h.PluginEngine == nil {
		return false
	}
	return h.PluginEngine.HasPluginCommand(rawMsg)
}
