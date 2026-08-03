package dao

import (
	"context"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TokenUsageDailyDAO 每日 Token 用量统计 DAO。
type TokenUsageDailyDAO struct{ db *gorm.DB }

func NewTokenUsageDailyDAO(db *gorm.DB) *TokenUsageDailyDAO { return &TokenUsageDailyDAO{db: db} }

// AddTokenUsage 累加指定日期的 Token 用量；当日记录不存在时自动创建（UPSERT）。
// date 格式为 YYYY-MM-DD。
func (d *TokenUsageDailyDAO) AddTokenUsage(ctx context.Context, date string, tokens int64) error {
	if tokens <= 0 {
		return nil
	}
	record := &models.TokenUsageDaily{Date: date, TokenCount: tokens}
	// 注意：DO UPDATE 表达式中列名必须限定表名。Postgres 中 INSERT 列清单与目标表
	// 同名列会冲突，未限定会报 "column reference is ambiguous" (SQLSTATE 42702)。
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "date"}},
		DoUpdates: clause.Assignments(map[string]any{
			"token_count": gorm.Expr("token_usage_dailies.token_count + ?", tokens),
		}),
	}).Create(record).Error
}

// ListByRange 查询 [startDate, endDate] 区间内的每日用量（按日期升序）。
// startDate / endDate 为空表示不限制对应边界，格式均为 YYYY-MM-DD。
func (d *TokenUsageDailyDAO) ListByRange(ctx context.Context, startDate, endDate string) ([]models.TokenUsageDaily, error) {
	q := d.db.WithContext(ctx).Model(&models.TokenUsageDaily{}).Order("date ASC")
	if startDate != "" {
		q = q.Where("date >= ?", startDate)
	}
	if endDate != "" {
		q = q.Where("date <= ?", endDate)
	}
	var list []models.TokenUsageDaily
	err := q.Find(&list).Error
	return list, err
}

// Total 返回历史累计 Token 用量。
func (d *TokenUsageDailyDAO) Total(ctx context.Context) (int64, error) {
	var total int64
	err := d.db.WithContext(ctx).Model(&models.TokenUsageDaily{}).
		Select("COALESCE(SUM(token_count), 0)").Scan(&total).Error
	return total, err
}
