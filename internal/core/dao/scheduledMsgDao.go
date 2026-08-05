package dao

import (
	"context"
	"time"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

// ScheduledMessageDAO 定时消息任务。
type ScheduledMessageDAO struct{ db *gorm.DB }

func NewScheduledMessageDAO(db *gorm.DB) *ScheduledMessageDAO { return &ScheduledMessageDAO{db: db} }

func (d *ScheduledMessageDAO) Create(ctx context.Context, m *models.ScheduledMessage) error {
	if m.ID == "" {
		m.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(m).Error
}

func (d *ScheduledMessageDAO) GetByID(ctx context.Context, id string) (*models.ScheduledMessage, error) {
	var m models.ScheduledMessage
	if err := d.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// List 分页列出（最新在前）。
func (d *ScheduledMessageDAO) List(ctx context.Context, limit, offset int) ([]models.ScheduledMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var list []models.ScheduledMessage
	err := d.db.WithContext(ctx).
		Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, err
}

func (d *ScheduledMessageDAO) Count(ctx context.Context) (int64, error) {
	var n int64
	err := d.db.WithContext(ctx).Model(&models.ScheduledMessage{}).Count(&n).Error
	return n, err
}

// ListActive 列出所有启用的任务（调度器加载用）。
func (d *ScheduledMessageDAO) ListActive(ctx context.Context) ([]models.ScheduledMessage, error) {
	var list []models.ScheduledMessage
	err := d.db.WithContext(ctx).Where("enabled = ?", true).Find(&list).Error
	return list, err
}

func (d *ScheduledMessageDAO) Update(ctx context.Context, m *models.ScheduledMessage) error {
	return d.db.WithContext(ctx).Save(m).Error
}

func (d *ScheduledMessageDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.ScheduledMessage{}).Error
}

// MarkRunResult 记录执行结果。
func (d *ScheduledMessageDAO) MarkRunResult(ctx context.Context, id string, runAt time.Time, errMsg string) error {
	return d.db.WithContext(ctx).Model(&models.ScheduledMessage{}).
		Where("id = ?", id).
		Updates(map[string]any{"last_run_at": runAt, "last_error": errMsg}).Error
}
