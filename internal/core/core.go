package core

import (
	"JuanNiang-Neo/internal/logging"
	"context"
	"os"
	"sync"

	"JuanNiang-Neo/internal/core/acl"
	"JuanNiang-Neo/internal/core/cache"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

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
		&models.MemoryGCConfig{},
		&models.LearnerConfig{},
	)
}

// InitAdminUser 首次启动时创建管理员账户 (初始密码 Admin123)。
func InitAdminUser(ctx context.Context, userDAO *dao.UserDAO) error {
	exists, err := userDAO.Exists(ctx)
	if err != nil {
		return err
	}
	if exists {
		logging.Info("管理员用户已存在，跳过初始化")
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
	logging.Warn("已创建默认管理员用户", "username", "admin", "password", "Admin123")
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

		cacheInst := cache.NewCache(redisClient, os.Getenv("REDIS_PREFIX"))

		bundle := dao.NewBundle(db)
		aclInst := acl.NewACL(bundle.ACL)

		if err := InitAdminUser(ctx, bundle.User); err != nil {
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
