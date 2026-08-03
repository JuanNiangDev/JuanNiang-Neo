package models

import "time"

// ReplyStrategy 回复策略。
type ReplyStrategy string

const (
	StrategyNeverReply ReplyStrategy = "never_reply" // 完全不回复
	StrategyAtOnly     ReplyStrategy = "at_only"     // 仅@我时回复
	StrategyAlways     ReplyStrategy = "always"      // 完全回复（默认）
	StrategyRelevance  ReplyStrategy = "relevance"   // 按相关性回复
)

// ReplyStrategyConfig 系统回复策略配置（单例，DB 中只保留一行）。
type ReplyStrategyConfig struct {
	ID                 string        `gorm:"primaryKey;type:uuid" json:"id"`
	Strategy           ReplyStrategy `gorm:"not null;default:'always'" json:"strategy"`
	RelevanceThreshold float64       `gorm:"default:0.5" json:"relevance_threshold"`
	BotName            string        `gorm:"default:''" json:"bot_name"`
	StripMarkdown      bool          `gorm:"default:false" json:"strip_markdown"`   // 是否去除 Agent 消息中的 Markdown 格式
	AgentLite          bool          `gorm:"default:false" json:"agent_lite"`       // AgentLite 模式：不调用工具/MCP，无 Agent 循环
	RelevancePrompt    string        `gorm:"type:text;default:''" json:"relevance_prompt"` // 相关性检测自定义提示词（空则用默认）
	RelevanceModel     string        `gorm:"default:''" json:"relevance_model"`     // 相关性检测使用的 Text Provider ID（空则用默认）
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

func (ReplyStrategyConfig) TableName() string { return "reply_strategy_config" }
