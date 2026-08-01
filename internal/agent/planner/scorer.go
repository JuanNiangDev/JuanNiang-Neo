package planner

import (
	"strings"

	"JuanNiang-Neo/internal/adapter"
)

// Scorer 规则打分引擎。
type Scorer struct{}

// NewScorer 创建打分器。
func NewScorer() *Scorer {
	return &Scorer{}
}

// Score 根据五个维度计算综合分数。
func (s *Scorer) Score(ev adapter.Event, ctx *EvaluationContext, w Weights) *ScoreResult {
	msg := ev.Message
	if msg == nil {
		return &ScoreResult{Total: 0, Passed: false}
	}
	if ctx == nil {
		ctx = &EvaluationContext{}
	}

	rawMsg := msg.RawMessage

	details := map[string]float64{
		"mention": s.calcMention(rawMsg, ctx),
		"keyword": s.calcKeyword(rawMsg, ctx),
		"context": s.calcContext(ctx),
		"quality": s.calcQuality(rawMsg),
		"history": s.calcHistory(ctx),
	}

	total := details["mention"]*w.MentionWeight +
		details["keyword"]*w.KeywordWeight +
		details["context"]*w.ContextWeight +
		details["quality"]*w.QualityWeight +
		details["history"]*w.HistoryWeight

	return &ScoreResult{
		Total:   total,
		Details: details,
	}
}

// calcMention @提及评分。
func (s *Scorer) calcMention(rawMsg string, ctx *EvaluationContext) float64 {
	botQQ := ctx.BotQQ
	botName := ctx.BotName
	botNick := ctx.BotNickname

	// [CQ:at,qq=xxx] 直接 @机器人
	if botQQ > 0 && strings.Contains(rawMsg, "qq="+strings.TrimSpace(strings.Repeat("?", 0))) {
		// 简化检测：检查是否有 CQ:at
		if strings.Contains(rawMsg, "[CQ:at") {
			return 1.0
		}
	}

	// 包含机器人名字/昵称
	lower := strings.ToLower(rawMsg)
	for _, name := range []string{botName, botNick} {
		if name != "" && strings.Contains(lower, strings.ToLower(name)) {
			return 0.8
		}
	}

	// 无@无名字
	return 0.0
}

// calcKeyword 关键词评分。
func (s *Scorer) calcKeyword(rawMsg string, ctx *EvaluationContext) float64 {
	// 检查 Skill 关键词
	for _, kw := range ctx.SkillKeywords {
		if kw != "" && strings.Contains(rawMsg, kw) {
			return 0.8
		}
	}

	// 含提问关键词
	questionWords := []string{"?", "？", "吗", "呢", "什么", "怎么", "如何", "帮", "请"}
	lower := strings.ToLower(rawMsg)
	for _, w := range questionWords {
		if strings.Contains(lower, w) {
			return 0.5
		}
	}

	return 0.0
}

// calcContext 上下文连续性评分。
func (s *Scorer) calcContext(ctx *EvaluationContext) float64 {
	if ctx.RecentInteraction {
		return 0.9
	}
	return 0.0
}

// calcQuality 内容质量评分。
func (s *Scorer) calcQuality(rawMsg string) float64 {
	if rawMsg == "" {
		return 0.0
	}

	// 纯 CQ 码（图片/表情）→ 低分
	cqOnly := rawMsg
	cqOnly = strings.TrimSpace(cqOnly)
	if strings.HasPrefix(cqOnly, "[CQ:") && strings.Count(cqOnly, "[CQ:") == 1 {
		return 0.1
	}

	runes := []rune(rawMsg)
	if len(runes) > 10 {
		return 1.0
	}
	if len(runes) > 5 {
		return 0.8
	}
	return 0.4
}

// calcHistory 历史互动评分。
func (s *Scorer) calcHistory(ctx *EvaluationContext) float64 {
	if ctx.PositiveHistory > 0 {
		return ctx.PositiveHistory
	}
	return 0.5 // 默认中性
}
