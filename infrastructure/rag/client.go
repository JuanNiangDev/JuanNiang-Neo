package rag

import (
	handler "JuanNiang-Neo/infrastructure/rag/handler"
	"fmt"
	"net/http"
	"time"
)

type basicInfo struct {
	BaseURL string
	Timeout time.Duration
}

type Option func(*basicInfo)

// WithBaseURL 设置 RAG-Service 地址。
func WithBaseURL(url string) Option {
	return func(c *basicInfo) {
		c.BaseURL = url
	}
}

// WithTimeout 设置超时时间。
func WithTimeout(d time.Duration) Option {
	return func(c *basicInfo) {
		c.Timeout = d
	}
}

// NewClient 创建 RAG 客户端。默认连接 localhost:3000。
// 创建时执行健康检查，服务不可达返回错误（调用方决定是否降级）。
func NewClient(opts ...Option) (*handler.Client, error) {
	info := basicInfo{
		BaseURL: "http://127.0.0.1:3000",
		Timeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(&info)
	}

	client := &handler.Client{
		Config: handler.Config{
			BaseURL: info.BaseURL,
			Timeout: info.Timeout,
		},
		HttpClient: &http.Client{
			Timeout: info.Timeout,
		},
	}

	if err := client.HealthCheck(); err != nil {
		return nil, fmt.Errorf("rag health check failed: %w", err)
	}
	return client, nil
}
