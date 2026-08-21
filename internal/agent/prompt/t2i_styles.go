package prompt

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"strings"
	"sync/atomic"
)

// t2iStylePlaceholder 系统锁定提示词中风格注入的占位符。
const t2iStylePlaceholder = "{{T2I_STYLE}}"

// t2iStylesDefaultPath 风格库默认路径，可用环境变量 T2I_STYLES_FILE 覆盖。
const t2iStylesDefaultPath = "data/t2i_styles.json"

// t2iStyle 单条风格定义。
type t2iStyle struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Vibe     string   `json:"vibe"`
	Palette  string   `json:"palette"`
	Accents  string   `json:"accents"`
	Layout   string   `json:"layout"`
}

// format 将风格格式化为注入提示词的文本块。
func (s *t2iStyle) format() string {
	var b strings.Builder
	b.WriteString(s.Name)
	b.WriteString(" —— ")
	b.WriteString(s.Vibe)
	b.WriteString("\n- 底色/正文/标题：")
	b.WriteString(s.Palette)
	b.WriteString("\n- 强调：")
	b.WriteString(s.Accents)
	b.WriteString("\n- 版式：")
	b.WriteString(s.Layout)
	return b.String()
}

// t2iStylesPath 实际路径；优先取环境变量 T2I_STYLES_FILE（非空去除空白后），
// 未设置或为空时回退默认路径。进程启动时求值一次。
var t2iStylesPath = func() string {
	if path := strings.TrimSpace(os.Getenv("T2I_STYLES_FILE")); path != "" {
		return path
	}
	return t2iStylesDefaultPath
}()

// 内置通用兜底风格（JSON 格式，仅文件缺失/为空时使用）。
// 具体风格一律在风格库文件中定义；本兜底仅保证功能不失效，不承载具体设计意图。
var fallbackT2IStylesJSON = `[{"name":"默认","vibe":"精致、沉稳、克制","palette":"底色柔和纸色/米白（如 #fafaf7），正文深灰（如 #3a3a3a）","accents":"最多 1 个低饱和强调色，面积小","layout":"充足留白、大标题 ≥32px、正文 14–16px、hairline 分隔、左对齐"}]`

// selectedT2IStyle 管理员在管理面板选择的固定风格（空 = 随机）。
// 使用 atomic.Value 保证并发安全（每次构建上下文并发读取）。
var selectedT2IStyle atomic.Value // string

// SetSelectedT2IStyle 设置管理面板选择的风格名；空值表示随机。
func SetSelectedT2IStyle(name string) {
	selectedT2IStyle.Store(name)
}

// GetSelectedT2IStyle 返回管理面板选择的风格名（空 = 随机）。
func GetSelectedT2IStyle() string {
	v, _ := selectedT2IStyle.Load().(string)
	return v
}

// parseT2IStyles 解析 JSON 风格数组，返回格式化后的文本切片。
func parseT2IStyles(raw string) []string {
	var styles []t2iStyle
	if err := json.Unmarshal([]byte(raw), &styles); err != nil {
		return nil
	}
	if len(styles) == 0 {
		return nil
	}
	out := make([]string, len(styles))
	for i, s := range styles {
		out[i] = s.format()
	}
	return out
}

// loadT2IStyles 读取风格库文件，解析为格式化文本切片。
// 文件不存在、空或解析失败时回退到内置兜底。
func loadT2IStyles() []string {
	b, err := os.ReadFile(t2iStylesPath)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		return parseT2IStyles(fallbackT2IStylesJSON)
	}
	if styles := parseT2IStyles(string(b)); styles != nil {
		return styles
	}
	return parseT2IStyles(fallbackT2IStylesJSON)
}

// T2IStyleListItem 风格库条目（供管理面板下拉展示）。
type T2IStyleListItem struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Vibe     string   `json:"vibe"`
}

// loadT2IStyleStructs 读取风格库文件并返回结构化条目（含兜底）。
func loadT2IStyleStructs() []t2iStyle {
	b, err := os.ReadFile(t2iStylesPath)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		var styles []t2iStyle
		_ = json.Unmarshal([]byte(fallbackT2IStylesJSON), &styles)
		return styles
	}
	var styles []t2iStyle
	if err := json.Unmarshal(b, &styles); err != nil || len(styles) == 0 {
		_ = json.Unmarshal([]byte(fallbackT2IStylesJSON), &styles)
	}
	return styles
}

// LoadT2IStyleList 返回风格库结构化列表（管理面板下拉用）。
func LoadT2IStyleList() []T2IStyleListItem {
	styles := loadT2IStyleStructs()
	out := make([]T2IStyleListItem, 0, len(styles))
	for _, s := range styles {
		out = append(out, T2IStyleListItem{
			Name:     s.Name,
			Category: s.Category,
			Tags:     s.Tags,
			Vibe:     s.Vibe,
		})
	}
	return out
}

// IsValidT2IStyle 校验风格选择值是否合法：空串（随机）、"random"
// 或风格库中定义的风格名。供服务层在持久化前做 allowlist 校验。
func IsValidT2IStyle(name string) bool {
	if name == "" || name == "random" {
		return true
	}
	for _, s := range loadT2IStyleStructs() {
		if s.Name == name {
			return true
		}
	}
	return false
}

// pickT2IStyle 选择本次渲染风格：
//   - 管理面板已指定风格（selectedT2IStyle 非空且存在）→ 用该风格
//   - 否则从风格库随机选一条
func pickT2IStyle() string {
	styles := loadT2IStyles()
	if selected := GetSelectedT2IStyle(); selected != "" {
		for _, s := range styles {
			if strings.HasPrefix(s, selected+" ——") {
				return s
			}
		}
		// 指定的风格不存在（文件被改/删除）→ 退回随机，避免空注入
	}
	return styles[rand.IntN(len(styles))]
}

// injectT2IStyle 将选中的风格替换进提示词占位符；无占位符则原样返回。
func injectT2IStyle(content string) string {
	style := pickT2IStyle()
	return strings.Replace(content, t2iStylePlaceholder, style, 1)
}
