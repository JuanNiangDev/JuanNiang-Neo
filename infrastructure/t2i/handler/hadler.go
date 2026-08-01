package caller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config T2I 客户端配置
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// Client T2I API 客户端
type Client struct {
	Config     Config
	HttpClient *http.Client
}

// ────────────────────── 内部辅助 ──────────────────────

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.Config.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	return resp, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody interface{}) (*http.Response, error) {
	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	return c.do(ctx, method, path, body)
}

func (c *Client) decodeJSON(resp *http.Response, dest interface{}) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

// ────────────────────── 健康检查 ──────────────────────

func (c *Client) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Config.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// T2I 服务无独立 /health 端点, 返回 404 属正常 (服务存活即可)。
	// 只要请求未报网络错误即视为健康。
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// ────────────────────── T2I API ──────────────────────

// Generate 生成图片, 返回图片 ID。
// 使用 GenerateAndGet 可直接获取图片二进制。
func (c *Client) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	req.AsJSON = true
	resp, err := c.doJSON(ctx, http.MethodPost, "/text2img/generate", req)
	if err != nil {
		return nil, err
	}
	var result GenerateResponse
	if err := c.decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GenerateImage 生成图片并直接返回图片二进制。
func (c *Client) GenerateImage(ctx context.Context, req GenerateRequest) ([]byte, error) {
	req.AsJSON = false
	resp, err := c.doJSON(ctx, http.MethodPost, "/text2img/generate", req)
	if err != nil {
		return nil, err
	}
	return c.readBody(resp)
}

// GenerateURL 生成图片并返回可访问的 URL。
func (c *Client) GenerateURL(ctx context.Context, req GenerateRequest) (string, error) {
	genResp, err := c.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	id := strings.TrimPrefix(genResp.Data.ID, "data/")
	return c.Config.BaseURL + "/text2img/data/" + id, nil
}

// GetImage 根据 ID 获取图片。
func (c *Client) GetImage(ctx context.Context, id string) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, "/text2img/data/"+id, nil)
	if err != nil {
		return nil, err
	}
	return c.readBody(resp)
}
