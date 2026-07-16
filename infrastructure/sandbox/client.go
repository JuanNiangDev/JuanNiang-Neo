package sandbox

import (
	"JuanNiang-Neo/infrastructure/sandbox/caller"
	"fmt"
	"net/http"
	"time"
)

type BasicInfo struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

type Option func(*BasicInfo)

// WithBaseURL 设置服务地址
func WithBaseURL(url string) Option {
	return func(c *BasicInfo) {
		c.BaseURL = url
	}
}

// WithAPIKey 设置 API 密钥
func WithAPIKey(key string) Option {
	return func(c *BasicInfo) {
		c.APIKey = key
	}
}

// WithTimeout 设置超时时间
func WithTimeout(d time.Duration) Option {
	return func(c *BasicInfo) {
		c.Timeout = d
	}
}

func NewClient(opts ...Option) (*caller.Client, error) {
	basicInfo := BasicInfo{
		BaseURL: "http://localhost:8080",
		Timeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(&basicInfo)
	}

	client := &caller.Client{
		Config: caller.Config{
			BaseURL: basicInfo.BaseURL,
			APIKey:  basicInfo.APIKey,
			Timeout: basicInfo.Timeout,
		},
		HttpClient: &http.Client{
			Timeout: basicInfo.Timeout,
		},
	}

	if err := client.HealthCheck(); err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	return client, nil
}
