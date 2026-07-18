package dao

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---------- 用户 DAO ----------

type UserDAO struct{ db *gorm.DB }

func NewUserDAO(db *gorm.DB) *UserDAO { return &UserDAO{db: db} }

func (d *UserDAO) Create(ctx context.Context, u *models.AdminUser) error {
	return d.db.WithContext(ctx).Create(u).Error
}

func (d *UserDAO) GetByUsername(ctx context.Context, username string) (*models.AdminUser, error) {
	var u models.AdminUser
	err := d.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *UserDAO) UpdatePassword(ctx context.Context, id uint, hash string) error {
	return d.db.WithContext(ctx).Model(&models.AdminUser{}).Where("id = ?", id).
		Update("password_hash", hash).Error
}

func (d *UserDAO) Exists(ctx context.Context) (bool, error) {
	var count int64
	err := d.db.WithContext(ctx).Model(&models.AdminUser{}).Count(&count).Error
	return count > 0, err
}

// ---------- Onebot11 Adapter DAO ----------

type Onebot11AdapterDao struct{ db *gorm.DB }

func NewOnebot11AdapterDao(db *gorm.DB) *Onebot11AdapterDao {
	return &Onebot11AdapterDao{db: db}
}

func (d *Onebot11AdapterDao) InitAdapterConfig(ctx context.Context) error {
	return d.db.Create(&models.Onebot11Adapter{
		ID:     1,
		Addr:   "0.0.0.0",
		Port:   8081,
		Token:  "wow-a-lovey-juan-niang",
		Admins: []int64{},
	}).WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Error
}

func (d *Onebot11AdapterDao) GetAdapterConfig(ctx context.Context) (*models.Onebot11Adapter, error) {
	var data models.Onebot11Adapter

	if err := d.db.WithContext(ctx).First(&data).Error; err != nil {
		return nil, err
	}

	return &data, nil
}

func (d *Onebot11AdapterDao) UpdateAdapterConfig(ctx context.Context, conf *models.Onebot11Adapter) error {
	return d.db.WithContext(ctx).Where("id = 1").Updates(conf).Error
}

// ---------- Admin QQ DAO ----------

type AdminQQDAO struct{ db *gorm.DB }

func NewAdminQQDAO(db *gorm.DB) *AdminQQDAO { return &AdminQQDAO{db: db} }

func (d *AdminQQDAO) List(ctx context.Context) ([]models.AdminQQ, error) {
	var list []models.AdminQQ
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *AdminQQDAO) Add(ctx context.Context, qq int64) error {
	return d.db.WithContext(ctx).Create(&models.AdminQQ{ID: qq}).Error
}

func (d *AdminQQDAO) Remove(ctx context.Context, qq int64) error {
	return d.db.WithContext(ctx).Where("id = ?", qq).Delete(&models.AdminQQ{}).Error
}

// ---------- Provider DAO ----------

type ProviderDAO struct{ db *gorm.DB }

func NewProviderDAO(db *gorm.DB) *ProviderDAO { return &ProviderDAO{db: db} }

func (d *ProviderDAO) Create(ctx context.Context, p *models.Provider) error {
	if p.ID == "" {
		p.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(p).Error
}

func (d *ProviderDAO) GetByID(ctx context.Context, id string) (*models.Provider, error) {
	var p models.Provider
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *ProviderDAO) Update(ctx context.Context, p *models.Provider) error {
	return d.db.WithContext(ctx).Save(p).Error
}

func (d *ProviderDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Provider{}).Error
}

func (d *ProviderDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.Provider{}).Where("id = ?", id).
		Update("is_active", active).Error
}

func (d *ProviderDAO) List(ctx context.Context, typ models.ModelType) ([]models.Provider, error) {
	var list []models.Provider
	q := d.db.WithContext(ctx)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	err := q.Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ProviderDAO) ListActive(ctx context.Context, typ models.ModelType) ([]models.Provider, error) {
	var list []models.Provider
	q := d.db.WithContext(ctx).Where("is_active = ?", true)
	if typ != "" {
		q = q.Where("type = ?", typ)
	}
	err := q.Order("created_at DESC").Find(&list).Error
	return list, err
}

// ---------- MCP Server DAO ----------

type MCPServerDAO struct{ db *gorm.DB }

func NewMCPServerDAO(db *gorm.DB) *MCPServerDAO { return &MCPServerDAO{db: db} }

func (d *MCPServerDAO) Create(ctx context.Context, m *models.MCPServer) error {
	if m.ID == "" {
		m.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(m).Error
}

func (d *MCPServerDAO) GetByID(ctx context.Context, id string) (*models.MCPServer, error) {
	var m models.MCPServer
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *MCPServerDAO) Update(ctx context.Context, m *models.MCPServer) error {
	return d.db.WithContext(ctx).Save(m).Error
}

func (d *MCPServerDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.MCPServer{}).Error
}

func (d *MCPServerDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.MCPServer{}).Where("id = ?", id).
		Update("is_active", active).Error
}

func (d *MCPServerDAO) List(ctx context.Context) ([]models.MCPServer, error) {
	var list []models.MCPServer
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *MCPServerDAO) ListActive(ctx context.Context) ([]models.MCPServer, error) {
	var list []models.MCPServer
	err := d.db.WithContext(ctx).Where("is_active = ?", true).
		Order("created_at DESC").Find(&list).Error
	return list, err
}

// ---------- Skill DAO ----------

type SkillDAO struct{ db *gorm.DB }

func NewSkillDAO(db *gorm.DB) *SkillDAO { return &SkillDAO{db: db} }

func (d *SkillDAO) Create(ctx context.Context, s *models.Skill) error {
	if s.ID == "" {
		s.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(s).Error
}

func (d *SkillDAO) GetByID(ctx context.Context, id string) (*models.Skill, error) {
	var s models.Skill
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *SkillDAO) Update(ctx context.Context, s *models.Skill) error {
	return d.db.WithContext(ctx).Save(s).Error
}

func (d *SkillDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Skill{}).Error
}

func (d *SkillDAO) List(ctx context.Context) ([]models.Skill, error) {
	var list []models.Skill
	err := d.db.WithContext(ctx).Order("priority DESC, created_at DESC").Find(&list).Error
	return list, err
}

func (d *SkillDAO) ListActive(ctx context.Context) ([]models.Skill, error) {
	var list []models.Skill
	err := d.db.WithContext(ctx).Where("is_active = ?", true).
		Order("priority DESC, created_at DESC").Find(&list).Error
	return list, err
}

func (d *SkillDAO) ListSystem(ctx context.Context) ([]models.Skill, error) {
	var list []models.Skill
	err := d.db.WithContext(ctx).Where("is_active = ? AND is_system = ?", true, true).
		Order("priority DESC").Find(&list).Error
	return list, err
}

// ---------- Tool Config DAO ----------

type ToolConfigDAO struct{ db *gorm.DB }

func NewToolConfigDAO(db *gorm.DB) *ToolConfigDAO { return &ToolConfigDAO{db: db} }

func (d *ToolConfigDAO) Create(ctx context.Context, t *models.ToolConfig) error {
	if t.ID == "" {
		t.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(t).Error
}

func (d *ToolConfigDAO) GetByID(ctx context.Context, id string) (*models.ToolConfig, error) {
	var t models.ToolConfig
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *ToolConfigDAO) GetByName(ctx context.Context, name string) (*models.ToolConfig, error) {
	var t models.ToolConfig
	err := d.db.WithContext(ctx).Where("name = ?", name).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *ToolConfigDAO) Update(ctx context.Context, t *models.ToolConfig) error {
	return d.db.WithContext(ctx).Save(t).Error
}

func (d *ToolConfigDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.ToolConfig{}).Error
}

func (d *ToolConfigDAO) List(ctx context.Context) ([]models.ToolConfig, error) {
	var list []models.ToolConfig
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ToolConfigDAO) ListActive(ctx context.Context) ([]models.ToolConfig, error) {
	var list []models.ToolConfig
	err := d.db.WithContext(ctx).Where("is_active = ?", true).
		Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ToolConfigDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.ToolConfig{}).Where("id = ?", id).
		Update("is_active", active).Error
}

// ---------- Prompt DAO ----------

type PromptDAO struct{ db *gorm.DB }

func NewPromptDAO(db *gorm.DB) *PromptDAO { return &PromptDAO{db: db} }

func (d *PromptDAO) Create(ctx context.Context, p *models.Prompt) error {
	if p.ID == "" {
		p.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(p).Error
}

func (d *PromptDAO) GetByID(ctx context.Context, id string) (*models.Prompt, error) {
	var p models.Prompt
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *PromptDAO) Update(ctx context.Context, p *models.Prompt) error {
	return d.db.WithContext(ctx).Save(p).Error
}

func (d *PromptDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Prompt{}).Error
}

func (d *PromptDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.Prompt{}).Where("id = ?", id).
		Update("is_active", active).Error
}

func (d *PromptDAO) List(ctx context.Context) ([]models.Prompt, error) {
	var list []models.Prompt
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *PromptDAO) ListByType(ctx context.Context, typ models.PromptType) ([]models.Prompt, error) {
	var list []models.Prompt
	err := d.db.WithContext(ctx).Where("is_active = ? AND type = ?", true, typ).
		Order("created_at DESC").Find(&list).Error
	return list, err
}

// ---------- ChatArea DAO ----------

type ChatAreaDAO struct{ db *gorm.DB }

func NewChatAreaDAO(db *gorm.DB) *ChatAreaDAO { return &ChatAreaDAO{db: db} }

func (d *ChatAreaDAO) GetOrCreate(ctx context.Context, areaType models.AreaType, targetID int64) (*models.ChatArea, error) {
	var area models.ChatArea
	err := d.db.WithContext(ctx).
		Where("area_type = ? AND target_id = ?", areaType, targetID).
		First(&area).Error
	if err == nil {
		return &area, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	area = models.ChatArea{
		ID:       newUUID(),
		AreaType: areaType,
		TargetID: targetID,
	}
	if err := d.db.WithContext(ctx).Create(&area).Error; err != nil {
		return nil, err
	}
	return &area, nil
}

func (d *ChatAreaDAO) GetByID(ctx context.Context, id string) (*models.ChatArea, error) {
	var a models.ChatArea
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (d *ChatAreaDAO) List(ctx context.Context) ([]models.ChatArea, error) {
	var list []models.ChatArea
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ChatAreaDAO) Count(ctx context.Context) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).Model(&models.ChatArea{}).Count(&count).Error
	return count, err
}

func (d *ChatAreaDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.ChatArea{}).Error
}

// ---------- Session DAO ----------

type SessionDAO struct{ db *gorm.DB }

func NewSessionDAO(db *gorm.DB) *SessionDAO { return &SessionDAO{db: db} }

func (d *SessionDAO) GetOrCreate(ctx context.Context, chatAreaID string) (*models.Session, error) {
	var s models.Session
	err := d.db.WithContext(ctx).Where("chat_area_id = ?", chatAreaID).First(&s).Error
	if err == nil {
		return &s, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	s = models.Session{
		ID:         newUUID(),
		ChatAreaID: chatAreaID,
	}
	if err := d.db.WithContext(ctx).Create(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *SessionDAO) GetByID(ctx context.Context, id string) (*models.Session, error) {
	var s models.Session
	err := d.db.WithContext(ctx).First(&s, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *SessionDAO) Update(ctx context.Context, s *models.Session) error {
	return d.db.WithContext(ctx).Save(s).Error
}

func (d *SessionDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Session{}).Error
}

func (d *SessionDAO) AddTokenUsage(ctx context.Context, id string, tokens int64) error {
	return d.db.WithContext(ctx).Model(&models.Session{}).Where("id = ?", id).
		Update("token_usage", gorm.Expr("token_usage + ?", tokens)).Error
}

func (d *SessionDAO) List(ctx context.Context) ([]models.Session, error) {
	var list []models.Session
	err := d.db.WithContext(ctx).Preload("ChatArea").Order("updated_at DESC").Find(&list).Error
	return list, err
}

// ---------- Memory DAO ----------

type ShortTermMemoryDAO struct{ db *gorm.DB }

func NewShortTermMemoryDAO(db *gorm.DB) *ShortTermMemoryDAO { return &ShortTermMemoryDAO{db: db} }

func (d *ShortTermMemoryDAO) GetOrCreate(ctx context.Context, chatAreaID string) (*models.ShortTermMemory, error) {
	var m models.ShortTermMemory
	err := d.db.WithContext(ctx).Where("chat_area_id = ?", chatAreaID).First(&m).Error
	if err == nil {
		return &m, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	m = models.ShortTermMemory{
		ID:         newUUID(),
		ChatAreaID: chatAreaID,
		WindowSize: 20,
	}
	if err := d.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *ShortTermMemoryDAO) Update(ctx context.Context, m *models.ShortTermMemory) error {
	return d.db.WithContext(ctx).Save(m).Error
}

type LongTermMemoryDAO struct{ db *gorm.DB }

func NewLongTermMemoryDAO(db *gorm.DB) *LongTermMemoryDAO { return &LongTermMemoryDAO{db: db} }

func (d *LongTermMemoryDAO) GetOrCreate(ctx context.Context, chatAreaID string) (*models.LongTermMemory, error) {
	var m models.LongTermMemory
	err := d.db.WithContext(ctx).Where("chat_area_id = ?", chatAreaID).First(&m).Error
	if err == nil {
		return &m, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	m = models.LongTermMemory{
		ID:           newUUID(),
		ChatAreaID:   chatAreaID,
		HotAreaSize:  10,
		HotMemoryTTL: 86400,
	}
	if err := d.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *LongTermMemoryDAO) Update(ctx context.Context, m *models.LongTermMemory) error {
	return d.db.WithContext(ctx).Save(m).Error
}

// LongTermMemoryItemDAO 管理长期记忆条目。

type LongTermMemoryItemDAO struct{ db *gorm.DB }

func NewLongTermMemoryItemDAO(db *gorm.DB) *LongTermMemoryItemDAO {
	return &LongTermMemoryItemDAO{db: db}
}

func (d *LongTermMemoryItemDAO) Create(ctx context.Context, item *models.LongTermMemoryItem) error {
	if item.ID == "" {
		item.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(item).Error
}

func (d *LongTermMemoryItemDAO) ListByChatArea(ctx context.Context, chatAreaID string, limit int) ([]models.LongTermMemoryItem, error) {
	var list []models.LongTermMemoryItem
	err := d.db.WithContext(ctx).
		Where("chat_area_id = ?", chatAreaID).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (d *LongTermMemoryItemDAO) SearchByContent(ctx context.Context, chatAreaID string, keyword string, limit int) ([]models.LongTermMemoryItem, error) {
	var list []models.LongTermMemoryItem
	err := d.db.WithContext(ctx).
		Where("chat_area_id = ? AND content ILIKE ?", chatAreaID, "%"+keyword+"%").
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (d *LongTermMemoryItemDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.LongTermMemoryItem{}).Error
}

func (d *LongTermMemoryItemDAO) CountByChatArea(ctx context.Context, chatAreaID string) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).
		Where("chat_area_id = ?", chatAreaID).Count(&count).Error
	return count, err
}

func (d *LongTermMemoryItemDAO) DeleteOldest(ctx context.Context, chatAreaID string, keep int) error {
	sub := d.db.WithContext(ctx).Model(&models.LongTermMemoryItem{}).
		Select("id").
		Where("chat_area_id = ?", chatAreaID).
		Order("created_at DESC").
		Limit(999999).
		Offset(keep)
	return d.db.WithContext(ctx).Where("id IN (?)", sub).Delete(&models.LongTermMemoryItem{}).Error
}

// ---------- Background Task DAO ----------

type BackgroundTaskDAO struct{ db *gorm.DB }

func NewBackgroundTaskDAO(db *gorm.DB) *BackgroundTaskDAO { return &BackgroundTaskDAO{db: db} }

func (d *BackgroundTaskDAO) Create(ctx context.Context, t *models.BackgroundTask) error {
	if t.ID == "" {
		t.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(t).Error
}

func (d *BackgroundTaskDAO) GetByID(ctx context.Context, id string) (*models.BackgroundTask, error) {
	var t models.BackgroundTask
	err := d.db.WithContext(ctx).First(&t, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *BackgroundTaskDAO) Update(ctx context.Context, t *models.BackgroundTask) error {
	return d.db.WithContext(ctx).Save(t).Error
}

func (d *BackgroundTaskDAO) UpdateStatus(ctx context.Context, id string, status models.TaskStatus) error {
	return d.db.WithContext(ctx).Model(&models.BackgroundTask{}).Where("id = ?", id).
		Update("status", status).Error
}

func (d *BackgroundTaskDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.BackgroundTask{}).Error
}

func (d *BackgroundTaskDAO) ListByChatArea(ctx context.Context, chatAreaID string) ([]models.BackgroundTask, error) {
	var list []models.BackgroundTask
	err := d.db.WithContext(ctx).Where("chat_area_id = ?", chatAreaID).
		Order("created_at DESC").Find(&list).Error
	return list, err
}

// ---------- Chat Record DAO ----------

type ChatRecordDAO struct{ db *gorm.DB }

func NewChatRecordDAO(db *gorm.DB) *ChatRecordDAO { return &ChatRecordDAO{db: db} }

func (d *ChatRecordDAO) Create(ctx context.Context, r *models.ChatRecord) error {
	return d.db.WithContext(ctx).Create(r).Error
}

func (d *ChatRecordDAO) BatchCreate(ctx context.Context, records []models.ChatRecord) error {
	return d.db.WithContext(ctx).Create(&records).Error
}

func (d *ChatRecordDAO) ListByChatArea(ctx context.Context, chatAreaID string, limit, offset int) ([]models.ChatRecord, int64, error) {
	var list []models.ChatRecord
	var total int64

	q := d.db.WithContext(ctx).Model(&models.ChatRecord{}).Where("chat_area_id = ?", chatAreaID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, total, err
}

func (d *ChatRecordDAO) GetToolCallRecords(ctx context.Context, chatAreaID string, limit, offset int) ([]models.ChatRecord, int64, error) {
	var list []models.ChatRecord
	var total int64

	q := d.db.WithContext(ctx).Model(&models.ChatRecord{}).
		Where("chat_area_id = ? AND role = ?", chatAreaID, "tool")
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, total, err
}

func (d *ChatRecordDAO) TotalTokenUsage(ctx context.Context) (int64, error) {
	var total int64
	err := d.db.WithContext(ctx).Model(&models.ChatRecord{}).
		Select("COALESCE(SUM(token_count), 0)").Scan(&total).Error
	return total, err
}

func (d *ChatRecordDAO) ListByChatAreaAndRole(ctx context.Context, chatAreaID, role string, limit, offset int) ([]models.ChatRecord, int64, error) {
	var list []models.ChatRecord
	var total int64

	q := d.db.WithContext(ctx).Model(&models.ChatRecord{}).
		Where("chat_area_id = ? AND role = ?", chatAreaID, role)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, total, err
}

// ---------- Plugin DAO ----------

type PluginDAO struct{ db *gorm.DB }

func NewPluginDAO(db *gorm.DB) *PluginDAO { return &PluginDAO{db: db} }

func (d *PluginDAO) Create(ctx context.Context, p *models.Plugin) error {
	if p.ID == "" {
		p.ID = newUUID()
	}
	return d.db.WithContext(ctx).Create(p).Error
}

func (d *PluginDAO) GetByID(ctx context.Context, id string) (*models.Plugin, error) {
	var p models.Plugin
	err := d.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *PluginDAO) GetByName(ctx context.Context, name string) (*models.Plugin, error) {
	var p models.Plugin
	err := d.db.WithContext(ctx).First(&p, "name = ?", name).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *PluginDAO) Update(ctx context.Context, p *models.Plugin) error {
	return d.db.WithContext(ctx).Save(p).Error
}

func (d *PluginDAO) Delete(ctx context.Context, id string) error {
	return d.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Plugin{}).Error
}

func (d *PluginDAO) List(ctx context.Context) ([]models.Plugin, error) {
	var list []models.Plugin
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *PluginDAO) SetActive(ctx context.Context, id string, active bool) error {
	return d.db.WithContext(ctx).Model(&models.Plugin{}).Where("id = ?", id).
		Update("is_active", active).Error
}

// ---------- ACL DAO ----------

type ACLDAO struct{ db *gorm.DB }

func NewACLDAO(db *gorm.DB) *ACLDAO { return &ACLDAO{db: db} }

func (d *ACLDAO) Create(ctx context.Context, r *models.ACLRule) error {
	return d.db.WithContext(ctx).Create(r).Error
}

func (d *ACLDAO) Delete(ctx context.Context, id int64) error {
	return d.db.WithContext(ctx).Delete(&models.ACLRule{}, id).Error
}

func (d *ACLDAO) List(ctx context.Context) ([]models.ACLRule, error) {
	var list []models.ACLRule
	err := d.db.WithContext(ctx).Order("created_at DESC").Find(&list).Error
	return list, err
}

func (d *ACLDAO) GetByUserAndChatArea(ctx context.Context, userID int64, chatAreaID string) (*models.ACLRule, error) {
	var r models.ACLRule
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND chat_area_id = ?", userID, chatAreaID).
		First(&r).Error
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ---------- 全局统计 ----------

type OverviewStats struct {
	ChatAreaCount   int64 `json:"chat_area_count"`
	MCPCount        int64 `json:"mcp_count"`
	AdapterCount    int64 `json:"adapter_count"`
	PluginCount     int64 `json:"plugin_count"`
	TotalTokenUsage int64 `json:"total_token_usage"`
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

// ---------- 工具函数 ----------

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// ---------- DAO Bundle ----------

// Bundle 汇聚所有 DAO，方便注入。
type Bundle struct {
	User            *UserDAO
	AdminQQ         *AdminQQDAO
	Provider        *ProviderDAO
	MCPServer       *MCPServerDAO
	Skill           *SkillDAO
	ToolConfig      *ToolConfigDAO
	Prompt          *PromptDAO
	ChatArea        *ChatAreaDAO
	Session         *SessionDAO
	ShortTermMemory *ShortTermMemoryDAO
	LongTermMemory  *LongTermMemoryDAO
	LongTermMemItem *LongTermMemoryItemDAO
	BackgroundTask  *BackgroundTaskDAO
	ChatRecord      *ChatRecordDAO
	Plugin          *PluginDAO
	ACL             *ACLDAO
	Onebot11Adapter *Onebot11AdapterDao
}

func NewBundle(db *gorm.DB) *Bundle {
	return &Bundle{
		User:            NewUserDAO(db),
		AdminQQ:         NewAdminQQDAO(db),
		Provider:        NewProviderDAO(db),
		MCPServer:       NewMCPServerDAO(db),
		Skill:           NewSkillDAO(db),
		ToolConfig:      NewToolConfigDAO(db),
		Prompt:          NewPromptDAO(db),
		ChatArea:        NewChatAreaDAO(db),
		Session:         NewSessionDAO(db),
		ShortTermMemory: NewShortTermMemoryDAO(db),
		LongTermMemory:  NewLongTermMemoryDAO(db),
		LongTermMemItem: NewLongTermMemoryItemDAO(db),
		BackgroundTask:  NewBackgroundTaskDAO(db),
		ChatRecord:      NewChatRecordDAO(db),
		Plugin:          NewPluginDAO(db),
		ACL:             NewACLDAO(db),
		Onebot11Adapter: NewOnebot11AdapterDao(db),
	}
}
