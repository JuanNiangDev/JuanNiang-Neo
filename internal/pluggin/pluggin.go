package pluggin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"
)

// Manifest 插件清单 (pluggin.yaml)。
type Manifest struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Author      string   `yaml:"author"`
	Description string   `yaml:"description"`
	Entry       string   `yaml:"entry"`
	Permissions []string `yaml:"permissions"`
}

// LoadedPlugin 已加载的插件。
type LoadedPlugin struct {
	Manifest Manifest
	State    *lua.LState
	Dir      string
}

// SendAdapter 插件可调用的消息发送接口。
type SendAdapter interface {
	SendPrivateMsg(userID int64, message any) (int64, error)
	SendGroupMsg(groupID int64, message any) (int64, error)
}

// PluginEngine Lua 插件引擎。
type PluginEngine struct {
	mu       sync.RWMutex
	plugins  map[string]*LoadedPlugin
	basePath string
	adapter  SendAdapter
}

func NewPluginEngine(basePath string, adapter SendAdapter) *PluginEngine {
	if basePath == "" {
		basePath = "data/pluggins"
	}
	return &PluginEngine{
		plugins:  make(map[string]*LoadedPlugin),
		basePath: basePath,
		adapter:  adapter,
	}
}

// LoadAll 加载所有已安装的插件。
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

// Load 加载单个插件。
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

	pe.plugins[name] = &LoadedPlugin{
		Manifest: *manifest,
		State:    L,
		Dir:      pluginDir,
	}

	slog.Info("插件加载成功", "name", name, "version", manifest.Version)
	return nil
}

// Unload 卸载插件。
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

// Reload 热加载插件。
func (pe *PluginEngine) Reload(name string) error {
	if err := pe.Unload(name); err != nil {
		return err
	}
	return pe.Load(name)
}

// List 列出所有已加载插件。
func (pe *PluginEngine) List() []Manifest {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	list := make([]Manifest, 0, len(pe.plugins))
	for _, p := range pe.plugins {
		list = append(list, p.Manifest)
	}
	return list
}

// HasPermission 检查插件是否有某权限。
func (p *LoadedPlugin) HasPermission(perm string) bool {
	for _, pp := range p.Manifest.Permissions {
		if pp == perm || pp == "*" {
			return true
		}
	}
	return false
}

// ---------- 事件处理 ----------

// EventData 传给插件的原始事件数据。
type EventData struct {
	PostType    string `json:"post_type"`
	MessageType string `json:"message_type"`
	UserID      int64  `json:"user_id"`
	GroupID     int64  `json:"group_id"`
	RawMessage  string `json:"raw_message"`
}

// OnMessage 调用插件的 on_message 回调。
func (pe *PluginEngine) OnMessage(event EventData) (consumed bool) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

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

// ---------- 内部方法 ----------

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
			msg := L.CheckString(1)
			slog.Info("[plugin:"+pluginName+"]", "msg", msg)
			return 0
		},
		"warn": func(L *lua.LState) int {
			msg := L.CheckString(1)
			slog.Warn("[plugin:"+pluginName+"]", "msg", msg)
			return 0
		},
		"error": func(L *lua.LState) int {
			msg := L.CheckString(1)
			slog.Error("[plugin:"+pluginName+"]", "msg", msg)
			return 0
		},
	})
	L.SetGlobal("log", logTable)

	// JSON
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

	// OneBot11 (if permitted)
	if hasPerm("onebot11") && pe.adapter != nil {
		obTable := L.NewTable()
		adapter := pe.adapter
		L.SetFuncs(obTable, map[string]lua.LGFunction{
			"send_private_msg": func(L *lua.LState) int {
				userID := int64(L.CheckNumber(1))
				msg := L.CheckString(2)
				_, err := adapter.SendPrivateMsg(userID, msg)
				if err != nil {
					L.Push(lua.LBool(false))
					L.Push(lua.LString(err.Error()))
					return 2
				}
				L.Push(lua.LBool(true))
				return 1
			},
			"send_group_msg": func(L *lua.LState) int {
				groupID := int64(L.CheckNumber(1))
				msg := L.CheckString(2)
				_, err := adapter.SendGroupMsg(groupID, msg)
				if err != nil {
					L.Push(lua.LBool(false))
					L.Push(lua.LString(err.Error()))
					return 2
				}
				L.Push(lua.LBool(true))
				return 1
			},
		})
		L.SetGlobal("onebot11", obTable)
	}

	// HTTP (if permitted) — 基础实现
	if hasPerm("http") {
		httpTable := L.NewTable()
		L.SetFuncs(httpTable, map[string]lua.LGFunction{
			"get": func(L *lua.LState) int {
				L.Push(lua.LString("http.get not implemented"))
				return 1
			},
			"post": func(L *lua.LState) int {
				L.Push(lua.LString("http.post not implemented"))
				return 1
			},
		})
		L.SetGlobal("http", httpTable)
	}
}

// ---------- 工具函数 ----------

func eventToLuaTable(L *lua.LState, ev EventData) *lua.LTable {
	t := L.NewTable()
	L.SetField(t, "post_type", lua.LString(ev.PostType))
	L.SetField(t, "message_type", lua.LString(ev.MessageType))
	L.SetField(t, "user_id", lua.LNumber(ev.UserID))
	L.SetField(t, "group_id", lua.LNumber(ev.GroupID))
	L.SetField(t, "raw_message", lua.LString(ev.RawMessage))
	return t
}

func luaTableToMap(t *lua.LTable) map[string]any {
	m := make(map[string]any)
	t.ForEach(func(k, v lua.LValue) {
		m[k.String()] = luaValueToGo(v)
	})
	return m
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
		return mapToLuaTable(L, val)
	case []any:
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

func mapToLuaTable(L *lua.LState, m map[string]any) *lua.LTable {
	t := L.NewTable()
	for k, v := range m {
		L.SetField(t, k, goToLuaValue(L, v))
	}
	return t
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
		return luaTableToMap(val)
	case *lua.LNilType:
		return nil
	default:
		return v.String()
	}
}
