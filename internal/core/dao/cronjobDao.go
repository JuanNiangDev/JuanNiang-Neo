package dao

import (
	"context"
	"time"

	"JuanNiang-Neo/internal/core/models"
	"gorm.io/gorm"
)

type CronJobDAO struct{ db *gorm.DB }

func NewCronJobDAO(db *gorm.DB) *CronJobDAO { return &CronJobDAO{db: db} }

func (d *CronJobDAO) Create(ctx context.Context, m *models.CronJob) error {
	if m.ID == "" {
		m.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(m).Error
}

func (d *CronJobDAO) GetByID(ctx context.Context, id string) (*models.CronJob, error) {
	var m models.CronJob
	err := d.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *CronJobDAO) Update(ctx context.Context, m *models.CronJob) error {
	return d.db.WithContext(ctx).Save(m).Error
}

func (d *CronJobDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.CronJob{}).Error
}

func (d *CronJobDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.CronJob{}).Where("id = ?", id).
		Update("is_active", active).Error
}

func (d *CronJobDAO) List(ctx context.Context) ([]models.CronJob, error) {
	var list []models.CronJob
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

// ListActive 返回所有已启用的 CronJob（供调度器使用）。
func (d *CronJobDAO) ListActive(ctx context.Context) ([]models.CronJob, error) {
	var list []models.CronJob
	err := d.db.WithContext(ctx).Where("is_active = ?", true).Find(&list).Error
	return list, err
}

// UpdateLastRun 更新最后执行时间和错误信息。
func (d *CronJobDAO) UpdateLastRun(ctx context.Context, id string, ts time.Time, lastErr string) error {
	updates := map[string]any{
		"last_run_at": ts,
		"last_error":  lastErr,
	}
	return d.db.WithContext(ctx).Model(&models.CronJob{}).Where("id = ?", id).Updates(updates).Error
}
