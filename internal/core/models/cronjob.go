package models

import (
	"time"

	"gorm.io/gorm"
)

// CronJob 定时任务，在指定时间模拟 OneBot11 事件注入 Agent。
type CronJob struct {
	ID           string      `gorm:"primaryKey;type:uuid" json:"id"`
	Name         string      `gorm:"not null" json:"name"`
	CronExpr     string      `gorm:"not null" json:"cron_expr"`                        // 标准 cron 表达式
	Message      string      `gorm:"not null" json:"message"`                          // 触发时发送的消息内容
	MessageType  string      `gorm:"default:'private'" json:"message_type"`            // private / group
	TargetID     int64       `gorm:"not null" json:"target_id"`                        // 目标 QQ 号或群号
	IsActive     bool        `gorm:"default:true" json:"is_active"`
	LastRunAt    *time.Time  `json:"last_run_at,omitempty"`
	LastError    string      `gorm:"default:''" json:"last_error"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (CronJob) TableName() string { return "cron_jobs" }
