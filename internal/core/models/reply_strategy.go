package models

import "time"

// ReplyStrategy 回复策略。
type ReplyStrategy string

const (
	StrategyNeverReply ReplyStrategy = "never_reply" // 完全不回复
	StrategyAtOnly     ReplyStrategy = "at_only"     // 仅@我时回复
	StrategyAlways     ReplyStrategy = "always"      // 完全回复（默认）
	StrategyPluginOnly ReplyStrategy = "plugin_only" // 仅 Plugin
	StrategyRelevance  ReplyStrategy = "relevance"   // 按相关性回复
)

// ReplyStrategyConfig 系统回复策略配置（单例，DB 中只保留一行）。
type ReplyStrategyConfig struct {
	ID                 string        `gorm:"primaryKey;type:uuid" json:"id"`
	Strategy           ReplyStrategy `gorm:"not null;default:'always'" json:"strategy"`
	RelevanceThreshold float64       `gorm:"default:0.5" json:"relevance_threshold"`
	BotName            string        `gorm:"default:''" json:"bot_name"`
	StripMarkdown      bool          `gorm:"default:false" json:"strip_markdown"` // 是否去除 Agent 消息中的 Markdown 格式
	AgentLite          bool          `gorm:"default:false" json:"agent_lite"`    // AgentLite 模式：不调用工具/MCP，无 Agent 循环
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

func (ReplyStrategyConfig) TableName() string { return "reply_strategy_config" }