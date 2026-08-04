package dao

import (
	"context"
	"time"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

// FishCalendarDAO 摸鱼人日历配置（单行表）。
type FishCalendarDAO struct{ db *gorm.DB }

func NewFishCalendarDAO(db *gorm.DB) *FishCalendarDAO { return &FishCalendarDAO{db: db} }

// InitConfig 初始化默认配置（不存在时插入）。
func (d *FishCalendarDAO) InitConfig(ctx context.Context) error {
	return d.db.WithContext(ctx).FirstOrCreate(&models.FishCalendarConfig{ID: 1}, models.FishCalendarConfig{ID: 1}).Error
}

// GetConfig 读取配置。
func (d *FishCalendarDAO) GetConfig(ctx context.Context) (*models.FishCalendarConfig, error) {
	var cfg models.FishCalendarConfig
	if err := d.db.WithContext(ctx).Where("id = 1").First(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpdateConfig 更新配置。
func (d *FishCalendarDAO) UpdateConfig(ctx context.Context, cfg *models.FishCalendarConfig) error {
	return d.db.WithContext(ctx).Where("id = 1").Save(cfg).Error
}

// MarkRunResult 记录执行结果（成功时间 / 错误信息）。
func (d *FishCalendarDAO) MarkRunResult(ctx context.Context, runAt time.Time, errMsg string) error {
	return d.db.WithContext(ctx).Model(&models.FishCalendarConfig{}).
		Where("id = 1").
		Updates(map[string]any{"last_run_at": runAt, "last_error": errMsg}).Error
}
