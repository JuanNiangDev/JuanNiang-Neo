package pluggin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"JuanNiang-Neo/internal/core/cache"
	"JuanNiang-Neo/internal/core/dao"
	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
)

// ---------- 清单 ----------

type Manifest struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Author      string   `yaml:"author"`
	Description string   `yaml:"description"`
	Entry       string   `yaml:"entry"`
	Permissions []string `yaml:"permissions"`
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
	CompactMemory(ctx context.Context, chatAreaID string) error
	GetChatAreaID(userID, groupID int64, messageType string) string
	GetProviderGroup() ProviderGroupAccess
	GetMCPGroup() MCPGroupAccess
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

type ProviderInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type MCPInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Active  bool   `json:"active"`
}

// ---------- 引擎 ----------

type PluginEngine struct {
	mu         sync.RWMutex
	plugins    map[string]*LoadedPlugin
	basePath   string
	adapter    SendAdapter
	db         *gorm.DB
	cache      *cache.Cache
	t2i        *t2icaller.Client
	sandbox    *sandboxcaller.Client
	dao        *dao.Bundle
	agentOp    AgentOperator
	currentEv  EventData
}

func NewPluginEngine(basePath string, adapter SendAdapter, db *gorm.DB, c *cache.Cache, t2i *t2icaller.Client, sb *sandboxcaller.Client, d *dao.Bundle, ag AgentOperator) *PluginEngine {
	if basePath == "" {
		basePath = "data/pluggins"
	}
	return &PluginEngine{
		plugins:  make(map[string]*LoadedPlugin),
		basePath: basePath,
		adapter:  adapter,
		db:       db,
		cache:    c,
		t2i:      t2i,
		sandbox:  sb,
		dao:      d,
		agentOp:  ag,
	}
}

func (pe *PluginEngine) LoadAll() error {
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
	pe.injectBaseAPI(L, name, manifest.Permissions)

	entryFile := filepath.Join(pluginDir, manifest.Entry)
	if err := L.DoFile(entryFile); err != nil {
		L.Close()
		return fmt.Errorf("执行 entry 失败: %w", err)
	}

	pe.plugins[name] = &LoadedPlugin{Manifest: *manifest, State: L, Dir: pluginDir}
	slog.Info("插件加载成功", "name", name, "version", manifest.Version)
	return nil
}

func (pe *PluginEngine) Unload(name string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	p, ok := pe.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not loaded", name)
	}
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
func (pe *PluginEngine) ListMaps() []map[string]any {
	list := pe.List()
	out := make([]map[string]any, len(list))
	for i, m := range list {
		out[i] = map[string]any{
			"name":        m.Name,
			"version":     m.Version,
			"author":      m.Author,
			"description": m.Description,
		}
	}
	return out
}

// ---------- 事件 ----------

type EventData struct {
	PostType    string `json:"post_type"`
	MessageType string `json:"message_type"`
	UserID      int64  `json:"user_id"`
	GroupID     int64  `json:"group_id"`
	RawMessage  string `json:"raw_message"`
}

func (pe *PluginEngine) OnMessage(event EventData) (consumed bool) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	pe.currentEv = event

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

	if pe.t2i == nil {
		L.SetFuncs(t2iTable, map[string]lua.LGFunction{
			"generate": func(L *lua.LState) int {
				L.Push(lua.LNil)
				L.Push(lua.LString("T2I 服务未启用"))
				return 2
			},
		})
		L.SetGlobal("t2i", t2iTable)
		return
	}

	t2i := pe.t2i
	L.SetFuncs(t2iTable, map[string]lua.LGFunction{
		"generate": func(L *lua.LState) int {
			html := L.CheckString(1)
			resp, err := t2i.Generate(context.Background(), t2icaller.GenerateRequest{
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
			html := L.CheckString(1)
			url, err := t2i.GenerateURL(context.Background(), t2icaller.GenerateRequest{
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
	})
	L.SetGlobal("t2i", t2iTable)
}

// ---------- Sandbox ----------

func (pe *PluginEngine) injectSandbox(L *lua.LState, pluginName string) {
	sbTable := L.NewTable()

	if pe.sandbox == nil {
		L.SetFuncs(sbTable, map[string]lua.LGFunction{
			"exec_shell": func(L *lua.LState) int {
				L.Push(lua.LNil)
				L.Push(lua.LString("Sandbox 服务未启用"))
				return 2
			},
			"exec_python": func(L *lua.LState) int {
				L.Push(lua.LNil)
				L.Push(lua.LString("Sandbox 服务未启用"))
				return 2
			},
			"create": func(L *lua.LState) int {
				L.Push(lua.LNil)
				L.Push(lua.LString("Sandbox 服务未启用"))
				return 2
			},
		})
		L.SetGlobal("sandbox", sbTable)
		return
	}

	sb := pe.sandbox
	L.SetFuncs(sbTable, map[string]lua.LGFunction{
		"create": func(L *lua.LState) int {
			sbox, err := sb.CreateSandbox(context.Background(), sandboxcaller.CreateSandboxRequest{})
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
			sid := L.CheckString(1)
			cmd := L.CheckString(2)
			result, err := sb.ExecShell(context.Background(), sid, sandboxcaller.ShellExecRequest{Command: cmd})
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
			sid := L.CheckString(1)
			code := L.CheckString(2)
			result, err := sb.ExecPython(context.Background(), sid, sandboxcaller.PythonExecRequest{Code: code})
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
	return &m, nil
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
