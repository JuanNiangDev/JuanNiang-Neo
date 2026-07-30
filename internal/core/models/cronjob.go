package models

import (
	"time"

	"gorm.io/gorm"
)

// CronJob 定时任务，在指定时间模拟 OneBot11 事件注入 Agent，或触发插件 on_timer_call 回调。
type CronJob struct {
	ID          string         `gorm:"primaryKey;type:uuid" json:"id"`
	Name        string         `gorm:"not null" json:"name"`
	CronExpr    string         `gorm:"not null" json:"cron_expr"`             // 标准 cron 表达式
	Message     string         `gorm:"default:''" json:"message"`             // 触发时发送给 Agent 的消息内容（空字符串表示不发给 Agent）
	MessageType string         `gorm:"default:'private'" json:"message_type"` // private / group
	TargetID    int64          `gorm:"default:0" json:"target_id"`            // 目标 QQ 号或群号（仅在发给 Agent 时使用）
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	PluginIDs   string         `gorm:"type:text;default:''" json:"plugin_ids"` // JSON 数组：触发时调用的插件目录名列表，如 ["my-plugin"]
	Payload     string         `gorm:"type:text;default:''" json:"payload"`    // JSON 字符串：传给 on_timer_call(event) 的 payload
	LastRunAt   *time.Time     `json:"last_run_at,omitempty"`
	LastError   string         `gorm:"default:''" json:"last_error"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (CronJob) TableName() string { return "cron_jobs" }
