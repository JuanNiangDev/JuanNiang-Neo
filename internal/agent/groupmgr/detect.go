package groupmgr

import (
	"context"
	"strings"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/metrics"
)

// QQ 群聊推荐卡片：OneBot 11 json 消息段 data 中的 app 标识（计入广告违规）。
var qqCardApps = []string{
	"com.tencent.contact.lua",    // 推荐联系人
	"com.tencent.troopsharecard", // 推荐群聊卡片
}

// stripCQ 剥离 CQ 码（避免命中 json 卡片/图片等富文本 payload）。
func stripCQ(raw string) string {
	return cqCodeRe.ReplaceAllString(raw, " ")
}

// detectGroupCard 检测 QQ 群聊推荐卡片（只在 CQ 段内匹配 app，避免命中段外文本）。
func detectGroupCard(raw string) bool {
	if raw == "" {
		return false
	}
	lower := strings.ToLower(raw)
	pos := 0
	for {
		s := strings.Index(lower[pos:], "[cq:json")
		if s < 0 {
			return false
		}
		s += pos
		e := strings.Index(lower[s:], "]")
		if e < 0 {
			e = len(lower) - s
		}
		segment := lower[s : s+e]
		for _, app := range qqCardApps {
			if strings.Contains(segment, app) {
				return true
			}
		}
		pos = s + e
	}
}

// detectMessage 群消息检测入口（Phase 0.5）。
// 顺序与旧插件一致：违禁言论（不消费）→ 图片刷屏（消费）→ +1 复读（消费）。
func (m *Manager) detectMessage(ctx context.Context, ev adapter.Event, cfg *models.GroupMgrConfig) bool {
	m.detectViolation(ctx, ev, cfg)
	if m.checkImageSpam(ctx, ev, cfg) {
		return true
	}
	if m.checkCopySpam(ctx, ev, cfg) {
		return true
	}
	return false
}

// detectViolation 违禁言论检测：卡片文本化 → RAG 核实（首选）→ 降级关键词。
// 返回 true 仅表示"已发起同步处罚"，不影响消费语义（违禁类一律不消费消息）。
func (m *Manager) detectViolation(ctx context.Context, ev adapter.Event, cfg *models.GroupMgrConfig) bool {
	msg := ev.Message
	raw := msg.RawMessage
	text := strings.TrimSpace(stripCQ(raw))
	if text == "" && !detectGroupCard(raw) {
		return false
	}
	card := detectGroupCard(raw)
	word, wordCat := m.wordHit(ctx, text)

	// 第一核实人：RAG 语义核实（卡片/词/无词统一跑）
	if v := m.verifyByRAG(ctx, text); v.ok {
		return m.handleRAGVerdict(ctx, ev, cfg, card, word, wordCat, v)
	}
	// RAG 不可用 → 降级关键词路径（= 旧插件行为）
	return m.handleKeywordPath(ctx, ev, cfg, card, word, wordCat)
}

// handleRAGVerdict RAG 核实后的分档决策：
//
//	高置信 ≥ HighScore          → 直接处罚（须命中词/卡片硬信号；卡片判 ad）
//	模棱两可 (LowScore, High)   → LLM 审核（须命中词/卡片硬信号；LLM 异常用 FallbackScore 分数兜底）
//	低置信 ≤ LowScore           → 有词/卡片送 LLM（硬信号终审）；无词放行
//
// 三档均要求硬信号（命中关键词或推荐卡片）：RAG-Service 向量库与知识/记忆共用，
// 无硬信号时高置信命中可能是知识/记忆的语义干扰，不应仅凭向量分数处罚。
// 无硬信号一律放行（回退到关键词路径已先判过，无词即无违规信号）。
func (m *Manager) handleRAGVerdict(ctx context.Context, ev adapter.Event, cfg *models.GroupMgrConfig,
	card bool, word, wordCat string, v ragVerdict) bool {
	high := v.score >= cfg.HighScore
	mid := v.score > cfg.LowScore && v.score < cfg.HighScore
	hasHardSignal := word != "" || card

	// 三档统一：无硬信号（未命中关键词且无推荐卡片）→ 放行
	// RAG-Service 向量库与知识/记忆共用同一实例，无硬信号时高置信命中
	// 可能是知识/记忆的语义干扰，不应仅凭向量分数处罚。
	if !hasHardSignal {
		metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "pass").Inc()
		log.Info("RAG 命中但无关键词/卡片硬信号，放行（防知识/记忆语义干扰）", "score", v.score, "user", ev.Message.UserID)
		return false
	}

	switch {
	case high:
		// 直接处罚：类别优先词类别 → 样本类别 → 卡片/黑词默认 ad
		category := "ad"
		switch {
		case wordCat == "sensitive":
			category = "sensitive"
		case wordCat == "gray" || wordCat == "black":
			category = "ad"
		case v.category == "sensitive":
			category = "sensitive"
		}
		reason := "RAG语义核实"
		if word != "" {
			reason = "RAG语义核实(" + word + ")"
		} else if card {
			reason = "RAG语义核实(推荐卡片)"
		}
		metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "punish").Inc()
		m.punish(ev, reason, category, "rag")
		m.sampleHit(ctx, v.tag)
		return true
	case mid:
		// 模棱两可 → LLM 审核
		if m.submitReview(ctx, ev, reviewCtx{
			text:     v.text,
			word:     word,
			wordCat:  wordCat,
			kind:     reviewKindBySignal(word, wordCat, card, "rag-mid"),
			highRisk: false,
			ragScore: &v.score,
			hard:     hasHardSignal,
		}) {
			metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "review").Inc()
			return true
		}
		// LLM 不可用：分数兜底（≥ FallbackScore 直罚）
		if v.score >= cfg.FallbackScore {
			metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "punish").Inc()
			m.punish(ev, "RAG语义核实(LLM不可用分数兜底)", categoryByWordOrCard(word, wordCat, card, v.category), "rag")
			return true
		}
		metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "pass").Inc()
		log.Info("RAG 模棱两可且 LLM 不可用，分数兜底放行", "score", v.score, "user", ev.Message.UserID)
		return false
	default:
		// 低置信：词/卡片是硬信号 → LLM 终审
		if m.submitReview(ctx, ev, reviewCtx{
			text:     v.text,
			word:     word,
			wordCat:  wordCat,
			kind:     reviewKindBySignal(word, wordCat, card, "rag-low"),
			highRisk: wordCat == "sensitive" || wordCat == "black" || card,
			hard:     true,
		}) {
			metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "review").Inc()
			return true
		}
		// LLM 不可用：回归旧语义——敏感/黑词/卡片直罚，灰词放行
		if wordCat == "sensitive" || wordCat == "black" || card {
			metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "punish").Inc()
			m.punish(ev, reasonByWord(word, wordCat, card), categoryByWordOrCard(word, wordCat, card, v.category), "rag")
			return true
		}
		metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "pass").Inc()
		return false
	}
}

// handleKeywordPath RAG 不可用时的降级路径（= 旧插件行为）。
func (m *Manager) handleKeywordPath(ctx context.Context, ev adapter.Event, cfg *models.GroupMgrConfig,
	card bool, word, wordCat string) bool {
	switch {
	case wordCat == "sensitive" || wordCat == "black" || card:
		// 高危复核；LLM 不可用 → 直接处罚
		kind := "high-risk"
		if card && word == "" {
			kind = "card"
		}
		if m.submitReview(ctx, ev, reviewCtx{
			word: word, wordCat: wordCat, kind: kind, highRisk: true, hard: true,
		}) {
			metrics.GroupMgrDetectionsTotal.WithLabelValues("keyword", "review").Inc()
			return true
		}
		metrics.GroupMgrDetectionsTotal.WithLabelValues("keyword", "punish").Inc()
		m.punish(ev, reasonByWord(word, wordCat, card), categoryByWordOrCard(word, wordCat, card, "ad"), "keyword")
		return true
	case wordCat == "gray":
		// 常规审查；LLM 不可用 → 放行（异步追罚语义）
		if m.submitReview(ctx, ev, reviewCtx{
			word: word, wordCat: "gray", kind: "gray", highRisk: false, hard: false,
		}) {
			metrics.GroupMgrDetectionsTotal.WithLabelValues("keyword", "review").Inc()
		} else {
			metrics.GroupMgrDetectionsTotal.WithLabelValues("keyword", "pass").Inc()
		}
		return false
	default:
		metrics.GroupMgrDetectionsTotal.WithLabelValues("keyword", "pass").Inc()
		return false
	}
}

// reviewKindBySignal 决定 LLM 审查类型。
func reviewKindBySignal(word, wordCat string, card bool, base string) string {
	switch {
	case card:
		return "card"
	case wordCat == "sensitive" || wordCat == "black":
		return "high-risk"
	case wordCat == "gray":
		return "gray"
	default:
		return base // rag-mid / rag-low：无词语义疑似
	}
}

func categoryByWordOrCard(word, wordCat string, card bool, sampleCat string) string {
	if card {
		return "ad"
	}
	switch wordCat {
	case "sensitive":
		return "sensitive"
	case "black", "gray":
		return "ad"
	}
	if sampleCat == "sensitive" {
		return "sensitive"
	}
	return "ad"
}

func reasonByWord(word, wordCat string, card bool) string {
	switch {
	case card:
		return "广告违规：推荐群聊卡片"
	case wordCat == "sensitive":
		return "敏感违规：" + word
	case wordCat == "black":
		return "广告违规(黑名单)：" + word
	case wordCat == "gray":
		return "广告违规(灰色词)：" + word
	default:
		return "违规内容"
	}
}
