package dao

import (
	"JuanNiang-Neo/internal/core/models"
	"context"

	"gorm.io/gorm"
)

type ChatRecordDAO struct{ db *gorm.DB }

func NewChatRecordDAO(db *gorm.DB) *ChatRecordDAO { return &ChatRecordDAO{db: db} }

func (d *ChatRecordDAO) Create(ctx context.Context, r *models.ChatRecord) error {
	return d.db.WithContext(ctx).Create(r).Error
}

func (d *ChatRecordDAO) BatchCreate(ctx context.Context, records []models.ChatRecord) error {
	return d.db.WithContext(ctx).Create(&records).Error
}

func (d *ChatRecordDAO) ListByChatArea(ctx context.Context, chatAreaID string, limit, offset int) ([]models.ChatRecord, int64, error) {
	return d.listByChatAreaWithRole(ctx, chatAreaID, "", limit, offset)
}

func (d *ChatRecordDAO) TotalTokenUsage(ctx context.Context) (int64, error) {
	var total int64
	err := d.db.WithContext(ctx).Model(&models.ChatRecord{}).
		Select("COALESCE(SUM(token_count), 0)").Scan(&total).Error
	return total, err
}

func (d *ChatRecordDAO) ListByChatAreaAndRole(ctx context.Context, chatAreaID, role string, limit, offset int) ([]models.ChatRecord, int64, error) {
	return d.listByChatAreaWithRole(ctx, chatAreaID, role, limit, offset)
}

// listByChatAreaWithRole 分页查询某 ChatArea 的聊天记录，role 为空时不过滤角色。
func (d *ChatRecordDAO) listByChatAreaWithRole(ctx context.Context, chatAreaID, role string, limit, offset int) ([]models.ChatRecord, int64, error) {
	var list []models.ChatRecord
	var total int64

	q := d.db.WithContext(ctx).Model(&models.ChatRecord{}).Where("chat_area_id = ?", chatAreaID)
	if role != "" {
		q = q.Where("role = ?", role)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, total, err
}
