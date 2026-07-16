package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Option func(info *BasicInfo)

type BasicInfo struct {
	Addr     string
	Password string
	DB       int
}

func WithAddr(addr string) Option {
	return func(info *BasicInfo) {
		info.Addr = addr
	}
}

func WithPassword(password string) Option {
	return func(info *BasicInfo) {
		info.Password = password
	}
}

func WithDB(db int) Option {
	return func(info *BasicInfo) {
		info.DB = db
	}
}

func NewRedisSentinelClient(opts ...Option) (*redis.Client, error) {
	// 构建基础选项
	basicInfo := &BasicInfo{
		Addr:     "localhost:6379",
		Password: "root",
		DB:       0,
	}
	// 遍历应用
	for _, opt := range opts {
		opt(basicInfo)
	}
	// 创建NewFailoverClient
	client := redis.NewClient(&redis.Options{Addr: basicInfo.Addr})
	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return client, nil
}
