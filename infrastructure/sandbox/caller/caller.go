package caller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"
)

// Config Bay 客户端配置
type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// Client Bay API 客户端
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
	if c.Config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.Config.APIKey)
	}

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

// ────────────────────── 健康检查 ──────────────────────

// HealthCheck 检测 Bay 服务是否可用
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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// ────────────────────── 沙箱生命周期 ──────────────────────

// CreateSandbox 创建一个新的沙箱实例
func (c *Client) CreateSandbox(ctx context.Context, req CreateSandboxRequest) (*SandboxInfo, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/sandboxes", req)
	if err != nil {
		return nil, err
	}
	var info SandboxInfo
	if err := c.decodeJSON(resp, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GetSandbox 获取沙箱详情
func (c *Client) GetSandbox(ctx context.Context, sandboxID string) (*SandboxInfo, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/sandboxes/"+sandboxID, nil)
	if err != nil {
		return nil, err
	}
	var info SandboxInfo
	if err := c.decodeJSON(resp, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ListSandboxes 列出沙箱
func (c *Client) ListSandboxes(ctx context.Context, limit int, cursor string, status string) (*SandboxList, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if status != "" {
		params.Set("status", status)
	}

	path := "/v1/sandboxes"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var list SandboxList
	if err := c.decodeJSON(resp, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// ExtendTTL 延长沙箱 TTL
func (c *Client) ExtendTTL(ctx context.Context, sandboxID string, extendBy int) (*SandboxInfo, error) {
	req := ExtendTTLRequest{ExtendBy: extendBy}
	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/sandboxes/"+sandboxID+"/extend_ttl", req)
	if err != nil {
		return nil, err
	}
	var info SandboxInfo
	if err := c.decodeJSON(resp, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// KeepAlive 保活——重置空闲超时计时器
func (c *Client) KeepAlive(ctx context.Context, sandboxID string) error {
	resp, err := c.do(ctx, http.MethodPost, "/v1/sandboxes/"+sandboxID+"/keepalive", nil)
	if err != nil {
		return err
	}
	var ok StatusOK
	return c.decodeJSON(resp, &ok)
}

// StopSandbox 停止沙箱（回收计算资源，保留存储）
func (c *Client) StopSandbox(ctx context.Context, sandboxID string) error {
	resp, err := c.do(ctx, http.MethodPost, "/v1/sandboxes/"+sandboxID+"/stop", nil)
	if err != nil {
		return err
	}
	var ok StatusOK
	return c.decodeJSON(resp, &ok)
}

// DeleteSandbox 永久删除沙箱
func (c *Client) DeleteSandbox(ctx context.Context, sandboxID string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/v1/sandboxes/"+sandboxID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

// ────────────────────── Python 执行 ──────────────────────

// ExecPython 在沙箱中执行 Python 代码
func (c *Client) ExecPython(ctx context.Context, sandboxID string, req PythonExecRequest) (*PythonExecResponse, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/sandboxes/"+sandboxID+"/python/exec", req)
	if err != nil {
		return nil, err
	}
	var result PythonExecResponse
	if err := c.decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ────────────────────── Shell 执行 ──────────────────────

// ExecShell 在沙箱中执行 Shell 命令
func (c *Client) ExecShell(ctx context.Context, sandboxID string, req ShellExecRequest) (*ShellExecResponse, error) {
	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/sandboxes/"+sandboxID+"/shell/exec", req)
	if err != nil {
		return nil, err
	}
	var result ShellExecResponse
	if err := c.decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ────────────────────── 文件系统 ──────────────────────

// ReadFile 读取沙箱中的文件
func (c *Client) ReadFile(ctx context.Context, sandboxID string, filePath string) (*FileReadResponse, error) {
	path := "/v1/sandboxes/" + sandboxID + "/filesystem/files?path=" + url.QueryEscape(filePath)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var result FileReadResponse
	if err := c.decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WriteFile 写入文件到沙箱
func (c *Client) WriteFile(ctx context.Context, sandboxID string, req FileWriteRequest) error {
	resp, err := c.doJSON(ctx, http.MethodPut, "/v1/sandboxes/"+sandboxID+"/filesystem/files", req)
	if err != nil {
		return err
	}
	var ok StatusOK
	return c.decodeJSON(resp, &ok)
}

// ListDirectory 列出沙箱中的目录
func (c *Client) ListDirectory(ctx context.Context, sandboxID string, dirPath string) (*FileListResponse, error) {
	queryPath := "."
	if dirPath != "" {
		queryPath = dirPath
	}
	path := "/v1/sandboxes/" + sandboxID + "/filesystem/directories?path=" + url.QueryEscape(queryPath)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var result FileListResponse
	if err := c.decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteFile 删除沙箱中的文件或目录
func (c *Client) DeleteFile(ctx context.Context, sandboxID string, filePath string) error {
	path := "/v1/sandboxes/" + sandboxID + "/filesystem/files?path=" + url.QueryEscape(filePath)
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	var ok StatusOK
	return c.decodeJSON(resp, &ok)
}

// UploadFile 上传文件到沙箱
func (c *Client) UploadFile(ctx context.Context, sandboxID string, destPath string, fileName string, fileData io.Reader) (*FileUploadResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// form field: path
	if err := writer.WriteField("path", destPath); err != nil {
		return nil, fmt.Errorf("write path field: %w", err)
	}

	// form field: file
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, fileData); err != nil {
		return nil, fmt.Errorf("copy file data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Config.BaseURL+"/v1/sandboxes/"+sandboxID+"/filesystem/upload", body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.Config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.Config.APIKey)
	}

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	var result FileUploadResponse
	if err := c.decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DownloadFile 从沙箱下载文件，返回文件内容字节
func (c *Client) DownloadFile(ctx context.Context, sandboxID string, filePath string) ([]byte, error) {
	path := "/v1/sandboxes/" + sandboxID + "/filesystem/download?path=" + url.QueryEscape(filePath)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}

	return io.ReadAll(resp.Body)
}

// ────────────────────── 执行历史 ──────────────────────

// GetExecutionHistory 获取沙箱的执行历史
func (c *Client) GetExecutionHistory(ctx context.Context, sandboxID string, limit, offset int, execType string) (*ExecutionHistoryList, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	if execType != "" {
		params.Set("exec_type", execType)
	}

	path := "/v1/sandboxes/" + sandboxID + "/history"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var list ExecutionHistoryList
	if err := c.decodeJSON(resp, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// GetExecution 获取单条执行历史记录
func (c *Client) GetExecution(ctx context.Context, sandboxID string, executionID string) (*ExecutionHistoryEntry, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/sandboxes/"+sandboxID+"/history/"+executionID, nil)
	if err != nil {
		return nil, err
	}
	var entry ExecutionHistoryEntry
	if err := c.decodeJSON(resp, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// GetLastExecution 获取最近一条执行历史记录
func (c *Client) GetLastExecution(ctx context.Context, sandboxID string, execType string) (*ExecutionHistoryEntry, error) {
	path := "/v1/sandboxes/" + sandboxID + "/history/last"
	if execType != "" {
		path += "?exec_type=" + url.QueryEscape(execType)
	}

	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var entry ExecutionHistoryEntry
	if err := c.decodeJSON(resp, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}
