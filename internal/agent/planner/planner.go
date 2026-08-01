package planner

import "JuanNiang-Neo/internal/adapter"

// ScoreResult 打分结果。
type ScoreResult struct {
	Total   float64            `json:"total"`
	Passed  bool               `json:"passed"`
	Details map[string]float64 `json:"details"` // 各维度分数
}

// PlannerResult LLM 规划器的输出。
type PlannerResult struct {
	ShouldReply bool           `json:"should_reply"`
	ReplyStyle  string         `json:"reply_style"` // "text" | "image" | "emoji_first" | "cq_mixed"
	Intent      string         `json:"intent"`      // "question" | "chat" | "command" | "task"
	ToolPlan    []ToolPlanItem `json:"tool_plan,omitempty"`
	MemoryHints []string       `json:"memory_hints,omitempty"` // 建议查询的记忆 agent_id
	Confidence  float64        `json:"confidence"`
	Summary     string         `json:"summary"` // LLM 规划总结
}

// ToolPlanItem 工具调用计划。
type ToolPlanItem struct {
	ToolName string `json:"tool_name"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority"` // 1=高 2=中 3=低
}

// Weights 打分权重配置。
type Weights struct {
	MentionWeight float64 `json:"mention_weight"` // @提及权重 (默认 0.35)
	KeywordWeight float64 `json:"keyword_weight"` // 关键词权重 (默认 0.25)
	ContextWeight float64 `json:"context_weight"` // 上下文权重 (默认 0.20)
	QualityWeight float64 `json:"quality_weight"` // 内容质量权重 (默认 0.10)
	HistoryWeight float64 `json:"history_weight"` // 历史权重 (默认 0.10)
}

// DefaultWeights 默认权重。
func DefaultWeights() Weights {
	return Weights{
		MentionWeight: 0.35,
		KeywordWeight: 0.25,
		ContextWeight: 0.20,
		QualityWeight: 0.10,
		HistoryWeight: 0.10,
	}
}

// Planner 两阶段规划器：阶段一规则打分，阶段二 LLM 规划。
type Planner struct {
	weights   Weights
	threshold float64
	scorer    *Scorer
	llm       *LLMPlanner
}

// New 创建 Planner。
func New(weights Weights, threshold float64) *Planner {
	if threshold <= 0 {
		threshold = 0.3
	}
	return &Planner{
		weights:   weights,
		threshold: threshold,
		scorer:    NewScorer(),
	}
}

// UpdateConfig 运行时更新配置。
func (p *Planner) UpdateConfig(weights Weights, threshold float64) {
	p.weights = weights
	p.threshold = threshold
}

// Threshold 返回当前阈值。
func (p *Planner) Threshold() float64 { return p.threshold }

// LLM 返回 LLM 规划器（可能为 nil）。
func (p *Planner) LLM() *LLMPlanner { return p.llm }

// SetLLM 设置 LLM 规划器。
func (p *Planner) SetLLM(llm *LLMPlanner) { p.llm = llm }

// Evaluate 评估事件：先打分，再决定是否需要 LLM 规划。
// 返回 (ScoreResult, shouldProceed)。
func (p *Planner) Evaluate(ev adapter.Event, context *EvaluationContext) (*ScoreResult, bool) {
	result := p.scorer.Score(ev, context, p.weights)
	result.Passed = result.Total >= p.threshold
	return result, result.Passed
}

// EvaluationContext 评估上下文。
type EvaluationContext struct {
	BotQQ             int64
	BotName           string
	BotNickname       string
	SkillKeywords     []string
	RecentInteraction bool    // 最近 5 条内有 bot 的发言
	PositiveHistory   float64 // 0-1 积极互动比例
}
