package provider

import "strings"

// ---------- 模型能力元数据（§5.7，可选阶段） ----------
// 数据源：MintWord aiProviders.ts 全量模型明细 + 2026-08 官方文档更新。
// 用途：max_tokens 默认值校验、前端能力提示、后续上下文截断策略的基础。

// ThinkingPattern 思考模式枚举（与 MintWord AiModel.thinkingPattern 对齐）。
const (
	ThinkingPatternReasoningEffort = "reasoning_effort" // openai 兼容: reasoning_effort 字段
	ThinkingPatternThinkingObject  = "thinking_object"  // deepseek/kimi/zhipu/glm: thinking={type:enabled}
	ThinkingPatternEnableThinking  = "enable_thinking"  // 通义/硅基: enable_thinking 布尔
	ThinkingPatternThinkingConfig  = "thinking_config"  // gemini-3/stepfun
	ThinkingPatternBudgetTokens    = "budget_tokens"    // anthropic 旧代/gemini 2.5
	ThinkingPatternAdaptive        = "adaptive"         // anthropic 新代/minimax-M3
	ThinkingPatternBuiltin         = "builtin"          // 模型内置思考，无参数
)

// ModelCapability 单个模型的能力元数据。
type ModelCapability struct {
	ContextWindow   int // token
	MaxOutput       int // token
	ThinkingSupport bool
	ThinkingPattern string   // 见 ThinkingPattern 常量
	SupportedParams []string // temperature, top_p, top_k, frequency_penalty, ...
}

// modelCatalog 模型能力表（初始数据，按厂商归并）。
var modelCatalog = map[string]ModelCapability{
	// OpenAI 系
	"gpt-5.6-sol":   {ContextWindow: 1050000, MaxOutput: 128000, ThinkingPattern: ThinkingPatternReasoningEffort, SupportedParams: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty"}},
	"gpt-5.6-terra": {ContextWindow: 1050000, MaxOutput: 128000, ThinkingPattern: ThinkingPatternReasoningEffort, SupportedParams: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty"}},
	"gpt-5.6-luna":  {ContextWindow: 1050000, MaxOutput: 128000, ThinkingPattern: ThinkingPatternReasoningEffort, SupportedParams: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty"}},
	// Anthropic
	"claude-opus-4-7":  {ContextWindow: 1000000, MaxOutput: 64000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternAdaptive, SupportedParams: []string{"temperature", "top_p", "top_k"}},
	"claude-opus-5":    {ContextWindow: 1000000, MaxOutput: 128000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternAdaptive, SupportedParams: []string{}},
	"claude-sonnet-5":  {ContextWindow: 1000000, MaxOutput: 128000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternAdaptive, SupportedParams: []string{}},
	"claude-haiku-4-5": {ContextWindow: 200000, MaxOutput: 8000, SupportedParams: []string{"temperature", "top_p", "top_k"}},
	// Gemini
	"gemini-3-flash":        {ContextWindow: 1000000, MaxOutput: 65000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternThinkingConfig, SupportedParams: []string{"temperature", "top_p", "top_k", "repetition_penalty"}},
	"gemini-3.6-flash":      {ContextWindow: 1000000, MaxOutput: 65000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternThinkingConfig, SupportedParams: []string{"temperature", "top_p", "top_k", "repetition_penalty"}},
	"gemini-3.5-flash":      {ContextWindow: 1000000, MaxOutput: 65000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternThinkingConfig, SupportedParams: []string{"temperature", "top_p", "top_k", "repetition_penalty"}},
	"gemini-3.5-flash-lite": {ContextWindow: 1000000, MaxOutput: 65000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternThinkingConfig, SupportedParams: []string{"temperature", "top_p", "top_k", "repetition_penalty"}},
	"gemini-2.5-pro":        {ContextWindow: 1000000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternBudgetTokens, SupportedParams: []string{"temperature", "top_p", "top_k", "repetition_penalty"}},
	// DeepSeek
	"deepseek-v4-pro":   {ContextWindow: 1000000, MaxOutput: 384000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternThinkingObject, SupportedParams: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty"}},
	"deepseek-v4-flash": {ContextWindow: 1000000, MaxOutput: 384000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternThinkingObject, SupportedParams: []string{"temperature", "top_p", "frequency_penalty", "presence_penalty"}},
	// Kimi
	"kimi-k3":   {ContextWindow: 1000000, MaxOutput: 131000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternReasoningEffort, SupportedParams: []string{"temperature", "top_p"}},
	"kimi-k2.6": {ContextWindow: 256000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternThinkingObject, SupportedParams: []string{"temperature", "top_p"}},
	// 阿里
	"qwen3.8-max":  {ContextWindow: 1000000, MaxOutput: 131000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternEnableThinking, SupportedParams: []string{"temperature", "top_p"}},
	"qwen3.7-max":  {ContextWindow: 1000000, MaxOutput: 64000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternEnableThinking, SupportedParams: []string{"temperature", "top_p"}},
	"qwen3.7-plus": {ContextWindow: 1000000, MaxOutput: 64000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternEnableThinking, SupportedParams: []string{"temperature", "top_p"}},
	// 智谱
	"glm-5.2": {ContextWindow: 1000000, MaxOutput: 131000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternThinkingObject, SupportedParams: []string{"temperature", "top_p"}},
	"glm-5.1": {ContextWindow: 202000, MaxOutput: 131000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternThinkingObject, SupportedParams: []string{"temperature", "top_p"}},
	// MiniMax
	"minimax-m3": {ContextWindow: 1000000, MaxOutput: 128000, ThinkingSupport: true, ThinkingPattern: ThinkingPatternAdaptive, SupportedParams: []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty"}},
	// 小米
	"mimo-v2.5-pro": {ContextWindow: 1000000, MaxOutput: 128000, SupportedParams: []string{"temperature", "top_p"}},
	"mimo-v2.5":     {ContextWindow: 1000000, MaxOutput: 128000, SupportedParams: []string{"temperature", "top_p"}},
}

// GetModelCapability 返回指定模型的能力元数据（按前缀模糊匹配，未命中返回零值）。
func GetModelCapability(model string) (ModelCapability, bool) {
	key := strings.ToLower(strings.TrimSpace(model))
	if c, ok := modelCatalog[key]; ok {
		return c, true
	}
	// 前缀匹配（如 deepseek-v4-flash-0731 → deepseek-v4-flash）
	for k, c := range modelCatalog {
		if strings.HasPrefix(key, k) && len(key) > len(k) {
			return c, true
		}
	}
	return ModelCapability{}, false
}

// ListModelCatalog 返回全部模型能力（供导出/测试）。
func ListModelCatalog() map[string]ModelCapability {
	out := make(map[string]ModelCapability, len(modelCatalog))
	for k, v := range modelCatalog {
		out[k] = v
	}
	return out
}
