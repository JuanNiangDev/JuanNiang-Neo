package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// ---------- 定时消息 ----------

// ScheduledMessageSegment 定时消息的单个消息段（JSON 存储）。
//
// Type 取值：
//   - text：纯文字，Content 为文本
//   - image：图片，Content 按 Source 解释（t2i=HTML 模板 / url=直链 / imgstore=图床 imgs://<id>）
//   - face：CQ 码表情，Content 为完整 CQ 码（如 "[CQ:face,id=66]"）
//
// DelaySeconds 本段发送完成后、开始发送下一段前的延迟（秒）。
type ScheduledMessageSegment struct {
	Type         string `json:"type"`
	Source       string `json:"source,omitempty"` // image 段：t2i / url / imgstore
	Content      string `json:"content"`
	DelaySeconds int    `json:"delay_seconds,omitempty"`
}

// ScheduledSegments 是 []ScheduledMessageSegment 的 GORM jsonb 兼容类型。
type ScheduledSegments []ScheduledMessageSegment

func (s ScheduledSegments) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (s *ScheduledSegments) Scan(value any) error {
	if value == nil {
		*s = nil
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return nil
	}
	return json.Unmarshal(b, s)
}

// ScheduledMessage 定时消息任务：一个任务含多段消息，段间可自定义延迟。
// 独立调度器（internal/agent/scheduledmsg），不复用 CronJob 系统。
type ScheduledMessage struct {
	ID         string            `gorm:"primaryKey;type:uuid"`
	Name       string            `gorm:"not null"`                 // 任务名
	Enabled    bool              `gorm:"not null;default:false"`   // 开关
	CronExpr   string            `gorm:"not null"`                 // 触发时间（6 字段秒级 cron）
	TargetType string            `gorm:"not null;default:'group'"` // group / private
	TargetID   int64             `gorm:"not null"`                 // 群号或 QQ 号
	Segments   ScheduledSegments `gorm:"type:jsonb"`               // 消息段数组
	LastRunAt  *time.Time        // 上次执行时间
	LastError  string            `gorm:"type:text"` // 上次执行错误
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

func (ScheduledMessage) TableName() string { return "scheduled_messages" }
