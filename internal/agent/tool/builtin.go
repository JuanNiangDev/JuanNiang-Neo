package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/provider"
	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"

	"github.com/openai/openai-go/v3"
)

// AdapterProvider 是 adapter.Provider 的接口抽象，避免循环引用。
type AdapterProvider interface {
	SendPrivateMsg(userID int64, message any) (int64, error)
	SendGroupMsg(groupID int64, message any) (int64, error)
	DeleteMsg(messageID int64) error
	GetMsg(messageID int64) (*adapter.MessageEvent, error)
	GetGroupInfo(groupID int64) (*adapter.GroupInfo, error)
	GetGroupMemberList(groupID int64) ([]adapter.GroupMemberInfo, error)
	KickGroupMember(groupID, userID int64, rejectAdd bool) error
	BanGroupMember(groupID, userID int64, duration int) error
	SetGroupWholeBan(groupID int64, enable bool) error
	SetGroupCard(groupID, userID int64, card string) error
	HandleFriendRequest(flag string, approve bool, remark string) error
	HandleGroupRequest(flag, subType string, approve bool, reason string) error
}

// ---------- 工具实现 ----------

type onebotTool struct {
	BaseTool
	executor func(ctx context.Context, args json.RawMessage) (string, error)
}

func (t *onebotTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.executor(ctx, args)
}

// RegisterBuiltinTools 注册所有内置工具到注册表。
// getSandbox / getT2I 使用 function getter 以支持运行时客户端热更新。
func RegisterBuiltinTools(
	registry *ToolRegistry,
	adapter AdapterProvider,
	getSandbox func() *sandboxcaller.Client,
	getT2I func() *t2icaller.Client,
	imageModel provider.Provider,
) {
	tools := []Tool{}

	// --- OneBot11 消息 ---

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "send_private_msg", "发送私聊消息，支持纯文本或消息段数组",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"user_id": map[string]any{"type": "integer", "description": "目标用户 QQ 号"},
					"message": map[string]any{
						"oneOf": []map[string]any{
							{"type": "string", "description": "纯文本消息"},
							{"type": "array", "items": map[string]any{"type": "object"}, "description": "富文本消息段数组"},
						},
						"description": "消息内容",
					},
				},
				"required": []string{"user_id", "message"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				UserID  int64           `json:"user_id"`
				Message json.RawMessage `json:"message"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", err
			}
			msg, err := BuildMessageFromJSON(p.Message)
			if err != nil {
				return "", fmt.Errorf("消息格式错误: %w", err)
			}
			id, err := adapter.SendPrivateMsg(p.UserID, msg)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("私聊消息已发送，message_id: %d", id), nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "send_group_msg", "发送群聊消息，支持纯文本或消息段数组",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"group_id": map[string]any{"type": "integer", "description": "目标群号"},
					"message":  map[string]any{"oneOf": []map[string]any{{"type": "string"}, {"type": "array"}}, "description": "消息内容"},
				},
				"required": []string{"group_id", "message"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				GroupID int64           `json:"group_id"`
				Message json.RawMessage `json:"message"`
			}
			json.Unmarshal(args, &p)
			msg, _ := BuildMessageFromJSON(p.Message)
			id, err := adapter.SendGroupMsg(p.GroupID, msg)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("群消息已发送，message_id: %d", id), nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "delete_msg", "撤回消息",
			Int64Param("message_id", "消息 ID", true), true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct{ MessageID int64 `json:"message_id"` }
			json.Unmarshal(args, &p)
			if err := adapter.DeleteMsg(p.MessageID); err != nil {
				return "", err
			}
			return "消息已撤回", nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "get_msg", "根据消息 ID 获取消息的完整内容（包括被引用的消息）",
			Int64Param("message_id", "消息 ID", true), true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct{ MessageID int64 `json:"message_id"` }
			json.Unmarshal(args, &p)
			msg, err := adapter.GetMsg(p.MessageID)
			if err != nil {
				return "", err
			}
			data, _ := json.Marshal(msg)
			return string(data), nil
		},
	})

	// --- OneBot11 群管理 ---

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "get_group_info", "获取群信息",
			Int64Param("group_id", "群号", true), true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct{ GroupID int64 `json:"group_id"` }
			json.Unmarshal(args, &p)
			info, err := adapter.GetGroupInfo(p.GroupID)
			if err != nil {
				return "", err
			}
			data, _ := json.Marshal(info)
			return string(data), nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "get_group_member_list", "获取群成员列表",
			Int64Param("group_id", "群号", true), true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct{ GroupID int64 `json:"group_id"` }
			json.Unmarshal(args, &p)
			list, err := adapter.GetGroupMemberList(p.GroupID)
			if err != nil {
				return "", err
			}
			data, _ := json.Marshal(list)
			return string(data), nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "kick_group_member", "踢出群成员",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"group_id":   map[string]any{"type": "integer", "description": "群号"},
					"user_id":    map[string]any{"type": "integer", "description": "要踢出的用户 QQ 号"},
					"reject_add": map[string]any{"type": "boolean", "description": "是否拒绝再次加群"},
				},
				"required": []string{"group_id", "user_id"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				GroupID   int64 `json:"group_id"`
				UserID    int64 `json:"user_id"`
				RejectAdd bool  `json:"reject_add"`
			}
			json.Unmarshal(args, &p)
			if err := adapter.KickGroupMember(p.GroupID, p.UserID, p.RejectAdd); err != nil {
				return "", err
			}
			return fmt.Sprintf("已将 %d 踢出群 %d", p.UserID, p.GroupID), nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "ban_group_member", "禁言群成员",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"group_id": map[string]any{"type": "integer", "description": "群号"},
					"user_id":  map[string]any{"type": "integer", "description": "目标用户 QQ 号"},
					"duration": map[string]any{"type": "integer", "description": "禁言时长(秒)"},
				},
				"required": []string{"group_id", "user_id", "duration"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				GroupID  int64 `json:"group_id"`
				UserID   int64 `json:"user_id"`
				Duration int   `json:"duration"`
			}
			json.Unmarshal(args, &p)
			if err := adapter.BanGroupMember(p.GroupID, p.UserID, p.Duration); err != nil {
				return "", err
			}
			return fmt.Sprintf("已将 %d 禁言 %d 秒", p.UserID, p.Duration), nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "set_group_whole_ban", "开启/关闭全员禁言",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"group_id": map[string]any{"type": "integer", "description": "群号"},
					"enable":   map[string]any{"type": "boolean", "description": "true=开启, false=关闭"},
				},
				"required": []string{"group_id", "enable"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				GroupID int64 `json:"group_id"`
				Enable  bool  `json:"enable"`
			}
			json.Unmarshal(args, &p)
			if err := adapter.SetGroupWholeBan(p.GroupID, p.Enable); err != nil {
				return "", err
			}
			return "全员禁言状态已更新", nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "set_group_card", "设置群名片(群昵称)",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"group_id": map[string]any{"type": "integer", "description": "群号"},
					"user_id":  map[string]any{"type": "integer", "description": "目标用户 QQ 号"},
					"card":     map[string]any{"type": "string", "description": "新群名片"},
				},
				"required": []string{"group_id", "user_id", "card"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				GroupID int64  `json:"group_id"`
				UserID  int64  `json:"user_id"`
				Card    string `json:"card"`
			}
			json.Unmarshal(args, &p)
			if err := adapter.SetGroupCard(p.GroupID, p.UserID, p.Card); err != nil {
				return "", err
			}
			return "群名片已更新", nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "handle_friend_request", "处理好友申请",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"flag":    map[string]any{"type": "string", "description": "请求 flag"},
					"approve": map[string]any{"type": "boolean", "description": "是否同意"},
					"remark":  map[string]any{"type": "string", "description": "好友备注"},
				},
				"required": []string{"flag", "approve"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Flag    string `json:"flag"`
				Approve bool   `json:"approve"`
				Remark  string `json:"remark"`
			}
			json.Unmarshal(args, &p)
			if err := adapter.HandleFriendRequest(p.Flag, p.Approve, p.Remark); err != nil {
				return "", err
			}
			return "好友申请已处理", nil
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "handle_group_request", "处理群请求(加群/邀请)",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"flag":     map[string]any{"type": "string", "description": "请求 flag"},
					"sub_type": map[string]any{"type": "string", "description": "add=加群, invite=邀请入群"},
					"approve":  map[string]any{"type": "boolean", "description": "是否同意"},
					"reason":   map[string]any{"type": "string", "description": "原因"},
				},
				"required": []string{"flag", "sub_type", "approve"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Flag    string `json:"flag"`
				SubType string `json:"sub_type"`
				Approve bool   `json:"approve"`
				Reason  string `json:"reason"`
			}
			json.Unmarshal(args, &p)
			if err := adapter.HandleGroupRequest(p.Flag, p.SubType, p.Approve, p.Reason); err != nil {
				return "", err
			}
			return "群请求已处理", nil
		},
	})

	// --- 时间 ---

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "get_time", "获取当前日期和时间",
			TimeParams(), true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			return time.Now().Format("2006-01-02 15:04:05 Monday"), nil
		},
	})

	// --- 沙箱管理工具 (非长耗时) ---

	if getSandbox != nil {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "create_sandbox", "创建一个新的沙箱实例，返回 sandbox_id 用于后续命令执行等操作",
				openai.FunctionParameters{
					"type":       "object",
					"properties": map[string]any{},
				}, true, false),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				sandbox := getSandbox()
				if sandbox == nil {
					return "", fmt.Errorf("沙箱服务未启用")
				}
				sbox, err := sandbox.CreateSandbox(ctx, sandboxcaller.CreateSandboxRequest{})
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("沙箱创建成功，sandbox_id: %s, status: %s", sbox.ID, sbox.Status), nil
			},
		})

		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "list_sandboxes", "列出已有的沙箱实例",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"status": map[string]any{"type": "string", "description": "按状态筛选(可选): running/stopped"},
					},
				}, true, false),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				sandbox := getSandbox()
				if sandbox == nil {
					return "", fmt.Errorf("沙箱服务未启用")
				}
				var p struct{ Status string `json:"status"` }
				json.Unmarshal(args, &p)
				list, err := sandbox.ListSandboxes(ctx, 20, "", p.Status)
				if err != nil {
					return "", err
				}
				data, _ := json.Marshal(list)
				return string(data), nil
			},
		})
	}

	// --- 沙箱工具 (长耗时, 需要 sandbox_id) ---

	if getSandbox != nil {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "browser_search", "在沙箱中执行浏览器搜索(长耗时)",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"sandbox_id": map[string]any{"type": "string", "description": "沙箱 ID"},
						"query":      map[string]any{"type": "string", "description": "搜索关键词"},
					},
					"required": []string{"sandbox_id", "query"},
				}, true, true),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				sandbox := getSandbox()
				if sandbox == nil {
					return "", fmt.Errorf("沙箱服务未启用")
				}
				var p struct {
					SandboxID string `json:"sandbox_id"`
					Query     string `json:"query"`
				}
				json.Unmarshal(args, &p)
				result, err := sandbox.ExecShell(ctx, p.SandboxID, sandboxcaller.ShellExecRequest{
					Command: fmt.Sprintf("search '%s'", p.Query),
				})
				if err != nil {
					return "", err
				}
				data, _ := json.Marshal(result)
				return string(data), nil
			},
		})

		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "command_exec", "在沙箱中执行系统命令(长耗时)",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"sandbox_id": map[string]any{"type": "string", "description": "沙箱 ID"},
						"command":    map[string]any{"type": "string", "description": "要执行的命令"},
					},
					"required": []string{"sandbox_id", "command"},
				}, true, true),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				sandbox := getSandbox()
				if sandbox == nil {
					return "", fmt.Errorf("沙箱服务未启用")
				}
				var p struct {
					SandboxID string `json:"sandbox_id"`
					Command   string `json:"command"`
				}
				json.Unmarshal(args, &p)
				result, err := sandbox.ExecShell(ctx, p.SandboxID, sandboxcaller.ShellExecRequest{
					Command: p.Command,
				})
				if err != nil {
					return "", err
				}
				data, _ := json.Marshal(result)
				return string(data), nil
			},
		})

		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "code_exec", "在沙箱中执行 Python 代码(长耗时)",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"sandbox_id": map[string]any{"type": "string", "description": "沙箱 ID"},
						"code":       map[string]any{"type": "string", "description": "Python 代码"},
					},
					"required": []string{"sandbox_id", "code"},
				}, true, true),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				sandbox := getSandbox()
				if sandbox == nil {
					return "", fmt.Errorf("沙箱服务未启用")
				}
				var p struct {
					SandboxID string `json:"sandbox_id"`
					Code      string `json:"code"`
				}
				json.Unmarshal(args, &p)
				result, err := sandbox.ExecPython(ctx, p.SandboxID, sandboxcaller.PythonExecRequest{
					Code: p.Code,
				})
				if err != nil {
					return "", err
				}
				data, _ := json.Marshal(result)
				return string(data), nil
			},
		})
	}

	// --- 文生图 (长耗时) ---

	if getT2I != nil {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "text_to_image", "根据 HTML/模板生成图片(长耗时)",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"html": map[string]any{"type": "string", "description": "HTML 内容"},
					},
					"required": []string{"html"},
				}, true, true),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				t2i := getT2I()
				if t2i == nil {
					return "", fmt.Errorf("T2I 服务未启用")
				}
				var p struct{ HTML string `json:"html"` }
				json.Unmarshal(args, &p)
				url, err := t2i.GenerateURL(ctx, t2icaller.GenerateRequest{HTML: p.HTML})
				if err != nil {
					return "", fmt.Errorf("T2I 生成失败: %w", err)
				}
				// 返回 CQ 图片消息码，OneBot11 协议端会自动解析为图片段
				return fmt.Sprintf("[CQ:image,file=%s]", url), nil
			},
		})
	}

	// --- Vision / 识图 ---

	if imageModel != nil {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "vision", "使用识图模型识别图片内容",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"image_url": map[string]any{"type": "string", "description": "图片 URL"},
						"prompt":    map[string]any{"type": "string", "description": "关于图片的问题"},
					},
					"required": []string{"image_url", "prompt"},
				}, true, false),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				return "识图功能需要图片字节数据，请通过其他方式获取图片后调用。", nil
			},
		})
	} else {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "vision", "识别图片内容",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"image_url": map[string]any{"type": "string", "description": "图片 URL"},
						"prompt":    map[string]any{"type": "string", "description": "关于图片的问题"},
					},
					"required": []string{"image_url", "prompt"},
				}, true, false),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				return "未配置识图模型(Image Model)，无法识别图片。请联系管理员配置。", nil
			},
		})
	}

	for _, t := range tools {
		registry.Register(t)
	}
}
