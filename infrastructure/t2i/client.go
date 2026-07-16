package t2i

import (
	handler "JuanNiang-Neo/infrastructure/t2i/handler"
	"fmt"
	"net/http"
	"time"
)

type basicInfo struct {
	BaseURL string
	Timeout time.Duration
}

type Option func(*basicInfo)

// WithBaseURL 设置服务地址。
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

// NewClient 创建 T2I 客户端。默认连接 localhost:8999。
func NewClient(opts ...Option) (*handler.Client, error) {
	info := basicInfo{
		BaseURL: "http://localhost:8999",
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
		return nil, fmt.Errorf("t2i health check failed: %w", err)
	}
	return client, nil
}
