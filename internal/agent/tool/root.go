package tool

import (
	"encoding/json"
	"fmt"

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
}

func (b BaseTool) ID() string                            { return b.id }
func (b BaseTool) Name() string                          { return b.name }
func (b BaseTool) Description() string                   { return b.description }
func (b BaseTool) Parameters() openai.FunctionParameters { return b.parameters }
func (b BaseTool) IsBuiltin() bool                       { return b.builtin }
func (b BaseTool) IsLongRunning() bool                   { return b.longRunning }

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
