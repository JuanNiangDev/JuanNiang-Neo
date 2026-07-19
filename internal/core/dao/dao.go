package dao

import (
	"context"
	"crypto/rand"
	"fmt"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/gorm"
)

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
