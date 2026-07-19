package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- Background Task ----------

type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusFailed  TaskStatus = "failed"
)

type BackgroundTask struct {
	ID         string     `gorm:"primaryKey;type:uuid"`
	ChatAreaID string     `gorm:"not null;index"`
	ChatArea   ChatArea   `gorm:"foreignKey:ChatAreaID"`
	Status     TaskStatus `gorm:"default:pending;index"`
	Steps      JSONMap    `gorm:"type:jsonb;default:'[]'"`
	Results    JSONMap    `gorm:"type:jsonb;default:'{}'"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// ---------- 后台任务步骤结果 ----------

type TaskStepResult struct {
	TaskID     string
	StepID     string
	ChatAreaID string
	Status     TaskStatus
	Result     string
	Error      string
}
