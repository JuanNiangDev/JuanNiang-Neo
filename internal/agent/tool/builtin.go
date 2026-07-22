package tool

import (
	"context"
	"encoding/base64"
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

// SessionInfo 当前会话信息（供工具使用）。
type SessionInfo struct {
	MessageType string `json:"message_type"` // private / group
	TargetID    int64  `json:"target_id"`    // user_id (私聊) / group_id (群聊)
	SenderQQ    int64  `json:"sender_qq"`
	SenderName  string `json:"sender_name"` // 昵称或群名片
	SenderRole  string `json:"sender_role"` // owner / admin / member (群聊); 私聊为空
	SelfQQ      int64  `json:"self_qq"`
	SelfName    string `json:"self_name"` // 机器人昵称
	Admins      string `json:"admins"`   // 管理员 QQ 列表
}

// RegisterBuiltinTools 注册所有内置工具到注册表。
// getSandbox / getT2I 使用 function getter 以支持运行时客户端热更新。
func RegisterBuiltinTools(
	registry *ToolRegistry,
	adapter AdapterProvider,
	getSandbox func() *sandboxcaller.Client,
	getT2I func() *t2icaller.Client,
	imageModel provider.Provider,
	getSessionCtx func() string,
	getCurrentMsg func() *adapter.MessageEvent,
	getRecentMsgs func(ctx context.Context, msgType string, targetID int64, limit int) ([]string, error),
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
				// 使用 Python requests + Bing 搜索
				encodedQuery := fmt.Sprintf("%q", p.Query)
				code := fmt.Sprintf(`import json, re, urllib.request, urllib.parse
query = %s
try:
    url = "https://www.bing.com/search?q=" + urllib.parse.quote(query) + "&setlang=zh-cn"
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"})
    resp = urllib.request.urlopen(req, timeout=15)
    html = resp.read().decode("utf-8", errors="ignore")
    results = []
    # Bing 搜索结果: <li class="b_algo"> 包含 <h2><a href="...">title</a></h2>
    blocks = re.findall(r'<li class="b_algo"[^>]*>(.*?)</li>', html, re.DOTALL)
    for block in blocks:
        m = re.search(r'<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>', block, re.DOTALL)
        if m:
            title = re.sub(r'<[^>]+>', '', m.group(2)).strip()
            url = m.group(1)
            if title and url.startswith("http"):
                results.append({"title": title, "url": url})
        if len(results) >= 10:
            break
    if not results:
        results = [{"title": "无结果", "url": ""}]
    print(json.dumps({"success": True, "results": results}, ensure_ascii=False))
except Exception as e:
    print(json.dumps({"success": False, "error": str(e)}))
`, encodedQuery)
				result, err := sandbox.ExecPython(ctx, p.SandboxID, sandboxcaller.PythonExecRequest{
					Code: code,
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
				imgBytes, err := t2i.GenerateImage(ctx, t2icaller.GenerateRequest{
					HTML: p.HTML,
					Options: &t2icaller.GenerateOptions{
						Type:    t2icaller.ImageTypeJPEG,
						Quality: 80,
					},
				})
				if err != nil {
					return "", fmt.Errorf("T2I 生成失败: %w", err)
				}
				// base64 内嵌图片，避免 QQ 协议端无法访问 T2I 内部地址
				b64 := base64.StdEncoding.EncodeToString(imgBytes)
				return fmt.Sprintf("[CQ:image,file=base64://%s]", b64), nil
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

	// --- 会话信息 ---

	if getSessionCtx != nil {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "get_session_info", "获取当前聊天会话信息（私聊/群聊、对方QQ/群号、发送者信息、机器人身份等）",
				TimeParams(), true, false),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				return getSessionCtx(), nil
			},
		})
	}

	// --- QQ 超级表情 ---

	if getCurrentMsg != nil {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "send_face", "发送 QQ 超级表情到当前会话。⚠️ 必须先调用 list_super_faces 查询表情 ID，再传入正确的 face_id！不要自己编造 ID。注意：这是 QQ 内置表情系统，非图片。",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"face_id": map[string]any{"type": "integer", "description": "QQ 表情 ID。必须先调用 list_super_faces 查询！常见经典小黄脸参考: 0=惊讶, 1=撇嘴, 2=色, 3=发呆, 4=得意, 5=流泪, 6=害羞, 7=闭嘴, 10=发怒, 14=微笑, 18=可爱, 21=疑问, 22=无语, 28=再见, 37=呲牙, 39=偷笑, 55=流汗, 63=委屈, 66=坏笑, 74=可怜, 76=酷, 89=尴尬, 97=大笑, 111=爱心, 142=抱拳, 182=耶, 188=狗头, 201=点赞, 211=笑哭, 277=鲜花"},
					},
					"required": []string{"face_id"},
				}, true, false),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					FaceID int `json:"face_id"`
				}
				json.Unmarshal(args, &p)
				msg := getCurrentMsg()
				if msg == nil {
					return "未获取到当前会话信息", nil
				}
				cqCode := fmt.Sprintf("[CQ:face,id=%d]", p.FaceID)
				switch msg.MessageType {
				case "private":
					if _, err := adapter.SendPrivateMsg(msg.UserID, cqCode); err != nil {
						return "", err
					}
				case "group":
					if _, err := adapter.SendGroupMsg(msg.GroupID, cqCode); err != nil {
						return "", err
					}
				}
				return fmt.Sprintf("QQ 表情 (face_id=%d) 已发送", p.FaceID), nil
			},
		})

		// list_super_faces
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "list_super_faces", "查询所有可用的 QQ 超级表情（sub_type=3 动态表情 + sub_type=5 手势表情）。返回 ID 和名称列表。⚠️ 使用 send_face 前必须先调用此工具查询正确的 face_id！",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{},
				}, true, false),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				type faceEntry struct {
					ID     int    `json:"id"`
					Name   string `json:"name"`
					SubType int   `json:"sub_type"`
				}
				// sub_type=3 (动态超级表情): 98条
				sub3 := []faceEntry{
					{5, "流泪", 3}, {53, "蛋糕", 3}, {114, "篮球", 3}, {181, "戳一戳", 3},
					{311, "打call", 3}, {312, "变形", 3}, {314, "仔细分析", 3}, {317, "菜汪", 3},
					{318, "崇拜", 3}, {319, "比心", 3}, {320, "庆祝", 3}, {324, "吃糖", 3},
					{325, "惊吓", 3}, {337, "花朵脸", 3}, {338, "我想开了", 3}, {339, "舔屏", 3},
					{341, "打招呼", 3}, {342, "酸Q", 3}, {343, "我方了", 3}, {344, "大怨种", 3},
					{346, "你真棒棒", 3}, {349, "坚强", 3}, {350, "贴贴", 3}, {351, "敲敲", 3},
					{360, "亲亲", 3}, {361, "狗狗笑哭", 3}, {362, "好兄弟", 3}, {363, "狗狗可怜", 3},
					{364, "超级赞", 3}, {365, "狗狗生气", 3}, {366, "芒狗", 3}, {367, "狗狗疑问", 3},
					{368, "奥特笑哭", 3}, {369, "彩虹", 3}, {370, "祝贺", 3}, {371, "冒泡", 3},
					{372, "气呼呼", 3}, {373, "忙", 3}, {374, "波波流泪", 3}, {375, "超级鼓掌", 3},
					{376, "跺脚", 3}, {377, "嗨", 3}, {378, "企鹅笑哭", 3}, {379, "企鹅流泪", 3},
					{380, "真棒", 3}, {381, "路过", 3}, {382, "emo", 3}, {383, "企鹅爱心", 3},
					{384, "晚安", 3}, {385, "太气了", 3}, {386, "呜呜呜", 3}, {387, "太好笑", 3},
					{388, "太头疼", 3}, {389, "太赞了", 3}, {390, "太头秃", 3}, {391, "太沧桑", 3},
					{395, "略略略", 3}, {396, "狼狗", 3}, {397, "抛媚眼", 3}, {398, "超级ok", 3},
					{399, "tui", 3}, {400, "快乐", 3}, {401, "超级转圈", 3}, {402, "别说话", 3},
					{403, "出去玩", 3}, {404, "闪亮登场", 3}, {405, "好运来", 3}, {406, "姐是女王", 3},
					{407, "我听听", 3}, {408, "臭美", 3}, {409, "送你花花", 3}, {410, "么么哒", 3},
					{411, "一起嗨", 3}, {412, "开心", 3}, {413, "摇起来", 3}, {424, "续标识", 3},
					{425, "求放过", 3}, {426, "玩火", 3}, {427, "偷感", 3}, {450, "撇嘴", 3},
					{451, "色", 3}, {452, "微笑", 3}, {453, "发呆", 3}, {454, "得意", 3},
					{455, "害羞", 3}, {456, "闭嘴", 3}, {457, "睡", 3}, {458, "我吗", 3},
					{459, "优雅", 3}, {460, "硬撑", 3}, {461, "宕机", 3}, {462, "无语", 3},
					{463, "新年快乐", 3}, {464, "马上到", 3}, {465, "拆红包", 3}, {466, "羞羞哒", 3},
					{467, "摇花手", 3}, {468, "失眠", 3}, {469, "坚毅", 3}, {472, "心动", 3},
					{474, "给你一拳", 3}, {475, "干饭", 3}, {476, "不是吧", 3}, {477, "你懂的", 3},
					{478, "对的对的", 3}, {479, "不对不对", 3}, {480, "散味儿", 3}, {481, "学习", 3},
					{482, "热化了", 3}, {483, "略", 3}, {484, "比爱心", 3},
				}
				// sub_type=5 (手势表情): 2种
				sub5 := []faceEntry{
					{2, "比心", 5}, {4, "比心_心碎", 5},
				}

				var b strings.Builder
				b.WriteString(fmt.Sprintf("共 %d 个超级表情，按 sub_type 分组:\n", len(sub3)+len(sub5)))
				b.WriteString("\n=== sub_type=3 动态超级表情 ===\n")
				for _, e := range sub3 {
					b.WriteString(fmt.Sprintf("  ID=%d  %s\n", e.ID, e.Name))
				}
				b.WriteString("\n=== sub_type=5 手势表情 ===\n")
				for _, e := range sub5 {
					b.WriteString(fmt.Sprintf("  ID=%d  %s\n", e.ID, e.Name))
				}

				return b.String(), nil
			},
		})
	}

	// --- 消息查询 ---

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "get_recent_messages", "获取当前会话中往上 N 条消息。用于了解上下文，判断群聊中是否有人@你或提到你。",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{"type": "integer", "description": "获取的消息数量，默认 10"},
				},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Limit int `json:"limit"`
			}
			json.Unmarshal(args, &p)
			if p.Limit <= 0 {
				p.Limit = 10
			}
			if p.Limit > 50 {
				p.Limit = 50
			}
			msg := getCurrentMsg()
			if msg == nil {
				return "未获取到当前会话信息", nil
			}
			targetID := int64(0)
			if msg.MessageType == "private" {
				targetID = msg.UserID
			} else {
				targetID = msg.GroupID
			}
			msgs, err := getRecentMsgs(ctx, msg.MessageType, targetID, p.Limit)
			if err != nil {
				return "", err
			}
			if len(msgs) == 0 {
				return "暂无更早的消息记录", nil
			}
			result := fmt.Sprintf("最近 %d 条消息:\n", len(msgs))
			for i, m := range msgs {
				result += fmt.Sprintf("[%d] %s\n", i+1, m)
			}
			return result, nil
		},
	})

	for _, t := range tools {
		registry.Register(t)
	}
}
