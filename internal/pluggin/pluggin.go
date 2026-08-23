package pluggin

import (
	"bytes"
	"context"
	crand "crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"database/sql"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/provider"

	"github.com/google/uuid"
	lua "github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	ragcaller "JuanNiang-Neo/infrastructure/rag/handler"
	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/core/cache"
	"JuanNiang-Neo/internal/core/dao"
)

// ---------- 清单 ----------

type Manifest struct {
	// PPID 插件持久化唯一标识（UUID），用于跨目录重命名/迁移时稳定识别插件。
	// 为空时由 Load 自动生成并写回 pluggin.yaml。
	PPID        string   `yaml:"ppid"`
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Author      string   `yaml:"author"`
	Description string   `yaml:"description"`
	Entry       string   `yaml:"entry"`
	Permissions []string `yaml:"permissions"`
	// System=true 表示系统内置插件，禁止通过 API 删除或停用。
	// 这类插件通常随二进制分发，由 PluginEngine 启动时自动写入磁盘并加载。
	System bool `yaml:"system"`
	// Enabled 为插件的启用/停用状态。true=启用，false=停用。
	// 由 API TogglePlugin 写入 YAML，启动时 LoadAll 据此决定是否加载。
	// 系统插件（System=true）忽略此字段，始终加载。
	Enabled bool `yaml:"enabled"`
}

type LoadedPlugin struct {
	Manifest Manifest
	State    *lua.LState
	Dir      string

	// stateMu 串行化该插件 LState 的所有执行（事件回调 / 异步回调 / 命令 handler）。
	// 引擎全局锁 pe.mu 只保护数据结构，不再在持锁时执行 Lua——一个插件的
	// 慢调用（如同步 OneBot11 API）不会阻塞其它插件与 Web API。
	stateMu sync.Mutex
	// closed 在 stateMu 下读写：标记 LState 已关闭，执行方检查后跳过。
	closed bool
	// currentEv 该插件当前正在处理的事件（stateMu 下读写）。
	// 事件派发/异步回调执行该插件 Lua 前写入；jn.agent.get_current_chat_area /
	// compact_memory 在 Lua 中读取。异步任务触发时快照事件、回调前恢复——
	// 事件 A 触发的回调即使在其他事件派发后仍读到 A 的会话数据。
	currentEv EventData
}

// close 关闭插件 LState。与执行互斥：等待在途回调完成后关闭，
// 并置 closed 标记让后续执行方跳过已关闭的 LState。
func (p *LoadedPlugin) close() {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.State.Close()
}

// ---------- 适配器接口 ----------

type SendAdapter interface {
	SendPrivateMsg(userID int64, message any) (int64, error)
	SendGroupMsg(groupID int64, message any) (int64, error)
	SendGroupForwardMsg(groupID int64, nodes []adapter.ForwardNode) (int64, error)
	DeleteMsg(messageID int64) error
	GetMsg(messageID int64) (map[string]any, error)
	GetGroupInfo(groupID int64) (map[string]any, error)
	GetGroupMemberList(groupID int64) ([]map[string]any, error)
	KickGroupMember(groupID, userID int64, rejectAdd bool) error
	BanGroupMember(groupID, userID int64, duration int) error
	SetGroupWholeBan(groupID int64, enable bool) error
	SetGroupCard(groupID, userID int64, card string) error
	HandleFriendRequest(flag string, approve bool, remark string) error
	HandleGroupRequest(flag, subType string, approve bool, reason string) error
	GetLoginInfo() (map[string]any, error)
	GetStrangerInfo(userID int64) (map[string]any, error)
	GetFriendList() ([]map[string]any, error)
	GetGroupList() ([]map[string]any, error)
	GetGroupMemberInfo(groupID, userID int64) (map[string]any, error)
	GetGroupHonorInfo(groupID int64) (map[string]any, error)
	SendLike(userID int64, times int) error
	GetStatus() (map[string]any, error)
	GetVersionInfo() (map[string]any, error)
}

// ---------- Agent 操作接口 ----------

// AgentOperator 插件通过此接口操作 Agent 核心功能。
type AgentOperator interface {
	SetProviderActive(ctx context.Context, id string, active bool) error
	SetMCPActive(ctx context.Context, id string, active bool) error
	SetToolActive(ctx context.Context, name string, active bool) error
	SwitchProvider(ctx context.Context, id string) error
	SetT2IActive(ctx context.Context, active bool) error
	SetSandboxActive(ctx context.Context, active bool) error
	CompactMemory(ctx context.Context, chatAreaID string) error
	GetChatAreaID(userID, groupID int64, messageType string) string
	GetProviderGroup() ProviderGroupAccess
	GetMCPGroup() MCPGroupAccess
	GetToolRegistry() ToolRegistryAccess
	GetT2IClient() *t2icaller.Client
	GetSandboxClient() *sandboxcaller.Client
	GetRAGClient() *ragcaller.Client // RAG 向量检索客户端（nil=未启用）
}

// ProviderGroupAccess 暴露给插件的 Provider 管理接口。
type ProviderGroupAccess interface {
	List() []ProviderInfo
	GetActive(id string) bool
}

// LLMAccess 插件通过此接口调用 Bot 的 LLM：复用 Bot 自身启用的文本模型
// Provider 及其配置（模型 / 采样参数 / 密钥），插件不接触任何密钥。
type LLMAccess interface {
	// Available 当前是否有启用的文本模型 Provider。
	Available() bool
	// Chat 调用 Bot 当前启用的文本模型。req 中未设置的采样参数回退到 Bot 配置。
	Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error)
}

// MCPGroupAccess 暴露给插件的 MCP 管理接口。
type MCPGroupAccess interface {
	ListMCPs() []MCPInfo
	IsConnected(id string) bool
}

// ToolRegistryAccess 暴露给插件的 Tool 管理接口。
type ToolRegistryAccess interface {
	ListTools() []ToolInfo
	IsActive(name string) bool
}

type ProviderInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Model  string `json:"model"`
	Active bool   `json:"active"`
}

type MCPInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Active   bool   `json:"active"`    // 运行时连接状态
	IsActive bool   `json:"is_active"` // DB 配置的启用状态
}

type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Builtin     bool   `json:"builtin"`
	LongRunning bool   `json:"long_running"`
	Active      bool   `json:"active"`
}

// ---------- 引擎 ----------

type PluginEngine struct {
	mu        sync.RWMutex
	plugins   map[string]*LoadedPlugin
	basePath  string
	adapter   SendAdapter
	db        *gorm.DB
	cache     *cache.Cache
	t2i       *t2icaller.Client
	sandbox   *sandboxcaller.Client
	dao       *dao.Bundle
	agentOp   AgentOperator
	llmAccess LLMAccess
	// asyncAPIs 异步 API 注册表：kind（如 "chat"）→ 派发规格。xxx_async 类接口
	// 提交后立即返回 req_id，阻塞操作在独立 goroutine 完成，完成后派发到插件
	// Lua 入口函数（如 on_chat_response）。注册表集中管理，后续新增
	// t2i/http/agent 等异步只需注册一个 kind。
	// 用 atomic 指针存储：注册表只在引擎构造期写入（RegisterAsyncAPI），
	// submitAsync 无锁读取——插件 Lua 回调（持 stateMu）内调用 *_async API
	// 不得再请求 pe.mu（否则写锁下 RLock 自死锁 / 读锁下与 writer 互锁）。
	asyncAPIs atomic.Pointer[map[string]*AsyncAPI]
	// registerMu 串行化 RegisterAsyncAPI 的 load-copy-store：并发 writer 时防止
	// 后注册的 Store 覆盖先注册的 kind（静默丢失）。读者（submitAsync）无锁
	// Load，不受影响；重复注册 panic 行为保留。
	registerMu sync.Mutex
	// asyncCh 异步任务完成后的回调队列（runAsyncCallbacks 消费）。
	asyncCh chan asyncTask
	// asyncSeq req_id 自增序号。
	asyncSeq uint64
	// loadMu 保护 loading 表：并发 Load(name) 去重（singleflight 语义）。
	loadMu   sync.Mutex
	loading  map[string]*pluginLoad
	commands *CommandRegistry
}

// pluginLoad 一次进行中的插件加载（并发 Load 去重：等待者复用同一次
// entry 执行结果，避免重复 L.DoFile 造成的命令注册/异步提交副作用）。
type pluginLoad struct {
	done chan struct{} // 加载结束（成功或失败）时关闭
	err  error         // 加载结果错误（成功为 nil）
}

// AsyncAPI 描述一类可异步执行的 API（注册进引擎异步注册表）。
// 插件侧调用形态：xxx_async(...) 立即返回 req_id，完成后引擎调用插件 Lua
// 入口函数 Entry：on_xxx_response(req_id, <Encode 返回值...>)，与事件派发互斥。
type AsyncAPI struct {
	// Entry 插件侧 Lua 入口函数名，如 "on_chat_response"。
	Entry string
	// Encode 在锁内把 Go 结果转成 Lua 返回值（req_id 已由派发器先压入）。
	// 成功：err 为 nil、result 为业务结果；失败：result 为 nil、err 非 nil。
	Encode func(L *lua.LState, result any, err error) []lua.LValue
	// WithCtx 为 true 时，回调入口签名带调用现场：on_xxx_response(req_id, ctx, <结果...>)。
	// ctx 由 xxx_async 的最后一个 table 参数提供（可选），按 req_id 关联保存在
	// Lua 侧 jn_async_ctx 注册表，回调时原样带回（不序列化，可含函数）并删除。
	WithCtx bool
}

// asyncTask 一次异步调用的完成事件（runAsyncCallbacks 消费）。
// event 为任务触发时该插件的当前事件快照：回调执行前恢复插件 currentEv，
// 保证 on_xxx_response 内 jn.agent.get_current_chat_area / compact_memory
// 读到任务触发时的会话，而不是派发时最新的其他事件。
type asyncTask struct {
	pluginName string
	kind       string
	api        *AsyncAPI
	reqID      uint64
	result     any
	err        error
	event      EventData
}

func NewPluginEngine(basePath string, adapter SendAdapter, db *gorm.DB, c *cache.Cache, t2i *t2icaller.Client, sb *sandboxcaller.Client, d *dao.Bundle, ag AgentOperator, llm LLMAccess) *PluginEngine {
	if basePath == "" {
		basePath = "data/pluggins"
	}
	pe := &PluginEngine{
		plugins:   make(map[string]*LoadedPlugin),
		basePath:  basePath,
		adapter:   adapter,
		db:        db,
		cache:     c,
		t2i:       t2i,
		sandbox:   sb,
		dao:       d,
		agentOp:   ag,
		llmAccess: llm,
		asyncCh:   make(chan asyncTask, 128),
		loading:   make(map[string]*pluginLoad),
		commands:  NewCommandRegistry(),
	}
	// 异步注册表初始为空 map；RegisterAsyncAPI 以 copy-on-write 方式填充
	pe.asyncAPIs.Store(&map[string]*AsyncAPI{})
	// 注册内置异步 API：chat → 插件入口 on_chat_response
	pe.RegisterAsyncAPI("chat", AsyncAPI{
		Entry: "on_chat_response",
		Encode: func(L *lua.LState, result any, err error) []lua.LValue {
			if err != nil {
				return []lua.LValue{lua.LNil, lua.LString(err.Error())}
			}
			return []lua.LValue{lua.LString(result.(string)), lua.LNil}
		},
	})
	// t2i → on_t2i_response（带调用现场 ctx）；结果：图片 ID / URL 字符串
	pe.RegisterAsyncAPI("t2i", AsyncAPI{
		Entry:   "on_t2i_response",
		WithCtx: true,
		Encode: func(L *lua.LState, result any, err error) []lua.LValue {
			if err != nil {
				return []lua.LValue{lua.LNil, lua.LString(err.Error())}
			}
			return []lua.LValue{lua.LString(result.(string)), lua.LNil}
		},
	})
	// http → on_http_response（带调用现场 ctx）；结果：{status, body}
	pe.RegisterAsyncAPI("http", AsyncAPI{
		Entry:   "on_http_response",
		WithCtx: true,
		Encode: func(L *lua.LState, result any, err error) []lua.LValue {
			if err != nil {
				return []lua.LValue{lua.LNil, lua.LString(err.Error())}
			}
			hr, ok := result.(*httpResult)
			if !ok {
				return []lua.LValue{lua.LNil, lua.LString("http: 未知结果类型")}
			}
			t := L.NewTable()
			t.RawSetString("status", lua.LNumber(hr.Status))
			t.RawSetString("body", lua.LString(hr.Body))
			return []lua.LValue{t, lua.LNil}
		},
	})
	// sandbox → on_sandbox_response（带调用现场 ctx）；结果：标量字段表
	// （create→{sandbox_id,status}、exec_shell→{output,exit_code}、exec_python→{output,error}）
	pe.RegisterAsyncAPI("sandbox", AsyncAPI{
		Entry:   "on_sandbox_response",
		WithCtx: true,
		Encode: func(L *lua.LState, result any, err error) []lua.LValue {
			if err != nil {
				return []lua.LValue{lua.LNil, lua.LString(err.Error())}
			}
			m, ok := result.(map[string]any)
			if !ok {
				return []lua.LValue{lua.LNil, lua.LString("sandbox: 未知结果类型")}
			}
			return []lua.LValue{luaTableFromMap(L, m), lua.LNil}
		},
	})
	// timer → on_timer_response（带调用现场 ctx）；jn.timer.after(秒) 到点触发，
	// 无业务结果；err 为 nil 表示正常到点（context 超时/取消时返回错误）
	pe.RegisterAsyncAPI("timer", AsyncAPI{
		Entry:   "on_timer_response",
		WithCtx: true,
		Encode: func(L *lua.LState, result any, err error) []lua.LValue {
			if err != nil {
				return []lua.LValue{lua.LNil, lua.LString(err.Error())}
			}
			return []lua.LValue{lua.LNil, lua.LNil}
		},
	})
	// rag → on_rag_response（带调用现场 ctx）；结果：add 回 tag 字符串，search 回 [{tag,score}] 表
	pe.RegisterAsyncAPI("rag", AsyncAPI{
		Entry:   "on_rag_response",
		WithCtx: true,
		Encode: func(L *lua.LState, result any, err error) []lua.LValue {
			if err != nil {
				return []lua.LValue{lua.LNil, lua.LString(err.Error())}
			}
			switch v := result.(type) {
			case string:
				return []lua.LValue{lua.LString(v), lua.LNil}
			case []ragcaller.SearchHit:
				t := L.NewTable()
				for i, hit := range v {
					item := L.NewTable()
					item.RawSetString("tag", lua.LString(hit.Tag.String()))
					item.RawSetString("score", lua.LNumber(hit.Score))
					t.RawSetInt(i+1, item)
				}
				return []lua.LValue{t, lua.LNil}
			default:
				return []lua.LValue{lua.LNil, lua.LString("rag: 未知结果类型")}
			}
		},
	})
	// 注册内置 /help 命令
	pe.registerBuiltinCommands()
	// 异步任务消费者：与事件派发互斥执行 Lua，保证 LState 安全
	go pe.runAsyncCallbacks()
	return pe
}

// Commands 返回命令注册表（供外部读取，如 Web API 查询命令列表）。
func (pe *PluginEngine) Commands() *CommandRegistry { return pe.commands }

// IsSystem 查询指定插件是否为系统插件（按已加载 manifest 的 System 字段）。
func (pe *PluginEngine) IsSystem(name string) bool {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	p, ok := pe.plugins[name]
	if !ok {
		return false
	}
	return p.Manifest.System
}

// registerBuiltinCommands 注册内置命令（/help）。
// Builtin 标记：handler 是纯 Go 闭包（不触碰 LState），执行不依赖 system 插件加载状态。
func (pe *PluginEngine) registerBuiltinCommands() {
	pe.commands.Register("system", []string{"help"}, CommandOpts{
		Description: "查看所有可用命令，或查看某个命令的子命令与用法",
		Usage:       "/help [命令路径...]",
		Builtin:     true,
	}, func(args []string, event EventData) (bool, string, error) {
		// /help 或 /help <cmd> [sub...]
		reply := pe.commands.FormatHelp(args)
		return true, reply, nil
	})
}

func (pe *PluginEngine) LoadAll() error {
	// 确保内置 SDK 与 system 插件已落盘
	pe.ensureEmbeddedAssets()

	entries, err := os.ReadDir(pe.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// 跳过 sdk 目录（不是插件）
		if entry.Name() == "sdk" {
			continue
		}
		// 读取 manifest 判断 enabled 状态（非系统插件且 enabled=false 则跳过）
		manifest, _ := pe.readManifest(filepath.Join(pe.basePath, entry.Name()))
		if manifest != nil && !manifest.Enabled && !manifest.System {
			log.Info("插件已禁用，跳过加载", "name", entry.Name())
			continue
		}
		if err := pe.Load(entry.Name()); err != nil {
			log.Error("插件加载失败", "name", entry.Name(), "err", err)
		}
	}
	return nil
}

func (pe *PluginEngine) Load(name string) (retErr error) {
	// 并发 Load 去重（singleflight 语义）：同名加载进行中时等待其完成并复用
	// 结果，避免重复执行 entry（L.DoFile 的命令注册/异步提交等副作用）。
	// 已加载的预检保留：快速失败，避免无谓的 manifest 读取。
	pe.loadMu.Lock()
	if l, ok := pe.loading[name]; ok {
		pe.loadMu.Unlock()
		<-l.done
		return l.err
	}
	pe.mu.RLock()
	_, loaded := pe.plugins[name]
	pe.mu.RUnlock()
	if loaded {
		pe.loadMu.Unlock()
		return fmt.Errorf("plugin %q already loaded", name)
	}
	l := &pluginLoad{done: make(chan struct{})}
	pe.loading[name] = l
	pe.loadMu.Unlock()
	// 注册完成后统一收尾：记录结果、删除 loading、唤醒等待者。
	defer func() {
		l.err = retErr
		pe.loadMu.Lock()
		delete(pe.loading, name)
		pe.loadMu.Unlock()
		close(l.done)
	}()

	pluginDir := filepath.Join(pe.basePath, name)
	manifest, err := pe.readManifest(pluginDir)
	if err != nil {
		return fmt.Errorf("读取 pluggin.yaml 失败: %w", err)
	}

	// PPID 为空时自动生成并写回 yaml，保证每个插件都有稳定唯一标识
	if manifest.PPID == "" {
		manifest.PPID = newPluginUUID()
		if werr := pe.writeManifest(pluginDir, manifest); werr != nil {
			log.Warn("写回插件 PPID 失败", "name", name, "err", werr)
		} else {
			log.Info("已为插件生成 PPID", "name", name, "ppid", manifest.PPID)
		}
	}

	L := lua.NewState()
	// 先创建插件对象并挂到 LState（userdata 引用）：injectAgent / submitAsync 等
	// 在 Lua 执行期间经 pluginRef 无锁取回插件指针（读 per-plugin currentEv），
	// 不引入 pe.mu → stateMu 的锁序依赖。
	p := &LoadedPlugin{State: L, Dir: pluginDir}
	attachPluginRef(L, p)
	// 先注入 SDK：让插件可以使用 require("jn")
	pe.injectSDK(L, name)
	pe.injectBaseAPI(L, name, manifest.Permissions)
	// 注入命令注册 API（依赖当前 plugin name）
	pe.injectCommandAPI(L, name)

	// 执行插件入口不持锁：LState 尚未发布到 plugins 表，其它 goroutine 不可见；
	// 插件入口内提交异步任务（如 http.get_async）不会与 Load 的写锁互相嵌套
	// （写锁内取读锁 = RWMutex 不可重入死锁）。
	entryFile := filepath.Join(pluginDir, manifest.Entry)
	if err := L.DoFile(entryFile); err != nil {
		L.Close()
		return fmt.Errorf("执行 entry 失败: %w", err)
	}

	p.Manifest = *manifest
	pe.mu.Lock()
	if _, dup := pe.plugins[name]; dup {
		pe.mu.Unlock()
		L.Close()
		return fmt.Errorf("plugin %q already loaded", name)
	}
	pe.plugins[name] = p
	pe.mu.Unlock()
	log.Info("插件加载成功", "name", name, "version", manifest.Version, "system", manifest.System)
	return nil
}

// ErrPluginNotLoaded 标识卸载/重载目标插件未加载（非真实失败）。
// 调用方可借 errors.Is 区分"从未加载"与卸载失败，避免依赖错误文案。
var ErrPluginNotLoaded = errors.New("plugin not loaded")

func (pe *PluginEngine) Unload(name string) error {
	pe.mu.Lock()
	p, ok := pe.plugins[name]
	if !ok {
		pe.mu.Unlock()
		return fmt.Errorf("%w: plugin %q", ErrPluginNotLoaded, name)
	}
	// 系统插件禁止卸载
	if p.Manifest.System {
		pe.mu.Unlock()
		return fmt.Errorf("system 插件 %q 不允许卸载", name)
	}
	// 先从 map 移除（新执行/回调拿不到引用），再移除命令，最后锁外关闭 LState
	delete(pe.plugins, name)
	pe.commands.UnregisterPlugin(name)
	pe.mu.Unlock()

	p.close()
	log.Info("插件已卸载", "name", name)
	return nil
}

func (pe *PluginEngine) Reload(name string) error {
	if err := pe.Unload(name); err != nil {
		return err
	}
	return pe.Load(name)
}

func (pe *PluginEngine) List() []Manifest {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	list := make([]Manifest, 0, len(pe.plugins))
	for _, p := range pe.plugins {
		list = append(list, p.Manifest)
	}
	return list
}

// ListMaps 返回插件列表 (map 格式, 供 Web API 使用)。
// 包含 manifest 元数据 + 权限列表 + 该插件注册的命令。
// 已加载的插件 is_active=true，未加载的（已禁用/未启动）is_active=false。
func (pe *PluginEngine) ListMaps() []map[string]any {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	// 收集已加载的插件名，同时构建输出
	loaded := make(map[string]bool)
	out := make([]map[string]any, 0)
	for name, p := range pe.plugins {
		loaded[name] = true
		m := p.Manifest
		entry := map[string]any{
			"id":               name, // 目录名，用于 API 操作 (toggle/delete)
			"name":             m.Name,
			"version":          m.Version,
			"author":           m.Author,
			"description":      m.Description,
			"permissions":      m.Permissions,
			"is_system":        m.System,
			"is_active":        true, // 已加载即视为 active
			"supports_cronjob": p.SupportsCronJob(),
		}
		// 头像文件修改时间（Unix 秒）：前端用作头像 URL 版本参数，
		// 头像变更后 URL 变化、浏览器缓存自动失效；无头像时不带该字段。
		if info, err := os.Stat(filepath.Join(pe.basePath, name, "avatar.png")); err == nil {
			entry["avatar_mtime"] = info.ModTime().Unix()
		}
		// 使用 Load 时的 name（目录名）查找命令，而非 manifest.Name
		// 因为命令注册使用的 plugin 参数是 Load 的 name 参数
		if pe.commands != nil {
			entry["commands"] = pe.commands.ListByPlugin(name)
		} else {
			entry["commands"] = []map[string]any{}
		}
		out = append(out, entry)
	}

	// 扫描磁盘，添加已存在但未加载的插件（已禁用的）
	entries, err := os.ReadDir(pe.basePath)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == "sdk" || loaded[entry.Name()] {
				continue
			}
			// 读取 pluggin.yaml 获取元数据
			manifest, err := pe.readManifest(filepath.Join(pe.basePath, entry.Name()))
			if err != nil {
				continue
			}
			out = append(out, map[string]any{
				"id":               entry.Name(), // 目录名，用于 API 操作
				"name":             manifest.Name,
				"version":          manifest.Version,
				"author":           manifest.Author,
				"description":      manifest.Description,
				"permissions":      manifest.Permissions,
				"is_system":        manifest.System,
				"is_active":        manifest.Enabled, // 从 YAML enabled 字段读取，兼容旧版默认 true
				"supports_cronjob": false,            // 未加载的插件无法检测，默认 false
				"commands":         []map[string]any{},
			})
		}
	}

	// 稳定排序（按名称），避免 map 随机遍历导致每次刷新顺序乱跳
	sort.Slice(out, func(i, j int) bool {
		ni := strings.ToLower(fmt.Sprintf("%v", out[i]["name"]))
		nj := strings.ToLower(fmt.Sprintf("%v", out[j]["name"]))
		if ni == nj {
			return strings.ToLower(fmt.Sprintf("%v", out[i]["id"])) < strings.ToLower(fmt.Sprintf("%v", out[j]["id"]))
		}
		return ni < nj
	})

	return out
}

// ---------- 事件 ----------

type EventData struct {
	PostType    string         `json:"post_type"`
	MessageType string         `json:"message_type"`
	UserID      int64          `json:"user_id"`
	GroupID     int64          `json:"group_id"`
	RawMessage  string         `json:"raw_message"`
	Admins      []string       `json:"admins"`
	Webhook     map[string]any `json:"webhook,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"` // on_timer_call 携带的 payload

	// --- notice 事件字段 ---
	NoticeType string         `json:"notice_type,omitempty"` // group_upload / group_admin / group_decrease / group_increase / group_ban / friend_add / group_recall / friend_recall / notify
	SubType    string         `json:"sub_type,omitempty"`    // 子类型
	OperatorID int64          `json:"operator_id,omitempty"` // 操作者 QQ
	TargetID   int64          `json:"target_id,omitempty"`   // 被操作者 QQ（禁言/踢人等）
	Duration   int64          `json:"duration,omitempty"`    // 禁言时长（秒）
	File       map[string]any `json:"file,omitempty"`        // 群文件上传信息

	// --- request 事件字段 ---
	RequestType string `json:"request_type,omitempty"` // friend / group
	Comment     string `json:"comment,omitempty"`      // 验证消息
	Flag        string `json:"flag,omitempty"`         // 请求标识（用于同意/拒绝）

	// --- message 事件附加字段 ---
	MessageID int64          `json:"message_id,omitempty"` // 消息 ID
	Sender    map[string]any `json:"sender,omitempty"`     // 发送者信息（nickname/card/sex/age）
}

// DispatchResult 是 Plugin 引擎统一派发结果。
type DispatchResult struct {
	Consumed  bool          // 任一插件 consumed=true：消息不进 Agent（所有 on_message 仍全部执行，不短路）
	Event     adapter.Event // 透传事件（插件不允许修改事件）
	SkipReply bool          // skip_reply 标记：跳过回复策略检查直接给 Agent（强制必回）
}

// HasPluginCommand 检查消息是否匹配已注册的插件命令（不执行，仅供策略层判断）。
func (pe *PluginEngine) HasPluginCommand(raw string) bool {
	return pe.commands.HasCommand(raw)
}

// sendReply 内部辅助：根据 event 的 message_type 回复到对应会话。
// 仅当 PluginEngine 持有 adapter 时有效。
func (pe *PluginEngine) sendReply(event EventData, content string) {
	if pe.adapter == nil {
		return
	}
	switch event.MessageType {
	case "private":
		_, _ = pe.adapter.SendPrivateMsg(event.UserID, content)
	case "group":
		_, _ = pe.adapter.SendGroupMsg(event.GroupID, content)
	}
}

// OnWebhook 触发插件的 on_webhook 回调。
// 当 webhook adapter 收到事件时调用此方法。
// 插件可以在 on_webhook 中返回 true 表示已消费事件。
func (pe *PluginEngine) OnWebhook(event EventData) (consumed bool) {
	for _, p := range pe.snapshot() {
		if !p.HasPermission("webhook") {
			continue
		}
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		// 设置该插件当前事件（回调内 jn.agent API 读到本事件）
		p.currentEv = event
		fn := p.State.GetGlobal("on_webhook")
		if fn.Type() != lua.LTFunction {
			p.stateMu.Unlock()
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 2, nil); err != nil {
			log.Error("插件 on_webhook 错误", "plugin", p.Manifest.Name, "err", err)
			p.stateMu.Unlock()
			continue
		}
		consumedRet := p.State.Get(-2)
		p.State.Pop(2)
		p.stateMu.Unlock()
		if consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool)) {
			return true
		}
	}
	return false
}

// ListWebhookPlugins 返回所有启用 webhook 的插件及其 URL 路径（GET /webhook 列表用）。
// 判定条件：已加载 + 拥有 webhook 权限 + 定义了 on_webhook 回调。
// GetGlobal 在插件 stateMu 下执行（与异步回调互斥，LState 非线程安全）。
func (pe *PluginEngine) ListWebhookPlugins() []adapter.WebhookPluginInfo {
	var out []adapter.WebhookPluginInfo
	for _, p := range pe.snapshot() {
		if !p.HasPermission("webhook") {
			continue
		}
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		fn := p.State.GetGlobal("on_webhook")
		hasFn := fn.Type() == lua.LTFunction
		p.stateMu.Unlock()
		if !hasFn {
			continue
		}
		out = append(out, adapter.WebhookPluginInfo{
			Name:    p.Manifest.Name,
			Path:    "/webhook/" + p.Manifest.Name,
			Enabled: true,
		})
	}
	return out
}

// RouteWebhook routes a webhook request to a specific plugin by name.
// Returns (consumed, reply).
func (pe *PluginEngine) RouteWebhook(pluginName string, path string, method string, payload map[string]any) (consumed bool, reply string) {
	p := pe.pluginByName(pluginName)
	if p == nil {
		return false, "plugin not found"
	}
	if !p.HasPermission("webhook") {
		return false, "plugin does not have webhook permission"
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.closed {
		return false, "plugin is closed"
	}
	event := EventData{
		PostType: "webhook",
		Webhook: map[string]any{
			"path":    path,
			"method":  method,
			"payload": payload,
		},
	}
	fn := p.State.GetGlobal("on_webhook")
	if fn.Type() != lua.LTFunction {
		return false, "plugin has no on_webhook handler"
	}
	// 设置该插件当前事件（回调内 jn.agent API 读到本事件）
	p.currentEv = event
	table := eventToLuaTable(p.State, event)
	p.State.Push(fn)
	p.State.Push(table)
	if err := p.State.PCall(1, 2, nil); err != nil {
		return false, err.Error()
	}
	replyRet := p.State.Get(-1)
	consumedRet := p.State.Get(-2)
	p.State.Pop(2)

	r := ""
	if replyRet.Type() == lua.LTString {
		r = string(replyRet.(lua.LString))
	}
	c := consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool))
	return c, r
}

// OnNotice 通知事件（群成员增减、禁言、文件上传等）。
func (pe *PluginEngine) OnNotice(event EventData) {
	for _, p := range pe.snapshot() {
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		// 设置该插件当前事件（回调内 jn.agent API 读到本事件）
		p.currentEv = event
		fn := p.State.GetGlobal("on_notice")
		if fn.Type() != lua.LTFunction {
			p.stateMu.Unlock()
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 0, nil); err != nil {
			log.Error("插件 on_notice 错误", "plugin", p.Manifest.Name, "err", err)
		}
		p.stateMu.Unlock()
	}
}

// OnRequest 请求事件（加好友、加群邀请）。
func (pe *PluginEngine) OnRequest(event EventData) {
	for _, p := range pe.snapshot() {
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		// 设置该插件当前事件（回调内 jn.agent API 读到本事件）
		p.currentEv = event
		fn := p.State.GetGlobal("on_request")
		if fn.Type() != lua.LTFunction {
			p.stateMu.Unlock()
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 0, nil); err != nil {
			log.Error("插件 on_request 错误", "plugin", p.Manifest.Name, "err", err)
		}
		p.stateMu.Unlock()
	}
}

// Dispatch 统一派发所有事件类型给 Plugin。
// Plugin 可以消费事件、修改事件内容、或标记 skip_reply_check。
// snapshot 返回当前已加载插件的快照（短临界区）。
// 调用方在锁外逐个持 p.stateMu 执行 Lua——不再有「持全局锁执行 Lua」的路径，
// 插件的慢调用（同步 OneBot11 API、LLM 等）只阻塞自身，不影响 Web API 与其它插件。
func (pe *PluginEngine) snapshot() []*LoadedPlugin {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	out := make([]*LoadedPlugin, 0, len(pe.plugins))
	for _, p := range pe.plugins {
		out = append(out, p)
	}
	return out
}

// pluginByName 返回指定目录名的已加载插件；未加载返回 nil。
func (pe *PluginEngine) pluginByName(name string) *LoadedPlugin {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.plugins[name]
}

// pluginRefKey LState 全局 userdata 键：Load 时把插件指针挂到 LState。
// 供 Lua 执行期间（injectAgent 读 per-plugin currentEv）无锁取回，
// 不引入 pe.mu → stateMu 的锁序依赖。
const pluginRefKey = "__jn_plugin_ref"

// attachPluginRef 把插件指针挂到 LState 全局（Load 创建 LState 后立即调用）。
func attachPluginRef(L *lua.LState, p *LoadedPlugin) {
	ud := L.NewUserData()
	ud.Value = p
	L.SetGlobal(pluginRefKey, ud)
}

// pluginRef 从 LState 取回插件指针（Lua 执行期间调用；无锁）。
// 手动构造的 LState（如测试辅助）未挂引用时返回 nil。
func pluginRef(L *lua.LState) *LoadedPlugin {
	if ud, ok := L.GetGlobal(pluginRefKey).(*lua.LUserData); ok {
		if p, ok := ud.Value.(*LoadedPlugin); ok {
			return p
		}
	}
	return nil
}

// currentEventOf 返回 LState 所属插件的当前事件（Lua 执行期间调用，持 stateMu）。
// 未挂插件引用时返回零值 EventData（不影响调用方）。
func currentEventOf(L *lua.LState) EventData {
	if p := pluginRef(L); p != nil {
		return p.currentEv
	}
	return EventData{}
}

// execCommand 在插件 stateMu 下执行命令 handler（保证 LState 串行，且不再持有
// pe.mu / commands 锁——handler 内动态注册命令不会触发读锁升级写锁死锁）。
// 插件未加载/已卸载时**不执行**：命令 handler 闭包捕获插件 LState，若插件已被
// Unload 关闭（Match 与执行之间存在窗口），执行会 panic。
// 内置命令（Builtin）是纯 Go 闭包，不触碰 LState，直接执行、不依赖插件加载状态。
func (pe *PluginEngine) execCommand(node *CommandNode, args []string, event EventData) (bool, string, error) {
	if node.Builtin {
		return node.Handler(args, event)
	}
	p := pe.pluginByName(node.PluginName)
	if p == nil {
		return false, "", nil
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.closed {
		return false, "", nil
	}
	// 设置该插件当前事件（命令 handler 内 jn.agent API 可读到本事件）
	p.currentEv = event
	return node.Handler(args, event)
}

func (pe *PluginEngine) Dispatch(ev adapter.Event) DispatchResult {
	switch ev.PostType {
	case "webhook":
		if ev.Webhook != nil {
			pluginEvent := EventData{
				PostType: "webhook",
				Admins:   ev.Admins,
				Webhook: map[string]any{
					"path":    ev.Webhook.Path,
					"method":  ev.Webhook.Method,
					"payload": ev.Webhook.Payload,
				},
			}
			consumed, reply := pe.onWebhook(pluginEvent)
			if reply != "" {
				pe.sendWebhookReply(ev, reply)
			}
			return DispatchResult{Consumed: consumed, Event: ev}
		}
	case "notice":
		if ev.Notice != nil {
			n := ev.Notice
			pluginEvent := EventData{
				PostType:   "notice",
				NoticeType: n.NoticeType,
				SubType:    n.SubType,
				UserID:     n.UserID,
				GroupID:    n.GroupID,
				OperatorID: n.OperatorID,
				TargetID:   n.TargetID,
				Duration:   n.Duration,
				Admins:     ev.Admins,
			}
			if n.File != nil {
				pluginEvent.File = map[string]any{
					"id": n.File.ID, "name": n.File.Name,
					"size": n.File.Size, "busid": n.File.BusID,
				}
			}
			consumed := pe.onNotice(pluginEvent)
			return DispatchResult{Consumed: consumed, Event: ev}
		}
	case "request":
		if ev.Request != nil {
			r := ev.Request
			pluginEvent := EventData{
				PostType:    "request",
				RequestType: r.RequestType,
				SubType:     r.SubType,
				UserID:      r.UserID,
				GroupID:     r.GroupID,
				Comment:     r.Comment,
				Flag:        r.Flag,
				Admins:      ev.Admins,
			}
			consumed := pe.onRequest(pluginEvent)
			return DispatchResult{Consumed: consumed, Event: ev}
		}
	case "cronjob":
		pluginEvent := EventData{
			PostType: "cronjob",
			Admins:   ev.Admins,
		}
		if ev.Message != nil {
			pluginEvent.RawMessage = ev.Message.RawMessage
			pluginEvent.MessageType = ev.Message.MessageType
			pluginEvent.UserID = ev.Message.UserID
			pluginEvent.GroupID = ev.Message.GroupID
		}
		// 解析 CronJob 配置的 Payload（JSON 字符串 → map，透传给 on_cronjob）
		if ev.CronJobPayload != "" {
			var p map[string]any
			if err := json.Unmarshal([]byte(ev.CronJobPayload), &p); err == nil {
				pluginEvent.Payload = p
			} else {
				log.Warn("CronJob: payload 解析失败", "payload", ev.CronJobPayload, "err", err)
			}
		}
		consumed := pe.onCronJob(pluginEvent, ev.CronJobPluginIDs)
		return DispatchResult{Consumed: consumed, Event: ev}
	case "message":
		if ev.Message != nil {
			return pe.dispatchMessage(ev)
		}
	}

	return DispatchResult{Event: ev}
}

// dispatchMessage 消息事件的统一派发（锁外执行，per-plugin stateMu 串行）。
// 派发规则：
//  1. 命令注册表优先：/cmd 命中即 Consumed（命令消息不进 Agent）
//  2. 其余消息遍历各插件 on_message 回调，返回 (consumed, skip_reply)：
//     - consumed=true → 消息不进 Agent，但**不短路**，所有插件 on_message 仍全部执行
//     - skip_reply=true → 跳过回复策略检查强制进 Agent（consumed 优先）
//  3. 插件不能修改事件（modified_event 已移除）
func (pe *PluginEngine) dispatchMessage(ev adapter.Event) DispatchResult {
	msg := ev.Message
	pluginEvent := EventData{
		PostType:    "message",
		MessageType: msg.MessageType,
		UserID:      msg.UserID,
		GroupID:     msg.GroupID,
		RawMessage:  msg.RawMessage,
		MessageID:   msg.MessageID,
		Admins:      ev.Admins,
		Sender: map[string]any{
			"user_id":  msg.Sender.UserID,
			"nickname": msg.Sender.Nickname,
			"sex":      msg.Sender.Sex,
			"age":      float64(msg.Sender.Age),
			"card":     msg.Sender.Card,
		},
	}

	// 1. 命令注册表：命中即消费（命令消息不进 Agent）。
	// 匹配在 commands 锁内完成（不执行 Lua），handler 在插件 stateMu 下执行。
	if strings.HasPrefix(strings.TrimSpace(pluginEvent.RawMessage), "/") {
		node, args, hint := pe.commands.Match(pluginEvent.RawMessage)
		var c bool
		var reply string
		var err error
		switch {
		case node != nil:
			c, reply, err = pe.execCommand(node, args, pluginEvent)
		case hint != "":
			c, reply = true, hint
		}
		if err != nil {
			log.Error("命令派发错误", "raw", pluginEvent.RawMessage, "err", err)
		}
		if c || reply != "" {
			if reply != "" {
				pe.sendReply(pluginEvent, reply)
			}
			return DispatchResult{Consumed: true, Event: ev}
		}
	}

	// 2. 派发给各插件的 on_message：不短路，全部执行；收集 consumed 与 skip_reply
	var anyConsumed, anySkipReply bool
	for _, p := range pe.snapshot() {
		if !p.HasPermission("onebot11") {
			continue
		}
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		// 设置该插件当前事件：回调内 jn.agent.get_current_chat_area 读到本事件
		p.currentEv = pluginEvent
		fn := p.State.GetGlobal("on_message")
		if fn.Type() != lua.LTFunction {
			p.stateMu.Unlock()
			continue
		}
		table := eventToLuaTable(p.State, pluginEvent)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 2, nil); err != nil { // 返回值: consumed, skip_reply
			log.Error("插件 on_message 错误", "plugin", p.Manifest.Name, "err", err)
			p.stateMu.Unlock()
			continue
		}

		consumedRet := p.State.Get(-2)
		skipReplyRet := p.State.Get(-1)
		p.State.Pop(2)
		p.stateMu.Unlock()

		if consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool)) {
			anyConsumed = true
		}
		if skipReplyRet.Type() == lua.LTBool && bool(skipReplyRet.(lua.LBool)) {
			anySkipReply = true
		}
	}

	return DispatchResult{Consumed: anyConsumed, Event: ev, SkipReply: anySkipReply}
}

// onCronJob 派发 cronjob 事件给插件（锁外执行）。
// pluginIDs 指定要通知的插件目录名列表（与 map key / p.Dir 目录名一致，
// 不是 Manifest.Name 显示名）；为空则通知所有插件。
func (pe *PluginEngine) onCronJob(event EventData, pluginIDs []string) bool {
	for _, p := range pe.snapshot() {
		// 按 CronJob 配置的插件列表过滤
		if len(pluginIDs) > 0 && !containsString(pluginIDs, filepath.Base(p.Dir)) {
			continue
		}
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		// 设置该插件当前事件（回调内 jn.agent API 读到本事件）
		p.currentEv = event
		fn := p.State.GetGlobal("on_cronjob")
		if fn.Type() != lua.LTFunction {
			p.stateMu.Unlock()
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 1, nil); err != nil {
			log.Error("插件 on_cronjob 错误", "plugin", p.Manifest.Name, "err", err)
			p.stateMu.Unlock()
			continue
		}
		consumedRet := p.State.Get(-1)
		p.State.Pop(1)
		p.stateMu.Unlock()
		if consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool)) {
			return true
		}
	}
	return false
}

// containsString 检查字符串切片中是否包含目标值。
func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// onNotice 派发 notice 事件给插件（锁外执行，per-plugin stateMu 串行）。
func (pe *PluginEngine) onNotice(event EventData) bool {
	for _, p := range pe.snapshot() {
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		// 设置该插件当前事件（回调内 jn.agent API 读到本事件）
		p.currentEv = event
		fn := p.State.GetGlobal("on_notice")
		if fn.Type() != lua.LTFunction {
			p.stateMu.Unlock()
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 1, nil); err != nil {
			log.Error("插件 on_notice 错误", "plugin", p.Manifest.Name, "err", err)
			p.stateMu.Unlock()
			continue
		}
		consumedRet := p.State.Get(-1)
		p.State.Pop(1)
		p.stateMu.Unlock()
		if consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool)) {
			return true
		}
	}
	return false
}

// onRequest 派发 request 事件给插件（锁外执行，per-plugin stateMu 串行）。
func (pe *PluginEngine) onRequest(event EventData) bool {
	for _, p := range pe.snapshot() {
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		// 设置该插件当前事件（回调内 jn.agent API 读到本事件）
		p.currentEv = event
		fn := p.State.GetGlobal("on_request")
		if fn.Type() != lua.LTFunction {
			p.stateMu.Unlock()
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 1, nil); err != nil {
			log.Error("插件 on_request 错误", "plugin", p.Manifest.Name, "err", err)
			p.stateMu.Unlock()
			continue
		}
		consumedRet := p.State.Get(-1)
		p.State.Pop(1)
		p.stateMu.Unlock()
		if consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool)) {
			return true
		}
	}
	return false
}

// onWebhook 派发 webhook 事件给插件（锁外执行，per-plugin stateMu 串行）。
func (pe *PluginEngine) onWebhook(event EventData) (consumed bool, reply string) {
	for _, p := range pe.snapshot() {
		if !p.HasPermission("webhook") {
			continue
		}
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		// 设置该插件当前事件（回调内 jn.agent API 读到本事件）
		p.currentEv = event
		fn := p.State.GetGlobal("on_webhook")
		if fn.Type() != lua.LTFunction {
			p.stateMu.Unlock()
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 2, nil); err != nil {
			log.Error("插件 on_webhook 错误", "plugin", p.Manifest.Name, "err", err)
			p.stateMu.Unlock()
			continue
		}
		replyRet := p.State.Get(-1)
		consumedRet := p.State.Get(-2)
		p.State.Pop(2)
		p.stateMu.Unlock()
		if consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool)) {
			r := ""
			if replyRet.Type() == lua.LTString {
				r = string(replyRet.(lua.LString))
			}
			return true, r
		}
	}
	return false, ""
}

// sendWebhookReply 将 webhook 处理结果回传给 webhook adapter。
func (pe *PluginEngine) sendWebhookReply(ev adapter.Event, reply string) {
	// Webhook reply 由 webhook adapter 层处理，这里仅记录日志。
	log.Info("Webhook reply", "reply_len", len(reply))
}

// SupportsCronJob 检查插件是否支持定时任务回调（定义了 on_cronjob 全局函数）。
// 在插件 stateMu 下读取 LState（与异步回调互斥，LState 非线程安全）。
func (p *LoadedPlugin) SupportsCronJob() bool {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.closed {
		return false
	}
	fn := p.State.GetGlobal("on_cronjob")
	return fn.Type() == lua.LTFunction
}

// ReloadAll 卸载所有非系统插件后重新加载全部插件。
func (pe *PluginEngine) ReloadAll() error {
	var toClose []*LoadedPlugin
	pe.mu.Lock()
	// 先卸载非系统插件
	for name, p := range pe.plugins {
		if p.Manifest.System {
			continue
		}
		pe.commands.UnregisterPlugin(name)
		delete(pe.plugins, name)
		toClose = append(toClose, p)
	}
	pe.mu.Unlock()

	// 锁外逐个关闭 LState（等待在途回调完成），再加载全部插件
	for _, p := range toClose {
		p.close()
	}
	return pe.LoadAll()
}

func (p *LoadedPlugin) HasPermission(perm string) bool {
	for _, pp := range p.Manifest.Permissions {
		if pp == perm || pp == "*" {
			return true
		}
	}
	return false
}

// ====================================================================
// API 注入
// ====================================================================

func (pe *PluginEngine) injectBaseAPI(L *lua.LState, pluginName string, permissions []string) {
	hasPerm := func(p string) bool {
		for _, pp := range permissions {
			if pp == p || pp == "*" {
				return true
			}
		}
		return false
	}

	// 异步调用现场注册表：req_id → ctx table（WithCtx 异步 API 使用）。
	// 每个 LState 独立，插件重载/卸载时随 LState 销毁自动清理。
	L.SetGlobal("jn_async_ctx", L.NewTable())

	// Logger
	logTable := L.NewTable()
	L.SetFuncs(logTable, map[string]lua.LGFunction{
		"info": func(L *lua.LState) int {
			log.Info("[plugin:"+pluginName+"]", "msg", L.CheckString(1))
			return 0
		},
		"warn": func(L *lua.LState) int {
			log.Warn("[plugin:"+pluginName+"]", "msg", L.CheckString(1))
			return 0
		},
		"error": func(L *lua.LState) int {
			log.Error("[plugin:"+pluginName+"]", "msg", L.CheckString(1))
			return 0
		},
	})
	L.SetGlobal("log", logTable)

	// JSON
	pe.injectJSON(L)

	// OneBot11
	if hasPerm("onebot11") {
		pe.injectOneBot11(L, pluginName)
	}

	// HTTP
	if hasPerm("http") {
		pe.injectHTTP(L, pluginName)
	}

	// Database
	if hasPerm("database") && pe.db != nil {
		pe.injectDatabase(L, pluginName)
	}

	// Cache
	if hasPerm("cache") && pe.cache != nil {
		pe.injectCache(L, pluginName)
	}

	// T2I
	if hasPerm("t2i") {
		pe.injectT2I(L, pluginName)
	}

	// Sandbox
	if hasPerm("sandbox") {
		pe.injectSandbox(L, pluginName)
	}

	// RAG（向量检索服务）
	if hasPerm("rag") {
		pe.injectRAG(L, pluginName)
	}

	// Agent
	if hasPerm("agent") && pe.dao != nil {
		pe.injectAgent(L)
	}

	// LLM：调用 Bot 自身 LLM（复用 Bot Provider 配置，需要 llm 权限）
	if hasPerm("llm") && pe.llmAccess != nil {
		pe.injectLLM(L, pluginName)
	}

	// Timer：异步延时（需要 timer 权限）
	if hasPerm("timer") {
		pe.injectTimer(L, pluginName)
	}

	// File：插件目录内文本文件读写（需要 file 权限）
	if hasPerm("file") {
		pe.injectFileAPI(L, pluginName)
	}

	// Config：动态配置（无需权限，默认注入）
	pe.injectConfigAPI(L, pluginName)
}

// injectSDK 将 jn.lua 内容写入 package.preload["jn"]，
// 插件后续 require("jn") 时即获得带类型注解的 SDK 表。
// SDK 内部捕获此处注入的全局 (log/json/onebot11/...) 作为字段。
func (pe *PluginEngine) injectSDK(L *lua.LState, pluginName string) {
	sdkDir := filepath.Join(pe.basePath, "sdk")
	// 添加到 package.path，使 IDE 与 require 都能找到
	pathScript := fmt.Sprintf(`package.path = "%s/?.lua;" .. (package.path or "")`,
		strings.ReplaceAll(sdkDir, "\\", "/"))
	if err := L.DoString(pathScript); err != nil {
		log.Warn("设置 package.path 失败", "plugin", pluginName, "err", err)
	}
}

// injectCommandAPI 注入 jn.command.register 的底层绑定。
// SDK (jn.lua) 通过此全局函数注册命令到 PluginEngine.commands。
func (pe *PluginEngine) injectCommandAPI(L *lua.LState, pluginName string) {
	registry := pe.commands
	internal := L.NewTable()
	L.SetFuncs(internal, map[string]lua.LGFunction{
		"register_command": func(L *lua.LState) int {
			// 参数: path (string|table), handler (function), opts (table, optional)
			pathArg := L.Get(1)
			handlerFn := L.Get(2)
			optsArg := L.Get(3)

			var path []string
			switch p := pathArg.(type) {
			case lua.LString:
				path = strings.Fields(string(p))
			case *lua.LTable:
				p.ForEach(func(_, v lua.LValue) {
					path = append(path, v.String())
				})
			default:
				L.Push(lua.LBool(false))
				L.Push(lua.LString("path 必须是字符串或字符串数组"))
				return 2
			}
			if len(path) == 0 {
				L.Push(lua.LBool(false))
				L.Push(lua.LString("path 不能为空"))
				return 2
			}
			opts := CommandOpts{}
			if optsArg.Type() == lua.LTTable {
				t := optsArg.(*lua.LTable)
				if d := t.RawGetString("description"); d.Type() == lua.LTString {
					opts.Description = string(d.(lua.LString))
				}
				if u := t.RawGetString("usage"); u.Type() == lua.LTString {
					opts.Usage = string(u.(lua.LString))
				}
			}

			plugin := pluginName
			var handler CommandHandler

			if handlerFn.Type() == lua.LTFunction {
				// 保留 handler 引用，防止被 GC
				refKey := fmt.Sprintf("__jn_cmd_handler_%s_%s", pluginName, strings.Join(path, "_"))
				L.SetGlobal(refKey, handlerFn)

				handler = CommandHandler(func(args []string, event EventData) (bool, string, error) {
					// 在插件 LState 中调用 handler
					argTable := L.NewTable()
					for i, a := range args {
						L.SetTable(argTable, lua.LNumber(i+1), lua.LString(a))
					}
					evTable := eventToLuaTable(L, event)
					if err := L.CallByParam(lua.P{
						Fn:      handlerFn,
						NRet:    2,
						Protect: true,
					}, argTable, evTable); err != nil {
						return true, "", err
					}
					retConsumed := L.Get(-2)
					retReply := L.Get(-1)
					L.Pop(2)
					consumed := false
					if retConsumed.Type() == lua.LTBool {
						consumed = bool(retConsumed.(lua.LBool))
					}
					reply := ""
					if retReply.Type() == lua.LTString {
						reply = string(retReply.(lua.LString))
					}
					return consumed, reply, nil
				})
			}
			// handler==nil 表示仅作为分组节点，不直接执行

			registry.Register(plugin, path, opts, handler)
			L.Push(lua.LBool(true))
			return 1
		},
	})
	L.SetGlobal("__jn_internal", internal)
}

// ---------- JSON ----------

func (pe *PluginEngine) injectJSON(L *lua.LState) {
	jsonTable := L.NewTable()
	L.SetFuncs(jsonTable, map[string]lua.LGFunction{
		"encode": func(L *lua.LState) int {
			val := luaValueToGo(L.Get(1))
			data, err := json.Marshal(val)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LString(string(data)))
			return 1
		},
		"decode": func(L *lua.LState) int {
			str := L.CheckString(1)
			var val any
			if err := json.Unmarshal([]byte(str), &val); err != nil {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(goToLuaValue(L, val))
			return 1
		},
	})
	L.SetGlobal("json", jsonTable)
}

// luaMsgID 将 Lua 的 message_id 参数转为 int64，兼容字符串（事件表传递格式）与数字（旧插件直传）。
// 事件表 message_id 以字符串传递：QQ message_id 可达 19 位（> 2^53），
// 用 Lua number（float64）会丢精度，导致 delete_msg/get_msg 撤回或查询失败。
func luaMsgID(v lua.LValue) (int64, error) {
	switch t := v.(type) {
	case lua.LString:
		return strconv.ParseInt(string(t), 10, 64)
	case lua.LNumber:
		return int64(t), nil
	default:
		return 0, fmt.Errorf("message_id 必须是数字或字符串，got %s", v.Type())
	}
}

// luaTableToSegments 将 Lua 的 {{type="text",data={text="hello"}}, ...} 转为 []adapter.Segment
func luaTableToSegments(L *lua.LState, tbl *lua.LTable) []adapter.Segment {
	var segs []adapter.Segment
	tbl.ForEach(func(_, segVal lua.LValue) {
		segTable, ok := segVal.(*lua.LTable)
		if !ok {
			return
		}
		seg := adapter.Segment{Data: make(map[string]any)}
		if t := segTable.RawGetString("type"); t.Type() == lua.LTString {
			seg.Type = string(t.(lua.LString))
		}
		if d := segTable.RawGetString("data"); d.Type() == lua.LTTable {
			d.(*lua.LTable).ForEach(func(k, v lua.LValue) {
				if v.Type() == lua.LTString {
					seg.Data[k.String()] = string(v.(lua.LString))
				} else if v.Type() == lua.LTNumber {
					seg.Data[k.String()] = float64(v.(lua.LNumber))
				}
			})
		}
		segs = append(segs, seg)
	})
	return segs
}

// ---------- OneBot11 ----------

func (pe *PluginEngine) injectOneBot11(L *lua.LState, pluginName string) {
	if pe.adapter == nil {
		return
	}
	sendAdp := pe.adapter

	// resolveImage 解析图片路径。URL/base64 直接透传，相对路径从插件目录读取并转 base64。
	// imgs:// 是图床图片引用，由 adapter 发送层统一解析，这里原样透传。
	resolveImage := func(path string) string {
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "base64://") || strings.HasPrefix(path, "imgs://") {
			return path
		}
		pluginDir := filepath.Join(pe.basePath, pluginName)
		fullPath := filepath.Join(pluginDir, path)
		t0 := time.Now()
		data, err := os.ReadFile(fullPath)
		readDur := time.Since(t0)
		if err != nil {
			log.Warn("读取插件图片文件失败", "plugin", pluginName, "path", fullPath, "err", err)
			return path
		}
		ext := strings.TrimPrefix(filepath.Ext(fullPath), ".")
		if ext == "" {
			ext = "png"
		}
		t1 := time.Now()
		b64 := "base64://" + base64.StdEncoding.EncodeToString(data)
		encDur := time.Since(t1)
		log.Debug("插件图片处理耗时", "plugin", pluginName, "file", path, "size_bytes", len(data),
			"read_us", readDur.Microseconds(), "encode_us", encDur.Microseconds(), "total_us", time.Since(t0).Microseconds())
		return b64
	}

	// buildSegments 将 Lua table 转为 []Segment，自动解析 image 的 file 字段。
	buildSegments := func(tbl *lua.LTable) []adapter.Segment {
		segs := luaTableToSegments(L, tbl)
		for i := range segs {
			if segs[i].Type == "image" {
				if file, ok := segs[i].Data["file"].(string); ok && file != "" {
					segs[i].Data["file"] = resolveImage(file)
				}
			}
		}
		return segs
	}

	// buildForwardNodes 将 Lua 合并转发节点数组转为 []adapter.ForwardNode：
	//   构造节点 {user_id=…, nickname=“…”, content=“文本或消息段数组”}
	//   引用节点 {id=消息ID}
	buildForwardNodes := func(tbl *lua.LTable) []adapter.ForwardNode {
		nodes := make([]adapter.ForwardNode, 0, tbl.Len())
		tbl.ForEach(func(_, v lua.LValue) {
			nt, ok := v.(*lua.LTable)
			if !ok {
				return
			}
			var n adapter.ForwardNode
			if id := nt.RawGetString("id"); id.Type() == lua.LTNumber {
				n.ID = int64(id.(lua.LNumber))
			}
			if u := nt.RawGetString("user_id"); u.Type() == lua.LTNumber {
				n.Uin = int64(u.(lua.LNumber))
			}
			if nm := nt.RawGetString("nickname"); nm.Type() == lua.LTString {
				n.Name = string(nm.(lua.LString))
			}
			if c := nt.RawGetString("content"); c != lua.LNil {
				if ct, ok := c.(*lua.LTable); ok {
					n.Content = buildSegments(ct)
				} else {
					n.Content = c.String()
				}
			}
			nodes = append(nodes, n)
		})
		return nodes
	}

	// applyReplyTo 在消息前插入引用回复段（可选第 3 参数 reply_to=消息ID）：
	// 字符串消息含 CQ 码时先解析为段数组再前插；纯文本与段数组统一前插 Reply 段。
	// reply_to 为 nil/非法时原样返回消息。
	applyReplyTo := func(msg any, replyTo lua.LValue) any {
		if replyTo == lua.LNil {
			return msg
		}
		id, err := luaMsgID(replyTo)
		if err != nil {
			log.Warn("插件 reply_to 参数无效，忽略引用", "reply_to", replyTo.String(), "err", err)
			return msg
		}
		replySeg := adapter.Reply(strconv.FormatInt(id, 10))
		switch v := msg.(type) {
		case string:
			if adapter.HasCQCode(v) {
				return append([]adapter.Segment{replySeg}, adapter.ParseCQCodes(v)...)
			}
			return []adapter.Segment{replySeg, adapter.Text(v)}
		case []adapter.Segment:
			return append([]adapter.Segment{replySeg}, v...)
		default:
			return msg
		}
	}

	obTable := L.NewTable()
	funcs := map[string]lua.LGFunction{
		"send_private_msg": func(L *lua.LState) int {
			userID := int64(L.CheckNumber(1))
			arg := L.Get(2)
			var msg any
			if arg.Type() == lua.LTTable {
				msg = buildSegments(arg.(*lua.LTable))
			} else if arg.Type() == lua.LTString {
				msg = string(arg.(lua.LString))
			} else {
				msg = arg.String()
			}
			msg = applyReplyTo(msg, L.Get(3)) // 可选 reply_to=被引用消息ID
			// 插件发消息异步，不阻塞命令 handler 返回
			go func() {
				t0 := time.Now()
				if _, err := sendAdp.SendPrivateMsg(userID, msg); err != nil {
					log.Warn("插件异步发送私聊消息失败", "plugin", pluginName, "user_id", userID, "err", err)
				} else {
					log.Debug("插件异步发送私聊消息完成", "plugin", pluginName, "user_id", userID, "dur_ms", time.Since(t0).Milliseconds())
				}
			}()
			return pushOk(L)
		},
		"send_group_msg": func(L *lua.LState) int {
			groupID := int64(L.CheckNumber(1))
			arg := L.Get(2)
			var msg any
			if arg.Type() == lua.LTTable {
				msg = buildSegments(arg.(*lua.LTable))
			} else if arg.Type() == lua.LTString {
				msg = string(arg.(lua.LString))
			} else {
				msg = arg.String()
			}
			msg = applyReplyTo(msg, L.Get(3)) // 可选 reply_to=被引用消息ID
			// 插件发消息异步，不阻塞命令 handler 返回
			go func() {
				t0 := time.Now()
				if _, err := sendAdp.SendGroupMsg(groupID, msg); err != nil {
					log.Warn("插件异步发送群消息失败", "plugin", pluginName, "group_id", groupID, "err", err)
				} else {
					log.Debug("插件异步发送群消息完成", "plugin", pluginName, "group_id", groupID, "dur_ms", time.Since(t0).Milliseconds())
				}
			}()
			return pushOk(L)
		},
		// send_group_sticker / send_private_sticker 发送表情包库表情（短 UUID）。
		// 底层由 adapter 把 stk://<短UUID> 解析为 base64（subType=1），插件只接触表情 ID。
		"send_group_sticker": func(L *lua.LState) int {
			groupID := int64(L.CheckNumber(1))
			stickerID := L.CheckString(2)
			go func() {
				msg := fmt.Sprintf("[CQ:image,file=stk://%s,subType=1]", stickerID)
				if _, err := sendAdp.SendGroupMsg(groupID, msg); err != nil {
					log.Warn("插件异步发送群表情失败", "plugin", pluginName, "group_id", groupID, "sticker", stickerID, "err", err)
				}
			}()
			return pushOk(L)
		},
		"send_private_sticker": func(L *lua.LState) int {
			userID := int64(L.CheckNumber(1))
			stickerID := L.CheckString(2)
			go func() {
				msg := fmt.Sprintf("[CQ:image,file=stk://%s,subType=1]", stickerID)
				if _, err := sendAdp.SendPrivateMsg(userID, msg); err != nil {
					log.Warn("插件异步发送私聊表情失败", "plugin", pluginName, "user_id", userID, "sticker", stickerID, "err", err)
				}
			}()
			return pushOk(L)
		},
		"send_private_msg_sync": func(L *lua.LState) int {
			userID := int64(L.CheckNumber(1))
			arg := L.Get(2)
			var msg any
			if arg.Type() == lua.LTTable {
				msg = buildSegments(arg.(*lua.LTable))
			} else if arg.Type() == lua.LTString {
				msg = string(arg.(lua.LString))
			} else {
				msg = arg.String()
			}
			msg = applyReplyTo(msg, L.Get(3)) // 可选 reply_to=被引用消息ID
			_, err := sendAdp.SendPrivateMsg(userID, msg)
			return pushResult(L, err)
		},
		"send_group_msg_sync": func(L *lua.LState) int {
			groupID := int64(L.CheckNumber(1))
			arg := L.Get(2)
			var msg any
			if arg.Type() == lua.LTTable {
				msg = buildSegments(arg.(*lua.LTable))
			} else if arg.Type() == lua.LTString {
				msg = string(arg.(lua.LString))
			} else {
				msg = arg.String()
			}
			msg = applyReplyTo(msg, L.Get(3)) // 可选 reply_to=被引用消息ID
			_, err := sendAdp.SendGroupMsg(groupID, msg)
			return pushResult(L, err)
		},
		// send_group_forward_msg / _sync 发送合并转发消息（转发卡片）：
		//   构造节点 {user_id=…, nickname=“…”, content=“文本或消息段数组”}
		//   引用节点 {id=群内已有消息ID}
		"send_group_forward_msg": func(L *lua.LState) int {
			groupID := int64(L.CheckNumber(1))
			nodes := buildForwardNodes(L.CheckTable(2))
			if len(nodes) == 0 {
				return pushResult(L, fmt.Errorf("合并转发节点不能为空"))
			}
			// 插件发消息异步，不阻塞命令 handler 返回
			go func() {
				if _, err := sendAdp.SendGroupForwardMsg(groupID, nodes); err != nil {
					log.Warn("插件异步发送群合并转发失败", "plugin", pluginName, "group_id", groupID, "nodes", len(nodes), "err", err)
				}
			}()
			return pushOk(L)
		},
		"send_group_forward_msg_sync": func(L *lua.LState) int {
			groupID := int64(L.CheckNumber(1))
			nodes := buildForwardNodes(L.CheckTable(2))
			if len(nodes) == 0 {
				return pushResult(L, fmt.Errorf("合并转发节点不能为空"))
			}
			id, err := sendAdp.SendGroupForwardMsg(groupID, nodes)
			if err != nil {
				return pushResult(L, err)
			}
			L.Push(lua.LNumber(id))
			return 1
		},
		"delete_msg": func(L *lua.LState) int {
			id, err := luaMsgID(L.Get(1))
			if err != nil {
				L.Push(lua.LBool(false))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			err = sendAdp.DeleteMsg(id)
			return pushResult(L, err)
		},
		"get_msg": func(L *lua.LState) int {
			id, err := luaMsgID(L.Get(1))
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			msg, err := sendAdp.GetMsg(id)
			return pushResultJSON(L, msg, err)
		},
		"get_group_info": func(L *lua.LState) int {
			info, err := sendAdp.GetGroupInfo(int64(L.CheckNumber(1)))
			return pushResultJSON(L, info, err)
		},
		"get_group_member_list": func(L *lua.LState) int {
			list, err := sendAdp.GetGroupMemberList(int64(L.CheckNumber(1)))
			return pushResultJSON(L, list, err)
		},
		"get_group_member_info": func(L *lua.LState) int {
			info, err := sendAdp.GetGroupMemberInfo(int64(L.CheckNumber(1)), int64(L.CheckNumber(2)))
			return pushResultJSON(L, info, err)
		},
		"get_group_honor_info": func(L *lua.LState) int {
			info, err := sendAdp.GetGroupHonorInfo(int64(L.CheckNumber(1)))
			return pushResultJSON(L, info, err)
		},
		"kick_group_member": func(L *lua.LState) int {
			n := L.GetTop()
			reject := false
			if n >= 3 {
				reject = bool(L.CheckBool(3))
			}
			err := sendAdp.KickGroupMember(int64(L.CheckNumber(1)), int64(L.CheckNumber(2)), reject)
			return pushResult(L, err)
		},
		"ban_group_member": func(L *lua.LState) int {
			err := sendAdp.BanGroupMember(int64(L.CheckNumber(1)), int64(L.CheckNumber(2)), int(L.CheckInt(3)))
			return pushResult(L, err)
		},
		"set_group_whole_ban": func(L *lua.LState) int {
			err := sendAdp.SetGroupWholeBan(int64(L.CheckNumber(1)), bool(L.CheckBool(2)))
			return pushResult(L, err)
		},
		"set_group_card": func(L *lua.LState) int {
			err := sendAdp.SetGroupCard(int64(L.CheckNumber(1)), int64(L.CheckNumber(2)), L.CheckString(3))
			return pushResult(L, err)
		},
		"handle_friend_request": func(L *lua.LState) int {
			err := sendAdp.HandleFriendRequest(L.CheckString(1), bool(L.CheckBool(2)), L.CheckString(3))
			return pushResult(L, err)
		},
		"handle_group_request": func(L *lua.LState) int {
			err := sendAdp.HandleGroupRequest(L.CheckString(1), L.CheckString(2), bool(L.CheckBool(3)), L.CheckString(4))
			return pushResult(L, err)
		},
		"get_login_info": func(L *lua.LState) int {
			info, err := sendAdp.GetLoginInfo()
			return pushResultJSON(L, info, err)
		},
		"get_stranger_info": func(L *lua.LState) int {
			info, err := sendAdp.GetStrangerInfo(int64(L.CheckNumber(1)))
			return pushResultJSON(L, info, err)
		},
		"get_friend_list": func(L *lua.LState) int {
			list, err := sendAdp.GetFriendList()
			return pushResultJSON(L, list, err)
		},
		"get_group_list": func(L *lua.LState) int {
			list, err := sendAdp.GetGroupList()
			return pushResultJSON(L, list, err)
		},
		"send_like": func(L *lua.LState) int {
			err := sendAdp.SendLike(int64(L.CheckNumber(1)), int(L.CheckInt(2)))
			return pushResult(L, err)
		},
		"get_status": func(L *lua.LState) int {
			s, err := sendAdp.GetStatus()
			return pushResultJSON(L, s, err)
		},
		"get_version_info": func(L *lua.LState) int {
			v, err := sendAdp.GetVersionInfo()
			return pushResultJSON(L, v, err)
		},
		"read_file_base64": func(L *lua.LState) int {
			filePath := L.CheckString(1)
			pluginDir := filepath.Join(pe.basePath, pluginName)
			fullPath := filepath.Join(pluginDir, filePath)
			data, err := os.ReadFile(fullPath)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LString("base64://" + base64.StdEncoding.EncodeToString(data)))
			return 1
		},
	}
	L.SetFuncs(obTable, funcs)
	L.SetGlobal("onebot11", obTable)
}

// ---------- HTTP ----------

// httpAsyncTimeout http 异步任务总超时（httpClient 内建 30s，此处兜底）。
const httpAsyncTimeout = 60 * time.Second

func (pe *PluginEngine) injectHTTP(L *lua.LState, pluginName string) {
	// 按代理地址缓存 http.Client；Transport 构造（HTTP/1.1 强制 + 代理拨号）
	// 见 http_client.go::buildHTTPTransport。proxy 参数为可选字符串：
	//   空 / http(s)://host:port / socks4://host:port / socks4a://host:port / socks5://[user:pass@]host:port
	clientCache := newHTTPClientCache()

	// parseStringTable 将 Lua 字符串表转为 map[string]string（如 headers）。
	parseStringTable := func(tbl *lua.LTable) map[string]string {
		m := make(map[string]string)
		tbl.ForEach(func(k, v lua.LValue) {
			if ks, ok := k.(lua.LString); ok {
				if vs, ok2 := v.(lua.LString); ok2 {
					m[string(ks)] = string(vs)
				}
			}
		})
		return m
	}

	// optProxy 取第 idx 位的代理地址参数（仅 string 视为 proxy，其余为空）。
	optProxy := func(idx int) string {
		v := L.Get(idx)
		if v.Type() == lua.LTString {
			return string(v.(lua.LString))
		}
		return ""
	}

	httpTable := L.NewTable()
	L.SetFuncs(httpTable, map[string]lua.LGFunction{
		// get(url, proxy?) 同步 GET。可选 proxy 指定代理。
		"get": func(L *lua.LState) int {
			url := L.CheckString(1)
			proxyURL := ""
			if L.GetTop() >= 2 {
				proxyURL = optProxy(2)
			}
			cl, err := clientCache.get(proxyURL)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			resp, err := cl.Get(url)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			result := L.NewTable()
			L.SetField(result, "status", lua.LNumber(resp.StatusCode))
			L.SetField(result, "body", lua.LString(string(body)))
			L.Push(result)
			return 1
		},
		// post(url, content_type?, body?, proxy?) 同步 POST。可选第 4 位 proxy。
		"post": func(L *lua.LState) int {
			url := L.CheckString(1)
			contentType := "application/json"
			var bodyStr string
			proxyURL := ""
			top := L.GetTop()
			if top >= 4 {
				proxyURL = optProxy(4)
			}
			if top >= 3 {
				contentType = L.CheckString(2)
				bodyStr = L.CheckString(3)
			} else if top >= 2 {
				bodyStr = L.CheckString(2)
			}
			cl, err := clientCache.get(proxyURL)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			resp, err := cl.Post(url, contentType, bytes.NewBufferString(bodyStr))
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			result := L.NewTable()
			L.SetField(result, "status", lua.LNumber(resp.StatusCode))
			L.SetField(result, "body", lua.LString(string(body)))
			L.Push(result)
			return 1
		},
		// 异步版：立即返回 req_id（失败返回 0 + 错误串），完成回调 on_http_response(req_id, ctx, result, err)
		// 参数签名（向后兼容）：
		//   get_async(url, ctx?, headers?, proxy?)          旧签名 + 第 4 位 proxy 字符串；第 3 位 headers 表
		//   get_async(url, {proxy=…, headers=…, ctx=…})     新 opts 表写法（proxy 为可选键）
		"get_async": func(L *lua.LState) int {
			url := L.CheckString(1)
			var headers map[string]string
			proxyURL := ""
			ctxIdx := 0
			var optsCtx lua.LValue // opts 表内 ctx 键（新写法）
			top := L.GetTop()
			// 第 4 位：proxy 字符串（positional 写法）
			if top >= 4 && L.Get(4).Type() == lua.LTString {
				proxyURL = string(L.Get(4).(lua.LString))
			}
			// 第 3 位：headers 表（旧签名）
			if top >= 3 && L.Get(3).Type() == lua.LTTable && headers == nil {
				headers = parseStringTable(L.Get(3).(*lua.LTable))
			}
			// 第 2 位：opts 表（含 proxy 键）或旧 ctx 现场表
			if top >= 2 && L.Get(2).Type() == lua.LTTable {
				tbl := L.Get(2).(*lua.LTable)
				if p := tbl.RawGetString("proxy"); p.Type() == lua.LTString {
					proxyURL = string(p.(lua.LString))
					if h := tbl.RawGetString("headers"); h.Type() == lua.LTTable {
						headers = parseStringTable(h.(*lua.LTable))
					}
					if c := tbl.RawGetString("ctx"); c != lua.LNil {
						optsCtx = c // 回调 ctx=opts.ctx（任意 Lua 值）
					}
				} else {
					ctxIdx = 2 // 旧语义：ctx 现场表
				}
			}
			run := func(ctx context.Context) (any, error) {
				cl, err := clientCache.get(proxyURL)
				if err != nil {
					return nil, err
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				if err != nil {
					return nil, err
				}
				for k, v := range headers {
					req.Header.Set(k, v)
				}
				resp, err := cl.Do(req)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				return &httpResult{Status: resp.StatusCode, Body: string(body)}, nil
			}
			id, err := pe.submitAsync(L, pluginName, "http", httpAsyncTimeout, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			if optsCtx != nil {
				saveAsyncCtxValue(L, id, optsCtx)
			} else if ctxIdx > 0 {
				saveAsyncCtx(L, id, ctxIdx)
			}
			L.Push(lua.LNumber(id))
			return 1
		},
		// post_async(url, content_type?, body?, proxy?, ctx?) 异步 POST：
		//   旧签名尾部 table = ctx；新签名第 4 位为 proxy 字符串（ctx 自然后移至第 5 位）
		"post_async": func(L *lua.LState) int {
			url := L.CheckString(1)
			contentType := "application/json"
			var bodyStr string
			proxyURL := ""
			top := L.GetTop()
			ctxIdx := 0
			// 第 4 位：proxy 字符串（新）
			if top >= 4 && L.Get(4).Type() == lua.LTString {
				proxyURL = string(L.Get(4).(lua.LString))
			}
			// 尾部 table 参数视为调用现场 ctx（与 body 字符串区分；
			// 第 4 位被 proxy 占用时 ctx 在后移的第 5 位）
			if top >= 2 && L.Get(top).Type() == lua.LTTable {
				ctxIdx = top
				top--
			}
			if top >= 3 {
				contentType = L.CheckString(2)
				bodyStr = L.CheckString(3)
			} else if top >= 2 {
				bodyStr = L.CheckString(2)
			}
			run := func(ctx context.Context) (any, error) {
				cl, err := clientCache.get(proxyURL)
				if err != nil {
					return nil, err
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(bodyStr))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", contentType)
				resp, err := cl.Do(req)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				return &httpResult{Status: resp.StatusCode, Body: string(body)}, nil
			}
			id, err := pe.submitAsync(L, pluginName, "http", httpAsyncTimeout, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			if ctxIdx > 0 {
				saveAsyncCtx(L, id, ctxIdx)
			}
			L.Push(lua.LNumber(id))
			return 1
		},
	})
	L.SetGlobal("http", httpTable)
}

// ---------- Database ----------

func (pe *PluginEngine) injectDatabase(L *lua.LState, pluginName string) {
	prefix := "pluggin_" + pluginName + "_"
	db := pe.db

	// 读取可选的绑定参数（第 2 个参数，Lua 数组表）。
	// 未传或非数组时返回 nil，表示走裸 SQL（无参数化）。
	readParams := func(L *lua.LState) []any {
		if L.GetTop() < 2 {
			return nil
		}
		params, ok := luaValueToGo(L.Get(2)).([]any)
		if !ok {
			return nil
		}
		return params
	}

	// 把结果集 Rows 扫描为 []map[string]any 并作为唯一返回值推回 Lua。
	scanRows := func(L *lua.LState, rows *sql.Rows) int {
		defer rows.Close()
		cols, _ := rows.Columns()
		var result []map[string]any
		for rows.Next() {
			values := make([]any, len(cols))
			valuePtrs := make([]any, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			rows.Scan(valuePtrs...)
			row := make(map[string]any)
			for i, col := range cols {
				row[col] = values[i]
			}
			result = append(result, row)
		}
		L.Push(goToLuaValue(L, result))
		return 1
	}

	dbTable := L.NewTable()
	L.SetFuncs(dbTable, map[string]lua.LGFunction{
		"query": func(L *lua.LState) int {
			sqlStr := L.CheckString(1)
			sqlStr = prefixSQL(sqlStr, prefix)

			args := readParams(L)
			var rows *sql.Rows
			var err error
			if args == nil {
				rows, err = db.Raw(sqlStr).Rows()
			} else {
				rows, err = db.Raw(sqlStr, args...).Rows()
			}
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			return scanRows(L, rows)
		},
		"exec": func(L *lua.LState) int {
			sqlStr := L.CheckString(1)
			sqlStr = prefixSQL(sqlStr, prefix)

			args := readParams(L)
			var result *gorm.DB
			if args == nil {
				result = db.Exec(sqlStr)
			} else {
				result = db.Exec(sqlStr, args...)
			}
			if result.Error != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(result.Error.Error()))
				return 2
			}
			L.Push(lua.LNumber(result.RowsAffected))
			return 1
		},
	})
	L.SetGlobal("database", dbTable)
}

func prefixSQL(sql, prefix string) string {
	return sql
}

// ---------- Cache ----------

func (pe *PluginEngine) injectCache(L *lua.LState, pluginName string) {
	prefix := "pluggin:" + pluginName + ":"
	c := pe.cache

	cacheTable := L.NewTable()
	L.SetFuncs(cacheTable, map[string]lua.LGFunction{
		"get": func(L *lua.LState) int {
			key := L.CheckString(1)
			var result map[string]any
			if err := c.Get(context.Background(), prefix+key, &result); err != nil {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(goToLuaValue(L, result))
			return 1
		},
		"set": func(L *lua.LState) int {
			key := L.CheckString(1)
			val := luaValueToGo(L.Get(2))
			ttl := 0
			if L.GetTop() >= 3 {
				ttl = int(L.CheckInt(3))
			}
			if err := c.Set(context.Background(), prefix+key, val, time.Duration(ttl)*time.Second); err != nil {
				L.Push(lua.LBool(false))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LBool(true))
			return 1
		},
		"del": func(L *lua.LState) int {
			key := L.CheckString(1)
			if err := c.Del(context.Background(), prefix+key); err != nil {
				L.Push(lua.LBool(false))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LBool(true))
			return 1
		},
		"exists": func(L *lua.LState) int {
			key := L.CheckString(1)
			n, err := c.Exists(context.Background(), prefix+key)
			if err != nil {
				L.Push(lua.LNumber(0))
				return 1
			}
			L.Push(lua.LNumber(n))
			return 1
		},
	})
	L.SetGlobal("cache", cacheTable)
}

// ---------- T2I ----------

// t2iAsyncTimeout t2i 异步任务默认总超时（T2I 渲染较慢；opts.timeout 可覆盖）。
const t2iAsyncTimeout = 120 * time.Second

// luaTableToT2IOptions 解析 t2i.generate / t2i.generate_url 的可选 options 表
// （键名与 T2I 服务 GenerateOptions 的 JSON 字段一致）；未传或为 nil 时返回 nil。
// 未知键返回错误，便于插件尽早发现拼写问题。
func luaTableToT2IOptions(L *lua.LState, idx int) (*t2icaller.GenerateOptions, error) {
	if L.Get(idx) == lua.LNil {
		return nil, nil
	}
	tbl := L.CheckTable(idx)
	opts := &t2icaller.GenerateOptions{}
	var parseErr error
	tbl.ForEach(func(k, v lua.LValue) {
		if parseErr != nil {
			return
		}
		switch lua.LVAsString(k) {
		case "type":
			opts.Type = t2icaller.ImageType(lua.LVAsString(v))
		case "quality":
			opts.Quality = int(lua.LVAsNumber(v))
		case "omit_background":
			opts.OmitBackground = lua.LVAsBool(v)
		case "full_page":
			fp := lua.LVAsBool(v)
			opts.FullPage = &fp
		case "viewport_width":
			opts.ViewportWidth = int(lua.LVAsNumber(v))
		case "viewport_height":
			opts.ViewportHeight = int(lua.LVAsNumber(v))
		case "scale":
			opts.Scale = lua.LVAsString(v)
		case "animations":
			opts.Animations = t2icaller.Animation(lua.LVAsString(v))
		case "caret":
			opts.Caret = t2icaller.Caret(lua.LVAsString(v))
		case "device_scale_factor_level":
			opts.DeviceScaleFactor = t2icaller.ScaleLevel(lua.LVAsString(v))
		case "timeout":
			opts.Timeout = float64(lua.LVAsNumber(v))
		default:
			parseErr = fmt.Errorf("未知 T2I 选项: %s", lua.LVAsString(k))
		}
	})
	if parseErr != nil {
		return nil, parseErr
	}
	return opts, nil
}

// ragAsyncTimeout RAG 异步 API 总超时（写入/检索本地服务，30s 足够）。
const ragAsyncTimeout = 30 * time.Second

// parseRagUUID 解析插件传入的 RAG tag（必须是合法 UUID 字符串）。
func parseRagUUID(s string) (uuid.UUID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("RAG tag 必须是 UUID 字符串: %s", s)
	}
	return u, nil
}

// injectRAG 注入 rag 全局表（需要 rag 权限）：RAG-Service 原始 API。
//
// 契约：面向原始 RAG-Service（tag=UUID，全文入库自动分块），
// **不要**与知识/记忆集合的 v5 派生 tag 混用（避免污染两侧检索）。
// 客户端始终经 AgentOperator 动态获取（Web 配置热更新即时生效）。
func (pe *PluginEngine) injectRAG(L *lua.LState, pluginName string) {
	getCurrentClient := func() *ragcaller.Client {
		if pe.agentOp != nil {
			return pe.agentOp.GetRAGClient()
		}
		return nil
	}

	// searchToLua 把检索结果转成 Lua 数组表 [{tag, score}]。
	searchToLua := func(hits []ragcaller.SearchHit) *lua.LTable {
		t := L.NewTable()
		for i, hit := range hits {
			item := L.NewTable()
			item.RawSetString("tag", lua.LString(hit.Tag.String()))
			item.RawSetString("score", lua.LNumber(hit.Score))
			t.RawSetInt(i+1, item)
		}
		return t
	}

	ragTable := L.NewTable()
	L.SetFuncs(ragTable, map[string]lua.LGFunction{
		// add(tag, text) 同步写入（幂等 upsert，长文自动分块）
		"add": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("RAG 服务未启用"))
				return 2
			}
			tag, err := parseRagUUID(L.CheckString(1))
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			text := L.CheckString(2)
			_, err = client.Upsert(context.Background(), tag, text)
			return pushResult(L, err)
		},
		// add_async(tag, text [, ctx]) → req_id；回调 on_rag_response(req_id, ctx, tag, err)
		"add_async": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString("RAG 服务未启用"))
				return 2
			}
			tag, err := parseRagUUID(L.CheckString(1))
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			text := L.CheckString(2)
			run := func(ctx context.Context) (any, error) {
				if _, err := client.Upsert(ctx, tag, text); err != nil {
					return nil, err
				}
				return tag.String(), nil
			}
			id, err := pe.submitAsync(L, pluginName, "rag", ragAsyncTimeout, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			saveAsyncCtx(L, id, 3) // 第 3 位：可选 ctx 现场表
			L.Push(lua.LNumber(id))
			return 1
		},
		// search(query, k?, min_score?) 同步检索，返回 [{tag, score}]（按分数降序）
		"search": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("RAG 服务未启用"))
				return 2
			}
			query := L.CheckString(1)
			k := 10
			var minScore *float64
			top := L.GetTop()
			if top >= 2 && L.Get(2).Type() == lua.LTNumber {
				k = int(float64(L.Get(2).(lua.LNumber)))
			}
			if top >= 3 && L.Get(3).Type() == lua.LTNumber {
				ms := float64(L.Get(3).(lua.LNumber))
				minScore = &ms
			}
			hits, err := client.Search(context.Background(), query, k, minScore)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(searchToLua(hits))
			return 1
		},
		// search_async(query, k?, min_score? [, ctx]) → req_id；回调 on_rag_response(req_id, ctx, results, err)
		"search_async": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString("RAG 服务未启用"))
				return 2
			}
			query := L.CheckString(1)
			k := 10
			var minScore *float64
			ctxIdx := 0
			top := L.GetTop()
			// 尾部 table = ctx 现场表（与 search 的数值参数区分）
			if top >= 2 && L.Get(top).Type() == lua.LTTable {
				ctxIdx = top
				top--
			}
			if top >= 2 && L.Get(2).Type() == lua.LTNumber {
				k = int(float64(L.Get(2).(lua.LNumber)))
			}
			if top >= 3 && L.Get(3).Type() == lua.LTNumber {
				ms := float64(L.Get(3).(lua.LNumber))
				minScore = &ms
			}
			run := func(ctx context.Context) (any, error) {
				return client.Search(ctx, query, k, minScore)
			}
			id, err := pe.submitAsync(L, pluginName, "rag", ragAsyncTimeout, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			if ctxIdx > 0 {
				saveAsyncCtx(L, id, ctxIdx)
			}
			L.Push(lua.LNumber(id))
			return 1
		},
	})
	L.SetGlobal("rag", ragTable)
}

func (pe *PluginEngine) injectT2I(L *lua.LState, pluginName string) {
	t2iTable := L.NewTable()

	// 获取当前 T2I 客户端：优先从 agentOp 获取最新运行时实例，否则使用注入时的实例
	getCurrentClient := func() *t2icaller.Client {
		if pe.agentOp != nil {
			return pe.agentOp.GetT2IClient()
		}
		return pe.t2i
	}

	L.SetFuncs(t2iTable, map[string]lua.LGFunction{
		"generate": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("T2I 服务未启用"))
				return 2
			}
			html := L.CheckString(1)
			opts, optErr := luaTableToT2IOptions(L, 2)
			if optErr != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(optErr.Error()))
				return 2
			}
			resp, err := client.Generate(context.Background(), t2icaller.GenerateRequest{
				HTML:    html,
				AsJSON:  true,
				Options: opts,
			})
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LString(resp.Data.ID))
			return 1
		},
		"generate_url": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("T2I 服务未启用"))
				return 2
			}
			html := L.CheckString(1)
			opts, optErr := luaTableToT2IOptions(L, 2)
			if optErr != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(optErr.Error()))
				return 2
			}
			url, err := client.GenerateURL(context.Background(), t2icaller.GenerateRequest{
				HTML:    html,
				AsJSON:  true,
				Options: opts,
			})
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LString(url))
			return 1
		},
		// 异步版：立即返回 req_id（失败返回 0 + 错误串），完成回调 on_t2i_response(req_id, ctx, result, err)
		"generate_async": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString("T2I 服务未启用"))
				return 2
			}
			html := L.CheckString(1)
			opts, optErr := luaTableToT2IOptions(L, 2)
			if optErr != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(optErr.Error()))
				return 2
			}
			// 异步任务总超时：opts.timeout（秒）优先，缺省 120s
			timeout := t2iAsyncTimeout
			if opts != nil && opts.Timeout > 0 {
				timeout = time.Duration(opts.Timeout * float64(time.Second))
			}
			run := func(ctx context.Context) (any, error) {
				resp, err := client.Generate(ctx, t2icaller.GenerateRequest{
					HTML:    html,
					AsJSON:  true,
					Options: opts,
				})
				if err != nil {
					return nil, err
				}
				return resp.Data.ID, nil
			}
			id, err := pe.submitAsync(L, pluginName, "t2i", timeout, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			saveAsyncCtx(L, id, 3) // 第 3 位：可选 ctx 现场表（html, opts, ctx）
			L.Push(lua.LNumber(id))
			return 1
		},
		"generate_url_async": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString("T2I 服务未启用"))
				return 2
			}
			html := L.CheckString(1)
			opts, optErr := luaTableToT2IOptions(L, 2)
			if optErr != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(optErr.Error()))
				return 2
			}
			timeout := t2iAsyncTimeout
			if opts != nil && opts.Timeout > 0 {
				timeout = time.Duration(opts.Timeout * float64(time.Second))
			}
			run := func(ctx context.Context) (any, error) {
				url, err := client.GenerateURL(ctx, t2icaller.GenerateRequest{
					HTML:    html,
					AsJSON:  true,
					Options: opts,
				})
				if err != nil {
					return nil, err
				}
				return url, nil
			}
			id, err := pe.submitAsync(L, pluginName, "t2i", timeout, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			saveAsyncCtx(L, id, 3)
			L.Push(lua.LNumber(id))
			return 1
		},
		// 开关管理
		"toggle": func(L *lua.LState) int {
			if pe.agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			active := bool(L.CheckBool(1))
			err := pe.agentOp.SetT2IActive(context.Background(), active)
			return pushResult(L, err)
		},
		"is_active": func(L *lua.LState) int {
			if pe.dao == nil {
				L.Push(lua.LBool(false))
				return 1
			}
			cfg, err := pe.dao.T2I.GetConfig(context.Background())
			if err != nil {
				L.Push(lua.LBool(false))
				return 1
			}
			L.Push(lua.LBool(cfg.IsActive))
			return 1
		},
		"get_config": func(L *lua.LState) int {
			if pe.dao == nil {
				return pushResult(L, fmt.Errorf("dao 不可用"))
			}
			cfg, err := pe.dao.T2I.GetConfig(context.Background())
			return pushResultJSON(L, cfg, err)
		},
	})
	L.SetGlobal("t2i", t2iTable)
}

// ---------- Sandbox ----------

// sandboxAsyncTimeout sandbox 异步任务默认总超时（沙箱执行代码可能较慢）。
const sandboxAsyncTimeout = 120 * time.Second

func (pe *PluginEngine) injectSandbox(L *lua.LState, pluginName string) {
	sbTable := L.NewTable()

	// 获取当前 Sandbox 客户端：优先从 agentOp 获取最新运行时实例
	getCurrentClient := func() *sandboxcaller.Client {
		if pe.agentOp != nil {
			return pe.agentOp.GetSandboxClient()
		}
		return pe.sandbox
	}

	L.SetFuncs(sbTable, map[string]lua.LGFunction{
		"create": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("Sandbox 服务未启用"))
				return 2
			}
			sbox, err := client.CreateSandbox(context.Background(), sandboxcaller.CreateSandboxRequest{})
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			result := L.NewTable()
			L.SetField(result, "sandbox_id", lua.LString(sbox.ID))
			L.SetField(result, "status", lua.LString(string(sbox.Status)))
			L.Push(result)
			return 1
		},
		"exec_shell": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("Sandbox 服务未启用"))
				return 2
			}
			sid := L.CheckString(1)
			cmd := L.CheckString(2)
			result, err := client.ExecShell(context.Background(), sid, sandboxcaller.ShellExecRequest{Command: cmd})
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LString(result.Output))
			if result.ExitCode != nil {
				L.Push(lua.LNumber(*result.ExitCode))
			} else {
				L.Push(lua.LNumber(0))
			}
			return 2
		},
		"exec_python": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNil)
				L.Push(lua.LString("Sandbox 服务未启用"))
				return 2
			}
			sid := L.CheckString(1)
			code := L.CheckString(2)
			result, err := client.ExecPython(context.Background(), sid, sandboxcaller.PythonExecRequest{Code: code})
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LString(result.Output))
			if result.Error != nil {
				L.Push(lua.LString(*result.Error))
			} else {
				L.Push(lua.LString(""))
			}
			return 2
		},
		// 异步版：立即返回 req_id（失败返回 0 + 错误串），完成回调 on_sandbox_response(req_id, ctx, result, err)
		"create_async": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString("Sandbox 服务未启用"))
				return 2
			}
			run := func(ctx context.Context) (any, error) {
				sbox, err := client.CreateSandbox(ctx, sandboxcaller.CreateSandboxRequest{})
				if err != nil {
					return nil, err
				}
				return map[string]any{"sandbox_id": sbox.ID, "status": string(sbox.Status)}, nil
			}
			id, err := pe.submitAsync(L, pluginName, "sandbox", sandboxAsyncTimeout, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			saveAsyncCtx(L, id, 1) // 第 1 位：可选 ctx 现场表
			L.Push(lua.LNumber(id))
			return 1
		},
		"exec_shell_async": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString("Sandbox 服务未启用"))
				return 2
			}
			sid := L.CheckString(1)
			cmd := L.CheckString(2)
			run := func(ctx context.Context) (any, error) {
				result, err := client.ExecShell(ctx, sid, sandboxcaller.ShellExecRequest{Command: cmd})
				if err != nil {
					return nil, err
				}
				exitCode := 0
				if result.ExitCode != nil {
					exitCode = *result.ExitCode
				}
				return map[string]any{"output": result.Output, "exit_code": exitCode}, nil
			}
			id, err := pe.submitAsync(L, pluginName, "sandbox", sandboxAsyncTimeout, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			saveAsyncCtx(L, id, 3) // 第 3 位：可选 ctx 现场表（sid, cmd, ctx）
			L.Push(lua.LNumber(id))
			return 1
		},
		"exec_python_async": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString("Sandbox 服务未启用"))
				return 2
			}
			sid := L.CheckString(1)
			code := L.CheckString(2)
			run := func(ctx context.Context) (any, error) {
				result, err := client.ExecPython(ctx, sid, sandboxcaller.PythonExecRequest{Code: code})
				if err != nil {
					return nil, err
				}
				errStr := ""
				if result.Error != nil {
					errStr = *result.Error
				}
				return map[string]any{"output": result.Output, "error": errStr}, nil
			}
			id, err := pe.submitAsync(L, pluginName, "sandbox", sandboxAsyncTimeout, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			saveAsyncCtx(L, id, 3)
			L.Push(lua.LNumber(id))
			return 1
		},
		// 开关管理
		"toggle": func(L *lua.LState) int {
			if pe.agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			active := bool(L.CheckBool(1))
			err := pe.agentOp.SetSandboxActive(context.Background(), active)
			return pushResult(L, err)
		},
		"list": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				return pushResultJSON(L, nil, fmt.Errorf("Sandbox 服务未启用"))
			}
			list, err := client.ListSandboxes(context.Background(), 50, "", "")
			return pushResultJSON(L, list, err)
		},
		"delete": func(L *lua.LState) int {
			client := getCurrentClient()
			if client == nil {
				return pushResult(L, fmt.Errorf("Sandbox 服务未启用"))
			}
			sid := L.CheckString(1)
			err := client.DeleteSandbox(context.Background(), sid)
			return pushResult(L, err)
		},
		"is_active": func(L *lua.LState) int {
			if pe.dao == nil {
				L.Push(lua.LBool(false))
				return 1
			}
			cfg, err := pe.dao.Sandbox.GetConfig(context.Background())
			if err != nil {
				L.Push(lua.LBool(false))
				return 1
			}
			L.Push(lua.LBool(cfg.IsActive))
			return 1
		},
		"get_config": func(L *lua.LState) int {
			if pe.dao == nil {
				return pushResult(L, fmt.Errorf("dao 不可用"))
			}
			cfg, err := pe.dao.Sandbox.GetConfig(context.Background())
			return pushResultJSON(L, cfg, err)
		},
	})
	L.SetGlobal("sandbox", sbTable)
}

// ---------- Agent ----------

func (pe *PluginEngine) injectAgent(L *lua.LState) {
	daoBundle := pe.dao
	agentOp := pe.agentOp

	agentTable := L.NewTable()

	funcs := map[string]lua.LGFunction{
		// 查询
		"get_providers": func(L *lua.LState) int {
			list, err := daoBundle.Provider.List(context.Background(), "")
			return pushResultJSON(L, list, err)
		},
		"get_mcp_servers": func(L *lua.LState) int {
			list, err := daoBundle.MCPServer.List(context.Background())
			return pushResultJSON(L, list, err)
		},
		"get_skills": func(L *lua.LState) int {
			list, err := daoBundle.Skill.List(context.Background())
			return pushResultJSON(L, list, err)
		},
		"get_sessions": func(L *lua.LState) int {
			list, err := daoBundle.Session.List(context.Background())
			return pushResultJSON(L, list, err)
		},
		"get_prompts": func(L *lua.LState) int {
			list, err := daoBundle.Prompt.List(context.Background())
			return pushResultJSON(L, list, err)
		},
		"get_tools": func(L *lua.LState) int {
			list, err := daoBundle.ToolConfig.List(context.Background())
			return pushResultJSON(L, list, err)
		},
		"get_plugins": func(L *lua.LState) int {
			list, err := daoBundle.Plugin.List(context.Background())
			return pushResultJSON(L, list, err)
		},

		// Provider 管理
		"set_provider_active": func(L *lua.LState) int {
			if agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			id := L.CheckString(1)
			active := bool(L.CheckBool(2))
			err := agentOp.SetProviderActive(context.Background(), id, active)
			return pushResult(L, err)
		},

		// MCP 管理
		"set_mcp_active": func(L *lua.LState) int {
			if agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			id := L.CheckString(1)
			active := bool(L.CheckBool(2))
			err := agentOp.SetMCPActive(context.Background(), id, active)
			return pushResult(L, err)
		},
		"list_mcps": func(L *lua.LState) int {
			if agentOp == nil || pe.dao == nil {
				return pushResult(L, fmt.Errorf("agent operator 或 dao 不可用"))
			}
			// 从 DB 查询所有已配置的 MCP（含未启用的），再合并运行时连接状态
			dbList, err := pe.dao.MCPServer.List(context.Background())
			if err != nil {
				return pushResultJSON(L, nil, err)
			}
			mcpGroup := agentOp.GetMCPGroup()
			runtimeList := mcpGroup.ListMCPs()
			connectedMap := make(map[string]bool, len(runtimeList))
			for _, r := range runtimeList {
				connectedMap[r.ID] = r.Active
			}
			out := make([]MCPInfo, 0, len(dbList))
			for _, m := range dbList {
				out = append(out, MCPInfo{
					ID:       m.ID,
					Name:     m.Name,
					URL:      m.ServerURL,
					Active:   connectedMap[m.ID],
					IsActive: m.IsActive,
				})
			}
			return pushResultJSON(L, out, nil)
		},
		"toggle_mcp": func(L *lua.LState) int {
			if agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			id := L.CheckString(1)
			active := bool(L.CheckBool(2))
			err := agentOp.SetMCPActive(context.Background(), id, active)
			return pushResult(L, err)
		},

		// Tool 管理
		"list_tools": func(L *lua.LState) int {
			if agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			toolReg := agentOp.GetToolRegistry()
			list := toolReg.ListTools()
			return pushResultJSON(L, list, nil)
		},
		"toggle_tool": func(L *lua.LState) int {
			if agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			name := L.CheckString(1)
			active := bool(L.CheckBool(2))
			err := agentOp.SetToolActive(context.Background(), name, active)
			return pushResult(L, err)
		},

		// Provider 运行时查询与切换
		"list_runtime_providers": func(L *lua.LState) int {
			if agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			pg := agentOp.GetProviderGroup()
			list := pg.List()
			return pushResultJSON(L, list, nil)
		},
		"switch_provider": func(L *lua.LState) int {
			if agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			id := L.CheckString(1)
			err := agentOp.SwitchProvider(context.Background(), id)
			return pushResult(L, err)
		},

		// 当前 Chat-Area（读该插件 currentEv：事件派发/异步回调执行前写入，
		// 异步回调读到的是任务触发时的会话，而非派发时最新的其他事件）
		"get_current_chat_area": func(L *lua.LState) int {
			ev := currentEventOf(L)
			result := L.NewTable()
			L.SetField(result, "post_type", lua.LString(ev.PostType))
			L.SetField(result, "message_type", lua.LString(ev.MessageType))
			L.SetField(result, "user_id", lua.LNumber(ev.UserID))
			L.SetField(result, "group_id", lua.LNumber(ev.GroupID))
			if agentOp != nil {
				areaID := agentOp.GetChatAreaID(ev.UserID, ev.GroupID, ev.MessageType)
				L.SetField(result, "chat_area_id", lua.LString(areaID))
			}
			L.Push(result)
			return 1
		},

		// 短期记忆 Compact
		"compact_memory": func(L *lua.LState) int {
			if agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			ev := currentEventOf(L)
			areaID := agentOp.GetChatAreaID(ev.UserID, ev.GroupID, ev.MessageType)
			if areaID == "" {
				return pushResult(L, fmt.Errorf("无法获取当前 Chat-Area ID"))
			}
			err := agentOp.CompactMemory(context.Background(), areaID)
			if err != nil {
				return pushResult(L, err)
			}
			L.Push(lua.LString("短期记忆已 Compact 并写入长期记忆"))
			return 1
		},
	}

	L.SetFuncs(agentTable, funcs)
	L.SetGlobal("agent", agentTable)
}

// ---------- LLM ----------

// injectLLM 注入 llm 全局表：插件通过 Bot 自身启用的文本模型 Provider 调用 LLM
// （模型 / 采样参数 / 密钥全部复用 Bot 配置，插件不接触密钥）。
//   - llm.available() -> boolean                当前是否有可用文本模型
//   - llm.chat(messages, opts) -> content, err  同步调用（适合命令等低频路径）
//   - llm.chat_async(messages, opts) -> req_id  异步调用（不阻塞事件循环）
//
// messages: 单字符串（role=user）或数组，元素为字符串（role=user）或
//
//	{role="system|user|assistant", content="..."}。
//
// opts: {temperature=?, max_tokens=?, timeout=?秒}，缺省回退 Bot Provider 配置。
// chat_async 立即返回 req_id（失败返回 0 并附加错误串）；完成后引擎调用插件
// 入口函数 on_chat_response(req_id, content, err)，err 为 nil 表示成功。
// 派发规格由异步注册表（kind "chat"）定义，见 RegisterAsyncAPI。
func (pe *PluginEngine) injectLLM(L *lua.LState, pluginName string) {
	llmTable := L.NewTable()
	funcs := map[string]lua.LGFunction{
		"available": func(L *lua.LState) int {
			L.Push(lua.LBool(pe.llmAccess != nil && pe.llmAccess.Available()))
			return 1
		},
		"chat": func(L *lua.LState) int {
			req, err := luaChatRequest(L, 1, 2)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			ctx, cancel := context.WithTimeout(context.Background(), llmTimeoutFromOpts(L, 2))
			defer cancel()
			resp, err := pe.llmAccess.Chat(ctx, req)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LString(resp.Message.Content))
			return 1
		},
		"chat_async": func(L *lua.LState) int {
			req, err := luaChatRequest(L, 1, 2)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			run := func(ctx context.Context) (any, error) {
				resp, err := pe.llmAccess.Chat(ctx, req)
				if err != nil {
					return nil, err
				}
				return resp.Message.Content, nil
			}
			id, err := pe.submitAsync(L, pluginName, "chat", llmTimeoutFromOpts(L, 2), run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			// req_id 为 uint64；Lua number 在 2^53 内精确，实际序列号远达不到
			L.Push(lua.LNumber(id))
			return 1
		},
	}
	L.SetFuncs(llmTable, funcs)
	L.SetGlobal("llm", llmTable)
}

// ---------- Timer ----------

// injectTimer 注入 timer 全局表：插件可用 jn.timer 做异步延时（不阻塞事件循环）。
//   - timer.after(seconds, ctx?) -> req_id    到点后回调 on_timer_response(req_id, ctx, result, err)
//
// seconds 支持小数（秒）；可选 ctx 表作为调用现场，回调时原样取回。
// 与 chat_async 等其它异步 API 一致：任务在独立 goroutine 计时，完成后经
// 引擎异步注册表（kind "timer"）串行派发回插件，不阻塞事件派发与其它插件。
func (pe *PluginEngine) injectTimer(L *lua.LState, pluginName string) {
	timerTable := L.NewTable()
	funcs := map[string]lua.LGFunction{
		"after": func(L *lua.LState) int {
			secs := L.CheckNumber(1)
			d := time.Duration(float64(secs) * float64(time.Second))
			if d < 0 {
				d = 0
			}
			run := func(ctx context.Context) (any, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(d):
					return nil, nil
				}
			}
			// 兜底超时比目标延时多留 5s，确保正常到点走 time.After 而非 context 取消
			id, err := pe.submitAsync(L, pluginName, "timer", d+5*time.Second, run)
			if err != nil {
				L.Push(lua.LNumber(0))
				L.Push(lua.LString(err.Error()))
				return 2
			}
			saveAsyncCtx(L, id, 2) // 第 2 位：可选 ctx 现场表
			L.Push(lua.LNumber(id))
			return 1
		},
	}
	L.SetFuncs(timerTable, funcs)
	L.SetGlobal("timer", timerTable)
}

// RegisterAsyncAPI 注册一类异步 API（如 "chat" → 插件入口 on_chat_response）。
// 注册表集中管理 xxx_async 类接口的派发规格，后续新增 t2i/http/agent 等异步
// 只需注册一个 kind。重复注册 panic（内置 kind 在 NewPluginEngine 注册）。
// 以 copy-on-write 更新 atomic map：submitAsync 无锁读取，插件 Lua 回调内
// 调用 *_async 不再与引擎锁交互（消除持锁回调内提交异步任务的死锁）。
func (pe *PluginEngine) RegisterAsyncAPI(kind string, api AsyncAPI) {
	// 持专用写锁跨 dup 检查 + map 复制 + Store：并发注册不同 kind 时，
	// 后注册者必须基于最新 map 构建，否则会覆盖丢失先注册的 kind
	pe.registerMu.Lock()
	defer pe.registerMu.Unlock()
	old := pe.asyncAPIs.Load()
	if _, dup := (*old)[kind]; dup {
		panic("pluggin: 异步 API 重复注册: " + kind)
	}
	// 注意必须真正复制 map：Go map 是引用类型，直接改共享 map 会与
	// submitAsync 的并发读取产生 data race
	m := make(map[string]*AsyncAPI, len(*old)+1)
	for k, v := range *old {
		m[k] = v
	}
	m[kind] = &api
	pe.asyncAPIs.Store(&m)
}

// submitAsync 提交一个异步任务：run 在独立 goroutine 中执行（不得触碰任何 LState），
// 完成后派发到插件的 Entry 入口函数。返回 req_id；kind 未注册返回 error。
// 无锁：asyncAPIs 注册表是 atomic 只读 map（构造期填充），此处不请求 pe.mu。
// L 用于取回该插件当前事件快照（pluginRef 无锁，Lua 执行期间调用）。
func (pe *PluginEngine) submitAsync(L *lua.LState, pluginName, kind string, timeout time.Duration, run func(ctx context.Context) (any, error)) (uint64, error) {
	api, ok := (*pe.asyncAPIs.Load())[kind]
	if !ok {
		return 0, fmt.Errorf("pluggin: 异步 API %q 未注册", kind)
	}
	// 快照触发事件（持 stateMu 执行 Lua 期间读取，无需额外锁）
	ev := currentEventOf(L)
	id := atomic.AddUint64(&pe.asyncSeq, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		result, err := run(ctx)
		pe.asyncCh <- asyncTask{pluginName: pluginName, kind: kind, api: api, reqID: id, result: result, err: err, event: ev}
	}()
	return id, nil
}

// runAsyncCallbacks 消费异步任务队列，串行派发到插件 Lua 入口函数
// on_<kind>_response(req_id, [ctx,] <Encode 返回值...>)。在插件 stateMu 下执行，
// 与事件派发（dispatchMessage/onNotice/...）对同一 LState 互斥；不再持有引擎全局锁，
// 回调内调用 *_async API（jn.timer.after 等）不会自死锁。插件已卸载则丢弃任务。
// WithCtx 的 API 回调前从 jn_async_ctx 取出调用现场（一次性消费）。
func (pe *PluginEngine) runAsyncCallbacks() {
	for task := range pe.asyncCh {
		p := pe.pluginByName(task.pluginName)
		if p == nil {
			// 插件可能仍在加载中（entry 提交的异步任务先于发布完成）。
			// 等待该次加载结束（成功则发布、失败则无插件）再决定派发/丢弃。
			pe.loadMu.Lock()
			l, loading := pe.loading[task.pluginName]
			pe.loadMu.Unlock()
			if loading {
				<-l.done
				p = pe.pluginByName(task.pluginName)
			}
			if p == nil {
				continue // 加载失败或插件已卸载，丢弃任务
			}
		}
		p.stateMu.Lock()
		if p.closed {
			p.stateMu.Unlock()
			continue
		}
		// 恢复任务触发时的事件：回调内 jn.agent API 读到任务所属会话
		p.currentEv = task.event
		L := p.State
		fn := L.GetGlobal(task.api.Entry)
		if fn.Type() == lua.LTFunction {
			vals := task.api.Encode(L, task.result, task.err)
			nargs := 1 // req_id
			L.Push(fn)
			L.Push(lua.LNumber(task.reqID))
			if task.api.WithCtx {
				// 取出调用现场（req_id → ctx table），回调后即删，防泄漏
				ctxVal := lua.LNil
				if t := L.GetGlobal("jn_async_ctx"); t.Type() == lua.LTTable {
					key := lua.LNumber(task.reqID)
					if v := t.(*lua.LTable).RawGet(key); v != lua.LNil {
						ctxVal = v
						t.(*lua.LTable).RawSet(key, lua.LNil)
					}
				}
				L.Push(ctxVal)
				nargs++
			}
			for _, v := range vals {
				L.Push(v)
			}
			if err := L.PCall(nargs+len(vals), 0, nil); err != nil {
				log.Error("插件异步回调错误", "plugin", task.pluginName, "kind", task.kind, "err", err)
			}
		}
		p.stateMu.Unlock()
	}
}

// saveAsyncCtx 保存异步调用的调用现场表（可选）：jn_async_ctx[req_id] = ctx。
// ctx 缺省（非 table）时不保存；回调时由 runAsyncCallbacks 取出并删除。
func saveAsyncCtx(L *lua.LState, reqID uint64, ctxIdx int) {
	ctx := L.Get(ctxIdx)
	if ctx.Type() != lua.LTTable {
		return
	}
	t := L.GetGlobal("jn_async_ctx")
	if t.Type() != lua.LTTable {
		return
	}
	t.(*lua.LTable).RawSet(lua.LNumber(reqID), ctx)
}

// saveAsyncCtxValue 保存异步调用的调用现场值（如 opts 表内的 ctx 键，
// 不支持从 LState 按索引读取的场景）。回调时由 runAsyncCallbacks 取出并删除。
func saveAsyncCtxValue(L *lua.LState, reqID uint64, ctx lua.LValue) {
	if ctx.Type() != lua.LTTable {
		return
	}
	t := L.GetGlobal("jn_async_ctx")
	if t.Type() != lua.LTTable {
		return
	}
	t.(*lua.LTable).RawSet(lua.LNumber(reqID), ctx)
}

// httpResult http 异步请求结果（Encode 阶段转成 Lua {status, body} 表）。
type httpResult struct {
	Status int
	Body   string
}

// luaTableFromMap 把标量字段的 map 转成 Lua table（异步 Encode 阶段使用，
// 回调前在锁内执行，L 可用）。仅支持 string/int/int64/float64/bool，其余置 nil。
func luaTableFromMap(L *lua.LState, m map[string]any) *lua.LTable {
	t := L.NewTable()
	for k, v := range m {
		switch vv := v.(type) {
		case string:
			t.RawSetString(k, lua.LString(vv))
		case int:
			t.RawSetString(k, lua.LNumber(vv))
		case int64:
			t.RawSetString(k, lua.LNumber(vv))
		case float64:
			t.RawSetString(k, lua.LNumber(vv))
		case bool:
			t.RawSetString(k, lua.LBool(vv))
		default:
			t.RawSetString(k, lua.LNil)
		}
	}
	return t
}

// luaChatRequest 将 Lua 参数转换为 provider.ChatRequest。
func luaChatRequest(L *lua.LState, msgIdx int, optsIdx int) (provider.ChatRequest, error) {
	var req provider.ChatRequest
	v := L.Get(msgIdx)
	switch v.Type() {
	case lua.LTString:
		req.Messages = []provider.ChatMessage{{Role: "user", Content: v.String()}}
	case lua.LTTable:
		tbl := v.(*lua.LTable)
		tbl.ForEach(func(_, val lua.LValue) {
			switch val.Type() {
			case lua.LTString:
				req.Messages = append(req.Messages, provider.ChatMessage{Role: "user", Content: val.String()})
			case lua.LTTable:
				item := val.(*lua.LTable)
				role := lvString(item.RawGetString("role"), "user")
				content := lvString(item.RawGetString("content"), "")
				req.Messages = append(req.Messages, provider.ChatMessage{Role: role, Content: content})
			}
		})
	default:
		return req, fmt.Errorf("llm: messages 参数必须是字符串或数组")
	}
	if len(req.Messages) == 0 {
		return req, fmt.Errorf("llm: messages 不能为空")
	}
	if opts := L.Get(optsIdx); opts.Type() == lua.LTTable {
		ot := opts.(*lua.LTable)
		if t := ot.RawGetString("temperature"); t.Type() == lua.LTNumber {
			req.Temperature = float32(t.(lua.LNumber))
		}
		if mt := ot.RawGetString("max_tokens"); mt.Type() == lua.LTNumber {
			req.MaxTokens = int(mt.(lua.LNumber))
		}
	}
	return req, nil
}

// lvString 取 Lua 字符串/数字值，缺省回退 def。
func lvString(v lua.LValue, def string) string {
	switch v.Type() {
	case lua.LTString:
		return string(v.(lua.LString))
	case lua.LTNumber:
		return strconv.FormatFloat(float64(v.(lua.LNumber)), 'f', -1, 64)
	}
	return def
}

// llmTimeoutFromOpts 从 opts.timeout（秒）解析调用超时，缺省 60s。
func llmTimeoutFromOpts(L *lua.LState, optsIdx int) time.Duration {
	timeout := 60 * time.Second
	if opts := L.Get(optsIdx); opts.Type() == lua.LTTable {
		if t := opts.(*lua.LTable).RawGetString("timeout"); t.Type() == lua.LTNumber {
			if sec := float64(t.(lua.LNumber)); sec > 0 {
				timeout = time.Duration(sec * float64(time.Second))
			}
		}
	}
	return timeout
}

// ====================================================================
// Helper functions
// ====================================================================

func pushResult(L *lua.LState, err error) int {
	if err != nil {
		L.Push(lua.LBool(false))
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LBool(true))
	return 1
}

// pushOk 异步消息发送成功占位，返回 {true, "ok"}
func pushOk(L *lua.LState) int {
	L.Push(lua.LBool(true))
	L.Push(lua.LString("ok"))
	return 2
}

func pushResultJSON(L *lua.LState, v any, err error) int {
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	if v == nil {
		L.Push(lua.LNil)
		return 1
	}
	data, _ := json.Marshal(v)
	var result any
	json.Unmarshal(data, &result)
	L.Push(goToLuaValue(L, result))
	return 1
}

// ====================================================================
// Manifest
// ====================================================================

// newPluginUUID 生成 RFC 4122 v4 风格的 UUID 字符串。
func newPluginUUID() string {
	b := make([]byte, 16)
	_, _ = crand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (pe *PluginEngine) readManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "pluggin.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Entry == "" {
		m.Entry = "main.lua"
	}
	// 兼容旧 YAML：若文件中未显式声明 enabled，默认视为 true
	if !bytes.Contains(data, []byte("\nenabled")) && !bytes.HasPrefix(bytes.TrimSpace(data), []byte("enabled")) {
		m.Enabled = true
	}
	return &m, nil
}

// writeManifest 将清单写回 pluggin.yaml。
func (pe *PluginEngine) writeManifest(dir string, m *Manifest) error {
	path := filepath.Join(dir, "pluggin.yaml")
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SetEnabled 更新插件 YAML 中的 enabled 字段。
// 返回的是磁盘更新结果，不影响已加载/未加载的运行时状态。
func (pe *PluginEngine) SetEnabled(name string, enabled bool) error {
	dir := filepath.Join(pe.basePath, name)
	m, err := pe.readManifest(dir)
	if err != nil {
		return fmt.Errorf("读取插件清单失败: %w", err)
	}
	m.Enabled = enabled
	return pe.writeManifest(dir, m)
}

// ====================================================================
// Go ↔ Lua type conversion
// ====================================================================

func eventToLuaTable(L *lua.LState, ev EventData) *lua.LTable {
	t := L.NewTable()
	L.SetField(t, "post_type", lua.LString(ev.PostType))
	L.SetField(t, "message_type", lua.LString(ev.MessageType))
	L.SetField(t, "user_id", lua.LNumber(ev.UserID))
	L.SetField(t, "group_id", lua.LNumber(ev.GroupID))
	L.SetField(t, "raw_message", lua.LString(ev.RawMessage))
	if len(ev.Admins) > 0 {
		admins := L.NewTable()
		for _, a := range ev.Admins {
			admins.Append(lua.LString(a))
		}
		L.SetField(t, "admins", admins)
	}
	if len(ev.Webhook) > 0 {
		L.SetField(t, "webhook", goToLuaValue(L, ev.Webhook))
	}
	if len(ev.Payload) > 0 {
		L.SetField(t, "payload", goToLuaValue(L, ev.Payload))
	}
	// notice 字段
	if ev.NoticeType != "" {
		L.SetField(t, "notice_type", lua.LString(ev.NoticeType))
		L.SetField(t, "sub_type", lua.LString(ev.SubType))
		L.SetField(t, "operator_id", lua.LNumber(ev.OperatorID))
		L.SetField(t, "target_id", lua.LNumber(ev.TargetID))
		L.SetField(t, "duration", lua.LNumber(ev.Duration))
		if len(ev.File) > 0 {
			L.SetField(t, "file", goToLuaValue(L, ev.File))
		}
	}
	// request 字段
	if ev.RequestType != "" {
		L.SetField(t, "request_type", lua.LString(ev.RequestType))
		L.SetField(t, "comment", lua.LString(ev.Comment))
		L.SetField(t, "flag", lua.LString(ev.Flag))
	}
	// message 附加字段
	if ev.MessageID != 0 {
		// 以字符串传递：QQ message_id 可达 19 位（> 2^53），float64 会丢精度导致 delete_msg/get_msg 失效
		L.SetField(t, "message_id", lua.LString(strconv.FormatInt(ev.MessageID, 10)))
	}
	if len(ev.Sender) > 0 {
		L.SetField(t, "sender", goToLuaValue(L, ev.Sender))
	}
	return t
}

func goToLuaValue(L *lua.LState, v any) lua.LValue {
	switch val := v.(type) {
	case string:
		return lua.LString(val)
	case float64:
		return lua.LNumber(val)
	case int64:
		return lua.LNumber(val)
	case int:
		return lua.LNumber(val)
	case bool:
		return lua.LBool(val)
	case map[string]any:
		t := L.NewTable()
		for k, vv := range val {
			L.SetField(t, k, goToLuaValue(L, vv))
		}
		return t
	case []any:
		arr := L.NewTable()
		for i, item := range val {
			L.SetTable(arr, lua.LNumber(i+1), goToLuaValue(L, item))
		}
		return arr
	case []string:
		arr := L.NewTable()
		for i, item := range val {
			L.SetTable(arr, lua.LNumber(i+1), lua.LString(item))
		}
		return arr
	case []map[string]any:
		arr := L.NewTable()
		for i, item := range val {
			L.SetTable(arr, lua.LNumber(i+1), goToLuaValue(L, item))
		}
		return arr
	case nil:
		return lua.LNil
	default:
		data, _ := json.Marshal(v)
		return lua.LString(string(data))
	}
}

func luaValueToGo(v lua.LValue) any {
	switch val := v.(type) {
	case lua.LString:
		return string(val)
	case lua.LNumber:
		return float64(val)
	case lua.LBool:
		return bool(val)
	case *lua.LTable:
		if val.Len() > 0 {
			arr := make([]any, 0, val.Len())
			val.ForEach(func(_, v lua.LValue) {
				arr = append(arr, luaValueToGo(v))
			})
			return arr
		}
		m := make(map[string]any)
		val.ForEach(func(k, v lua.LValue) {
			m[k.String()] = luaValueToGo(v)
		})
		return m
	case *lua.LNilType:
		return nil
	default:
		return v.String()
	}
}

// ====================================================================
// 内嵌资源：SDK 与 system 插件
// ====================================================================

//go:embed sdk/jn.lua
var jnSDKSource string

//go:embed systemplugin/pluggin.yaml
var systemPluginManifest string

//go:embed systemplugin/main.lua
var systemPluginMain string

//go:embed systemplugin/avatar.png
var systemPluginAvatar []byte

// ensureEmbeddedAssets 在启动时把内嵌的 SDK 与 system 插件落盘到 data/pluggins/。
// SDK 与 system 插件始终覆盖写入，确保 Docker 挂载卷中的版本与二进制一致。
func (pe *PluginEngine) ensureEmbeddedAssets() {
	// 0. 确保 basePath 存在且可写
	if err := os.MkdirAll(pe.basePath, 0o755); err != nil {
		log.Error("无法创建插件根目录，内嵌资源写入跳过", "basePath", pe.basePath, "err", err)
		return
	}

	// 1. SDK 总是覆盖
	sdkDir := filepath.Join(pe.basePath, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		log.Warn("创建 SDK 目录失败", "err", err)
	} else {
		sdkFile := filepath.Join(sdkDir, "jn.lua")
		if err := os.WriteFile(sdkFile, []byte(jnSDKSource), 0o644); err != nil {
			log.Warn("写入 SDK 文件失败", "path", sdkFile, "err", err)
		} else {
			log.Info("SDK 已同步到磁盘", "path", sdkFile)
		}
	}

	// 2. system 插件始终覆盖（随二进制更新同步）
	sysDir := filepath.Join(pe.basePath, "system")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		log.Warn("创建 system 插件目录失败", "err", err)
		return
	}
	if err := os.WriteFile(filepath.Join(sysDir, "pluggin.yaml"), []byte(systemPluginManifest), 0o644); err != nil {
		log.Warn("写入 system pluggin.yaml 失败", "err", err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "main.lua"), []byte(systemPluginMain), 0o644); err != nil {
		log.Warn("写入 system main.lua 失败", "err", err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "avatar.png"), systemPluginAvatar, 0o644); err != nil {
		log.Warn("写入 system avatar.png 失败", "err", err)
	}
	log.Info("system 插件已同步到磁盘")
}
