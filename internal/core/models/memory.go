package models

import "time"

// MemoryAgentID 定义各层记忆在 Mem0 中的 agent_id 常量。
const (
	Mem0AgentChatRecall      = "chat_recall"
	Mem0AgentFact            = "fact_memory"
	Mem0AgentProfile         = "profile_memory"
	Mem0AgentSummary         = "summary_memory"
	Mem0AgentRaw             = "raw_memory"
	Mem0AgentHeuristicBehav  = "heuristic_behavior"
	Mem0AgentHeuristicExpr   = "heuristic_expression"
	Mem0AgentHeuristicJargon = "heuristic_jargon"
)

// MemoryGCConfig 记忆 GC 配置（单例，DB 中只保留一行）。
type MemoryGCConfig struct {
	ID            string    `gorm:"primaryKey;type:uuid" json:"id"`
	Enable        bool      `gorm:"default:true" json:"enable"`
	ColdThreshold int       `gorm:"default:7" json:"cold_threshold"` // 冷记忆阈值（天）
	MaxPerAgent   int       `gorm:"default:1000" json:"max_per_agent"`
	IntervalMins  int       `gorm:"default:60" json:"interval_mins"` // GC 间隔（分钟）
	LastRunAt     time.Time `json:"last_run_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (MemoryGCConfig) TableName() string { return "memory_gc_config" }

// LearnerConfig 学习引擎配置（单例）。
type LearnerConfig struct {
	ID                 string    `gorm:"primaryKey;type:uuid" json:"id"`
	BehaviorEnabled    bool      `gorm:"default:true" json:"behavior_enabled"`
	ExpressionEnabled  bool      `gorm:"default:true" json:"expression_enabled"`
	JargonEnabled      bool      `gorm:"default:true" json:"jargon_enabled"`
	LearnInterval      int       `gorm:"default:1" json:"learn_interval"` // 每 N 轮触发一次学习
	MaxConcurrentLearn int       `gorm:"default:2" json:"max_concurrent_learn"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (LearnerConfig) TableName() string { return "learner_config" }
