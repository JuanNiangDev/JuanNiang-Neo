package pluggin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// redrock_poke 表情包回复插件的行为回归测试。
// 插件源码在 JuanNiang-Plugins 仓库，这里内联最小配置 + main.lua 的关键行为：
//   - images API 在引擎无 DAO 时返回空表（候选池为空 → 回退文本回复）
//   - emoji_weight/text_weight 权重与 emoji_rate 附加概率的回退路径
//   - 权限缺失时 jn.images 不注入

const pokeTestMainLua = `
local jn = require("jn")

local EMOJI_FOLDER = tostring(jn.config.get("emoji_folder") or "")
local EMOJI_W = math.max(tonumber(jn.config.get("emoji_weight")) or 1, 0)
local TEXT_W = math.max(tonumber(jn.config.get("text_weight")) or 1, 0)
local EMOJI_RATE = math.min(math.max(tonumber(jn.config.get("emoji_rate")) or 0, 0), 100)

local function has_emoji_source()
    return EMOJI_FOLDER ~= "" and jn.images ~= nil
end

function on_notice(event)
    if event.notice_type ~= "notify" or event.sub_type ~= "poke" then return end
    if jn.config.get("enabled") == false then return end
    local bot_qq = tonumber(jn.config.get("bot_qq"))
    if not bot_qq then return end
    local target = tonumber(event.target_id)
    if not target or target ~= bot_qq then return end
    local from_qq = event.user_id

    local use_emoji = false
    if EMOJI_W + TEXT_W > 0 and has_emoji_source() then
        use_emoji = math.random(EMOJI_W + TEXT_W) <= EMOJI_W
    end
    local attach = (not use_emoji) and EMOJI_RATE > 0
        and has_emoji_source() and math.random(100) <= EMOJI_RATE
    local img = nil
    if use_emoji or attach then
        local ok, list = pcall(function() return jn.images.list(EMOJI_FOLDER) end)
        if ok and type(list) == "table" and #list > 0 then
            img = list[math.random(#list)]
        end
    end
    if use_emoji and not img then use_emoji = false end

    local msg = string.format("[CQ:at,qq=%d] ", from_qq)
    if use_emoji and img then
        msg = msg .. string.format("[CQ:image,file=imgs://%s]", img.id)
    else
        msg = msg .. "文本回复"
        if attach and img then
            msg = msg .. string.format(" [CQ:image,file=imgs://%s]", img.id)
        end
    end
    jn.onebot11.send_group_msg(event.group_id, msg)
end
`

const pokeTestConfigLua = `
configs:
  - key: enabled
    type: bool
    default: true
    value: true
  - key: bot_qq
    type: string
    default: ""
    value: "10000"
  - key: emoji_folder
    type: string
    default: 表情包
    value: %q
  - key: emoji_weight
    type: string
    default: "1"
    value: %q
  - key: text_weight
    type: string
    default: "2"
    value: %q
  - key: emoji_rate
    type: string
    default: "30"
    value: %q
`

// loadPokeTestPlugin 写入插件文件并以指定权限手动装载（参考 loadPluginManual）。
func loadPokeTestPlugin(t *testing.T, pe *PluginEngine, perms []string, folder, eW, tW, rate string) *lua.LState {
	t.Helper()
	pluginDir := filepath.Join(pe.basePath, "poketest")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgSrc := fmt.Sprintf(pokeTestConfigLua, folder, eW, tW, rate)
	if err := os.WriteFile(filepath.Join(pluginDir, "config.yaml"), []byte(cfgSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "main.lua"), []byte(pokeTestMainLua), 0o644); err != nil {
		t.Fatal(err)
	}
	// require("jn") 需要插件目录内有 SDK（真实引擎启动时会同步，测试手动拷贝）
	if sdk, err := os.ReadFile("sdk/jn.lua"); err == nil {
		_ = os.WriteFile(filepath.Join(pluginDir, "jn.lua"), sdk, 0o644)
	}

	L := lua.NewState()
	t.Cleanup(L.Close)
	pe.injectSDK(L, "poketest")
	pe.injectBaseAPI(L, "poketest", perms)
	L.DoString(fmt.Sprintf(`package.path = %q .. ";" .. package.path`, pluginDir+"/?.lua"))
	if err := L.DoFile(filepath.Join(pluginDir, "main.lua")); err != nil {
		t.Fatalf("加载插件失败: %v", err)
	}
	pe.mu.Lock()
	p := &LoadedPlugin{Manifest: Manifest{Name: "poketest"}, State: L, Dir: pluginDir}
	attachPluginRef(L, p)
	pe.plugins["poketest"] = p
	pe.mu.Unlock()
	t.Cleanup(func() {
		pe.mu.Lock()
		delete(pe.plugins, "poketest")
		pe.mu.Unlock()
	})
	return L
}

func runOnNoticePoke(t *testing.T, pe *PluginEngine, L *lua.LState, groupID, userID, targetID int64) {
	t.Helper()
	p := pluginForL(pe, L)
	if p == nil {
		t.Fatal("插件未加载")
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	fn := L.GetGlobal("on_notice")
	if fn.Type() != lua.LTFunction {
		t.Fatal("on_notice 未定义")
	}
	L.Push(fn)
	ev := L.NewTable()
	L.SetField(ev, "notice_type", lua.LString("notify"))
	L.SetField(ev, "sub_type", lua.LString("poke"))
	L.SetField(ev, "group_id", lua.LNumber(groupID))
	L.SetField(ev, "user_id", lua.LNumber(userID))
	L.SetField(ev, "target_id", lua.LNumber(targetID))
	L.Push(ev)
	if err := L.PCall(1, 0, nil); err != nil {
		t.Fatalf("on_notice 执行失败: %v", err)
	}
}

// TestNormalizeImageFolderArg 目录参数归一化：空/纯斜杠→根，首尾斜杠去掉后补单个前导斜杠。
func TestNormalizeImageFolderArg(t *testing.T) {
	cases := map[string]string{
		"":         "/",
		"/":        "/",
		"///":      "/",
		"表情包":      "/表情包",
		"表情包/":     "/表情包",
		"/表情包/":    "/表情包",
		"//表情包//":  "/表情包",
		"  /a b  ": "/a b",
		" /a b/ ":  "/a b",
	}
	for in, want := range cases {
		if got := normalizeImageFolderArg(in); got != want {
			t.Fatalf("normalizeImageFolderArg(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestImagesAPIEmptyWithoutDAO 引擎无 DAO 时 images.list/search 返回空表而非报错。
func TestImagesAPIEmptyWithoutDAO(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	L := lua.NewState()
	t.Cleanup(L.Close)
	pe.injectSDK(L, "x")
	pe.injectBaseAPI(L, "x", []string{"image"})

	images := L.GetGlobal("images")
	if images.Type() != lua.LTTable {
		t.Fatal("jn.images 未注入")
	}
	L.SetGlobal("r_list", L.NewTable())
	if err := L.DoString("r_list = images.list('表情包'); r_search = images.search('奶蛙')"); err != nil {
		t.Fatalf("images API 调用失败: %v", err)
	}
	if v := L.GetGlobal("r_list").(*lua.LTable).Len(); v != 0 {
		t.Fatalf("list 应为空表，实际 %d 项", v)
	}
	if v := L.GetGlobal("r_search").(*lua.LTable).Len(); v != 0 {
		t.Fatalf("search 应为空表，实际 %d 项", v)
	}
}

// TestImagesAPINotInjectedWithoutPermission 无 image 权限时 jn.images 不存在。
func TestImagesAPINotInjectedWithoutPermission(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	L := lua.NewState()
	t.Cleanup(L.Close)
	pe.injectSDK(L, "x")
	pe.injectBaseAPI(L, "x", []string{"onebot11"})
	if L.GetGlobal("images").Type() == lua.LTTable {
		t.Fatal("无 image 权限时不应注入 jn.images")
	}
}

// TestRedrockPokeTextFallbackWhenPoolEmpty 候选池为空（无 DAO）时，
// 即使表情包权重拉满也回退文本回复，且不产生 imgs:// 段。
func TestRedrockPokeTextFallbackWhenPoolEmpty(t *testing.T) {
	pe, adp := newMiniTestEngine(t, nil)
	L := loadPokeTestPlugin(t, pe, []string{"onebot11", "image"}, "表情包", "5", "0", "100")
	runOnNoticePoke(t, pe, L, 619463411, 2706880859, 10000)
	// send_group_msg 异步 goroutine 发送，轮询等待
	waitFor(t, func() bool {
		adp.mu.Lock()
		defer adp.mu.Unlock()
		return len(adp.groups) > 0
	})

	adp.mu.Lock()
	sent := strings.Join(adp.groups, "\n")
	adp.mu.Unlock()
	if len(adp.groups) == 0 {
		t.Fatal("应发送一条回复")
	}
	if !strings.Contains(sent, "[CQ:at,qq=2706880859]") {
		t.Fatalf("回复应 @ 戳人者: %s", sent)
	}
	if strings.Contains(sent, "imgs://") {
		t.Fatalf("候选池为空时不应包含表情包段: %s", sent)
	}
	if !strings.Contains(sent, "文本回复") {
		t.Fatalf("应回退文本回复: %s", sent)
	}
}

// TestRedrockPokeIgnoresOthersPoke 被戳对象不是机器人时不应回复。
func TestRedrockPokeIgnoresOthersPoke(t *testing.T) {
	pe, adp := newMiniTestEngine(t, nil)
	adp.notify = make(chan struct{}, 16)
	L := loadPokeTestPlugin(t, pe, []string{"onebot11", "image"}, "表情包", "1", "1", "0")
	runOnNoticePoke(t, pe, L, 619463411, 2706880859, 99999)

	// send_group_msg 在 goroutine 里异步落账：有界等待发送通知——
	// 通知到达即失败（说明戳别人触发了回复）；超时后复查账本兜底
	select {
	case <-adp.notify:
		adp.mu.Lock()
		n := len(adp.groups)
		adp.mu.Unlock()
		t.Fatalf("戳别人时不应回复，实际发送 %d 条", n)
	case <-time.After(300 * time.Millisecond):
	}
	adp.mu.Lock()
	n := len(adp.groups)
	adp.mu.Unlock()
	if n != 0 {
		t.Fatalf("戳别人时不应回复，实际发送 %d 条", n)
	}
}
