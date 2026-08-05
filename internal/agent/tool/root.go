package tool

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"JuanNiang-Neo/internal/adapter"

	"github.com/openai/openai-go/v3"
)

// NewToolConfig 的类型定义保留在此，供外部使用。

type ToolConfig struct {
	ID          string
	Name        string
	Description string
	Parameters  openai.FunctionParameters
	Executor    func(args json.RawMessage) (string, error)
	LongRunning bool
}

// BaseTool 提供工具的基础实现。
type BaseTool struct {
	id          string
	name        string
	description string
	parameters  openai.FunctionParameters
	builtin     bool
	longRunning bool

	// available 服务可用性回调（如 T2I/Sandbox 客户端 getter）；
	// nil=始终可用；返回 false 时工具不会注册进 Eino 工具列表（自动卸载）。
	available func() bool
}

func (b BaseTool) ID() string                            { return b.id }
func (b BaseTool) Name() string                          { return b.name }
func (b BaseTool) Description() string                   { return b.description }
func (b BaseTool) Parameters() openai.FunctionParameters { return b.parameters }
func (b BaseTool) IsBuiltin() bool                       { return b.builtin }
func (b BaseTool) IsLongRunning() bool                   { return b.longRunning }

// SetAvailable 设置工具可用性回调（服务未启用/未配置时返回 false，
// 工具会被 BuildEinoTools 过滤，不再出现在 Eino Agent 的工具列表中）。
func (b *BaseTool) SetAvailable(fn func() bool) {
	b.available = fn
}

// IsAvailable 判断工具当前是否可用。
func (b BaseTool) IsAvailable() bool {
	if b.available == nil {
		return true
	}
	return b.available()
}

// ---------- 便利构造 ----------

func NewTool(id, name, desc string, params openai.FunctionParameters, builtin bool, longRunning bool) BaseTool {
	return BaseTool{
		id:          id,
		name:        name,
		description: desc,
		parameters:  params,
		builtin:     builtin,
		longRunning: longRunning,
	}
}

// FlexInt64 兼容 JSON number 与 string 形式的 int64 字段。
// LLM 经常把数字字段（QQ 号、群号等）输出为字符串，使用该类型可避免整个参数解析失败。
type FlexInt64 int64

// UnmarshalJSON 同时接受 JSON number 与字符串形式的数字。
func (f *FlexInt64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = FlexInt64(v)
	return nil
}

// StringParam 便捷的字符串参数定义。
func StringParam(name, desc string, required bool) openai.FunctionParameters {
	return openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			name: map[string]any{
				"type":        "string",
				"description": desc,
			},
		},
		"required": requiredStrings(required, name),
	}
}

// Int64Param 便捷的 int64 参数定义。
func Int64Param(name, desc string, required bool) openai.FunctionParameters {
	return openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			name: map[string]any{
				"type":        "integer",
				"description": desc,
			},
		},
		"required": requiredStrings(required, name),
	}
}

// MessageParam 消息参数 (支持富文本: string 或 []Segment)。
func MessageParam() openai.FunctionParameters {
	return openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"oneOf": []map[string]any{
					{"type": "string", "description": "纯文本消息"},
					{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"type": map[string]any{"type": "string", "description": "消息段类型: text/image/at/face/reply/record/video"},
								"data": map[string]any{"type": "object", "description": "消息段数据"},
							},
						},
						"description": "富文本消息段数组",
					},
				},
				"description": "消息内容，支持纯文本或消息段数组",
			},
		},
		"required": []string{"message"},
	}
}

// GroupIDUserIDParams 群/用户 ID 参数。
func GroupIDUserIDParams() openai.FunctionParameters {
	return openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"user_id": map[string]any{
				"type":        "integer",
				"description": "用户 QQ 号",
			},
			"group_id": map[string]any{
				"type":        "integer",
				"description": "群号",
			},
		},
		"required": []string{"user_id", "group_id"},
	}
}

// TimeParams 时间参数。
func TimeParams() openai.FunctionParameters {
	return openai.FunctionParameters{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func requiredStrings(required bool, name string) []string {
	if required {
		return []string{name}
	}
	return nil
}

// BuildMessageFromJSON 解析 JSON 消息段数组并返回 adapter 可用的消息片段。
// 支持两种格式：
//   - 纯字符串: "hello" → string（含 CQ 码时由 normalizeMessage 解析）
//   - 段数组: [{"type":"text","data":{"text":"hi"}},{"type":"image","data":{"file":"url"}}] → []Segment
func BuildMessageFromJSON(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("空消息")
	}

	// 尝试解析为字符串
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}

	// 解析为消息段数组 → 转换为 []Segment
	var rawSegments []map[string]any
	if err := json.Unmarshal(raw, &rawSegments); err != nil {
		return nil, fmt.Errorf("无效的消息格式: %w", err)
	}

	segments := make([]adapter.Segment, 0, len(rawSegments))
	for _, rs := range rawSegments {
		segType, _ := rs["type"].(string)
		if segType == "" {
			continue
		}
		data := make(map[string]any)
		if d, ok := rs["data"]; ok {
			if dm, ok := d.(map[string]any); ok {
				data = dm
			}
		}
		segments = append(segments, adapter.Segment{Type: segType, Data: data})
	}

	if len(segments) == 0 {
		return nil, fmt.Errorf("消息段数组为空")
	}
	return segments, nil
}

// BuildMessageLoose 解析消息内容：标准 JSON（字符串/消息段数组）解析失败时回退为原始文本。
// 用于容错 LLM 把消息内容直接当作工具参数传入（如 "[CQ:image,file=...] 文字"）的情况。
func BuildMessageLoose(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("消息内容为空")
	}
	msg, err := BuildMessageFromJSON(raw)
	if err != nil {
		// 非标准 JSON（如裸 CQ 码文本）：直接作为消息内容
		msg = string(raw)
	}
	if s, ok := msg.(string); ok && strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("消息内容为空")
	}
	return msg, nil
}
