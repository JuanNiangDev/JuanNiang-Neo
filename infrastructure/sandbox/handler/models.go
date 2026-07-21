package caller

import (
	"fmt"
	"strings"
	"time"
)

// BayTime 兼容 Bay API 的多种时间格式（可能不带时区后缀）
type BayTime struct {
	time.Time
}

func (bt *BayTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	formats := []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.999999Z07:00",
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
	}
	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			bt.Time = t
			return nil
		}
	}
	return fmt.Errorf("cannot parse BayTime: %s", s)
}

// SandboxStatus 沙箱状态枚举
type SandboxStatus string

const (
	SandboxStatusIdle     SandboxStatus = "idle"
	SandboxStatusStarting SandboxStatus = "starting"
	SandboxStatusReady    SandboxStatus = "ready"
	SandboxStatusFailed   SandboxStatus = "failed"
	SandboxStatusExpired  SandboxStatus = "expired"
)

// ────────────────────── 请求模型 ──────────────────────

// CreateSandboxRequest 创建沙箱请求体
type CreateSandboxRequest struct {
	Profile string `json:"profile,omitempty"`
	CargoID string `json:"cargo_id,omitempty"`
	TTL     int    `json:"ttl,omitempty"` // 0 表示永不过期
}

// ExtendTTLRequest 延长 TTL 请求体
type ExtendTTLRequest struct {
	ExtendBy int `json:"extend_by"`
}

// PythonExecRequest Python 执行请求体
type PythonExecRequest struct {
	Code        string `json:"code"`
	Timeout     int    `json:"timeout,omitempty"`
	IncludeCode bool   `json:"include_code,omitempty"`
	Description string `json:"description,omitempty"`
	Tags        string `json:"tags,omitempty"`
}

// ShellExecRequest Shell 执行请求体
type ShellExecRequest struct {
	Command     string `json:"command"`
	Timeout     int    `json:"timeout,omitempty"`
	CWD         string `json:"cwd,omitempty"`
	IncludeCode bool   `json:"include_code,omitempty"`
	Description string `json:"description,omitempty"`
	Tags        string `json:"tags,omitempty"`
}

// FileWriteRequest 写入文件请求体
type FileWriteRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ────────────────────── 响应模型 ──────────────────────

// RuntimeContainerInfo 运行时容器状态
type RuntimeContainerInfo struct {
	Name         string   `json:"name"`
	RuntimeType  string   `json:"runtime_type"`
	Status       string   `json:"status"`
	Version      *string  `json:"version"`
	Capabilities []string `json:"capabilities"`
	Healthy      *bool    `json:"healthy"`
}

// SandboxInfo 沙箱信息
type SandboxInfo struct {
	ID            string                 `json:"id"`
	Status        SandboxStatus          `json:"status"`
	Profile       string                 `json:"profile"`
	CargoID       string                 `json:"cargo_id"`
	Capabilities  []string               `json:"capabilities"`
	CreatedAt     BayTime                 `json:"created_at"`
	ExpiresAt     *BayTime                `json:"expires_at"`
	IdleExpiresAt *BayTime                `json:"idle_expires_at"`
	Containers    []RuntimeContainerInfo `json:"containers,omitempty"`
}

// SandboxList 沙箱列表（带分页游标）
type SandboxList struct {
	Items      []SandboxInfo `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

// PythonExecResponse Python 执行响应
type PythonExecResponse struct {
	Success         bool                   `json:"success"`
	Output          string                 `json:"output"`
	Error           *string                `json:"error"`
	Data            map[string]interface{} `json:"data,omitempty"`
	ExecutionID     *string                `json:"execution_id"`
	ExecutionTimeMs *int                   `json:"execution_time_ms"`
	Code            *string                `json:"code"`
}

// ShellExecResponse Shell 执行响应
type ShellExecResponse struct {
	Success         bool    `json:"success"`
	Output          string  `json:"output"`
	Error           *string `json:"error"`
	ExitCode        *int    `json:"exit_code"`
	ExecutionID     *string `json:"execution_id"`
	ExecutionTimeMs *int    `json:"execution_time_ms"`
	Command         *string `json:"command"`
}

// FileReadResponse 读取文件响应
type FileReadResponse struct {
	Content string `json:"content"`
}

// FileEntry 文件/目录条目
type FileEntry struct {
	Name string `json:"name"`
	Type string `json:"type"` // "file" | "directory"
	Size *int64 `json:"size"`
}

// FileListResponse 目录列表响应
type FileListResponse struct {
	Entries []FileEntry `json:"entries"`
}

// FileUploadResponse 上传文件响应
type FileUploadResponse struct {
	Status string `json:"status"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
}

// StatusOK 通用成功响应
type StatusOK struct {
	Status string `json:"status"`
}

// ExecutionHistoryEntry 执行历史条目
type ExecutionHistoryEntry struct {
	ID              string     `json:"id"`
	SessionID       *string    `json:"session_id"`
	ExecType        string     `json:"exec_type"`
	Code            string     `json:"code"`
	Success         bool       `json:"success"`
	ExecutionTimeMs int        `json:"execution_time_ms"`
	Output          *string    `json:"output"`
	Error           *string    `json:"error"`
	PayloadRef      *string    `json:"payload_ref"`
	Description     *string    `json:"description"`
	Tags            *string    `json:"tags"`
	Notes           *string    `json:"notes"`
	CreatedAt       BayTime   `json:"created_at"`
}

// ExecutionHistoryList 执行历史列表
type ExecutionHistoryList struct {
	Entries []ExecutionHistoryEntry `json:"entries"`
	Total   int                     `json:"total"`
}
