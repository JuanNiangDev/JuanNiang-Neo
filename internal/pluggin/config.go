package pluggin

import (
	"fmt"
	"os"
	"path/filepath"

	lua "github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"
)

// ====================================================================
// 插件动态配置（config.yaml）
// ====================================================================

// PluginConfigItem 描述一个可配置项（schema + 当前值）。
type PluginConfigItem struct {
	Key         string   `yaml:"key"`
	Type        string   `yaml:"type"` // bool | string | list
	Label       string   `yaml:"label"`
	Description string   `yaml:"description"`
	Default     any      `yaml:"default"`
	Value       any      `yaml:"value,omitempty"`
	Options     []string `yaml:"options,omitempty"`
}

// PluginConfig 是 config.yaml 的根结构。
type PluginConfig struct {
	Configs []PluginConfigItem `yaml:"configs"`
}

var validConfigTypes = map[string]bool{"bool": true, "string": true, "list": true}

func (pe *PluginEngine) configPath(dir string) string {
	return filepath.Join(dir, "config.yaml")
}

func (pe *PluginEngine) configDir(name string) string {
	return filepath.Join(pe.basePath, name)
}

func (pe *PluginEngine) readConfig(name string) (*PluginConfig, error) {
	path := pe.configPath(pe.configDir(name))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PluginConfig{}, nil
		}
		return nil, err
	}
	var cfg PluginConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (pe *PluginEngine) writeConfig(name string, cfg *PluginConfig) error {
	path := pe.configPath(pe.configDir(name))
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func normalizeConfigValue(item *PluginConfigItem) any {
	v := item.Value
	if v == nil {
		v = item.Default
	}
	switch item.Type {
	case "bool":
		b, _ := v.(bool)
		return b
	case "list":
		switch t := v.(type) {
		case []string:
			return t
		case []any:
			out := make([]string, 0, len(t))
			for _, x := range t {
				out = append(out, fmt.Sprintf("%v", x))
			}
			return out
		case string:
			if t == "" {
				return []string{}
			}
			return []string{t}
		}
		return []string{}
	case "string":
		if v == nil {
			return ""
		}
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	default:
		return v
	}
}

func (pe *PluginEngine) configSchema(name string) []map[string]any {
	cfg, err := pe.readConfig(name)
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(cfg.Configs))
	for i := range cfg.Configs {
		item := &cfg.Configs[i]
		out = append(out, map[string]any{
			"key":         item.Key,
			"type":        item.Type,
			"label":       item.Label,
			"description": item.Description,
			"default":     normalizeConfigValue(item),
			"value":       normalizeConfigValue(item),
			"options":     item.Options,
		})
	}
	return out
}

func (pe *PluginEngine) configValues(name string) map[string]any {
	cfg, err := pe.readConfig(name)
	if err != nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(cfg.Configs))
	for i := range cfg.Configs {
		item := &cfg.Configs[i]
		out[item.Key] = normalizeConfigValue(item)
	}
	return out
}

func (pe *PluginEngine) configValue(name, key string) any {
	return pe.configValues(name)[key]
}

// SaveConfig 保存某插件的配置值（写入 config.yaml 的 value 字段）。
func (pe *PluginEngine) SaveConfig(name string, values map[string]any) error {
	cfg, err := pe.readConfig(name)
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	for i := range cfg.Configs {
		item := &cfg.Configs[i]
		if v, ok := values[item.Key]; ok {
			item.Value = sanitizeConfigValue(item.Type, v)
		}
	}
	if err := pe.writeConfig(name, cfg); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	_ = pe.ReloadIfLoaded(name)
	return nil
}

// ReloadIfLoaded 若插件已加载则重载（配置变更后应用生效）。
func (pe *PluginEngine) ReloadIfLoaded(name string) error {
	pe.mu.Lock()
	_, loaded := pe.plugins[name]
	pe.mu.Unlock()
	if !loaded {
		return nil
	}
	return pe.Reload(name)
}

func sanitizeConfigValue(typ string, v any) any {
	switch typ {
	case "bool":
		switch t := v.(type) {
		case bool:
			return t
		case string:
			return t == "true" || t == "1" || t == "on"
		case float64:
			return t != 0
		}
		return false
	case "list":
		switch t := v.(type) {
		case []any:
			out := make([]string, 0, len(t))
			for _, x := range t {
				out = append(out, fmt.Sprintf("%v", x))
			}
			return out
		case []string:
			return t
		case string:
			return []string{t}
		}
		return []string{}
	default: // string
		if t, ok := v.(string); ok {
			return t
		}
		return fmt.Sprintf("%v", v)
	}
}

// injectConfigAPI 向插件 Lua 状态注入 config 全局表（无需权限，默认注入）。
func (pe *PluginEngine) injectConfigAPI(L *lua.LState, pluginName string) {
	configTable := L.NewTable()
	L.SetFuncs(configTable, map[string]lua.LGFunction{
		"get": func(L *lua.LState) int {
			key := L.CheckString(1)
			L.Push(goToLuaValue(L, pe.configValue(pluginName, key)))
			return 1
		},
		"all": func(L *lua.LState) int {
			L.Push(goToLuaValue(L, pe.configValues(pluginName)))
			return 1
		},
		"schema": func(L *lua.LState) int {
			L.Push(goToLuaValue(L, pe.configSchema(pluginName)))
			return 1
		},
	})
	L.SetGlobal("config", configTable)
}

// ConfigSchemaMap 供 Web API 使用（返回 schema + values）。
func (pe *PluginEngine) ConfigSchemaMap(name string) map[string]any {
	return map[string]any{
		"schema": pe.configSchema(name),
		"values": pe.configValues(name),
	}
}

// GetReadme 返回插件的 README.md 内容。
func (pe *PluginEngine) GetReadme(name string) (string, error) {
	path := filepath.Join(pe.configDir(name), "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetAvatar 返回插件的 avatar.png 内容。
func (pe *PluginEngine) GetAvatar(name string) ([]byte, error) {
	path := filepath.Join(pe.configDir(name), "avatar.png")
	return os.ReadFile(path)
}
