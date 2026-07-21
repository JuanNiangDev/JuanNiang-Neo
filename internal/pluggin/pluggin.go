package pluggin

import (
	"bytes"
	"context"
	_ "embed"
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
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Active bool   `json:"active"`
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
			"name":        m.Name,
			"version":     m.Version,
			"author":      m.Author,
			"description": m.Description,
			"permissions": m.Permissions,
			"is_system":   m.System,
			"is_active":   true, // 已加载即视为 active
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
				"name":        manifest.Name,
				"version":     manifest.Version,
				"author":      manifest.Author,
				"description": manifest.Description,
				"permissions": manifest.Permissions,
				"is_system":   manifest.System,
				"is_active":   manifest.Enabled, // 从 YAML enabled 字段读取，兼容旧版默认 true
				"commands":    []map[string]any{},
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
		if c {
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

// ---------- OneBot11 ----------

func (pe *PluginEngine) injectOneBot11(L *lua.LState, pluginName string) {
	if pe.adapter == nil {
		return
	}
	adapter := pe.adapter

	obTable := L.NewTable()
	funcs := map[string]lua.LGFunction{
		"send_private_msg": func(L *lua.LState) int {
			_, err := adapter.SendPrivateMsg(int64(L.CheckNumber(1)), L.CheckString(2))
			return pushResult(L, err)
		},
		"send_group_msg": func(L *lua.LState) int {
			_, err := adapter.SendGroupMsg(int64(L.CheckNumber(1)), L.CheckString(2))
			return pushResult(L, err)
		},
		"delete_msg": func(L *lua.LState) int {
			err := adapter.DeleteMsg(int64(L.CheckNumber(1)))
			return pushResult(L, err)
		},
		"get_msg": func(L *lua.LState) int {
			msg, err := adapter.GetMsg(int64(L.CheckNumber(1)))
			return pushResultJSON(L, msg, err)
		},
		"get_group_info": func(L *lua.LState) int {
			info, err := adapter.GetGroupInfo(int64(L.CheckNumber(1)))
			return pushResultJSON(L, info, err)
		},
		"get_group_member_list": func(L *lua.LState) int {
			list, err := adapter.GetGroupMemberList(int64(L.CheckNumber(1)))
			return pushResultJSON(L, list, err)
		},
		"get_group_member_info": func(L *lua.LState) int {
			info, err := adapter.GetGroupMemberInfo(int64(L.CheckNumber(1)), int64(L.CheckNumber(2)))
			return pushResultJSON(L, info, err)
		},
		"get_group_honor_info": func(L *lua.LState) int {
			info, err := adapter.GetGroupHonorInfo(int64(L.CheckNumber(1)))
			return pushResultJSON(L, info, err)
		},
		"kick_group_member": func(L *lua.LState) int {
			n := L.GetTop()
			reject := false
			if n >= 3 {
				reject = bool(L.CheckBool(3))
			}
			err := adapter.KickGroupMember(int64(L.CheckNumber(1)), int64(L.CheckNumber(2)), reject)
			return pushResult(L, err)
		},
		"ban_group_member": func(L *lua.LState) int {
			err := adapter.BanGroupMember(int64(L.CheckNumber(1)), int64(L.CheckNumber(2)), int(L.CheckInt(3)))
			return pushResult(L, err)
		},
		"set_group_whole_ban": func(L *lua.LState) int {
			err := adapter.SetGroupWholeBan(int64(L.CheckNumber(1)), bool(L.CheckBool(2)))
			return pushResult(L, err)
		},
		"set_group_card": func(L *lua.LState) int {
			err := adapter.SetGroupCard(int64(L.CheckNumber(1)), int64(L.CheckNumber(2)), L.CheckString(3))
			return pushResult(L, err)
		},
		"handle_friend_request": func(L *lua.LState) int {
			err := adapter.HandleFriendRequest(L.CheckString(1), bool(L.CheckBool(2)), L.CheckString(3))
			return pushResult(L, err)
		},
		"handle_group_request": func(L *lua.LState) int {
			err := adapter.HandleGroupRequest(L.CheckString(1), L.CheckString(2), bool(L.CheckBool(3)), L.CheckString(4))
			return pushResult(L, err)
		},
		"get_login_info": func(L *lua.LState) int {
			info, err := adapter.GetLoginInfo()
			return pushResultJSON(L, info, err)
		},
		"get_stranger_info": func(L *lua.LState) int {
			info, err := adapter.GetStrangerInfo(int64(L.CheckNumber(1)))
			return pushResultJSON(L, info, err)
		},
		"get_friend_list": func(L *lua.LState) int {
			list, err := adapter.GetFriendList()
			return pushResultJSON(L, list, err)
		},
		"get_group_list": func(L *lua.LState) int {
			list, err := adapter.GetGroupList()
			return pushResultJSON(L, list, err)
		},
		"send_like": func(L *lua.LState) int {
			err := adapter.SendLike(int64(L.CheckNumber(1)), int(L.CheckInt(2)))
			return pushResult(L, err)
		},
		"get_status": func(L *lua.LState) int {
			s, err := adapter.GetStatus()
			return pushResultJSON(L, s, err)
		},
		"get_version_info": func(L *lua.LState) int {
			v, err := adapter.GetVersionInfo()
			return pushResultJSON(L, v, err)
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
			L.Push(lua.LString(resp.ID))
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
			if agentOp == nil {
				return pushResult(L, fmt.Errorf("agent operator 不可用"))
			}
			mcpGroup := agentOp.GetMCPGroup()
			list := mcpGroup.ListMCPs()
			return pushResultJSON(L, list, nil)
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
// - SDK 始终覆盖（保持与二进制版本一致，IDE 类型与运行时同步）
// - system 插件仅在不存在时写入（允许用户自定义修改）
func (pe *PluginEngine) ensureEmbeddedAssets() {
	// 1. SDK 总是覆盖
	sdkDir := filepath.Join(pe.basePath, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		slog.Warn("创建 SDK 目录失败", "err", err)
	} else {
		sdkFile := filepath.Join(sdkDir, "jn.lua")
		if err := os.WriteFile(sdkFile, []byte(jnSDKSource), 0o644); err != nil {
			slog.Warn("写入 SDK 文件失败", "err", err)
		}
	}

	// 2. system 插件仅在不存在时写入
	sysDir := filepath.Join(pe.basePath, "system")
	if _, err := os.Stat(filepath.Join(sysDir, "pluggin.yaml")); os.IsNotExist(err) {
		if mkErr := os.MkdirAll(sysDir, 0o755); mkErr != nil {
			slog.Warn("创建 system 插件目录失败", "err", mkErr)
			return
		}
		if err := os.WriteFile(filepath.Join(sysDir, "pluggin.yaml"), []byte(systemPluginManifest), 0o644); err != nil {
			slog.Warn("写入 system pluggin.yaml 失败", "err", err)
		}
		if err := os.WriteFile(filepath.Join(sysDir, "main.lua"), []byte(systemPluginMain), 0o644); err != nil {
			slog.Warn("写入 system main.lua 失败", "err", err)
		}
		slog.Info("system 插件已落盘")
	}
}
