package dao

import (
	"context"
	"errors"
	"time"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

// ErrLayaCapabilityConfigChanged 表示检测期间配置已变化，旧结果未写入。
var ErrLayaCapabilityConfigChanged = errors.New("配置已变化，请重新检测")

type ReplyStrategyDAO struct{ db *gorm.DB }

func NewReplyStrategyDAO(db *gorm.DB) *ReplyStrategyDAO { return &ReplyStrategyDAO{db: db} }

// GetOrCreate 获取当前策略配置，不存在则创建默认行（always）。
func (d *ReplyStrategyDAO) GetOrCreate(ctx context.Context) (*models.ReplyStrategyConfig, error) {
	var cfg models.ReplyStrategyConfig
	err := d.db.WithContext(ctx).First(&cfg).Error
	if err == nil {
		return &cfg, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	cfg = models.ReplyStrategyConfig{
		ID:                 newUUID(),
		Strategy:           models.StrategyRelevance,
		RelevanceThreshold: 0.5,
		RelevanceTimeout:   10,
		JudgeFailPolicy:    "drop",
	}
	if err := d.db.WithContext(ctx).Create(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Update 更新策略配置。
func (d *ReplyStrategyDAO) Update(ctx context.Context, cfg *models.ReplyStrategyConfig) error {
	return d.db.WithContext(ctx).Save(cfg).Error
}

// UpdateLayaCapabilitySnapshot updates only the capability snapshot fields.
// Capability discovery runs outside the configuration update request, so it
// must never persist a stale full ReplyStrategyConfig row. Results are discarded
// if the saved discovery source changed while the request was in flight.
func (d *ReplyStrategyDAO) UpdateLayaCapabilitySnapshot(ctx context.Context, cfg *models.ReplyStrategyConfig, snapshot string, fetchedAt time.Time) error {
	result := d.db.WithContext(ctx).
		Model(&models.ReplyStrategyConfig{}).
		Where("id = ? AND laya_sticker_endpoint = ? AND laya_capabilities_endpoint = ? AND laya_sticker_api_key = ?",
			cfg.ID, cfg.LayaStickerEndpoint, cfg.LayaCapabilitiesEndpoint, cfg.LayaStickerAPIKey).
		Updates(map[string]any{
			"laya_capability_snapshot":   snapshot,
			"laya_capability_fetched_at": fetchedAt,
			"laya_capability_error":      "",
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrLayaCapabilityConfigChanged
	}
	return nil
}

// UpdateLayaCapabilityError records a failed discovery without touching the
// last successful snapshot or any administrator-owned decision settings.
func (d *ReplyStrategyDAO) UpdateLayaCapabilityError(ctx context.Context, cfg *models.ReplyStrategyConfig, message string) error {
	result := d.db.WithContext(ctx).
		Model(&models.ReplyStrategyConfig{}).
		Where("id = ? AND laya_sticker_endpoint = ? AND laya_capabilities_endpoint = ? AND laya_sticker_api_key = ?",
			cfg.ID, cfg.LayaStickerEndpoint, cfg.LayaCapabilitiesEndpoint, cfg.LayaStickerAPIKey).
		Updates(map[string]any{"laya_capability_error": message})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrLayaCapabilityConfigChanged
	}
	return nil
}
