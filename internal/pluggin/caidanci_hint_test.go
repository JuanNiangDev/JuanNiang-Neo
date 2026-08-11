package pluggin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	lua "github.com/yuin/gopher-lua"
)

// 回归测试：猜单词插件（redrock_caidanci_grade）的 /提示「词性+中文意思」提取逻辑。
//
// 历史 bug：原实现用 Lua 字符类 `pick.rest:match("^[^,;，；]+")` 切分首个释义。
// Lua 模式按字节匹配，全角 `，；` 的 UTF-8 字节（EF BC 8C / EF BC 9B）进入排除集后，
// 任何多字节编码含 0xBC/0x8C/0x9B/0xEF 的汉字都会被拦腰截断（如 `伤`=E4 BC A4、
// `痛`=E7 97 9B），产出半个汉字的非法 UTF-8 —— 实测全词库 3189/14837 个词的提示乱码
// （报告案例：sore → 「悲�」= 悲(E6 82 B2) + 伤 的首字节 E4）。
//
// 本测试用与插件一致的 Lua 逻辑（字节级 first_meaning + UTF-8 尾随保护）验证：
//  1. 任意单词的提示输出都是合法 UTF-8；
//  2. 首个释义按 ASCII/全角分隔符正确切分。
// 脚本与 Plugins/plugins/redrock_caidanci_grade/main.lua 中的实现保持一致。

const hintScript = `
local word_trans = {}

local function parse_row(line)
    local word, rest = line:match("^([^,]*),(.*)$")
    if not rest then return nil end
    local trans, tag
    if rest:sub(1, 1) == '"' then
        local close = rest:match(",([^,]*)$")
        local field = close and rest:sub(1, #rest - #close - 1) or rest
        if field:sub(1, 1) ~= '"' or field:sub(-1) ~= '"' then return nil end
        trans = field:sub(2, -2):gsub('""', '"')
        tag = close or ""
    else
        local head, tail = rest:match("^(.-),(.*)$")
        if head then
            trans, tag = head, tail
        else
            trans, tag = rest, ""
        end
    end
    if trans:find("\\n", 1, true) then
        trans = trans:gsub("\\r\\n", "\n"):gsub("\\n", "\n")
    end
    return word, trans, tag
end

local function build_index(lines)
    for _, line in ipairs(lines) do
        if line ~= "word,translation,tag" then
            local word, trans, tag = parse_row(line)
            if word and tag ~= "" and word:match("^[a-z]+$") then
                if word_trans[word] == nil then
                    word_trans[word] = trans
                end
            end
        end
    end
end

local POS_NAMES = {
    n = "名词", v = "动词", vt = "及物动词", vi = "不及物动词",
    a = "形容词", adj = "形容词", ad = "副词", adv = "副词",
    prep = "介词", pron = "代词", conj = "连词", interj = "感叹词",
    num = "数词", art = "冠词", aux = "助动词", modal = "情态动词",
    abbr = "缩写",
}

local function first_meaning(rest)
    local len = #rest
    for i = 1, len do
        local b = string.byte(rest, i)
        if b == 0x2C or b == 0x3B then -- ASCII , ;
            return rest:sub(1, i - 1)
        end
        if b == 0xEF and i + 2 <= len then -- 全角 ，(EF BC 8C) / ；(EF BC 9B)
            local b2 = string.byte(rest, i + 1)
            local b3 = string.byte(rest, i + 2)
            if (b2 == 0xBC and (b3 == 0x8C or b3 == 0x9B)) then
                return rest:sub(1, i - 1)
            end
        end
    end
    return rest
end

local function truncate_tail_utf8(s)
    local len = #s
    local cont = 0 -- 从尾部起连续的后续字节数
    while len > 0 do
        local b = string.byte(s, len)
        if b < 0x80 then
            return s:sub(1, len) -- ASCII 字符收尾，之前序列必然完整
        elseif b >= 0xC0 then
            local need -- 该起始字节应携带的后续字节数
            if b < 0xE0 then need = 1
            elseif b < 0xF0 then need = 2
            else need = 3 end
            if cont >= need then
                return s:sub(1, len + need) -- 序列完整，保留
            end
            return s:sub(1, len - 1) -- 尾部序列不完整，去掉起始字节
        end
        cont = cont + 1
        len = len - 1
    end
    return ""
end

local function pick_pos_meaning(word)
    local t = word_trans[word]
    if not t or t == "" then return nil end
    local pos_lines = {}
    for line in (t .. "\n"):gmatch("(.-)\n") do
        local pos, rest = line:match("^([a-z]+)%.%s*(.+)$")
        if pos and rest ~= "" then
            pos_lines[#pos_lines + 1] = { pos = pos, rest = rest }
        end
    end
    if #pos_lines == 0 then return nil end
    local pick = pos_lines[math.random(#pos_lines)]
    local meaning = truncate_tail_utf8(first_meaning(pick.rest))
    meaning = meaning:gsub("^%s+", ""):gsub("%s+$", "")
    if meaning == "" then
        meaning = truncate_tail_utf8(pick.rest):gsub("^%s+", ""):gsub("%s+$", "")
    end
    return POS_NAMES[pick.pos] or pick.pos, meaning
end

function load(lines)
    build_index(lines)
    return word_trans
end

function hint(word)
    return pick_pos_meaning(word)
end

function split(rest)
    return first_meaning(rest)
end

function tailtrim(s)
    return truncate_tail_utf8(s)
end
`

type luaVM struct {
	L *lua.LState
}

func newLuaVM(t *testing.T) *luaVM {
	t.Helper()
	L := lua.NewState()
	t.Cleanup(L.Close)
	if err := L.DoString(hintScript); err != nil {
		t.Fatalf("加载 Lua 脚本失败: %v", err)
	}
	return &luaVM{L: L}
}

func (vm *luaVM) loadCSV(t *testing.T, lines []string) *lua.LTable {
	t.Helper()
	tbl := vm.L.NewTable()
	for i, l := range lines {
		vm.L.SetTable(tbl, lua.LNumber(i+1), lua.LString(l))
	}
	vm.L.SetGlobal("csvLines", tbl)
	if err := vm.L.CallByParam(lua.P{Fn: vm.L.GetGlobal("load"), NRet: 1}, vm.L.GetGlobal("csvLines")); err != nil {
		t.Fatalf("load 失败: %v", err)
	}
	wordTbl, ok := vm.L.Get(-1).(*lua.LTable)
	vm.L.Pop(1)
	if !ok {
		t.Fatal("load 未返回 table")
	}
	return wordTbl
}

func (vm *luaVM) hint(t *testing.T, word string) (posName, meaning string) {
	t.Helper()
	if err := vm.L.CallByParam(lua.P{Fn: vm.L.GetGlobal("hint"), NRet: 2}, lua.LString(word)); err != nil {
		t.Fatalf("hint(%q) 失败: %v", word, err)
	}
	posName, meaning = vm.L.Get(-2).String(), vm.L.Get(-1).String()
	vm.L.Pop(2)
	return
}

func (vm *luaVM) split(t *testing.T, rest string) string {
	t.Helper()
	if err := vm.L.CallByParam(lua.P{Fn: vm.L.GetGlobal("split"), NRet: 1}, lua.LString(rest)); err != nil {
		t.Fatalf("split(%q) 失败: %v", rest, err)
	}
	s := vm.L.Get(-1).String()
	vm.L.Pop(1)
	return s
}

// 内联用例：覆盖报告案例与分隔符边界。
func TestCaidanciHintSplitByteSafe(t *testing.T) {
	vm := newLuaVM(t)
	cases := []struct {
		rest, want string
	}{
		{"悲伤的, 痛的, 引起痛苦的", "悲伤的"}, // 含 0xBC 字节的伤，旧实现会截断
		{"痛处, 溃疡, 疮", "痛处"},       // 含 0x9B 字节的痛
		{"项目；工程", "项目"},           // ASCII 分号
		{"悲伤的，痛的", "悲伤的"},         // 全角逗号
		{"管理; 行政", "管理"},
		{"英亩", "英亩"},              // 无分隔符
		{", 以逗号开头", ""},           // rest 以分隔符开头
		{"n. 痛处, 溃疡, 疮", "n. 痛处"}, // 非词性行整体保留（不含分隔符前缀）
	}
	for _, c := range cases {
		got := vm.split(t, c.rest)
		if got != c.want {
			t.Errorf("first_meaning(%q) = %q, want %q", c.rest, got, c.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("first_meaning(%q) 输出非法 UTF-8: %q", c.rest, got)
		}
	}
}

// truncate_tail_utf8 边界：合法 UTF-8 必须原样保留（防止数据丢失），
// 仅切除「起始字节声明长度超出实际」的不完整尾部序列。
func TestCaidanciHintTailTrim(t *testing.T) {
	vm := newLuaVM(t)
	cases := []struct {
		in, want string
	}{
		{"悲伤的", "悲伤的"}, // 完整 3 字节汉字结尾必须保留
		{"悲伤", "悲伤"},
		{"悲", "悲"},
		{"痛处", "痛处"},
		{"abc", "abc"},
		{"", ""},
		{"悲\xe4", "悲"},   // 伤的起始字节被截断（E4 声明 3 字节但只有 1 个）
		{"\xe7\x9a", ""}, // 的起始字节 + 1 个连续字节，不完整 → 全丢
		{"a\xe4", "a"},
	}
	for _, c := range cases {
		vm.L.SetGlobal("tstr", lua.LString(c.in))
		if err := vm.L.CallByParam(lua.P{Fn: vm.L.GetGlobal("tailtrim"), NRet: 1}, lua.LString(c.in)); err != nil {
			t.Fatalf("tailtrim(%q) 失败: %v", c.in, err)
		}
		got := vm.L.Get(-1).String()
		vm.L.Pop(1)
		if got != c.want {
			t.Errorf("truncate_tail_utf8(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 报告案例：sore 的提示必须是完整汉字（旧实现产出「悲�」）。
func TestCaidanciHintSore(t *testing.T) {
	vm := newLuaVM(t)
	vm.loadCSV(t, []string{
		"word,translation,tag",
		`sore,"a. 悲伤的, 痛的, 引起痛苦的\nn. 痛处, 溃疡, 疮",cet4`,
	})
	for i := 0; i < 20; i++ { // 多次取样覆盖两个词性行
		pos, meaning := vm.hint(t, "sore")
		if !utf8.ValidString(meaning) || !utf8.ValidString(pos) {
			t.Fatalf("sore 提示非法 UTF-8: pos=%q meaning=%q", pos, meaning)
		}
		switch meaning {
		case "悲伤的", "痛的", "痛处":
		default:
			t.Fatalf("sore 提示意思意外: %q", meaning)
		}
	}
}

// 全词库校验：加载真实 words.csv，任何单词的提示都必须是合法 UTF-8。
func TestCaidanciHintFullWordlist(t *testing.T) {
	path := findWordlist(t)
	if path == "" {
		t.Skip("words.csv 不存在，跳过全词库校验")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("读取 words.csv 失败: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")

	vm := newLuaVM(t)
	wordTbl := vm.loadCSV(t, lines)

	var bad []string
	wordTbl.ForEach(func(k, v lua.LValue) {
		pos, meaning := vm.hint(t, k.String())
		if !utf8.ValidString(meaning) || !utf8.ValidString(pos) {
			bad = append(bad, k.String()+" => "+pos+" / "+meaning)
		}
	})
	if len(bad) > 0 {
		sample := bad
		if len(sample) > 10 {
			sample = sample[:10]
		}
		t.Fatalf("%d 个单词提示为非法 UTF-8（样例）:\n%s", len(bad), strings.Join(sample, "\n"))
	}
	t.Logf("全词库提示校验通过")
}

func findWordlist(t *testing.T) string {
	t.Helper()
	candidates := []string{
		os.Getenv("CAIDANCI_WORDS_CSV"),
		"../../../Plugins/plugins/redrock_caidanci_grade/words.csv", // Bot/internal/pluggin → 工作区根
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if abs, err := filepath.Abs(c); err == nil {
			c = abs
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
