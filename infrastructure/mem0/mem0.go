// Package mem0 提供 Mem0 自部署服务的客户端封装。
// Mem0 (https://github.com/mem0ai/mem0) 作为向量记忆底座，
// 提供自动 embedding + 向量存储 + 语义搜索能力。
package mem0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"JuanNiang-Neo/internal/logging"
)

var log = logging.NewLogger("mem0")

// Client Mem0 HTTP 客户端。
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Memory 一条 Mem0 记忆。
type Memory struct {
	ID        string         `json:"id,omitempty"`
	UserID    string         `json:"user_id"`
	AgentID   string         `json:"agent_id"`
	Memory    string         `json:"memory"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
}

// SearchResult 搜索结果。
type SearchResult struct {
	ID       string         `json:"id"`
	Memory   string         `json:"memory"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SearchFilter 搜索过滤条件。
type SearchFilter struct {
	UserID  string `json:"user_id,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
}

// Option 功能选项。
type Option func(*Client)

// WithBaseURL 设置 Mem0 服务地址。
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// WithAPIKey 设置 API Key。
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// WithTimeout 设置 HTTP 超时。
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// NewClient 创建 Mem0 客户端。
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL: "http://localhost:8080",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Add 添加一条记忆。
func (c *Client) Add(m Memory) (*Memory, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("mem0 add marshal: %w", err)
	}

	resp, err := c.doRequest("POST", "/v1/memories/", body)
	if err != nil {
		return nil, err
	}

	var result Memory
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("mem0 add unmarshal: %w", err)
	}
	return &result, nil
}

// Search 语义搜索记忆。
func (c *Client) Search(query string, filter SearchFilter, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	req := map[string]any{
		"query": query,
		"limit": limit,
	}
	if filter.UserID != "" {
		req["user_id"] = filter.UserID
	}
	if filter.AgentID != "" {
		req["agent_id"] = filter.AgentID
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mem0 search marshal: %w", err)
	}

	resp, err := c.doRequest("POST", "/v1/memories/search/", body)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	if err := json.Unmarshal(resp, &results); err != nil {
		return nil, fmt.Errorf("mem0 search unmarshal: %w", err)
	}
	return results, nil
}

// Delete 删除一条记忆。
func (c *Client) Delete(memoryID string) error {
	_, err := c.doRequest("DELETE", "/v1/memories/"+memoryID+"/", nil)
	return err
}

// Update 更新记忆。
func (c *Client) Update(memoryID string, m Memory) error {
	body, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("mem0 update marshal: %w", err)
	}
	_, err = c.doRequest("PUT", "/v1/memories/"+memoryID+"/", body)
	return err
}

// Health 健康检查。
func (c *Client) Health() error {
	_, err := c.doRequest("GET", "/health/", nil)
	return err
}

func (c *Client) doRequest(method, path string, body []byte) ([]byte, error) {
	url := c.baseURL + path
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("mem0 request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mem0 %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mem0 read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mem0 %s %s: %d %s", method, path, resp.StatusCode, string(respBody))
	}

	log.Debug("Mem0 请求完成", "method", method, "path", path, "status", resp.StatusCode)
	return respBody, nil
}
