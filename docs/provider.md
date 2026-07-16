# Provider — OneBot11 适配器使用文档

## 概述

`internal/provider` 是 JuanNiang-Neo 的 OneBot11 协议适配器。它启动一个反向 WebSocket 服务器，等待 OneBot11 客户端（go-cqhttp、NapCat、LLOneBot 等）连接，收发事件并暴露全部 OneBot11 API 供 Agent 调用。

**设计原则：**
- **纯 OneBot11** — 不再支持其他协议，API 直接对应 OneBot11 规范。
- **Agent 友好** — 强类型事件 + 链式消息构建 + 同步 API 调用。
- **零依赖负担** — 仅依赖 `RomiChan/websocket` 和标准库。

---

## 快速开始

### 1. 创建并启动 Provider

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"

    "JuanNiang-Neo/internal/provider"
)

func main() {
    p := provider.New(provider.Config{
        Name:   "my-bot",
        Port:   8080,
        Token:  "your-secret-token",
        Admins: []int64{123456789},
    })

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    if err := p.Start(ctx); err != nil {
        slog.Error("启动失败", "err", err)
        return
    }
    defer p.Stop(context.Background())

    slog.Info("provider 已启动，等待客户端连接...", "addr", ":8080")

    // 事件循环
    for ev := range p.Events() {
        handleEvent(p, ev)
    }
}
```

### 2. 配置 OneBot11 客户端

以 **go-cqhttp** 为例，在 `config.yml` 中配置反向 WS：

```yaml
servers:
  - ws-reverse:
      universal: ws://127.0.0.1:8080
      access-token: your-secret-token
```

以 **NapCat** 为例，在 WebUI 中添加反向 WebSocket 服务端地址 `ws://127.0.0.1:8080`，填入 token。

客户端连接成功后日志输出：
```
INFO ws server 启动 addr=0.0.0.0:8080
INFO 客户端连接 self_id=123456789 remote_addr=127.0.0.1:xxxxx
```

---

## 事件处理

### 事件结构

所有事件通过 `p.Events()` 返回的只读 channel 投递。`Event` 是一个联合体 — 根据 `PostType` 填充对应的子结构：

```go
type Event struct {
    PostType string           // message / notice / request / meta_event
    Time     int64
    SelfID   int64
    Raw      json.RawMessage  // 原始 JSON，调试用

    Message *MessageEvent      // PostType == "message"
    Notice  *NoticeEvent        // PostType == "notice"
    Request *RequestEvent       // PostType == "request"
    Meta    *MetaEvent          // PostType == "meta_event"
}
```

### 消息事件

```go
type MessageEvent struct {
    MessageType string    // "private" | "group"
    SubType     string    // "friend" | "normal" | "anonymous" | "group_self"
    MessageID   int64
    UserID      int64
    GroupID     int64      // 群消息时有值
    Message     []Segment  // 结构化消息段
    RawMessage  string     // CQ 码格式原文
    Sender      struct {
        UserID   int64
        Nickname string
        Card     string   // 群名片
    }
}
```

使用示例：

```go
func handleEvent(p *provider.Provider, ev provider.Event) {
    switch ev.PostType {
    case "message":
        msg := ev.Message
        if msg == nil {
            return
        }

        // 过滤群消息
        if msg.MessageType != "group" {
            return
        }

        // 指令匹配
        if msg.RawMessage == "/ping" {
            p.SendGroupMsg(msg.GroupID, "pong!")
        }

        // 读取结构化消息段
        for _, seg := range msg.Message {
            switch seg.Type {
            case "text":
                slog.Info("文本", "text", seg.Data["text"])
            case "image":
                slog.Info("图片", "url", seg.Data["url"])
            case "at":
                slog.Info("@某人", "qq", seg.Data["qq"])
            }
        }

    case "notice":
        n := ev.Notice
        switch n.NoticeType {
        case "group_increase":
            p.SendGroupMsg(n.GroupID,
                provider.BuildMessage("欢迎新成员！", provider.Face(76)))
        case "group_decrease":
            p.SendGroupMsg(n.GroupID, "有人退群了...")
        }

    case "request":
        r := ev.Request
        if r.RequestType == "friend" {
            p.HandleFriendRequest(r.Flag, true, "")
        } else if r.RequestType == "group" {
            p.HandleGroupRequest(r.Flag, r.SubType, true, "")
        }
    }
}
```

---

## 发送消息

### 三种消息格式

`SendPrivateMsg` 和 `SendGroupMsg` 的 `message` 参数接受三种类型：

| 类型 | 示例 | 效果 |
|------|------|------|
| `string` | `"你好世界"` | 纯文本 |
| `Segment` | `provider.Image("https://...")` | 单段消息 |
| `[]Segment` | `provider.BuildMessage(...)` | 富文本消息段数组 |

### 纯文本

```go
p.SendGroupMsg(groupID, "Hello World")
p.SendPrivateMsg(userID, "你好！")
```

### 包含 CQ 码的文本 — 自动解析

字符串中包含 `[CQ:...]` 格式会被自动解析为消息段：

```go
p.SendGroupMsg(groupID, "[CQ:at,qq=123456] 看这张图 [CQ:image,file=https://example.com/pic.jpg]")
```

自动支持所有 CQ 码类型：`image`、`at`、`face`、`record`、`video`、`file`、`reply`。

### 消息段构建器

```go
p.SendGroupMsg(groupID, provider.Image("https://example.com/photo.jpg"))
p.SendGroupMsg(groupID, provider.Face(14))   // QQ 表情 14 (微笑)
p.SendGroupMsg(groupID, provider.At("123456"))
p.SendGroupMsg(groupID, provider.AtAll())
p.SendGroupMsg(groupID, provider.ReplyInt64(msgID))  // 回复某条消息
p.SendGroupMsg(groupID, provider.Record("voice.amr"))
p.SendGroupMsg(groupID, provider.Video("video.mp4"))
```

### 流式构建器 — `NewMsg()` (推荐)

```go
msg := provider.NewMsg().
    ReplyInt64(msg.MessageID).
    At(fmt.Sprint(msg.UserID)).
    Text(" 每日一图：\n").
    Image("https://picsum.photos/400/300").
    Face(76)

p.SendGroupMsg(groupID, msg)   // 直接传 *MessageBuilder
```

`NewMsg()` 返回的 `*MessageBuilder` 可以直接传给 `SendGroupMsg`/`SendPrivateMsg`，
也可以调用 `.Build()` 获取 `[]Segment`。

### 函数式 — `BuildMessage()`

同上效果，函数式风格：

```go
p.SendGroupMsg(groupID, provider.BuildMessage(
    provider.ReplyInt64(msg.MessageID),
    provider.At(fmt.Sprint(msg.UserID)),
    " 每日一图：\n",
    provider.Image("https://picsum.photos/400/300"),
    provider.Face(76),
))
```

---

## 消息段参考

| 函数构建器 | 流式方法 | OneBot11 type | 说明 |
|-----------|----------|---------------|------|
| `Text(s)` | `.Text(s)` | `text` | 纯文本 |
| `Image(file)` | `.Image(file)` | `image` | 图片。file=URL/base64/本地路径 |
| `FileMsg(file)` | `.File(file)` | `file` | 群文件上传 |
| `Face(id)` | `.Face(id)` | `face` | QQ 表情。id 为表情编号 |
| `At(qq)` | `.At(qq)` | `at` | @某人。qq="all"=全体 |
| `AtAll()` | `.AtAll()` | `at` | @全体成员 |
| `Record(file)` | `.Record(file)` | `record` | 语音 |
| `Video(file)` | `.Video(file)` | `video` | 视频 |
| `Reply(id)` | `.Reply(id)` | `reply` | 回复消息 |
| `ReplyInt64(id)` | `.ReplyInt64(id)` | `reply` | 同上，int64 版本 |
| `BuildMessage(parts...)` | `.Seg(seg)` | — | 追加任意 Segment |
| — | `.Build()` | — | 返回 `[]Segment` |
| — | `NewMsg()` | — | 创建空构建器 |

---

## 完整 API 参考

### 消息 API

```go
func (p *Provider) SendPrivateMsg(userID int64, message any) (int64, error)
func (p *Provider) SendGroupMsg(groupID int64, message any) (int64, error)
func (p *Provider) DeleteMsg(messageID int64) error
func (p *Provider) MarkMsgRead(messageID int64) error
func (p *Provider) GetMsg(messageID int64) (*MessageEvent, error)
```

### 用户 API

```go
func (p *Provider) GetLoginInfo() (*LoginInfo, error)          // 获取机器人自身信息
func (p *Provider) GetStrangerInfo(userID int64) (*StrangerInfo, error)
func (p *Provider) GetFriendList() ([]FriendInfo, error)
```

### 群信息 API

```go
func (p *Provider) GetGroupInfo(groupID int64) (*GroupInfo, error)
func (p *Provider) GetGroupList() ([]GroupInfo, error)
func (p *Provider) GetGroupMemberInfo(groupID, userID int64) (*GroupMemberInfo, error)
func (p *Provider) GetGroupMemberList(groupID int64) ([]GroupMemberInfo, error)
func (p *Provider) GetGroupHonorInfo(groupID int64) (*GroupHonorInfo, error)
```

### 群管理 API

```go
func (p *Provider) KickGroupMember(groupID, userID int64, rejectAdd bool) error
func (p *Provider) BanGroupMember(groupID, userID int64, duration int) error
func (p *Provider) SetGroupWholeBan(groupID int64, enable bool) error
func (p *Provider) SetGroupAdmin(groupID, userID int64, enable bool) error
func (p *Provider) SetGroupCard(groupID, userID int64, card string) error
func (p *Provider) SetGroupName(groupID int64, name string) error
func (p *Provider) LeaveGroup(groupID int64) error
func (p *Provider) SetGroupSpecialTitle(groupID, userID int64, title string) error
```

### 请求处理 API

```go
func (p *Provider) HandleFriendRequest(flag string, approve bool, remark string) error
func (p *Provider) HandleGroupRequest(flag, subType string, approve bool, reason string) error
```

### 媒体 API

```go
func (p *Provider) GetImage(file string) (*FileInfo, error)
func (p *Provider) GetRecord(file string) (*FileInfo, error)
func (p *Provider) CanSendImage() (bool, error)
func (p *Provider) CanSendRecord() (bool, error)
```

### 其他 API

```go
func (p *Provider) SendLike(userID int64, times int) error
func (p *Provider) GetCookies() (*Cookies, error)
func (p *Provider) GetCSRFToken() (*CSRF, error)
func (p *Provider) GetCredentials() (*Credentials, error)
func (p *Provider) GetStatus() (*Status, error)
func (p *Provider) GetVersionInfo() (*VersionInfo, error)
func (p *Provider) GetForwardMsg(messageID string) ([]ForwardNode, error)
func (p *Provider) SendGroupForwardMsg(groupID int64, nodes []ForwardNode) (int64, error)
```

### 辅助方法

```go
func (p *Provider) Events() <-chan Event                   // 事件通道
func (p *Provider) SelfID() int64                          // 当前机器人 QQ 号
func (p *Provider) Status() ProviderStatus                 // 运行状态
func (p *Provider) UpdateConfig(cfg Config)                // 热更新配置 (Token / Admins)
func (p *Provider) Start(ctx context.Context) error
func (p *Provider) Stop(ctx context.Context) error
```

### 状态查询

```go
st := p.Status()
// st.Running     — 是否运行中
// st.ListenAddr  — 监听地址
// st.SelfID      — 机器人 QQ 号
// st.ConnCount   — 已连接客户端数
// st.ConnIDs     — 已连接客户端的 self_id 列表

if st.ConnCount == 0 {
    slog.Warn("没有 OneBot11 客户端连接")
}
```

### 配置热更新

```go
// 运行时更新 token 和管理员列表（Addr/Port 需重启）
p.UpdateConfig(provider.Config{
    Token:  "new-token",
    Admins: []int64{123, 456},
})
```

---

## Agent 开发指南

### Agent 接口约定

每个 Agent 建议实现以下接口：

```go
type Agent interface {
    // Name 返回 Agent 唯一名称
    Name() string
    // Handle 处理事件。返回 true 表示事件已被消费，不再传递给后续 Agent。
    Handle(p *provider.Provider, ev provider.Event) (consumed bool)
}
```

### Agent 调度器示例

```go
type AgentHub struct {
    agents []Agent
}

func (h *AgentHub) Register(a Agent) {
    h.agents = append(h.agents, a)
}

func (h *AgentHub) Dispatch(p *provider.Provider, ev provider.Event) {
    for _, a := range h.agents {
        if a.Handle(p, ev) {
            return  // 事件已消费
        }
    }
}
```

### 完整 Agent 示例：复读机

```go
type RepeatAgent struct{}

func (RepeatAgent) Name() string { return "repeat" }

func (RepeatAgent) Handle(p *provider.Provider, ev provider.Event) bool {
    if ev.PostType != "message" || ev.Message == nil {
        return false
    }

    msg := ev.Message
    if msg.MessageType != "group" {
        return false
    }

    content := strings.TrimSpace(msg.RawMessage)
    if content == "复读" {
        _, err := p.SendGroupMsg(msg.GroupID, msg.RawMessage)
        if err != nil {
            slog.Error("复读失败", "err", err)
        }
        return true
    }
    return false
}
```

### 完整 Agent 示例：群管理

```go
type GroupManagerAgent struct{}

func (GroupManagerAgent) Name() string { return "group-manager" }

func (GroupManagerAgent) Handle(p *provider.Provider, ev provider.Event) bool {
    if ev.PostType != "notice" || ev.Notice == nil {
        return false
    }

    n := ev.Notice
    switch n.NoticeType {
    case "group_increase":
        // 新成员入群 — 发欢迎消息
        p.SendGroupMsg(n.GroupID, provider.BuildMessage(
            provider.At(fmt.Sprint(n.UserID)),
            " 欢迎进群！",
            provider.Face(76),
        ))
        return true

    case "group_decrease":
        // 成员退群 — 发通知
        p.SendGroupMsg(n.GroupID, fmt.Sprintf("成员 %d 离开了群", n.UserID))
        return true
    }

    return false
}

func (GroupManagerAgent) isAdmin(p *provider.Provider, groupID, userID int64) bool {
    info, err := p.GetGroupMemberInfo(groupID, userID)
    if err != nil {
        return false
    }
    return info.Role == "owner" || info.Role == "admin"
}
```

---

## 错误处理

API 调用返回的 `error` 已经是经过封装的友好信息：

```go
// 直接使用
id, err := p.SendGroupMsg(groupID, "hello")
if err != nil {
    slog.Error("发送失败", "err", err)
    return
}
slog.Info("发送成功", "msg_id", id)
```

常见的错误类型：

| 错误 | 原因 |
|------|------|
| `provider 未启动` | 还没调用 `Start()` |
| `no active connection` | 没有 OneBot11 客户端连接 |
| `api xxx timeout` | API 调用 10 秒超时 |
| `api xxx failed: retcode=...` | OneBot11 返回错误 |

---

## 配置参考

```go
type Config struct {
    Name   string  // 适配器名称，日志用
    Addr   string  // 监听地址，如 "127.0.0.1:8080"。为空时使用 Port
    Port   int     // 监听端口，如 8080。与 Addr 二选一
    Token  string  // 鉴权 token，与客户端配置一致（为空则跳过鉴权）
    Admins []int64 // 管理员 QQ 号列表
}
```

**鉴权流程：** Provider 优先检查 HTTP Header `Authorization: Bearer xxx`，其次检查 URL query `?access_token=xxx`。与客户端配置的 token 一致即放行。

---

## 性能说明

- **事件通道** 缓冲区 128，满时丢弃并打 WARN 日志
- **API 调用** 同步等待，默认 10 秒超时
- **并发安全** 所有连接操作有读写锁保护
- **WebSocket 写** 串行化（单个 `sync.Mutex`），符合 OneBot11 协议要求
