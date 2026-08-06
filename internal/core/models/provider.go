package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- LLM Provider ----------

type ModelType string

const (
	ModelTypeText      ModelType = "text_model"
	ModelTypeImage     ModelType = "image_model"
	ModelTypeEmbedding ModelType = "embedding_model"
)

type Provider struct {
	ID          string    `gorm:"primaryKey;type:uuid"`
	Name        string    `gorm:"not null"`
	Type        ModelType `gorm:"not null;index"`
	Endpoint    string    `gorm:"not null"`
	Token       string    `gorm:"not null"`
	Model       string    `gorm:"not null"`
	Temperature float32   `gorm:"default:0.7"`
	// EnableThinking 模型思考开关（旧字段）：请求体携带 thinking/enable_thinking 扩展参数
	EnableThinking bool `gorm:"default:false"`
	// APIMode 协议模式：chat_completions / anthropic_messages / openai_responses / gemini_native；空 = chat_completions
	APIMode string `gorm:"default:''"`
	// ThinkingEffort 思考档位：off/low/medium/high
	ThinkingEffort string `gorm:"default:''"`
	// ThinkingBudget anthropic/gemini 思考预算 token（0 = 协议默认）
	ThinkingBudget int `gorm:"default:0"`
	// MaxTokens 生成上限 token（0 = 协议默认）
	MaxTokens int `gorm:"default:0"`
	// TopP / TopK / 惩罚参数（指针：nil = 不携带）
	TopP              *float32 `gorm:"default:null"`
	TopK              *int     `gorm:"default:null"`
	FrequencyPenalty  *float32 `gorm:"default:null"`
	PresencePenalty   *float32 `gorm:"default:null"`
	RepetitionPenalty *float32 `gorm:"default:null"`
	// ProviderKey 厂商分组（deepseek/kimi/zhipu/...），驱动 thinking 矩阵；空 = 按 Name 关键词匹配
	ProviderKey string `gorm:"default:''"`
	// AuthHeader 认证头：""|bearer|x-api-key|api-key
	AuthHeader string `gorm:"default:''"`
	// URLMode URL 拼接：""|auto|exact
	URLMode   string `gorm:"default:''"`
	IsActive  bool   `gorm:"default:true;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (Provider) TableName() string {
	return "providers"
}
