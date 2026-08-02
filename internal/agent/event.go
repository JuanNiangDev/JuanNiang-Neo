package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/memory/shortterm"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/models"

	"github.com/cloudwego/eino/adk"
	einoschema "github.com/cloudwego/eino/schema"
)

// SilenceToken 是 LLM 在不回复时输出的固定标记。
// prompt 会告知 LLM："判定不回复时输出 __NO_REPLY__，系统检测到后自动丢弃"。
const SilenceToken = "__NO_REPLY__"

// ReplySettings 包含从回复策略配置中提取的 per-message 设置。
// 通过函数参数传递（而非 HagoCenter 共享字段），避免数据竞争。
type ReplySettings struct {
	Strategy           models.ReplyStrategy
	StripMarkdown      bool
	AgentLite          bool
	BotName            string
	RelevanceThreshold float64
}

// runEventLoop 是主事件循环，监听 OneBot11 事件并调用 Agent 处理。
// 当 adapter 的 events channel 关闭时（如重启），会尝试等待后重新获取新的 channel，
// 而不是直接退出事件循环。
func (h *HagoCenter) runEventLoop(ctx context.Context) {
	log.Info("事件循环已启动")
	adapterEvents := h.Adapter.Events()
	var webhookEvents <-chan adapter.Event
	if h.WebhookAdapter != nil {
		webhookEvents = h.WebhookAdapter.Events()
	}
	for {
		select {
		case <-ctx.Done():
			log.Info("事件循环已停止")
			return
		case ev, ok := <-adapterEvents:
			if !ok {
				log.Warn("Adapter events channel 已关闭，尝试重新获取...")
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				adapterEvents = h.Adapter.Events()
				continue
			}
			ev.Admins = h.Adapter.Admins()
			h.processEvent(ctx, ev)
		case ev, ok := <-webhookEvents:
			if !ok {
				webhookEvents = nil
				continue
			}
			h.processEvent(ctx, ev)
		case ev, ok := <-h.CronJobEvents:
			if !ok {
				continue
			}
			ev.Admins = h.Adapter.Admins()
			h.processEvent(ctx, ev)
		}
	}
}

// processEvent 三阶段架构：Plugin 拦截 → 消息过滤 → 回复策略 → Agent。
func (h *HagoCenter) processEvent(ctx context.Context, ev adapter.Event) {
	// Phase 1: Plugin 统一拦截
	if h.PluginEngine != nil {
		result := h.PluginEngine.Dispatch(ev)
		if result.Consumed {
			return
		}
		ev = result.Event
		// Phase 2: 仅 message 事件继续
		if ev.PostType != "message" || ev.Message == nil {
			return
		}
		// Phase 3: 回复策略检查 (skip_reply 标记时跳过)
		rs := h.getReplySettings(ctx)
		if !result.SkipReply && !ev.SkipReplyCheck {
			if !h.checkReplyStrategy(ctx, ev, rs) {
				return
			}
		}
		h.dispatchToAgent(ctx, ev, rs)
		return
	}

	// 无 Plugin 引擎时：只处理 message 事件
	if ev.PostType != "message" || ev.Message == nil {
		return
	}
	rs := h.getReplySettings(ctx)
	if !h.checkReplyStrategy(ctx, ev, rs) {
		return
	}
	h.dispatchToAgent(ctx, ev, rs)
}

// getReplySettings 从 DB 读取回复策略配置，返回 per-message 设置。
// 不写入 HagoCenter 共享字段，避免数据竞争。
func (h *HagoCenter) getReplySettings(ctx context.Context) ReplySettings {
	cfg, err := h.DAO.ReplyStrategy.GetOrCreate(ctx)
	if err != nil {
		log.Warn("获取回复策略失败，使用默认值", "err", err)
		return ReplySettings{Strategy: models.StrategyAlways}
	}
	return ReplySettings{
		Strategy:           cfg.Strategy,
		StripMarkdown:      cfg.StripMarkdown,
		AgentLite:          cfg.AgentLite,
		BotName:            cfg.BotName,
		RelevanceThreshold: cfg.RelevanceThreshold,
	}
}

// checkReplyStrategy 根据回复策略配置决定是否继续处理消息。
func (h *HagoCenter) checkReplyStrategy(ctx context.Context, ev adapter.Event, rs ReplySettings) bool {
	msg := ev.Message
	switch rs.Strategy {
	case models.StrategyNeverReply:
		log.Debug("回复策略: 完全不回复", "group_id", msg.GroupID, "user_id", msg.UserID)
		return false
	case models.StrategyAtOnly:
		if msg.MessageType == "group" && !h.isAtSelf(msg.RawMessage) {
			log.Debug("回复策略: 仅@我时回复，跳过", "group_id", msg.GroupID, "user_id", msg.UserID)
			return false
		}
		return true
	case models.StrategyRelevance:
		// 仅对群聊 message 做相关性判断
		if msg.MessageType != "group" {
			return true
		}
		if h.isAtSelf(msg.RawMessage) || h.isPluginCommand(msg.RawMessage) {
			return true
		}
		chatAreaID := ""
		area, err := h.DAO.ChatArea.GetOrCreate(ctx, models.AreaTypeGroup, msg.GroupID)
		if err == nil {
			chatAreaID = area.ID
		}
		recentMsgs, _ := h.getRecentMessages(ctx, chatAreaID, 10)
		score, reason := h.relevanceAgentEvaluate(ctx, msg, recentMsgs, rs.BotName)
		if score < rs.RelevanceThreshold {
			log.Debug("回复策略: 相关性不足", "score", score, "threshold", rs.RelevanceThreshold, "reason", reason)
			return false
		}
		log.Debug("回复策略: 相关性通过", "score", score, "reason", reason)
		return true
	default: // StrategyAlways
		return true
	}
}

// dispatchToAgent 根据消息类型获取 ChatArea，并通过并发控制派发给 Agent。
func (h *HagoCenter) dispatchToAgent(ctx context.Context, ev adapter.Event, rs ReplySettings) {
	msg := ev.Message
	chatArea := h.getChatArea(ctx, msg)
	if chatArea == nil {
		// 无法获取 ChatArea 时仍尝试处理（不限制并发）
		h.handleMessage(ctx, ev, nil, rs)
		return
	}

	// 异步执行：goroutine + 并发控制
	if h.Concurrency != nil {
		go func() {
			if err := h.Concurrency.Acquire(ctx, chatArea.ID); err != nil {
				log.Warn("Agent 并发获取失败", "err", err, "area", chatArea.ID)
				return
			}
			defer h.Concurrency.Release(chatArea.ID)
			h.handleMessage(ctx, ev, chatArea, rs)
		}()
	} else {
		h.handleMessage(ctx, ev, chatArea, rs)
	}
}

// getChatArea 根据消息类型获取或创建 ChatArea，失败返回 nil。
func (h *HagoCenter) getChatArea(ctx context.Context, msg *adapter.MessageEvent) *models.ChatArea {
	var chatAreaType models.AreaType
	var targetID int64
	switch msg.MessageType {
	case "private":
		chatAreaType = models.AreaTypePrivate
		targetID = msg.UserID
	case "group":
		chatAreaType = models.AreaTypeGroup
		targetID = msg.GroupID
	default:
		return nil
	}
	area, err := h.DAO.ChatArea.GetOrCreate(ctx, chatAreaType, targetID)
	if err != nil {
		log.Error("获取 ChatArea 失败", "err", err)
		return nil
	}
	return area
}

func (h *HagoCenter) handleMessage(ctx context.Context, ev adapter.Event, chatArea *models.ChatArea, rs ReplySettings) {
	msg := ev.Message
	userID := msg.UserID

	// 如果没有传入 chatArea，尝试获取（fallback 路径）
	if chatArea == nil {
		chatArea = h.getChatArea(ctx, msg)
		if chatArea == nil {
			return
		}
	}

	// ACL 检查
	if !isAdmin(userID, ev.Admins) && !h.ACL.CheckChat(ctx, userID, chatArea.ID) {
		log.Info("ACL 拒绝", "user_id", userID, "chat_area_id", chatArea.ID)
		return
	}

	// 确保 Session 存在（供 ChatRecord 持久化使用）
	if _, err := h.Session.GetOrCreate(ctx, chatArea.ID); err != nil {
		log.Error("获取 Session 失败", "err", err)
		return
	}

	userMsg := strings.TrimSpace(msg.RawMessage)
	matchedSkill, skillMatched := h.Skills.Match(userMsg)

	// ---------- 构建系统提示词（工具描述 + 长期记忆 + 核心提示词） ----------
	var longTermMems []string
	if h.Memory != nil {
		longTermMems, _ = h.Memory.GetLongTermMemory(ctx, chatArea.ID, "", 5)
	}
	toolList := h.buildToolList(ctx)
	toolDescs := ""
	for _, t := range toolList {
		toolDescs += fmt.Sprintf("- %s: %s\n", t.Function.Name, t.Function.Description)
	}

	sessionCtxStr := h.buildSessionContext(ctx, msg, ev.Admins)
	skillMem := ""
	if h.Memory != nil {
		skillMem = h.Memory.GetSkillMemory()
	}
	systemCtx, _ := h.Prompt.BuildFullContext(ctx, longTermMems, toolDescs, skillMem)

	// 缓存 Skill prompt（避免重复查询 DB）
	var skillPromptContents []string
	if skillMatched {
		for _, refID := range matchedSkill.PromptRefs {
			if refID == "" {
				continue
			}
			sp, err := h.Prompt.GetByID(ctx, refID)
			if err == nil {
				skillPromptContents = append(skillPromptContents, "[Active Skill: "+matchedSkill.Name+"]\n"+sp.Content)
			}
		}
	}

	// ---------- 构建 Eino 消息列表 ----------
	einoMsgs := []*einoschema.Message{
		{Role: einoschema.System, Content: systemCtx},
	}
	if sessionCtxStr != "" {
		einoMsgs = append(einoMsgs, &einoschema.Message{Role: einoschema.System, Content: sessionCtxStr})
	}
	for _, spc := range skillPromptContents {
		einoMsgs = append(einoMsgs, &einoschema.Message{Role: einoschema.System, Content: spc})
	}
	if h.Memory != nil {
		stMsgs, err := h.Memory.GetShortTermMessages(ctx, chatArea.ID)
		if err == nil {
			for _, m := range stMsgs {
				einoMsgs = append(einoMsgs, &einoschema.Message{Role: einoschema.RoleType(m.Role), Content: m.Content, Name: m.Name})
			}
		}
	}
	einoMsgs = append(einoMsgs, &einoschema.Message{Role: einoschema.User, Content: userMsg})

	// 写短期记忆 + 持久化聊天记录
	if h.Memory != nil {
		h.Memory.AddShortTermMessage(ctx, chatArea.ID, shortterm.ChatMessage{Role: "user", Content: userMsg})
	}
	h.Session.AppendRecord(ctx, chatArea.ID, userID, "user", userMsg, 0, nil)

	// ---------- 回复策略 & AgentLite ----------
	skipSilenceCheck := rs.Strategy == models.StrategyAlways
	agentLite := rs.AgentLite

	// 构建注入给 Eino Agent 的系统指令（Instruction）
	instruction := systemCtx
	if sessionCtxStr != "" {
		instruction += "\n\n" + sessionCtxStr
	}
	for _, spc := range skillPromptContents {
		instruction += "\n\n" + spc
	}
	if agentLite {
		instruction = "【AgentLite 精简模式】你无法使用任何工具、MCP 或外部能力。" +
			"如果用户提出的任务需要工具或外部操作才能完成（如发送消息、查天气、生成图片、执行代码、搜索等），" +
			"直接拒绝并说明当前处于精简模式无法执行该操作。回复中不要假装已经完成了需要工具的任务。\n\n" + instruction
	}

	// 将 per-message 状态注入 context（避免 HagoCenter 共享字段数据竞争）
	msgCtx := &MsgSessionCtx{
		Msg:                msg,
		SessionCtxStr:      sessionCtxStr,
		RecentMsgsFn:       h.getRecentMessagesByMsgType,
		DynamicInstruction: instruction,
		AgentLite:          agentLite,
		StripMarkdown:      rs.StripMarkdown,
		DisableSplit:       rs.AgentLite,
	}
	ctx = WithMsgSessionCtx(ctx, msgCtx)

	// ---------- 运行 Eino Agent ----------
	if h.EinoAgent == nil {
		log.Error("Eino Agent 未就绪")
		return
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: h.EinoAgent})
	iter := runner.Run(ctx, einoMsgs)

	// 收集 Agent 输出
	var assistantContent string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			log.Error("Eino Agent 错误", "err", event.Err)
			break
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		if mv.Role == einoschema.Assistant {
			if !mv.IsStreaming && mv.Message != nil {
				assistantContent += mv.Message.Content
			}
		}
	}

	// ---------- 后处理：静默检测 + 发送 + 记忆 ----------
	if assistantContent != "" {
		silenced := !skipSilenceCheck && msg.MessageType == "group" && isSilenceResponse(assistantContent)
		if silenced {
			log.Info("群聊静默响应已丢弃", "content", assistantContent, "group_id", msg.GroupID)
		} else {
			h.sendReply(msg, assistantContent, rs)
			h.recordChat(ctx, chatArea.ID, userID, "assistant", assistantContent, 0, nil)
			if h.Memory != nil {
				h.Memory.AddShortTermMessage(ctx, chatArea.ID, shortterm.ChatMessage{Role: "assistant", Content: assistantContent})
			}
		}
	}
}

// cqCodeRegexp 匹配 CQ 码: [CQ:type,key=value,...]
var cqCodeRegexp = regexp.MustCompile(`\[CQ:[a-zA-Z_]+(?:,[^\]]+)?\]`)

// urlRegexp 匹配 URL，提取为包级变量避免每次调用 splitMessages 时重新编译。
var urlRegexp = regexp.MustCompile(`https?://\S+`)

// sendReply 解析 CQ 码并组装消息段发送。
// rs 从调用链传入，避免读取 HagoCenter 共享字段导致数据竞争。
func (h *HagoCenter) sendReply(msg *adapter.MessageEvent, content string, rs ReplySettings) {
	var parts []string
	if rs.AgentLite {
		parts = []string{content}
	} else {
		parts = splitMessages(content)
	}
	for _, part := range parts {
		if rs.StripMarkdown {
			part = stripMarkdown(part)
		}
		// 解析 CQ 码并组装消息段
		segments := parseCQToSegments(part)
		var err error
		switch msg.MessageType {
		case "private":
			_, err = h.Adapter.SendPrivateMsg(msg.UserID, segments)
		case "group":
			_, err = h.Adapter.SendGroupMsg(msg.GroupID, segments)
		}
		if err != nil {
			log.Error("发送消息失败", "err", err)
		}
	}
}

// parseCQToSegments 将包含 CQ 码的文本解析为 adapter.Segment 数组。
// 纯文本部分 → {Type: "text", Data: {"text": "..."}}
// CQ 码部分 → {Type: "image", Data: {"file": "..."}} 等
func parseCQToSegments(content string) []adapter.Segment {
	var segments []adapter.Segment
	lastIdx := 0
	for _, loc := range cqCodeRegexp.FindAllStringIndex(content, -1) {
		// 前面的纯文本
		if loc[0] > lastIdx {
			text := content[lastIdx:loc[0]]
			if text != "" {
				segments = append(segments, adapter.Segment{Type: "text", Data: map[string]any{"text": text}})
			}
		}
		// CQ 码
		cqStr := content[loc[0]:loc[1]]
		seg := parseCQCode(cqStr)
		segments = append(segments, seg)
		lastIdx = loc[1]
	}
	// 尾部纯文本
	if lastIdx < len(content) {
		text := content[lastIdx:]
		if text != "" {
			segments = append(segments, adapter.Segment{Type: "text", Data: map[string]any{"text": text}})
		}
	}
	if len(segments) == 0 {
		segments = append(segments, adapter.Segment{Type: "text", Data: map[string]any{"text": content}})
	}
	return segments
}

// parseCQCode 解析单个 CQ 码字符串为 Segment。
// 例如: "[CQ:image,file=http://example.com/img.jpg]" → {Type: "image", Data: {"file": "http://..."}}
func parseCQCode(s string) adapter.Segment {
	// 去掉 [CQ: 和 ]
	inner := s[4 : len(s)-1] // "image,file=http://..."
	parts := strings.SplitN(inner, ",", 2)
	segType := parts[0]
	data := make(map[string]any)
	if len(parts) > 1 {
		for _, kv := range strings.Split(parts[1], ",") {
			eq := strings.IndexByte(kv, '=')
			if eq > 0 {
				data[kv[:eq]] = kv[eq+1:]
			}
		}
	}
	return adapter.Segment{Type: segType, Data: data}
}

// splitMessages 将 Agent 输出拆分为最多 3 段消息。
// 算法参考 Maibot：在自然断句处（。！？；\n）拆分，每段有效文字 ≤60 字。
// CQ 码和 URL 不计入有效字数。保留原始分隔符。
func splitMessages(content string) []string {
	effectiveLen := func(s string) int {
		s = cqCodeRegexp.ReplaceAllString(s, "")
		s = urlRegexp.ReplaceAllString(s, "")
		return len([]rune(strings.TrimSpace(s)))
	}

	total := effectiveLen(content)
	if total <= 60 {
		return []string{content}
	}

	// 按自然断句拆分，保留分隔符附着在前一段尾部
	splitRe := regexp.MustCompile(`[。！？；\n]`)
	matches := splitRe.FindAllStringIndex(content, -1)
	if len(matches) == 0 {
		return []string{content}
	}
	var parts []string
	start := 0
	for _, loc := range matches {
		// 包含分隔符在当前段尾部
		parts = append(parts, content[start:loc[1]])
		start = loc[1]
	}
	if start < len(content) {
		tail := strings.TrimSpace(content[start:])
		if tail != "" {
			parts = append(parts, tail)
		}
	}
	// 过滤空段
	var nonEmpty []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) <= 1 {
		return []string{content}
	}

	// 贪心合并到最多 3 段，每段 ≤60 有效字
	maxSegs := 3
	if total <= 120 {
		maxSegs = 2
	}
	var segments []string
	var buf string
	for _, p := range nonEmpty {
		candidate := buf + p
		if effectiveLen(candidate) > 60 && buf != "" {
			segments = append(segments, strings.TrimSpace(buf))
			buf = p
		} else {
			buf = candidate
		}
	}
	if buf != "" {
		segments = append(segments, strings.TrimSpace(buf))
	}

	// 硬限制 3 段：合并尾部多余的段
	for len(segments) > maxSegs {
		last := segments[len(segments)-1]
		prev := segments[len(segments)-2]
		segments = segments[:len(segments)-2]
		segments = append(segments, prev+last)
	}

	if len(segments) <= 1 {
		return []string{content}
	}
	return segments
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
		log.Error("记录聊天失败", "err", err)
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
