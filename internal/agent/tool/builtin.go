package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/provider"

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
	Admins      string `json:"admins"`    // 管理员 QQ 列表
}

// RegisterBuiltinTools 注册所有内置工具到注册表。
// getSandbox / getT2I 使用 function getter 以支持运行时客户端热更新。
func RegisterBuiltinTools(
	registry *ToolRegistry,
	adapter AdapterProvider,
	getSandbox func() *sandboxcaller.Client,
	getT2I func() *t2icaller.Client,
	imageModel provider.Provider,
	getSessionCtx func(ctx context.Context) string,
	getCurrentMsg func(ctx context.Context) *adapter.MessageEvent,
	getRecentMsgs func(ctx context.Context, msgType string, targetID int64, limit int) ([]string, error),
	listImages func(ctx context.Context, folder string, limit int) (string, error),
	searchImages func(ctx context.Context, keyword string, limit int) (string, error),
	listStickerTags func(ctx context.Context) (string, error),
	listStickers func(ctx context.Context, tag string, page, pageSize int) (string, error),
	searchStickers func(ctx context.Context, keyword string, limit int) (string, error),
	sendStickerByKeyword func(ctx context.Context, keyword, msgType string, targetID int64) (string, error),
	searchKnowledge func(ctx context.Context, keyword string, limit int) (string, error),
) {
	tools := []Tool{}

	// --- OneBot11 消息 ---

	// list_images 图床查询：Agent 发图前先查图床有哪些图片，拿到 ID 后按
	// imgs://<ID> 引用（发送层会自动转 base64，无需关心网络互通）。
	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "list_images", "查询图床中的图片列表（含 ID/名称/文件夹），用于发消息时引用图床图片 [CQ:image,file=imgs://图片ID]；发图前先调用本工具获取图片 ID",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"folder": map[string]any{"type": "string", "description": "虚拟文件夹路径（如 /meme），不填默认根目录 /"},
					"limit":  map[string]any{"type": "integer", "description": "返回条数上限（默认 20，最大 50）"},
				},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Folder string `json:"folder"`
				Limit  int    `json:"limit"`
			}
			_ = json.Unmarshal(args, &p)
			if listImages == nil {
				return "图床未初始化", nil
			}
			return listImages(ctx, p.Folder, p.Limit)
		},
	})

	// search_images 图床按名称搜索：Agent 不知道图片 ID 时按图片展示名搜索，
	// 拿到 ID 后可拼 [CQ:image,file=imgs://图片ID] 引用（发送层会自动转 base64）。
	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "search_images", "按图片展示名称模糊搜索图床中的图片（含 ID/名称/文件夹），用于按名找到图床图片后在消息中用 [CQ:image,file=imgs://图片ID] 引用；发图前先调用本工具获取图片 ID",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": "图片名称关键词（匹配名称，如 猫/meme/表情）"},
					"limit":   map[string]any{"type": "integer", "description": "返回条数上限（默认 20，最大 50）"},
				},
				"required": []string{"keyword"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Keyword string `json:"keyword"`
				Limit   int    `json:"limit"`
			}
			_ = json.Unmarshal(args, &p)
			if searchImages == nil {
				return "图床未初始化", nil
			}
			return searchImages(ctx, p.Keyword, p.Limit)
		},
	})

	// --- 表情包库 ---

	// send_sticker 单独发表情（subType=1）。富文本消息中请用图片方式 [CQ:image,file=imgs://图片ID]。
	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "send_sticker", "发送表情包库中的表情（OneBot11 表情段 subType=1）。先调用 list_stickers / search_stickers 获取表情 ID；适合单独发表情，富文本消息中的图片请用 [CQ:image,file=imgs://图片ID] 方式",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"sticker_id":   map[string]any{"type": "string", "description": "表情 ID（短 UUID）"},
					"message_type": map[string]any{"type": "string", "description": "发送目标：group 群聊 / private 私聊，不填默认当前会话"},
					"target_id":    map[string]any{"type": "integer", "description": "目标群号或 QQ 号，不填默认当前会话"},
				},
				"required": []string{"sticker_id"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				StickerID   string    `json:"sticker_id"`
				MessageType string    `json:"message_type"`
				TargetID    FlexInt64 `json:"target_id"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			if strings.TrimSpace(p.StickerID) == "" {
				return "", fmt.Errorf("缺少 sticker_id 参数")
			}
			// 用 CQ 码字符串构造表情段（subType=1），发送层自动把 stk:// 解析为 base64
			msg := fmt.Sprintf("[CQ:image,file=stk://%s,subType=1]", p.StickerID)
			msgType := p.MessageType
			targetID := int64(p.TargetID)
			if targetID == 0 {
				if cur := getCurrentMsg(ctx); cur != nil {
					msgType = cur.MessageType
					if cur.MessageType == "private" {
						targetID = cur.UserID
					} else {
						targetID = cur.GroupID
					}
				}
			}
			if targetID == 0 {
				return "", fmt.Errorf("缺少目标，且无法从当前会话推断")
			}
			if q := GetDeferredSendQueue(ctx); q != nil {
				q.Add(DeferredSend{MessageType: msgType, TargetID: targetID, Message: msg, Delivery: true})
				return "表情已加入发送队列，将在任务执行完成后统一发送", nil
			}
			var id int64
			var err error
			if msgType == "private" {
				id, err = adapter.SendPrivateMsg(targetID, msg)
			} else {
				id, err = adapter.SendGroupMsg(targetID, msg)
			}
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("表情已发送，message_id: %d", id), nil
		},
	})

	// list_sticker_tags 获取全部表情标签。
	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "list_sticker_tags", "获取表情包库的全部标签（表情 ID 查询时可按标签过滤）",
			openai.FunctionParameters{"type": "object", "properties": map[string]any{}}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			if listStickerTags == nil {
				return "表情包库未初始化", nil
			}
			return listStickerTags(ctx)
		},
	})

	// list_stickers 分页获取表情（按标签过滤）。
	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "list_stickers", "分页获取表情包库的表情（可按标签过滤），返回表情 ID/名称/简介，发送时用 send_sticker + 表情 ID",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"tag":       map[string]any{"type": "string", "description": "标签名（可选），只列出该标签下的表情"},
					"page":      map[string]any{"type": "integer", "description": "页码，从 1 开始"},
					"page_size": map[string]any{"type": "integer", "description": "每页条数（默认 20，最大 50）"},
				},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Tag      string `json:"tag"`
				Page     int    `json:"page"`
				PageSize int    `json:"page_size"`
			}
			_ = json.Unmarshal(args, &p)
			if listStickers == nil {
				return "表情包库未初始化", nil
			}
			return listStickers(ctx, p.Tag, p.Page, p.PageSize)
		},
	})

	// search_stickers 模糊匹配表情简介。
	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "search_stickers", "按关键词模糊匹配表情的名称与简介，返回表情 ID/名称/简介，发送时用 send_sticker + 表情 ID",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": "搜索关键词（匹配名称或简介）"},
					"limit":   map[string]any{"type": "integer", "description": "返回条数上限（默认 20，最大 50）"},
				},
				"required": []string{"keyword"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Keyword string `json:"keyword"`
				Limit   int    `json:"limit"`
			}
			_ = json.Unmarshal(args, &p)
			if searchStickers == nil {
				return "表情包库未初始化", nil
			}
			return searchStickers(ctx, p.Keyword, p.Limit)
		},
	})

	// send_sticker_by_keyword 一步发送：按关键词搜索并直接发送最匹配的表情，
	// 省去"先 search 再 send"两步调用，Agent 接梗/回应情绪时一次搞定。
	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "send_sticker_by_keyword", "按关键词搜索表情包库并直接发送最匹配的一个表情（一步完成，无需先查 ID）。适合接梗、回应情绪、表达态度等场景：直接描述你想表达的意思（如\"嘲笑\"、\"点赞\"、\"晚安\"、\"笑死\"），系统会搜索表情包库并发送最匹配的表情；未找到匹配时返回提示。想从某个标签下挑选表情用 list_stickers，知道具体 ID 用 send_sticker",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"keyword":      map[string]any{"type": "string", "description": "想表达的意思或搜索关键词（匹配名称/简介/标签）"},
					"message_type": map[string]any{"type": "string", "description": "发送目标：group 群聊 / private 私聊，不填默认当前会话"},
					"target_id":    map[string]any{"type": "integer", "description": "目标群号或 QQ 号，不填默认当前会话"},
				},
				"required": []string{"keyword"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Keyword     string    `json:"keyword"`
				MessageType string    `json:"message_type"`
				TargetID    FlexInt64 `json:"target_id"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			if strings.TrimSpace(p.Keyword) == "" {
				return "", fmt.Errorf("缺少 keyword 参数")
			}
			if sendStickerByKeyword == nil {
				return "表情包库未初始化", nil
			}
			return sendStickerByKeyword(ctx, p.Keyword, p.MessageType, int64(p.TargetID))
		},
	})

	// search_knowledge 知识库主动检索：区别于对话前自动注入的 knowledge context，
	// 让 Agent 在对话中按需查询知识库内容。
	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "search_knowledge", "按关键词主动检索知识库，返回相关知识内容。当你需要查阅团队/项目/领域的知识、术语、规则或资料时调用；对话开始时系统已自动注入过一些知识，但若需要更细或更多内容请主动调用本工具",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": "检索关键词（匹配知识关键词或内容）"},
					"limit":   map[string]any{"type": "integer", "description": "返回条数上限（默认 5，最大 20）"},
				},
				"required": []string{"keyword"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Keyword string `json:"keyword"`
				Limit   int    `json:"limit"`
			}
			_ = json.Unmarshal(args, &p)
			if searchKnowledge == nil {
				return "知识库未初始化", nil
			}
			return searchKnowledge(ctx, p.Keyword, p.Limit)
		},
	})

	tools = append(tools, &onebotTool{
		BaseTool: NewTool("", "send_private_msg", "发送私聊消息，支持纯文本或消息段数组",
			openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"user_id": map[string]any{"type": "integer", "description": "目标用户 QQ 号"},
					"message": map[string]any{
						"oneOf": []map[string]any{
							{"type": "string", "description": "消息文本，必须是 JSON 字符串（双引号包裹），可含 CQ 码：@某人 [CQ:at,qq=QQ号]、图片 [CQ:image,file=URL]、图床图片 [CQ:image,file=imgs://图床图片ID（用 list_images 查询）]、表情 [CQ:face,id=1]"},
							{"type": "array", "items": map[string]any{"type": "object"}, "description": "消息段数组：对象数组，每项含 type（text/image/at/face 等）与 data 字段"},
						},
						"description": "消息内容：JSON 字符串（含 CQ 码）或消息段数组，二选一",
					},
				},
				"required": []string{"user_id", "message"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				UserID  FlexInt64       `json:"user_id"`
				Message json.RawMessage `json:"message"`
			}
			// 容错：LLM 偶尔把消息内容直接当作参数传入（如 "[CQ:image,file=...] 文字"），
			// 此时将原始参数整体视为 message，目标回退到当前会话。
			if err := json.Unmarshal(args, &p); err != nil {
				log.Warn("send_private_msg 参数非标准 JSON，按消息内容容错解析", "err", err, "args_len", len(args))
				p.Message = args
			}

			msg, err := BuildMessageLoose(p.Message)
			if err != nil {
				return "", fmt.Errorf("消息内容无效: %w", err)
			}

			// LLM 常省略 user_id（意图为当前会话）：从当前消息上下文兜底
			userID := int64(p.UserID)
			if userID == 0 {
				if cur := getCurrentMsg(ctx); cur != nil && cur.MessageType == "private" {
					userID = cur.UserID
				}
			}
			if userID == 0 {
				return "", fmt.Errorf("缺少 user_id 参数，且无法从当前会话推断目标用户")
			}
			// 任务执行期间不直接发送：入队等待，任务完成后由事件循环统一发送
			if q := GetDeferredSendQueue(ctx); q != nil {
				q.Add(DeferredSend{MessageType: "private", TargetID: userID, Message: msg, Delivery: true})
				return "私聊消息已加入发送队列，将在任务执行完成后统一发送", nil
			}
			id, err := adapter.SendPrivateMsg(userID, msg)
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
					"message": map[string]any{
						"oneOf": []map[string]any{
							{"type": "string", "description": "消息文本，必须是 JSON 字符串（双引号包裹），可含 CQ 码：@某人 [CQ:at,qq=QQ号]、图片 [CQ:image,file=URL]、图床图片 [CQ:image,file=imgs://图床图片ID（用 list_images 查询）]、表情 [CQ:face,id=1]"},
							{"type": "array", "items": map[string]any{"type": "object"}, "description": "消息段数组：对象数组，每项含 type（text/image/at/face 等）与 data 字段"},
						},
						"description": "消息内容：JSON 字符串（含 CQ 码）或消息段数组，二选一",
					},
				},
				"required": []string{"group_id", "message"},
			}, true, false),
		executor: func(ctx context.Context, args json.RawMessage) (string, error) {
			var p struct {
				GroupID FlexInt64       `json:"group_id"`
				Message json.RawMessage `json:"message"`
			}
			// 容错：LLM 偶尔把消息内容直接当作参数传入（如 "[CQ:image,file=...] 文字"），
			// 此时将原始参数整体视为 message，目标回退到当前会话。
			if err := json.Unmarshal(args, &p); err != nil {
				log.Warn("send_group_msg 参数非标准 JSON，按消息内容容错解析", "err", err, "args_len", len(args))
				p.Message = args
			}

			msg, err := BuildMessageLoose(p.Message)
			if err != nil {
				return "", fmt.Errorf("消息内容无效: %w", err)
			}

			// LLM 常省略 group_id（意图为当前会话）：从当前消息上下文兜底
			groupID := int64(p.GroupID)
			if groupID == 0 {
				if cur := getCurrentMsg(ctx); cur != nil && cur.MessageType == "group" {
					groupID = cur.GroupID
				}
			}
			if groupID == 0 {
				return "", fmt.Errorf("缺少 group_id 参数，且无法从当前会话推断目标群")
			}
			// 任务执行期间不直接发送：入队等待，任务完成后由事件循环统一发送
			if q := GetDeferredSendQueue(ctx); q != nil {
				q.Add(DeferredSend{MessageType: "group", TargetID: groupID, Message: msg, Delivery: true})
				return "群消息已加入发送队列，将在任务执行完成后统一发送", nil
			}
			id, err := adapter.SendGroupMsg(groupID, msg)
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
			var p struct {
				MessageID int64 `json:"message_id"`
			}
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
			var p struct {
				MessageID int64 `json:"message_id"`
			}
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
			var p struct {
				GroupID int64 `json:"group_id"`
			}
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
			var p struct {
				GroupID int64 `json:"group_id"`
			}
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
					"duration": map[string]any{"type": "integer", "description": "禁言时长(秒)，0 表示解除禁言"},
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
			if p.Duration == 0 {
				return fmt.Sprintf("已将 %d 解除禁言（群 %d）", p.UserID, p.GroupID), nil
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
				var p struct {
					Status string `json:"status"`
				}
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

	// --- 沙箱工具 (需要 sandbox_id) ---

	if getSandbox != nil {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "browser_search", "在沙箱中执行浏览器搜索",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"sandbox_id": map[string]any{"type": "string", "description": "沙箱 ID"},
						"query":      map[string]any{"type": "string", "description": "搜索关键词"},
					},
					"required": []string{"sandbox_id", "query"},
				}, true, false),
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
			BaseTool: NewTool("", "command_exec", "在沙箱中执行系统命令",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"sandbox_id": map[string]any{"type": "string", "description": "沙箱 ID"},
						"command":    map[string]any{"type": "string", "description": "要执行的命令"},
					},
					"required": []string{"sandbox_id", "command"},
				}, true, false),
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
			BaseTool: NewTool("", "code_exec", "在沙箱中执行 Python 代码",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"sandbox_id": map[string]any{"type": "string", "description": "沙箱 ID"},
						"code":       map[string]any{"type": "string", "description": "Python 代码"},
					},
					"required": []string{"sandbox_id", "code"},
				}, true, false),
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

	// --- 文生图 ---

	if getT2I != nil {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "text_to_image", "根据 HTML/模板生成图片，返回图片 URL。图片不会自动发送，请你在要发送的消息中用 [CQ:image,file=URL] 拼接图片，可与文字组成一条富文本消息。",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"html": map[string]any{"type": "string", "description": "HTML 内容"},
					},
					"required": []string{"html"},
				}, true, false),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				t2i := getT2I()
				if t2i == nil {
					return "", fmt.Errorf("T2I 服务未启用")
				}
				var p struct {
					HTML string `json:"html"`
				}
				if err := json.Unmarshal(args, &p); err != nil {
					return "", fmt.Errorf("参数解析失败: %w", err)
				}

				// 使用 Generate 获取图片 ID（而非 GenerateImage 返回的原始字节）
				genResp, err := t2i.Generate(ctx, t2icaller.GenerateRequest{
					HTML: p.HTML,
					Options: &t2icaller.GenerateOptions{
						Type:    t2icaller.ImageTypeJPEG,
						Quality: 80,
					},
				})
				if err != nil {
					return "", fmt.Errorf("T2I 生成失败: %w", err)
				}

				// 构造图片 URL
				// T2I API 返回的 ID 已包含 "data/" 前缀（如 "data/rendered_xxx.png"），
				// 所以 URL = BaseURL + "/text2img/" + ID
				imageID := genResp.Data.ID
				imageURL := t2i.Config.BaseURL + "/text2img/" + imageID

				log.Info("T2I 图片已生成", "id", imageID, "image_url", imageURL)

				// 不自动发送：由 LLM 在消息中用 [CQ:image,file=URL] 拼接富文本发送
				return fmt.Sprintf("图片已生成。URL: %s，请在发送的消息中使用 [CQ:image,file=%s] 拼接图片（可与文字组成富文本消息）。", imageURL, imageURL), nil
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
				return getSessionCtx(ctx), nil
			},
		})
	}

	// --- QQ 超级表情 ---

	if getCurrentMsg != nil {
		tools = append(tools, &onebotTool{
			BaseTool: NewTool("", "send_face", "发送 QQ 表情到当前会话。经典小黄脸 face_id 参考: 0=惊讶, 1=撇嘴, 2=色, 3=发呆, 4=得意, 5=流泪, 6=害羞, 7=闭嘴, 10=发怒, 14=微笑, 18=可爱, 21=疑问, 22=无语, 28=再见, 37=呲牙, 39=偷笑, 55=流汗, 63=委屈, 66=坏笑, 74=可怜, 76=酷, 89=尴尬, 97=大笑, 111=爱心, 142=抱拳, 182=耶, 188=狗头, 201=点赞, 211=笑哭, 277=鲜花。超级表情(sub_type=3): 5=流泪, 53=蛋糕, 114=篮球, 181=戳一戳, 311=打call, 317=菜汪, 318=崇拜, 319=比心, 320=庆祝, 325=惊吓, 360=亲亲, 375=超级鼓掌, 383=企鹅爱心, 384=晚安, 386=呜呜呜。手势(sub_type=5): 2=比心, 4=比心_心碎",
				openai.FunctionParameters{
					"type": "object",
					"properties": map[string]any{
						"face_id":  map[string]any{"type": "integer", "description": "QQ 表情 ID，参考工具描述中的列表"},
						"sub_type": map[string]any{"type": "integer", "description": "表情 sub_type: 经典小黄脸不填, 超级表情填 3, 手势填 5"},
					},
					"required": []string{"face_id"},
				}, true, false),
			executor: func(ctx context.Context, args json.RawMessage) (string, error) {
				var p struct {
					FaceID  int `json:"face_id"`
					SubType int `json:"sub_type"`
				}
				json.Unmarshal(args, &p)
				msg := getCurrentMsg(ctx)
				if msg == nil {
					return "未获取到当前会话信息", nil
				}
				var cqCode string
				if p.SubType > 0 {
					cqCode = fmt.Sprintf("[CQ:face,id=%d,sub_type=%d]", p.FaceID, p.SubType)
				} else {
					cqCode = fmt.Sprintf("[CQ:face,id=%d]", p.FaceID)
				}
				// 任务执行期间不直接发送：入队等待，任务完成后由事件循环统一发送
				if q := GetDeferredSendQueue(ctx); q != nil {
					targetID := int64(0)
					if msg.MessageType == "private" {
						targetID = msg.UserID
					} else {
						targetID = msg.GroupID
					}
					q.Add(DeferredSend{MessageType: msg.MessageType, TargetID: targetID, Message: cqCode})
					return fmt.Sprintf("QQ 表情 (face_id=%d) 已加入发送队列，将在任务执行完成后统一发送", p.FaceID), nil
				}
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
			msg := getCurrentMsg(ctx)
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

	// T2I/Sandbox 相关工具绑定服务可用性回调：对应服务停用/未配置时返回 false，
	// BuildEinoTools 会将其过滤，实现自动卸载（LLM 不再看到这些工具）。
	// 服务重新启用后触发 RebuildEinoAgent 即可恢复。
	sandboxToolNames := map[string]bool{
		"create_sandbox": true,
		"list_sandboxes": true,
		"browser_search": true,
		"command_exec":   true,
		"code_exec":      true,
	}
	for _, t := range tools {
		bt, ok := t.(*onebotTool)
		if !ok {
			continue
		}
		switch {
		case sandboxToolNames[bt.Name()]:
			bt.SetAvailable(func() bool { return getSandbox() != nil })
		case bt.Name() == "text_to_image":
			bt.SetAvailable(func() bool { return getT2I() != nil })
		}
	}

	for _, t := range tools {
		registry.Register(t)
	}
}
