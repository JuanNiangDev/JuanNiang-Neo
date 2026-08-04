package provider

import (
	"context"
	"encoding/json"
	"sync"
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
	// EnableThinking 模型思考开关：true 时请求携带 thinking/enable_thinking 扩展参数
	EnableThinking bool `json:"enable_thinking"`
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

// HasImageModel 检查是否配置了识图模型。
func (pg *ProviderGroup) HasImageModel() bool {
	return pg.SelectModel(ModelTypeImage) != nil
}

func (pg *ProviderGroup) SyncConfig(conf ProviderConfig) {
	pg.mu.RLock()
	defer pg.mu.RUnlock()

	_, ok := pg.providers[conf.ID]
	if ok {
		delete(pg.providers, conf.ID)
	}

	pg.providers[conf.ID] = NewProvider(conf)
}
