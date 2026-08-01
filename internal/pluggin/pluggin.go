package pluggin

import (
	"bytes"
	"context"
	crand "crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"JuanNiang-Neo/internal/adapter"

	lua "github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

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
}

// ---------- 适配器接口 ----------

type SendAdapter interface {
	SendPrivateMsg(userID int64, message any) (int64, error)
	SendGroupMsg(groupID int64, message any) (int64, error)
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
}

// ProviderGroupAccess 暴露给插件的 Provider 管理接口。
type ProviderGroupAccess interface {
	List() []ProviderInfo
	GetActive(id string) bool
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
	currentEv EventData
	commands  *CommandRegistry
}

func NewPluginEngine(basePath string, adapter SendAdapter, db *gorm.DB, c *cache.Cache, t2i *t2icaller.Client, sb *sandboxcaller.Client, d *dao.Bundle, ag AgentOperator) *PluginEngine {
	if basePath == "" {
		basePath = "data/pluggins"
	}
	pe := &PluginEngine{
		plugins:  make(map[string]*LoadedPlugin),
		basePath: basePath,
		adapter:  adapter,
		db:       db,
		cache:    c,
		t2i:      t2i,
		sandbox:  sb,
		dao:      d,
		agentOp:  ag,
		commands: NewCommandRegistry(),
	}
	// 注册内置 /help 命令
	pe.registerBuiltinCommands()
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
func (pe *PluginEngine) registerBuiltinCommands() {
	pe.commands.Register("system", []string{"help"}, CommandOpts{
		Description: "查看所有可用命令，或查看某个命令的子命令与用法",
		Usage:       "/help [命令路径...]",
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
			slog.Info("插件已禁用，跳过加载", "name", entry.Name())
			continue
		}
		if err := pe.Load(entry.Name()); err != nil {
			slog.Error("插件加载失败", "name", entry.Name(), "err", err)
		}
	}
	return nil
}

func (pe *PluginEngine) Load(name string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	if _, ok := pe.plugins[name]; ok {
		return fmt.Errorf("plugin %q already loaded", name)
	}

	pluginDir := filepath.Join(pe.basePath, name)
	manifest, err := pe.readManifest(pluginDir)
	if err != nil {
		return fmt.Errorf("读取 pluggin.yaml 失败: %w", err)
	}

	// PPID 为空时自动生成并写回 yaml，保证每个插件都有稳定唯一标识
	if manifest.PPID == "" {
		manifest.PPID = newPluginUUID()
		if werr := pe.writeManifest(pluginDir, manifest); werr != nil {
			slog.Warn("写回插件 PPID 失败", "name", name, "err", werr)
		} else {
			slog.Info("已为插件生成 PPID", "name", name, "ppid", manifest.PPID)
		}
	}

	L := lua.NewState()
	// 先注入 SDK：让插件可以使用 require("jn")
	pe.injectSDK(L, name)
	pe.injectBaseAPI(L, name, manifest.Permissions)
	// 注入命令注册 API（依赖当前 plugin name）
	pe.injectCommandAPI(L, name)

	entryFile := filepath.Join(pluginDir, manifest.Entry)
	if err := L.DoFile(entryFile); err != nil {
		L.Close()
		return fmt.Errorf("执行 entry 失败: %w", err)
	}

	pe.plugins[name] = &LoadedPlugin{Manifest: *manifest, State: L, Dir: pluginDir}
	slog.Info("插件加载成功", "name", name, "version", manifest.Version, "system", manifest.System)
	return nil
}

func (pe *PluginEngine) Unload(name string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	p, ok := pe.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not loaded", name)
	}
	// 系统插件禁止卸载
	if p.Manifest.System {
		return fmt.Errorf("system 插件 %q 不允许卸载", name)
	}
	// 移除该插件注册的所有命令
	pe.commands.UnregisterPlugin(name)

	p.State.Close()
	delete(pe.plugins, name)
	slog.Info("插件已卸载", "name", name)
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
			"id":             name, // 目录名，用于 API 操作 (toggle/delete)
			"name":           m.Name,
			"version":        m.Version,
			"author":         m.Author,
			"description":    m.Description,
			"permissions":    m.Permissions,
			"is_system":      m.System,
			"is_active":      true, // 已加载即视为 active
			"supports_timer": p.SupportsTimer(),
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
				"id":             entry.Name(), // 目录名，用于 API 操作
				"name":           manifest.Name,
				"version":        manifest.Version,
				"author":         manifest.Author,
				"description":    manifest.Description,
				"permissions":    manifest.Permissions,
				"is_system":      manifest.System,
				"is_active":      manifest.Enabled, // 从 YAML enabled 字段读取，兼容旧版默认 true
				"supports_timer": false,            // 未加载的插件无法检测，默认 false
				"commands":       []map[string]any{},
			})
		}
	}

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

// HasPluginCommand 检查消息是否匹配已注册的插件命令（不执行，仅供策略层判断）。
func (pe *PluginEngine) HasPluginCommand(raw string) bool {
	return pe.commands.HasCommand(raw)
}

func (pe *PluginEngine) OnMessage(event EventData) (consumed bool) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	pe.currentEv = event

	// 1. 优先派发给命令注册表（/cmd subcmd ...）
	if strings.HasPrefix(strings.TrimSpace(event.RawMessage), "/") {
		c, reply, err := pe.commands.Dispatch(event.RawMessage, event)
		if err != nil {
			slog.Error("命令派发错误", "raw", event.RawMessage, "err", err)
		}
		// 只要命令已消费或有回复内容，都视为已处理（避免消息流入 Agent）
		if c || reply != "" {
			if reply != "" {
				pe.sendReply(event, reply)
			}
			return true
		}
	}

	// 2. 没有命令命中，按原逻辑派发给插件的 on_message
	for _, p := range pe.plugins {
		if !p.HasPermission("onebot11") {
			continue
		}
		fn := p.State.GetGlobal("on_message")
		if fn.Type() != lua.LTFunction {
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 2, nil); err != nil {
			slog.Error("插件 on_message 错误", "plugin", p.Manifest.Name, "err", err)
			continue
		}
		consumedRet := p.State.Get(-2)
		p.State.Pop(2)
		if consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool)) {
			return true
		}
	}
	return false
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
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	pe.currentEv = event

	for _, p := range pe.plugins {
		if !p.HasPermission("webhook") {
			continue
		}
		fn := p.State.GetGlobal("on_webhook")
		if fn.Type() != lua.LTFunction {
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 2, nil); err != nil {
			slog.Error("插件 on_webhook 错误", "plugin", p.Manifest.Name, "err", err)
			continue
		}
		consumedRet := p.State.Get(-2)
		p.State.Pop(2)
		if consumedRet.Type() == lua.LTBool && bool(consumedRet.(lua.LBool)) {
			return true
		}
	}
	return false
}

// RouteWebhook routes a webhook request to a specific plugin by name.
// Returns (consumed, reply).
func (pe *PluginEngine) RouteWebhook(pluginName string, path string, method string, payload map[string]any) (consumed bool, reply string) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	for _, p := range pe.plugins {
		if p.Manifest.Name != pluginName {
			continue
		}
		if !p.HasPermission("webhook") {
			return false, "plugin does not have webhook permission"
		}
		fn := p.State.GetGlobal("on_webhook")
		if fn.Type() != lua.LTFunction {
			return false, "plugin has no on_webhook handler"
		}
		event := EventData{
			PostType: "webhook",
			Webhook: map[string]any{
				"path":    path,
				"method":  method,
				"payload": payload,
			},
		}
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
	return false, "plugin not found"
}

// OnNotice 通知事件（群成员增减、禁言、文件上传等）。
func (pe *PluginEngine) OnNotice(event EventData) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	for _, p := range pe.plugins {
		fn := p.State.GetGlobal("on_notice")
		if fn.Type() != lua.LTFunction {
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 0, nil); err != nil {
			slog.Error("插件 on_notice 错误", "plugin", p.Manifest.Name, "err", err)
		}
	}
}

// OnRequest 请求事件（加好友、加群邀请）。
func (pe *PluginEngine) OnRequest(event EventData) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	for _, p := range pe.plugins {
		fn := p.State.GetGlobal("on_request")
		if fn.Type() != lua.LTFunction {
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 0, nil); err != nil {
			slog.Error("插件 on_request 错误", "plugin", p.Manifest.Name, "err", err)
		}
	}
}

// OnTimerCall 定时任务触发插件 on_timer_call 回调。
// pluginIDs 指定要调用的插件（目录名列表），payload 为 CronJob 配置的 JSON payload，
// admins 为系统管理员 QQ 列表。
// 实现 cronjob.PluginTimerDispatcher 接口。
func (pe *PluginEngine) OnTimerCall(pluginIDs []string, payload map[string]any, admins []string) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	event := EventData{
		PostType: "timer",
		Admins:   admins,
		Payload:  payload,
	}

	for _, name := range pluginIDs {
		p, ok := pe.plugins[name]
		if !ok {
			slog.Warn("OnTimerCall: 插件未加载", "plugin", name)
			continue
		}
		fn := p.State.GetGlobal("on_timer_call")
		if fn.Type() != lua.LTFunction {
			slog.Warn("OnTimerCall: 插件未定义 on_timer_call", "plugin", name)
			continue
		}
		table := eventToLuaTable(p.State, event)
		p.State.Push(fn)
		p.State.Push(table)
		if err := p.State.PCall(1, 0, nil); err != nil {
			slog.Error("插件 on_timer_call 错误", "plugin", name, "err", err)
		}
	}
}

// SupportsTimer 检查插件是否支持定时任务回调（定义了 on_timer_call 全局函数）。
func (p *LoadedPlugin) SupportsTimer() bool {
	fn := p.State.GetGlobal("on_timer_call")
	return fn.Type() == lua.LTFunction
}

// ReloadAll 卸载所有非系统插件后重新加载全部插件。
func (pe *PluginEngine) ReloadAll() error {
	pe.mu.Lock()
	// 先卸载非系统插件
	for name, p := range pe.plugins {
		if p.Manifest.System {
			continue
		}
		pe.commands.UnregisterPlugin(name)
		p.State.Close()
		delete(pe.plugins, name)
	}
	pe.mu.Unlock()

	// LoadAll 在内部会重新取锁并加载所有插件
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

	// Logger
	logTable := L.NewTable()
	L.SetFuncs(logTable, map[string]lua.LGFunction{
		"info": func(L *lua.LState) int {
			slog.Info("[plugin:"+pluginName+"]", "msg", L.CheckString(1))
			return 0
		},
		"warn": func(L *lua.LState) int {
			slog.Warn("[plugin:"+pluginName+"]", "msg", L.CheckString(1))
			return 0
		},
		"error": func(L *lua.LState) int {
			slog.Error("[plugin:"+pluginName+"]", "msg", L.CheckString(1))
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

	// Agent
	if hasPerm("agent") && pe.dao != nil {
		pe.injectAgent(L)
	}
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
		slog.Warn("设置 package.path 失败", "plugin", pluginName, "err", err)
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
	resolveImage := func(path string) string {
		if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "base64://") {
			return path
		}
		pluginDir := filepath.Join(pe.basePath, pluginName)
		fullPath := filepath.Join(pluginDir, path)
		t0 := time.Now()
		data, err := os.ReadFile(fullPath)
		readDur := time.Since(t0)
		if err != nil {
			slog.Warn("读取插件图片文件失败", "plugin", pluginName, "path", fullPath, "err", err)
			return path
		}
		ext := strings.TrimPrefix(filepath.Ext(fullPath), ".")
		if ext == "" {
			ext = "png"
		}
		t1 := time.Now()
		b64 := "base64://" + base64.StdEncoding.EncodeToString(data)
		encDur := time.Since(t1)
		slog.Debug("插件图片处理耗时", "plugin", pluginName, "file", path, "size_bytes", len(data),
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
			// 插件发消息异步，不阻塞命令 handler 返回
			go func() {
				t0 := time.Now()
				if _, err := sendAdp.SendPrivateMsg(userID, msg); err != nil {
					slog.Warn("插件异步发送私聊消息失败", "plugin", pluginName, "user_id", userID, "err", err)
				} else {
					slog.Debug("插件异步发送私聊消息完成", "plugin", pluginName, "user_id", userID, "dur_ms", time.Since(t0).Milliseconds())
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
			// 插件发消息异步，不阻塞命令 handler 返回
			go func() {
				t0 := time.Now()
				if _, err := sendAdp.SendGroupMsg(groupID, msg); err != nil {
					slog.Warn("插件异步发送群消息失败", "plugin", pluginName, "group_id", groupID, "err", err)
				} else {
					slog.Debug("插件异步发送群消息完成", "plugin", pluginName, "group_id", groupID, "dur_ms", time.Since(t0).Milliseconds())
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
			_, err := sendAdp.SendGroupMsg(groupID, msg)
			return pushResult(L, err)
		},
		"delete_msg": func(L *lua.LState) int {
			err := sendAdp.DeleteMsg(int64(L.CheckNumber(1)))
			return pushResult(L, err)
		},
		"get_msg": func(L *lua.LState) int {
			msg, err := sendAdp.GetMsg(int64(L.CheckNumber(1)))
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

func (pe *PluginEngine) injectHTTP(L *lua.LState, pluginName string) {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	httpTable := L.NewTable()
	L.SetFuncs(httpTable, map[string]lua.LGFunction{
		"get": func(L *lua.LState) int {
			url := L.CheckString(1)
			resp, err := httpClient.Get(url)
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
		"post": func(L *lua.LState) int {
			url := L.CheckString(1)
			contentType := "application/json"
			var bodyStr string
			if L.GetTop() >= 3 {
				contentType = L.CheckString(2)
				bodyStr = L.CheckString(3)
			} else if L.GetTop() >= 2 {
				bodyStr = L.CheckString(2)
			}
			resp, err := httpClient.Post(url, contentType, bytes.NewBufferString(bodyStr))
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
	})
	L.SetGlobal("http", httpTable)
}

// ---------- Database ----------

func (pe *PluginEngine) injectDatabase(L *lua.LState, pluginName string) {
	prefix := "pluggin_" + pluginName + "_"
	db := pe.db

	dbTable := L.NewTable()
	L.SetFuncs(dbTable, map[string]lua.LGFunction{
		"query": func(L *lua.LState) int {
			sql := L.CheckString(1)
			sql = prefixSQL(sql, prefix)

			rows, err := db.Raw(sql).Rows()
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
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
		},
		"exec": func(L *lua.LState) int {
			sql := L.CheckString(1)
			sql = prefixSQL(sql, prefix)
			result := db.Exec(sql)
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
			resp, err := client.Generate(context.Background(), t2icaller.GenerateRequest{
				HTML:   html,
				AsJSON: true,
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
			url, err := client.GenerateURL(context.Background(), t2icaller.GenerateRequest{
				HTML:   html,
				AsJSON: true,
			})
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			L.Push(lua.LString(url))
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
	engine := pe

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

		// 当前 Chat-Area
		"get_current_chat_area": func(L *lua.LState) int {
			ev := engine.currentEv
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
			ev := engine.currentEv
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
		L.SetField(t, "message_id", lua.LNumber(ev.MessageID))
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

// ensureEmbeddedAssets 在启动时把内嵌的 SDK 与 system 插件落盘到 data/pluggins/。
// SDK 与 system 插件始终覆盖写入，确保 Docker 挂载卷中的版本与二进制一致。
func (pe *PluginEngine) ensureEmbeddedAssets() {
	// 0. 确保 basePath 存在且可写
	if err := os.MkdirAll(pe.basePath, 0o755); err != nil {
		slog.Error("无法创建插件根目录，内嵌资源写入跳过", "basePath", pe.basePath, "err", err)
		return
	}

	// 1. SDK 总是覆盖
	sdkDir := filepath.Join(pe.basePath, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		slog.Warn("创建 SDK 目录失败", "err", err)
	} else {
		sdkFile := filepath.Join(sdkDir, "jn.lua")
		if err := os.WriteFile(sdkFile, []byte(jnSDKSource), 0o644); err != nil {
			slog.Warn("写入 SDK 文件失败", "path", sdkFile, "err", err)
		} else {
			slog.Info("SDK 已同步到磁盘", "path", sdkFile)
		}
	}

	// 2. system 插件始终覆盖（随二进制更新同步）
	sysDir := filepath.Join(pe.basePath, "system")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		slog.Warn("创建 system 插件目录失败", "err", err)
		return
	}
	if err := os.WriteFile(filepath.Join(sysDir, "pluggin.yaml"), []byte(systemPluginManifest), 0o644); err != nil {
		slog.Warn("写入 system pluggin.yaml 失败", "err", err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "main.lua"), []byte(systemPluginMain), 0o644); err != nil {
		slog.Warn("写入 system main.lua 失败", "err", err)
	}
	slog.Info("system 插件已同步到磁盘")
}
