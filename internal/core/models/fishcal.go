package models

import (
	"time"

	"gorm.io/gorm"
)

// ---------- 摸鱼人日历 ----------

// FishCalendarConfig 摸鱼人日历配置（单行表，独立于 CronJob 系统）。
// 每日按 CronExpr 触发：生成日历模板 → T2I 渲染成图片 → 发送到多个目标群。
type FishCalendarConfig struct {
	ID           uint       `gorm:"primarykey"`
	Enabled      bool       `gorm:"not null;default:false"`          // 总开关
	CronExpr     string     `gorm:"not null;default:'0 30 9 * * *'"` // 发送时间（6 字段秒级 cron）
	TargetGroups JSONSlice  `gorm:"type:jsonb;default:'[]'"`         // 目标群号列表（string 数组）
	LastRunAt    *time.Time // 上次成功执行时间
	LastError    string     `gorm:"type:text"` // 上次执行错误（成功为空）
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (FishCalendarConfig) TableName() string { return "fish_calendar_configs" }

// FishCalendarAffair 某一天的群务内容（按 YYYY-MM-DD 精确匹配）。
type FishCalendarAffair struct {
	ID        uint   `gorm:"primarykey"`
	Date      string `gorm:"not null;uniqueIndex"` // YYYY-MM-DD
	Content   string `gorm:"type:text;not null"`   // 当天群务文本
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (FishCalendarAffair) TableName() string { return "fish_calendar_affairs" }
