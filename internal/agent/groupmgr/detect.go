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
// 顺序：违禁言论（不消费）→ 图片刷屏（消费）→ +1 复读（消费）。
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

// detectViolation 违禁言论检测：RAG 语义匹配（第一核实人）→ LLM 统一判定 → 关键词兜底。
// 流程：
//  1. RAG 检索黑/白语录双集合：
//     黑名单命中（score ≥ BlackMinScore）→ 处罚；白名单命中（score ≥ WhiteMinScore）→ 放行；
//  2. 均未命中 → LLM 批量判定（3s 窗口凑批，逐条独立黑白判定）；
//  3. RAG/LLM 均不可用 → 关键词兜底（敏感/黑词直罚、灰词 LLM/放行）。
//
// 返回 true 仅表示"已发起同步处罚"，不影响消费语义（违禁类一律不消费消息）。
func (m *Manager) detectViolation(ctx context.Context, ev adapter.Event, cfg *models.GroupMgrConfig) bool {
	msg := ev.Message
	raw := msg.RawMessage
	text := strings.TrimSpace(stripCQ(raw))
	if text == "" && !detectGroupCard(raw) {
		return false
	}
	card := detectGroupCard(raw)
	// 关键词命中仅作最后兜底（RAG/LLM 均不可用时），不参与 RAG 判据
	word, wordCat := m.wordHit(ctx, text)

	// 第一核实人：RAG 语义匹配（黑白语录双集合）
	if v := m.verifyByRAG(ctx, text, true); v.ok {
		return m.handleRAGMatch(ctx, ev, cfg, card, word, wordCat, v)
	}
	// RAG 不可用 → 关键词兜底（= 旧插件行为）
	return m.handleKeywordPath(ctx, ev, cfg, card, word, wordCat)
}

// handleRAGMatch RAG 语义匹配后的分档决策：
//
//	黑名单命中 score ≥ BlackMinScore  → 直接处罚（白名单即使更高分也以黑优先，fail-closed）
//	白名单命中 score ≥ WhiteMinScore  → 放行
//	均未达阈值                         → LLM 统一判定（批窗口）；LLM 不可用 → 关键词兜底
func (m *Manager) handleRAGMatch(ctx context.Context, ev adapter.Event, cfg *models.GroupMgrConfig,
	card bool, word, wordCat string, v ragVerdict) bool {
	// 黑名单优先（fail-closed）：黑白同时命中且都过阈值时按黑处罚
	if v.black != nil && v.black.score >= cfg.BlackMinScore {
		category := "ad"
		if v.black.category == "sensitive" {
			category = "sensitive"
		}
		reason := "RAG黑名单语义匹配"
		metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "punish").Inc()
		// 检索追踪日志：方式=RAG + 命中集合/分数 + 命中语录前 20 字
		log.Info("违禁检测: 方式=RAG", "list", "black", "score", v.black.score, "hit", headText(v.black.text, 20), "user", ev.Message.UserID)
		m.punish(ctx, ev, reason, category, "rag")
		m.phraseHit(ctx, v.black.tag)
		log.Info("RAG 黑名单命中，处罚", "score", v.black.score, "phrase", v.black.text, "user", ev.Message.UserID)
		return true
	}
	// 白名单：命中即放行（须达阈值，防低分噪声误放行）
	if v.white != nil && v.white.score >= cfg.WhiteMinScore {
		// 检索追踪日志：方式=RAG + 命中集合/分数 + 命中语录前 20 字
		log.Info("违禁检测: 方式=RAG", "list", "white", "score", v.white.score, "hit", headText(v.white.text, 20), "user", ev.Message.UserID)
		m.phraseTouch(ctx, v.white.tag)
		metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "pass").Inc()
		log.Info("RAG 白名单命中，放行", "score", v.white.score, "phrase", v.white.text, "user", ev.Message.UserID)
		return false
	}

	// 均未达阈值 → LLM 统一判定（批窗口异步，不阻塞主循环）
	// 检索追踪日志：方式=RAG 但未命中黑白语录（含知识/记忆向量干扰时的低分命中）→ 送 LLM
	if v.black == nil && v.white == nil {
		log.Info("违禁检测: 方式=RAG未命中", "list", "none", "score", 0.0, "hit", "", "user", ev.Message.UserID)
	} else {
		var score float64
		var hit string
		var list string
		if v.black != nil {
			score, hit, list = v.black.score, v.black.text, "black"
		} else if v.white != nil {
			score, hit, list = v.white.score, v.white.text, "white"
		}
		log.Info("违禁检测: 方式=RAG未达阈值", "list", list, "score", score, "hit", headText(hit, 20), "user", ev.Message.UserID)
	}
	rc := reviewCtx{word: word, wordCat: wordCat, card: card}
	if v.black != nil {
		rc.ragScore = &v.black.score
		rc.ragPhrase = v.black.text
	} else if v.white != nil {
		rc.ragScore = &v.white.score
		rc.ragPhrase = v.white.text
	}
	if m.submitReview(ctx, ev, rc) {
		metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "review").Inc()
		return true
	}
	// LLM 不可用 → 关键词兜底
	metrics.GroupMgrDetectionsTotal.WithLabelValues("rag", "pass").Inc()
	return m.handleKeywordPath(ctx, ev, cfg, card, word, wordCat)
}

// handleKeywordPath 关键词兜底路径（仅 RAG 或 LLM 不可用时使用，= 旧插件行为）。
func (m *Manager) handleKeywordPath(ctx context.Context, ev adapter.Event, cfg *models.GroupMgrConfig,
	card bool, word, wordCat string) bool {
	switch {
	case wordCat == "sensitive" || wordCat == "black" || card:
		// 检索追踪日志：方式=关键词兜底（高危词命中）
		log.Info("违禁检测: 方式=关键词", "kind", "high-risk", "word", headText(word, 20), "cat", wordCat, "card", card, "user", ev.Message.UserID)
		// 高危复核；LLM 不可用 → 直接处罚
		kind := "high-risk"
		if card && word == "" {
			kind = "card"
		}
		if m.submitReview(ctx, ev, reviewCtx{
			word: word, wordCat: wordCat, kind: kind, highRisk: true, hard: true, card: card,
		}) {
			metrics.GroupMgrDetectionsTotal.WithLabelValues("keyword", "review").Inc()
			return true
		}
		metrics.GroupMgrDetectionsTotal.WithLabelValues("keyword", "punish").Inc()
		m.punish(ctx, ev, reasonByWord(word, wordCat, card), categoryByWordOrCard(word, wordCat, card, "ad"), "keyword")
		return true
	case wordCat == "gray":
		// 检索追踪日志：方式=关键词兜底（灰色词命中）
		log.Info("违禁检测: 方式=关键词", "kind", "gray", "word", headText(word, 20), "cat", wordCat, "card", card, "user", ev.Message.UserID)
		// 常规审查；LLM 不可用 → 放行（异步追罚语义）
		if m.submitReview(ctx, ev, reviewCtx{
			word: word, wordCat: "gray", kind: "gray", highRisk: false, hard: false, card: card,
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

// categoryByWordOrCard 处罚分类：推荐卡片优先 ad；敏感词 → sensitive；黑/灰词 → ad；
// 否则取样本分类（sensitive → sensitive，其余 ad）。
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

// reasonByWord 关键词兜底路径的违规理由文案（按命中类别/卡片拼装）。
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
