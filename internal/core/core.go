package core

import (
	"context"
	"os"
	"sync"

	"JuanNiang-Neo/internal/core/acl"
	"JuanNiang-Neo/internal/core/cache"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	"JuanNiang-Neo/internal/logging"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var log = logging.NewModule("core")

// AutoMigrate 自动迁移所有 GORM 模型。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.AdminUser{},
		&models.Provider{},
		&models.MCPServer{},
		&models.Skill{},
		&models.ToolConfig{},
		&models.Prompt{},
		&models.ChatArea{},
		&models.Session{},
		&models.ShortTermMemory{},
		&models.LongTermMemory{},
		&models.LongTermMemoryItem{},
		&models.BackgroundTask{},
		&models.ChatRecord{},
		&models.Plugin{},
		&models.ACLRule{},
		&models.Onebot11Adapter{},
		&models.T2IConfig{},
		&models.SandboxConfig{},
		&models.WebhookConfig{},
		&models.CronJob{},
		&models.ReplyStrategyConfig{},
		&models.SkillMemory{},
		&models.TokenUsageDaily{},
		&models.KnowledgeItem{},
		&models.ImageAsset{},
		&models.ImageFolder{},
		&models.Sticker{},
		&models.StickerTag{},
		&models.FishCalendarConfig{},
		&models.FishCalendarAffair{},
		&models.ScheduledMessage{},
	)
}

// InitAdminUser 首次启动时创建管理员账户 (初始密码 Admin123)。
func InitAdminUser(ctx context.Context, userDAO *dao.UserDAO) error {
	exists, err := userDAO.Exists(ctx)
	if err != nil {
		return err
	}
	if exists {
		log.Info("管理员用户已存在，跳过初始化")
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("Admin123"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := &models.AdminUser{
		Username:     "admin",
		PasswordHash: string(hash),
		Role:         "admin",
	}
	if err := userDAO.Create(ctx, user); err != nil {
		return err
	}
	log.Warn("已创建默认管理员用户", "username", "admin", "password", "Admin123")
	return nil
}

// Core 聚合所有核心模块的初始化结果。
type Core struct {
	DB    *gorm.DB
	Cache *cache.Cache
	DAO   *dao.Bundle
	ACL   *acl.ACL
}

var (
	instance *Core
	once     sync.Once
)

// Init 初始化核心模块 (DB + Redis + AutoMigrate + ACL + AdminUser)。
// 仅在首次启动时执行数据库迁移和用户初始化。
func Init(ctx context.Context, db *gorm.DB, redisClient *redis.Client) (*Core, error) {
	var initErr error
	once.Do(func() {
		if err := AutoMigrate(db); err != nil {
			initErr = err
			return
		}

		// 迁移：移除旧普通唯一索引（不允许软删后重名，SQLSTATE 23505）。
		// 新部分唯一索引（WHERE deleted_at IS NULL）已由 AutoMigrate 按新索引名创建，
		// 旧索引继续阻塞软删后重建同名记录，这里幂等清理（含 image_folders 历史索引）。
		for _, idx := range []string{
			"idx_image_folders_name",   // image_folders 历史索引
			"idx_sticker_tags_name",    // sticker_tags
			"idx_plugins_name",         // plugins
			"idx_admin_users_username", // admin_users
		} {
			if err := db.Exec("DROP INDEX IF EXISTS " + idx).Error; err != nil {
				initErr = err
				return
			}
		}

		// 回复策略收敛为仅 relevance：存量行（never_reply/at_only/always）
		// 统一迁移到唯一策略，避免历史配置在只保留 relevance 后失效或行为歧义。
		if err := db.Exec("UPDATE reply_strategy_config SET strategy = ? WHERE strategy <> ?",
			models.StrategyRelevance, models.StrategyRelevance).Error; err != nil {
			initErr = err
			return
		}

		cacheInst := cache.NewCache(redisClient, os.Getenv("REDIS_PREFIX"))

		bundle := dao.NewBundle(db)
		aclInst := acl.NewACL(bundle.ACL)

		if err := InitAdminUser(ctx, bundle.User); err != nil {
			initErr = err
			return
		}

		// 系统内置「常用」表情标签：幂等创建（不存在则建），不可删除
		if err := bundle.Sticker.EnsureCommonTag(ctx); err != nil {
			initErr = err
			return
		}

		instance = &Core{
			DB:    db,
			Cache: cacheInst,
			DAO:   bundle,
			ACL:   aclInst,
		}
	})
	if initErr != nil {
		return nil, initErr
	}
	return instance, nil
}

// Get 返回单例 Core 实例。如果尚未初始化则返回 nil。
func Get() *Core {
	return instance
}
