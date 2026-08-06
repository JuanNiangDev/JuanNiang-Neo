package provider

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"JuanNiang-Neo/internal/core/models"
)

// APIMode 协议模式（与 MintWord apiMode 对齐）。
type APIMode string

const (
	APIModeChatCompletions   APIMode = "chat_completions"
	APIModeAnthropicMessages APIMode = "anthropic_messages"
	APIModeOpenAIResponses   APIMode = "openai_responses"
	APIModeGeminiNative      APIMode = "gemini_native"
)

// ModelType 模型类型枚举。
type ModelType string

const (
	ModelTypeText      ModelType = "text_model"
	ModelTypeImage     ModelType = "image_model"
	ModelTypeEmbedding ModelType = "embedding_model"
)

// ---------- 配置 ----------

type ProviderConfig struct {
	ID          string    `json:"id"`
	Type        ModelType `json:"type"`
	Name        string    `json:"name"`
	Endpoint    string    `json:"endpoint"`
	Token       string    `json:"token"`
	Model       string    `json:"model"`
	Temperature float32   `json:"temperature"`
	// EnableThinking 模型思考开关（旧字段）：true 且 ThinkingEffort 为空时映射为 effort=medium
	EnableThinking bool `json:"enable_thinking"`
	// APIMode 协议模式：空 = chat_completions（存量兼容）
	APIMode APIMode `json:"api_mode,omitempty"`
	// ThinkingEffort 思考档位：off/low/medium/high（空 + EnableThinking=false = off）
	ThinkingEffort string `json:"thinking_effort,omitempty"`
	// ThinkingBudget anthropic/gemini 的 thinking 预算 token（0 = 协议默认）
	ThinkingBudget    int      `json:"thinking_budget,omitempty"`
	MaxTokens         int      `json:"max_tokens,omitempty"`
	TopP              *float32 `json:"top_p,omitempty"`
	TopK              *int     `json:"top_k,omitempty"`
	FrequencyPenalty  *float32 `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float32 `json:"presence_penalty,omitempty"`
	RepetitionPenalty *float32 `json:"repetition_penalty,omitempty"`
	// ProviderKey 厂商分组（deepseek/kimi/zhipu/...），驱动 thinking 矩阵与认证头分派；空 = 按 Name 关键词匹配
	ProviderKey string `json:"provider_key,omitempty"`
	// AuthHeader 认证头：""|bearer|x-api-key|api-key（空 = 按协议默认）
	AuthHeader string `json:"auth_header,omitempty"`
	// URLMode URL 拼接：""|auto|exact（auto=base+协议后缀自动拼接；exact=完整端点原样使用）
	URLMode string `json:"url_mode,omitempty"`
}

// apiMode 返回归一化后的协议模式（空 → chat_completions）。
func (c *ProviderConfig) apiMode() APIMode {
	if c.APIMode == "" {
		return APIModeChatCompletions
	}
	return c.APIMode
}

// urlMode 返回归一化后的 URL 模式（空 → auto）。
func (c *ProviderConfig) urlMode() string {
	if c.URLMode == "" {
		return "auto"
	}
	return c.URLMode
}

// ProviderConfigFromModel 把 GORM Provider 模型映射为运行时 ProviderConfig。
// 供 service / agent 启动加载统一使用，避免新字段在多处遗漏。
func ProviderConfigFromModel(m *models.Provider) ProviderConfig {
	return ProviderConfig{
		ID:                m.ID,
		Type:              ModelType(m.Type),
		Name:              m.Name,
		Endpoint:          m.Endpoint,
		Token:             m.Token,
		Model:             m.Model,
		Temperature:       m.Temperature,
		EnableThinking:    m.EnableThinking,
		APIMode:           APIMode(m.APIMode),
		ThinkingEffort:    m.ThinkingEffort,
		ThinkingBudget:    m.ThinkingBudget,
		MaxTokens:         m.MaxTokens,
		TopP:              m.TopP,
		TopK:              m.TopK,
		FrequencyPenalty:  m.FrequencyPenalty,
		PresencePenalty:   m.PresencePenalty,
		RepetitionPenalty: m.RepetitionPenalty,
		ProviderKey:       m.ProviderKey,
		AuthHeader:        m.AuthHeader,
		URLMode:           m.URLMode,
	}
}

// normalizeProviderKey 小写归一化厂商分组关键词。
func normalizeProviderKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// ---------- 请求/响应 ----------

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolCallFunc `json:"function"`
}

type ToolCallFunc struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	Tools       []ToolDef     `json:"tools,omitempty"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type ToolDef struct {
	Type     string      `json:"type"`
	Function ToolDefFunc `json:"function"`
}

type ToolDefFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ChatResponse struct {
	Message      ChatMessage `json:"message"`
	TokenUsage   int         `json:"token_usage"`
	FinishReason string      `json:"finish_reason"`
}

type ChatStreamChunk struct {
	Content      string     `json:"content,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
}

// ---------- Provider 接口 ----------

type Provider interface {
	ID() string
	Name() string
	Type() ModelType
	Model() string
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatStreamChunk, error)
	Vision(ctx context.Context, imageData []byte, prompt string) (string, error)
}

// ---------- Provider 容器 ----------

type ProviderGroup struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewProviderGroup() *ProviderGroup {
	return &ProviderGroup{
		providers: make(map[string]Provider),
	}
}

func (pg *ProviderGroup) AddProvider(pr Provider) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.providers[pr.ID()] = pr
}

func (pg *ProviderGroup) DelProvider(id string) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	delete(pg.providers, id)
}

func (pg *ProviderGroup) GetProvider(id string) (Provider, bool) {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	p, ok := pg.providers[id]
	return p, ok
}

func (pg *ProviderGroup) ListProviders() []Provider {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	list := make([]Provider, 0, len(pg.providers))
	for _, p := range pg.providers {
		list = append(list, p)
	}
	return list
}

// SelectModel 按类型选择一个可用的 Provider。返回 nil 表示无可用模型。
func (pg *ProviderGroup) SelectModel(typ ModelType) Provider {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	for _, p := range pg.providers {
		if p.Type() == typ {
			return p
		}
	}
	return nil
}

// SyncConfig 同步单个 Provider 配置到运行时（热更新路径）。
// 注意：写 map 必须持写锁，与 SelectModel/ListProviders 等读锁并发时避免数据竞争。
func (pg *ProviderGroup) SyncConfig(conf ProviderConfig) {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	_, ok := pg.providers[conf.ID]
	if ok {
		delete(pg.providers, conf.ID)
	}

	pg.providers[conf.ID] = NewProvider(conf)
}
