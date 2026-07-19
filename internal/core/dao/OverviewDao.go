package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type OverviewStats struct {
	ChatAreaCount   int64
	MCPCount        int64
	AdapterCount    int64
	PluginCount     int64
	TotalTokenUsage int64
}

func (d *UserDAO) GetOverviewStats(ctx context.Context, db *gorm.DB) (*OverviewStats, error) {
	var stats OverviewStats
	if err := db.WithContext(ctx).Model(&models.ChatArea{}).Count(&stats.ChatAreaCount).Error; err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&models.MCPServer{}).Count(&stats.MCPCount).Error; err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&models.Plugin{}).Count(&stats.PluginCount).Error; err != nil {
		return nil, err
	}
	stats.AdapterCount = 1 // 单Adapter
	total, err := NewChatRecordDAO(db).TotalTokenUsage(ctx)
	if err != nil {
		return nil, err
	}
	stats.TotalTokenUsage = total
	return &stats, nil
}
