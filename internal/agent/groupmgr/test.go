package groupmgr

import (
	"context"
	"strings"
)

// TestReport 链路测试报告（Web 面板粘贴文本 → 判定流水）。
type TestReport struct {
	Text        string  `json:"text"`
	Card        bool    `json:"card"`         // 推荐卡片命中
	Word        string  `json:"word"`         // 命中关键词（空=无）
	WordCat     string  `json:"word_cat"`     // black/gray/sensitive/空
	RAGOK       bool    `json:"rag_ok"`       // RAG 路径是否可用
	RAGScore    float64 `json:"rag_score"`    // 最高相似度分
	RAGSample   string  `json:"rag_sample"`   // 最相似样本
	RAGCategory string  `json:"rag_category"` // 最相似样本类别
	Verdict     string  `json:"verdict"`      // 最终判定：punish / review / pass / keyword_punish / keyword_review
	Reason      string  `json:"reason"`       // 判定说明
}

// TestViolation 链路测试：不处罚、不写库，仅返回判定流水（供 Web 面板诊断）。
func (m *Manager) TestViolation(ctx context.Context, text string) *TestReport {
	cfg := m.getCfg(ctx)
	text = strings.TrimSpace(text)
	rep := &TestReport{Text: text, RAGOK: false}

	card := detectGroupCard(text)
	rep.Card = card
	rep.Word, rep.WordCat = m.wordHit(ctx, stripCQ(text))

	if v := m.verifyByRAG(ctx, stripCQ(text)); v.ok {
		rep.RAGOK = true
		rep.RAGScore = v.score
		rep.RAGSample = v.text
		rep.RAGCategory = v.category
		hard := rep.Word != "" || card
		switch {
		case v.score >= cfg.HighScore:
			rep.Verdict = "punish"
			rep.Reason = "RAG 高置信 → 直接处罚"
		case v.score > cfg.LowScore:
			rep.Verdict = "review"
			rep.Reason = "RAG 模棱两可 → LLM 审核"
		case hard:
			rep.Verdict = "review"
			rep.Reason = "RAG 低置信但命中词/卡片 → LLM 终审"
		default:
			rep.Verdict = "pass"
			rep.Reason = "RAG 低置信且无硬信号 → 放行"
		}
		return rep
	}

	// RAG 不可用：关键词路径
	switch {
	case rep.WordCat == "sensitive" || rep.WordCat == "black" || card:
		rep.Verdict = "review"
		rep.Reason = "RAG 不可用 → 关键词高危复核（LLM 挂则直罚）"
	case rep.WordCat == "gray":
		rep.Verdict = "review"
		rep.Reason = "RAG 不可用 → 灰色词常规 LLM 审查"
	default:
		rep.Verdict = "pass"
		rep.Reason = "RAG 不可用且无关键词命中 → 放行"
	}
	return rep
}
