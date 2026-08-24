package groupmgr

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/metrics"
)

// LLM 审查参数（与旧插件一致）。
const (
	llmTimeout     = 60 * time.Second // 单次审查超时
	llmMaxText     = 600              // 送入 LLM 的消息最大长度（字符）
	llmDedupWindow = 600              // 同一条消息 10 分钟内不重复审查（秒）
	llmQueueSize   = 256              // 审查回调队列容量
)

// reviewCtx 一次异步 LLM 审查的现场上下文。
type reviewCtx struct {
	text     string   // 送审文本（RAG 命中时为样本文本，否则空 → 用原始消息）
	word     string   // 命中的关键词（可能为空）
	wordCat  string   // black/gray/sensitive/空
	kind     string   // gray / high-risk / card / rag-mid / rag-low
	highRisk bool     // 高危复核（异常回退直罚）
	hard     bool     // 有硬信号（词/卡片），LLM 异常时的直罚依据
	ragScore *float64 // RAG 分数（模棱两可分数兜底用）
}

// reviewResult LLM 裁决结果。
type reviewResult struct {
	Violation string `json:"violation"` // ad / sensitive / none
	Reason    string `json:"reason"`
}

// submitReview 提交异步 LLM 审查。返回 true = 已提交（等待回调裁决）；
// false = LLM 不可用/未启用/提交失败（调用方按兜底语义处理）。
func (m *Manager) submitReview(ctx context.Context, ev adapter.Event, rc reviewCtx) bool {
	cfg := m.getCfg(ctx)
	if !cfg.LLMReview {
		log.Info("LLM 审核未启用", "user", ev.Message.UserID)
		return false
	}
	p := m.providers.SelectModel(provider.ModelTypeText)
	if p == nil {
		log.Warn("无可用文本模型 Provider，LLM 审核跳过", "user", ev.Message.UserID)
		return false
	}

	msg := ev.Message
	pk := itoa(msg.GroupID) + ":" + itoa(msg.UserID)

	m.llmMu.Lock()
	if m.llmPending[pk] {
		m.llmMu.Unlock()
		return false // 同一用户已有在途审查（调用方按兜底处理：高危直罚/灰词放行）
	}
	// 同消息 10min 去重
	now := time.Now().Unix()
	if msg.MessageID > 0 {
		if ts, ok := m.llmReviewed[msg.MessageID]; ok && now-ts < llmDedupWindow {
			m.llmMu.Unlock()
			return false
		}
		m.llmReviewed[msg.MessageID] = now
		for id, ts := range m.llmReviewed { // 顺带清理过期
			if now-ts >= llmDedupWindow {
				delete(m.llmReviewed, id)
			}
		}
	}
	m.llmPending[pk] = true
	m.llmMu.Unlock()

	// 送审文本：剥离 CQ 码截断；RAG 样本文本仅作参考前缀
	text := strings.TrimSpace(stripCQ(msg.RawMessage))
	if text == "" {
		text = rc.text
	}
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > llmMaxText {
		text = string([]rune(text)[:llmMaxText])
	}

	// 提示词装配：高危复核 / 卡片复核（高危倾向）/ 常规审查（中性）
	sysPrompt := cfg.LLMGrayPrompt
	label := "灰色词"
	if sysPrompt == "" {
		sysPrompt = llmGrayPrompt
	}
	switch {
	case rc.kind == "card":
		sysPrompt = cfg.LLMHighRiskPrompt
		if sysPrompt == "" {
			sysPrompt = llmHighRiskPrompt
		}
		label = "推荐卡片信号"
	case rc.highRisk:
		sysPrompt = cfg.LLMHighRiskPrompt
		if sysPrompt == "" {
			sysPrompt = llmHighRiskPrompt
		}
		label = "高危关键词"
	}
	userPrompt := "待审查群消息（命中" + label
	if rc.word != "" {
		userPrompt += "：<" + rc.word + ">"
	}
	if rc.ragScore != nil {
		userPrompt += "，RAG 语义相似度 " + strconv.FormatFloat(*rc.ragScore, 'f', 3, 64)
	}
	userPrompt += "）：\n" + text
	if rc.text != "" && rc.text != text {
		userPrompt += "\n\n（参考：向量库最相似违规样本：「" + rc.text + "」）"
	}

	req := provider.ChatRequest{
		Messages: []provider.ChatMessage{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	// 异步执行：独立 goroutine，结果经内部 channel 由 Run 串行消费
	go func() {
		cctx, cancel := context.WithTimeout(context.Background(), llmTimeout)
		defer cancel()
		resp, err := p.Chat(cctx, req)
		out := reviewOutcome{
			groupID:   msg.GroupID,
			userID:    msg.UserID,
			messageID: msg.MessageID,
			admins:    ev.Admins,
			pk:        pk,
			rc:        rc,
			err:       err,
		}
		if resp != nil {
			out.content = resp.Message.Content
			out.tokens = resp.TokenUsage
		}
		m.llmResults <- out
	}()
	return true
}

// reviewOutcome 审查结果（channel 消息，Run 串行消费）。
type reviewOutcome struct {
	groupID, userID, messageID int64
	admins                     []string
	pk                         string
	rc                         reviewCtx
	content                    string
	tokens                     int
	err                        error
}

// Run 启动后台循环：串行消费 LLM 审查结果 + 管理员通知队列 pump。
func (m *Manager) Run(ctx context.Context) {
	log.Info("群管理后台循环已启动")
	for {
		select {
		case <-ctx.Done():
			log.Info("群管理后台循环已停止")
			return
		case out := <-m.llmResults:
			m.handleReview(ctx, out)
		}
	}
}

// handleReview 处理 LLM 审查结果（Run 内串行执行，天然互斥）。
func (m *Manager) handleReview(ctx context.Context, out reviewOutcome) {
	if out.tokens > 0 {
		metrics.LLMTokensTotal.WithLabelValues("review").Add(float64(out.tokens))
	}
	m.llmMu.Lock()
	delete(m.llmPending, out.pk)
	m.llmMu.Unlock()

	// 审查耗时期间可能已被加入白名单/成为管理员，复查后再处罚
	if m.isWhitelisted(ctx, out.userID) || m.isGroupAdmin(out.userID, out.admins, out.groupID) {
		return
	}

	ev := adapter.Event{
		PostType: "message",
		Admins:   out.admins,
		Message: &adapter.MessageEvent{
			MessageType: "group",
			GroupID:     out.groupID,
			UserID:      out.userID,
			MessageID:   out.messageID,
		},
	}

	highRisk := out.rc.highRisk
	if out.err != nil {
		log.Warn("LLM 审查失败", "err", out.err)
		metrics.GroupMgrLLMReviewsTotal.WithLabelValues("error").Inc()
		if highRisk && out.rc.hard {
			// 高危回退直罚（敏感/黑词/卡片）
			m.punish(ev, reasonByWord(out.rc.word, out.rc.wordCat, out.rc.kind == "card"), categoryByWordOrCard(out.rc.word, out.rc.wordCat, out.rc.kind == "card", "ad"), "keyword")
			return
		}
		if out.rc.ragScore != nil {
			// 模棱两可分数兜底
			cfg := m.getCfg(ctx)
			if *out.rc.ragScore >= cfg.FallbackScore {
				m.punish(ev, "RAG语义核实(LLM异常分数兜底)", categoryByWordOrCard(out.rc.word, out.rc.wordCat, out.rc.kind == "card", "ad"), "rag")
				return
			}
		}
		log.Info("LLM 审查失败，放行", "user", out.userID)
		return
	}

	var verdict reviewResult
	if err := json.Unmarshal([]byte(out.content), &verdict); err != nil {
		log.Warn("LLM 审查返回非 JSON，按失败处理", "content", out.content)
		metrics.GroupMgrLLMReviewsTotal.WithLabelValues("error").Inc()
		if highRisk && out.rc.hard {
			m.punish(ev, reasonByWord(out.rc.word, out.rc.wordCat, out.rc.kind == "card"), categoryByWordOrCard(out.rc.word, out.rc.wordCat, out.rc.kind == "card", "ad"), "keyword")
			return
		}
		return
	}
	switch verdict.Violation {
	case "ad", "sensitive":
		metrics.GroupMgrLLMReviewsTotal.WithLabelValues(verdict.Violation).Inc()
		category := verdict.Violation
		reason := verdict.Reason
		if reason == "" {
			reason = reasonByWord(out.rc.word, out.rc.wordCat, out.rc.kind == "card")
		}
		m.punish(ev, reason, category, "llm")
		// 学习闭环：LLM 确认违规 → 样本入库 + RAG upsert（异步不影响处罚）
		m.learnSample(ctx, out.content, ev, category)
	default:
		metrics.GroupMgrLLMReviewsTotal.WithLabelValues("none").Inc()
		log.Info("LLM 审查放行", "user", out.userID, "kind", out.rc.kind)
	}
}

// learnSample 学习闭环：LLM 确认违规的消息入库为样本（幂等去重）。
func (m *Manager) learnSample(ctx context.Context, raw string, ev adapter.Event, category string) {
	text := strings.TrimSpace(stripCQ(raw))
	if text == "" {
		text = strings.TrimSpace(stripCQ(ev.Message.RawMessage))
	}
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || len([]rune(text)) > 200 {
		return
	}
	id, err := m.dao.SampleAdd(ctx, text, category, "learn")
	if err != nil {
		log.Warn("学习样本入库失败", "err", err)
		return
	}
	m.upsertRAGSample(ctx, id, text)
	m.invalidateSampleSet()
	log.Info("违规样本已入库（学习闭环）", "sample", id, "category", category)
}
