package dao

import (
	"context"
	"strings"
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

// ---------- 按天群务 ----------

// AffairUpsert 设置某天的群务（存在则更新，content 为空视为删除）。
func (d *FishCalendarDAO) AffairUpsert(ctx context.Context, date, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return d.db.WithContext(ctx).Where("date = ?", date).Delete(&models.FishCalendarAffair{}).Error
	}
	var existing models.FishCalendarAffair
	err := d.db.WithContext(ctx).Where("date = ?", date).First(&existing).Error
	if err == nil {
		existing.Content = content
		return d.db.WithContext(ctx).Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return d.db.WithContext(ctx).Create(&models.FishCalendarAffair{Date: date, Content: content}).Error
}

// AffairGet 读取某天的群务（无则返回 nil, nil）。
func (d *FishCalendarDAO) AffairGet(ctx context.Context, date string) (*models.FishCalendarAffair, error) {
	var a models.FishCalendarAffair
	err := d.db.WithContext(ctx).Where("date = ?", date).First(&a).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// AffairListMonth 列出某月（YYYY-MM）已配置的群务。
func (d *FishCalendarDAO) AffairListMonth(ctx context.Context, yearMonth string) ([]models.FishCalendarAffair, error) {
	var list []models.FishCalendarAffair
	err := d.db.WithContext(ctx).Where("date LIKE ?", yearMonth+"-%").
		Order("date ASC").Find(&list).Error
	return list, err
}
