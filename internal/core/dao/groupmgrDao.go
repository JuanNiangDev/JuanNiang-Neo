package dao

import (
	"context"
	"errors"
	"strconv"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GroupMgrDAO 群管理系统功能的全部持久化（config/words/samples/violations/whitelist/admins/stats）。
type GroupMgrDAO struct{ db *gorm.DB }

func NewGroupMgrDAO(db *gorm.DB) *GroupMgrDAO { return &GroupMgrDAO{db: db} }

// ---------- 配置（单行） ----------

// InitConfig 初始化默认配置（不存在时插入；默认关闭，用户面板启用）。
func (d *GroupMgrDAO) InitConfig(ctx context.Context) error {
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&models.GroupMgrConfig{
		ID:            1,
		HighScore:     0.75,
		LowScore:      0.5,
		FallbackScore: 0.6,
	}).Error
}

// GetConfig 读取配置。
func (d *GroupMgrDAO) GetConfig(ctx context.Context) (*models.GroupMgrConfig, error) {
	var cfg models.GroupMgrConfig
	if err := d.db.WithContext(ctx).Where("id = 1").First(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpdateConfig 更新配置。
func (d *GroupMgrDAO) UpdateConfig(ctx context.Context, cfg *models.GroupMgrConfig) error {
	return d.db.WithContext(ctx).Where("id = 1").Save(cfg).Error
}

// ---------- 词条 ----------

// WordListAll 列出全部词条（按分类+词序）。
func (d *GroupMgrDAO) WordListAll(ctx context.Context) ([]models.GroupMgrWord, error) {
	var list []models.GroupMgrWord
	err := d.db.WithContext(ctx).Order("category ASC, word ASC").Find(&list).Error
	return list, err
}

// WordListByCategory 按分类列出词条。
func (d *GroupMgrDAO) WordListByCategory(ctx context.Context, category string) ([]models.GroupMgrWord, error) {
	var list []models.GroupMgrWord
	err := d.db.WithContext(ctx).Where("category = ?", category).Order("word ASC").Find(&list).Error
	return list, err
}

// WordUpsert 新增词条（幂等：已存在则更新分类/来源，不重复插入）。
func (d *GroupMgrDAO) WordUpsert(ctx context.Context, word, category, source string) error {
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "word"}},
		DoUpdates: clause.AssignmentColumns([]string{"category", "source", "updated_at"}),
	}).Create(&models.GroupMgrWord{Word: word, Category: category, Source: source}).Error
}

// WordDelete 删除词条（软删；唯一索引软删后重建同名由部分唯一索引约束）。
func (d *GroupMgrDAO) WordDelete(ctx context.Context, id uint) error {
	return d.db.WithContext(ctx).Delete(&models.GroupMgrWord{}, id).Error
}

// WordCount 词条总数（种子导入判断用）。
func (d *GroupMgrDAO) WordCount(ctx context.Context) (int64, error) {
	var n int64
	err := d.db.WithContext(ctx).Model(&models.GroupMgrWord{}).Count(&n).Error
	return n, err
}

// ---------- 样本 ----------

// SampleListAll 列出全部样本。
func (d *GroupMgrDAO) SampleListAll(ctx context.Context) ([]models.GroupMgrSample, error) {
	var list []models.GroupMgrSample
	err := d.db.WithContext(ctx).Order("id ASC").Find(&list).Error
	return list, err
}

// SampleAdd 新增样本（幂等：text 已存在则刷新分类/来源并返回其 ID）。
func (d *GroupMgrDAO) SampleAdd(ctx context.Context, text, category, source string) (uint, error) {
	var existing models.GroupMgrSample
	err := d.db.WithContext(ctx).Where("text = ?", text).First(&existing).Error
	if err == nil {
		if existing.Category != category || existing.Source != source {
			existing.Category = category
			existing.Source = source
			if uerr := d.db.WithContext(ctx).Save(&existing).Error; uerr != nil {
				return 0, uerr
			}
		}
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	s := models.GroupMgrSample{Text: text, Category: category, Source: source}
	if err := d.db.WithContext(ctx).Create(&s).Error; err != nil {
		return 0, err
	}
	return s.ID, nil
}

// SampleDelete 删除样本。
func (d *GroupMgrDAO) SampleDelete(ctx context.Context, id uint) error {
	return d.db.WithContext(ctx).Delete(&models.GroupMgrSample{}, id).Error
}

// SampleIncrHit 命中计数 +1。
func (d *GroupMgrDAO) SampleIncrHit(ctx context.Context, id uint) error {
	return d.db.WithContext(ctx).Model(&models.GroupMgrSample{}).Where("id = ?", id).
		UpdateColumn("hit_count", gorm.Expr("hit_count + 1")).Error
}

// ---------- 违规记录 ----------

// ViolationGet 读取某群某用户的违规次数（无记录返回 0, nil）。
func (d *GroupMgrDAO) ViolationGet(ctx context.Context, groupID, userID int64) (int, error) {
	var v models.GroupMgrViolation
	err := d.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return v.Count, nil
}

// ViolationSet 写入违规次数（0 视为删除）。
func (d *GroupMgrDAO) ViolationSet(ctx context.Context, groupID, userID int64, count int) error {
	if count <= 0 {
		return d.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).
			Delete(&models.GroupMgrViolation{}).Error
	}
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "group_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"count", "updated_at"}),
	}).Create(&models.GroupMgrViolation{GroupID: groupID, UserID: userID, Count: count}).Error
}

// ViolationList 列出全部违规记录。
func (d *GroupMgrDAO) ViolationList(ctx context.Context) ([]models.GroupMgrViolation, error) {
	var list []models.GroupMgrViolation
	err := d.db.WithContext(ctx).Order("updated_at DESC").Find(&list).Error
	return list, err
}

// ViolationDelete 删除单条违规记录（面板"删除某行即重置该用户违规"）。
func (d *GroupMgrDAO) ViolationDelete(ctx context.Context, id uint) error {
	return d.db.WithContext(ctx).Delete(&models.GroupMgrViolation{}, id).Error
}

// ViolationClearUser 清除某 QQ 号全部群的违规记录，返回清除条数。
func (d *GroupMgrDAO) ViolationClearUser(ctx context.Context, userID int64) (int, error) {
	res := d.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.GroupMgrViolation{})
	return int(res.RowsAffected), res.Error
}

// ---------- 白名单 / 手动管理员 ----------

// WlList 白名单列表。
func (d *GroupMgrDAO) WlList(ctx context.Context) ([]models.GroupMgrWhitelist, error) {
	var list []models.GroupMgrWhitelist
	err := d.db.WithContext(ctx).Order("qq ASC").Find(&list).Error
	return list, err
}

// WlAdd 加入白名单（幂等）。
func (d *GroupMgrDAO) WlAdd(ctx context.Context, qq int64) error {
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.GroupMgrWhitelist{QQ: qq}).Error
}

// WlDelete 移出白名单。
func (d *GroupMgrDAO) WlDelete(ctx context.Context, qq int64) error {
	return d.db.WithContext(ctx).Where("qq = ?", qq).Delete(&models.GroupMgrWhitelist{}).Error
}

// AdminList 手动管理员列表。
func (d *GroupMgrDAO) AdminList(ctx context.Context) ([]models.GroupMgrAdmin, error) {
	var list []models.GroupMgrAdmin
	err := d.db.WithContext(ctx).Order("qq ASC").Find(&list).Error
	return list, err
}

// AdminAdd 加入手动管理员（幂等）。
func (d *GroupMgrDAO) AdminAdd(ctx context.Context, qq int64) error {
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.GroupMgrAdmin{QQ: qq}).Error
}

// AdminDelete 移除手动管理员。
func (d *GroupMgrDAO) AdminDelete(ctx context.Context, qq int64) error {
	return d.db.WithContext(ctx).Where("qq = ?", qq).Delete(&models.GroupMgrAdmin{}).Error
}

// ---------- 统计 / 状态 kv ----------

// StatGet 读取 kv（无则返回空串）。
func (d *GroupMgrDAO) StatGet(ctx context.Context, key string) (string, error) {
	var s models.GroupMgrStat
	err := d.db.WithContext(ctx).Where("key = ?", key).First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return s.Value, nil
}

// StatSet 写 kv（幂等 upsert）。
func (d *GroupMgrDAO) StatSet(ctx context.Context, key, value string) error {
	return d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&models.GroupMgrStat{Key: key, Value: value}).Error
}

// StatIncr 数值 +1（非数值态按 1 起算）。
func (d *GroupMgrDAO) StatIncr(ctx context.Context, key string) (int64, error) {
	v, err := d.StatGet(ctx, key)
	if err != nil {
		return 0, err
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	n++
	return n, d.StatSet(ctx, key, strconv.FormatInt(n, 10))
}

// StatListPrefix 列出前缀匹配的 kv（统计页聚合展示用）。
func (d *GroupMgrDAO) StatListPrefix(ctx context.Context, prefix string) ([]models.GroupMgrStat, error) {
	var list []models.GroupMgrStat
	err := d.db.WithContext(ctx).Where("key LIKE ?", prefix+"%").Order("key ASC").Find(&list).Error
	return list, err
}
