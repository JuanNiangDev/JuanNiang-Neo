package pluggin

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/agent/provider"

	lua "github.com/yuin/gopher-lua"
)

// ---------- 测试替身 ----------

// fakeAdapter 空实现 SendAdapter，记录关键调用。
type fakeAdapter struct {
	mu         sync.Mutex
	deleted    []int64
	groups     []string
	muted      []string
	kicked     []string
	mutedUsers map[int64]bool // 模拟处于禁言中的用户
}

func (f *fakeAdapter) SendPrivateMsg(userID int64, message any) (int64, error) { return 0, nil }
func (f *fakeAdapter) SendGroupMsg(groupID int64, message any) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groups = append(f.groups, fmt.Sprintf("%v", message))
	return 0, nil
}
func (f *fakeAdapter) DeleteMsg(messageID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, messageID)
	return nil
}
func (f *fakeAdapter) GetMsg(messageID int64) (map[string]any, error) { return nil, nil }
func (f *fakeAdapter) GetGroupInfo(groupID int64) (map[string]any, error) {
	return map[string]any{"group_name": "test"}, nil
}
func (f *fakeAdapter) GetGroupMemberList(groupID int64) ([]map[string]any, error) { return nil, nil }
func (f *fakeAdapter) KickGroupMember(groupID, userID int64, rejectAdd bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.kicked = append(f.kicked, fmt.Sprintf("%d:%d", groupID, userID))
	return nil
}
func (f *fakeAdapter) BanGroupMember(groupID, userID int64, duration int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.muted = append(f.muted, fmt.Sprintf("%d:%d:%d", groupID, userID, duration))
	return nil
}
func (f *fakeAdapter) SetGroupWholeBan(groupID int64, enable bool) error     { return nil }
func (f *fakeAdapter) SetGroupCard(groupID, userID int64, card string) error { return nil }
func (f *fakeAdapter) HandleFriendRequest(flag string, approve bool, remark string) error {
	return nil
}
func (f *fakeAdapter) HandleGroupRequest(flag, subType string, approve bool, reason string) error {
	return nil
}
func (f *fakeAdapter) GetLoginInfo() (map[string]any, error) { return nil, nil }
func (f *fakeAdapter) GetStrangerInfo(userID int64) (map[string]any, error) {
	return nil, nil
}
func (f *fakeAdapter) GetFriendList() ([]map[string]any, error) { return nil, nil }
func (f *fakeAdapter) GetGroupList() ([]map[string]any, error)  { return nil, nil }
func (f *fakeAdapter) GetGroupMemberInfo(groupID, userID int64) (map[string]any, error) {
	info := map[string]any{"role": "member", "shut_up_timestamp": int64(0)}
	f.mu.Lock()
	if f.mutedUsers[userID] {
		info["shut_up_timestamp"] = time.Now().Add(30 * time.Minute).Unix()
	}
	f.mu.Unlock()
	return info, nil
}
func (f *fakeAdapter) GetGroupHonorInfo(groupID int64) (map[string]any, error) { return nil, nil }
func (f *fakeAdapter) SendLike(userID int64, times int) error                  { return nil }
func (f *fakeAdapter) GetStatus() (map[string]any, error)                      { return nil, nil }
func (f *fakeAdapter) GetVersionInfo() (map[string]any, error)                 { return nil, nil }

func (f *fakeAdapter) deletedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.deleted)
}

func (f *fakeAdapter) mutedCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]string{}, f.muted...)
	return out
}

// fakeLLM 可编程 LLM 替身，实现 LLMAccess。
type fakeLLM struct {
	mu    sync.Mutex
	reply string
	err   error
	calls int
}

func (f *fakeLLM) Available() bool { return true }

func (f *fakeLLM) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &provider.ChatResponse{Message: provider.ChatMessage{Role: "assistant", Content: f.reply}}, nil
}

func (f *fakeLLM) setReply(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reply = s
}

func (f *fakeLLM) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// ---------- 工具 ----------

// copyPlugin 把真实插件目录复制到临时目录，避免测试中的 save_config 写回污染真实文件。
func copyPlugin(t *testing.T, dst string) {
	t.Helper()
	src := filepath.Join("..", "..", "..", "Plugins", "plugins", "redrock_group_manager")
	// 跨仓库端到端测试：Plugins 仓库需与 Bot 同级检出；CI 无此目录时跳过，
	// Go 侧机制测试（TestLLMInjectSyncAndAsync）仍正常执行。
	if _, err := os.Stat(src); os.IsNotExist(err) {
		t.Skipf("插件源码目录不存在（需 Plugins 仓库同级检出）: %s", src)
	}
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatalf("复制插件目录失败: %v", err)
	}
}

func newTestEngine(t *testing.T, llm LLMAccess) (*PluginEngine, *fakeAdapter, string) {
	t.Helper()
	adp := &fakeAdapter{mu: sync.Mutex{}, mutedUsers: make(map[int64]bool)}
	base := t.TempDir()
	pluginDir := filepath.Join(base, "redrock_group_manager")
	copyPlugin(t, pluginDir)
	pe := NewPluginEngine(base, adp, nil, nil, nil, nil, nil, nil, llm)
	return pe, adp, base
}

// loadPluginManual 手动注入 + 执行插件入口，并把插件注册进 engine（供异步回调 runner 使用）。
func loadPluginManual(t *testing.T, pe *PluginEngine, name string) *lua.LState {
	t.Helper()
	pluginDir := filepath.Join(pe.basePath, name)
	L := lua.NewState()
	t.Cleanup(L.Close)

	pe.injectSDK(L, name)
	pe.injectBaseAPI(L, name, []string{"onebot11", "database", "file", "llm", "timer"})
	// database 需要 gorm db（测试环境不提供），手写空实现（插件 init_db 用）
	dbTable := L.NewTable()
	L.SetFuncs(dbTable, map[string]lua.LGFunction{
		"exec": func(L *lua.LState) int {
			L.Push(lua.LBool(true))
			return 1
		},
		"query": func(L *lua.LState) int {
			L.Push(L.NewTable())
			return 1
		},
	})
	L.SetGlobal("database", dbTable)
	pe.injectCommandAPI(L, name)

	// require("jn")：优先加载插件目录内已同步的 jn.lua
	L.DoString(fmt.Sprintf(`package.path = %q .. ";" .. package.path`, pluginDir+"/?.lua"))

	if err := L.DoFile(filepath.Join(pluginDir, "main.lua")); err != nil {
		t.Fatalf("加载插件 %s 失败: %v", name, err)
	}

	pe.mu.Lock()
	pe.plugins[name] = &LoadedPlugin{Manifest: Manifest{Name: name}, State: L, Dir: pluginDir}
	pe.mu.Unlock()

	// 测试结束先卸载插件（从 map 移除），再关闭 LState：避免长延时异步任务
	// （如 jn.timer.after 的 5~30 秒延时）在 LState 关闭后仍被 runAsyncCallbacks
	// 派发而对已关闭 LState 做 PCall 而 panic。与真实 Unload 语义一致（卸载后
	// 遗留任务因 ok=false 被丢弃）。
	t.Cleanup(func() {
		pe.mu.Lock()
		delete(pe.plugins, name)
		pe.mu.Unlock()
	})
	return L
}

// pluginForL 返回拥有指定 LState 的已加载插件（测试辅助，用于拿 stateMu 串行化）。
func pluginForL(pe *PluginEngine, L *lua.LState) *LoadedPlugin {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	for _, p := range pe.plugins {
		if p.State == L {
			return p
		}
	}
	return nil
}

// runOnMessage 构造群消息事件并调用插件 on_message，返回其第一个返回值（consumed）。
// on_message 返回 (consumed, skip_reply)：consumed=true 消息不进 Agent（不短路）；
// modified_event 已移除，插件不能修改事件。
func runOnMessage(t *testing.T, pe *PluginEngine, L *lua.LState, groupID, userID int64, raw, msgID string) bool {
	// on_message 内可能触发 *_async 任务，引擎 goroutine 会持插件 stateMu 回调同一
	// LState；这里也持 stateMu 串行化，避免 -race 竞争。
	p := pluginForL(pe, L)
	if p == nil {
		t.Fatal("插件未加载")
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.closed {
		t.Fatal("插件已关闭")
	}
	t.Helper()
	fn := L.GetGlobal("on_message")
	if fn.Type() != lua.LTFunction {
		t.Fatal("on_message 未定义")
	}
	L.Push(fn)
	ev := L.NewTable()
	L.SetField(ev, "post_type", lua.LString("message"))
	L.SetField(ev, "message_type", lua.LString("group"))
	L.SetField(ev, "group_id", lua.LNumber(groupID))
	L.SetField(ev, "user_id", lua.LNumber(userID))
	L.SetField(ev, "raw_message", lua.LString(raw))
	L.SetField(ev, "message_id", lua.LString(msgID))
	L.SetField(ev, "admins", L.NewTable())
	L.Push(ev)
	if err := L.PCall(1, 2, nil); err != nil {
		t.Fatalf("on_message 执行失败: %v", err)
	}
	consumed := bool(L.Get(-2).(lua.LBool))
	L.Pop(2)
	return consumed
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

// luaGetGlobal / luaDoString / luaDoFile 在插件 stateMu 内访问 LState：异步回调
// 由引擎 goroutine 持同一插件的 stateMu 执行 PCall，LState 非线程安全，
// 测试侧全局读写必须与回调互斥，否则 -race 下报数据竞争。
func luaGetGlobal(pe *PluginEngine, L *lua.LState, name string) lua.LValue {
	p := pluginForL(pe, L)
	if p == nil {
		return lua.LNil
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.closed {
		return lua.LNil
	}
	return L.GetGlobal(name)
}

func luaDoString(pe *PluginEngine, L *lua.LState, src string) error {
	p := pluginForL(pe, L)
	if p == nil {
		return fmt.Errorf("插件未加载")
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.closed {
		return fmt.Errorf("插件已关闭")
	}
	return L.DoString(src)
}

func luaDoFile(pe *PluginEngine, L *lua.LState, path string) error {
	p := pluginForL(pe, L)
	if p == nil {
		return fmt.Errorf("插件未加载")
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.closed {
		return fmt.Errorf("插件已关闭")
	}
	return L.DoFile(path)
}

// dispatchCommand 派发命令（与生产 dispatchMessage 的语义一致：匹配在 commands
// 锁内完成，handler 在插件 stateMu 下执行）。
func dispatchCommand(pe *PluginEngine, raw string, ev EventData) (consumed bool, reply string, err error) {
	node, args, hint := pe.commands.Match(raw)
	if node != nil {
		return pe.execCommand(node, args, ev)
	}
	if hint != "" {
		return true, hint, nil
	}
	return false, "", nil
}

// ---------- 测试: jn.llm Go 机制（同步 / 异步 / 回调 / 错误） ----------

const llmTestPluginMain = `-- llm 机制测试插件（异步注册表 kind "chat" → on_chat_response）
avail = llm.available()
local content, err = llm.chat("hi", { timeout = 10 })
synced = content
done_count = 0
last_rid = -1
last_content = ""
last_err = ""
cb_submitted = -1
function on_chat_response(req_id, content, err)
    last_rid = req_id
    last_content = tostring(content)
    last_err = tostring(err)
    done_count = done_count + 1
end
cb_submitted = llm.chat_async("hi", { timeout = 10 })
`

func writeTestPlugin(t *testing.T, base, name string) {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pluggin.yaml"),
		[]byte("name: "+name+"\nversion: \"1.0.0\"\nentry: main.lua\npermissions:\n  - llm\nenabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.lua"), []byte(llmTestPluginMain), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLLMInjectSyncAndAsync(t *testing.T) {
	llm := &fakeLLM{reply: `{"violation":"none"}`}
	adp := &fakeAdapter{}
	base := t.TempDir()
	pe := NewPluginEngine(base, adp, nil, nil, nil, nil, nil, nil, llm)
	writeTestPlugin(t, base, "llmtest")

	L := lua.NewState()
	t.Cleanup(L.Close)
	pe.injectSDK(L, "llmtest")
	pe.injectBaseAPI(L, "llmtest", []string{"llm"})
	pe.mu.Lock()
	pe.plugins["llmtest"] = &LoadedPlugin{Manifest: Manifest{Name: "llmtest"}, State: L, Dir: filepath.Join(base, "llmtest")}
	pe.mu.Unlock()

	luaDoString(pe, L, fmt.Sprintf(`package.path = %q .. ";" .. package.path`, filepath.Join(base, "llmtest")+"/?.lua"))
	if err := luaDoFile(pe, L, filepath.Join(base, "llmtest", "main.lua")); err != nil {
		t.Fatalf("加载测试插件失败: %v", err)
	}

	if v := luaGetGlobal(pe, L, "avail"); v.Type() != lua.LTBool || !bool(v.(lua.LBool)) {
		t.Fatalf("llm.available() 应为 true, got %v", v)
	}
	if v := luaGetGlobal(pe, L, "synced"); v.Type() != lua.LTString || v.String() != `{"violation":"none"}` {
		t.Fatalf("llm.chat 同步结果 = %v", v)
	}
	// 异步：chat_async 立即返回 req_id > 0，完成后引擎派发 on_chat_response
	submitted := luaGetGlobal(pe, L, "cb_submitted")
	if submitted.Type() != lua.LTNumber || float64(submitted.(lua.LNumber)) <= 0 {
		t.Fatalf("chat_async 应返回 req_id > 0, got %v", submitted)
	}
	waitFor(t, func() bool {
		v := luaGetGlobal(pe, L, "done_count")
		return v.Type() == lua.LTNumber && float64(v.(lua.LNumber)) >= 1
	})
	if v := luaGetGlobal(pe, L, "last_rid"); v.String() != submitted.String() {
		t.Fatalf("on_chat_response req_id = %v, want 与提交返回值一致 %v", v, submitted)
	}
	if v := luaGetGlobal(pe, L, "last_content"); v.String() != `{"violation":"none"}` {
		t.Fatalf("异步回调 content = %v", v.String())
	}
	if v := luaGetGlobal(pe, L, "last_err"); v.String() != "nil" {
		t.Fatalf("成功回调 err 应为 nil（tostring 为 \"nil\"）, got %v", v.String())
	}
	if llm.callCount() < 2 {
		t.Fatalf("LLM 调用次数 = %d, want >= 2", llm.callCount())
	}

	// 错误路径：Chat 返回错误 → on_chat_response 收到 (req_id, nil, err)
	llm.setReply("")
	llm.mu.Lock()
	llm.err = fmt.Errorf("boom")
	llm.mu.Unlock()
	luaDoString(pe, L, `err_submitted = llm.chat_async("hi", {})`)
	waitFor(t, func() bool {
		v := luaGetGlobal(pe, L, "done_count")
		return v.Type() == lua.LTNumber && float64(v.(lua.LNumber)) >= 2
	})
	if !strings.Contains(luaGetGlobal(pe, L, "last_err").String(), "boom") {
		t.Fatalf("错误回调 err = %v, want 包含 boom", luaGetGlobal(pe, L, "last_err"))
	}
}

// ---------- 测试: 真实 redrock_group_manager 插件行为 ----------

func TestRedrockBlackGraySensitive(t *testing.T) {
	llm := &fakeLLM{reply: `{"violation":"none"}`}
	pe, adp, _ := newTestEngine(t, llm)
	L := loadPluginManual(t, pe, "redrock_group_manager")

	// 1. 黑色地带：命中 black.txt → 送 LLM 高危复核（不消费）；判 none → 放行不撤回
	consumed := runOnMessage(t, pe, L, 10001, 20001, "对接全国 无本续贷 一手收单", "90001")
	if consumed {
		t.Fatal("黑色词消息应送 LLM 高危复核（不消费）")
	}
	waitFor(t, func() bool { return llm.callCount() >= 1 })
	time.Sleep(300 * time.Millisecond) // 等回调（none → 不撤回）
	if adp.deletedCount() != 0 {
		t.Fatalf("黑词 LLM 判 none 不应撤回, deleted=%d", adp.deletedCount())
	}

	// 1b. 黑色地带：LLM 判 ad → 高危复核确认后撤回处罚
	llm.setReply(`{"violation":"ad","reason":"无本续贷为高利贷广告"}`)
	consumed = runOnMessage(t, pe, L, 10001, 20001, "对接全国 无本续贷 一手收单", "90008")
	if consumed {
		t.Fatal("黑色词消息应送 LLM 高危复核（不消费）")
	}
	waitFor(t, func() bool { return adp.deletedCount() >= 1 })

	// 2. 敏感地带：操逼（cn_pornographic）→ 高危复核；判 none → 放行不撤回
	llm.setReply(`{"violation":"none","reason":"正常语境"}`)
	before := adp.deletedCount()
	consumed = runOnMessage(t, pe, L, 10001, 20002, "你个操逼的", "90002")
	if consumed {
		t.Fatal("敏感词消息应送 LLM 高危复核（不消费）")
	}
	waitFor(t, func() bool { return llm.callCount() >= 1 })
	time.Sleep(300 * time.Millisecond)
	if adp.deletedCount() != before {
		t.Fatalf("敏感词 LLM 判 none 不应撤回: before=%d after=%d", before, adp.deletedCount())
	}

	// 3. 灰色地带：命中 all.txt 灰色词（考研/加群）→ 不消费、不立即处罚，异步常规审查；判 ad → 追罚撤回
	llm.setReply(`{"violation":"ad","reason":"以资料分享为幌子拉群引流"}`)
	before = adp.deletedCount()
	consumed = runOnMessage(t, pe, L, 10001, 20003, "考研上岸的学长学姐，加群一起交流经验", "90003")
	if consumed {
		t.Fatal("灰色词消息不应被消费（先放行）")
	}
	waitFor(t, func() bool { return adp.deletedCount() > before })

	// 4. 灰色地带：LLM 判 none → 不处罚
	llm.setReply(`{"violation":"none","reason":"正常讨论考研"}`)
	before = adp.deletedCount()
	consumed = runOnMessage(t, pe, L, 10001, 20004, "有没有考研上岸的学长学姐", "90004")
	if consumed {
		t.Fatal("灰色词消息不应被消费")
	}
	time.Sleep(300 * time.Millisecond)
	if adp.deletedCount() != before {
		t.Fatalf("LLM 判 none 不应撤回: before=%d after=%d", before, adp.deletedCount())
	}

	// 5. 管理命令：/豁免 20005 解除禁言（不加白名单）；/白名单 20005 加入白名单
	//    → 白名单用户发黑色词不参与检测（不触发 LLM）
	adp.mu.Lock()
	adp.mutedUsers[20005] = true
	adp.mu.Unlock()
	cmdEvent := EventData{
		PostType: "message", MessageType: "group",
		GroupID: 10001, UserID: 99999, RawMessage: "/豁免 20005",
		Admins: []string{"99999"}, MessageID: 1,
	}
	consumed, reply, err := dispatchCommand(pe, "/豁免 20005", cmdEvent)
	if err != nil || !consumed {
		t.Fatalf("/豁免 派发失败 consumed=%v reply=%q err=%v", consumed, reply, err)
	}
	waitFor(t, func() bool {
		for _, m := range adp.mutedCalls() {
			if m == "10001:20005:0" {
				return true
			}
		}
		return false
	})

	// /白名单 20005：加入白名单（当前 /豁免 已不再加入白名单）
	consumed, _, err = dispatchCommand(pe, "/白名单 20005", cmdEvent)
	if err != nil || !consumed {
		t.Fatalf("/白名单 派发失败 consumed=%v err=%v", consumed, err)
	}
	llmBefore := llm.callCount()
	consumed = runOnMessage(t, pe, L, 10001, 20005, "对接全国 无本续贷", "90005")
	if consumed {
		t.Fatal("白名单用户发黑色词不应被消费")
	}
	time.Sleep(200 * time.Millisecond)
	if llm.callCount() != llmBefore {
		t.Fatalf("白名单用户发黑色词不应触发 LLM")
	}
	before = adp.deletedCount()

	// 6. 解除豁免（/取消豁免 别名同样注册）→ 移出白名单 → 恢复检测，黑词送 LLM 复核
	cmdEvent.RawMessage = "/解除豁免 20005"
	consumed, _, err = dispatchCommand(pe, "/解除豁免 20005", cmdEvent)
	if err != nil || !consumed {
		t.Fatalf("/解除豁免 派发失败 consumed=%v err=%v", consumed, err)
	}
	llm.setReply(`{"violation":"ad","reason":"恢复检测"}`)
	consumed = runOnMessage(t, pe, L, 10001, 20005, "无本续贷 全国一手收单", "90006")
	if consumed {
		t.Fatal("解除豁免后黑色词应送 LLM 复核（不消费）")
	}
	waitFor(t, func() bool { return adp.deletedCount() > before })

	// 7. 命令注册完整性
	for _, cmd := range []string{"/豁免", "/解除豁免", "/取消豁免", "/groupstats"} {
		if !pe.commands.HasCommand(cmd) {
			t.Fatalf("命令 %s 未注册", cmd)
		}
	}
}

// 防未使用告警（io 保留给后续扩展）
var _ = io.Discard

// ---------- 测试: t2i/http 异步 API（xxx_async → on_xxx_response，带调用现场 ctx） ----------

// t2iHTTPAsyncTestMain 异步机制测试插件：{{HTTP_URL}} 由 Go 侧替换为 httptest server。
const t2iHTTPAsyncTestMain = `-- t2i/http 异步机制测试
-- 成功路径（带 ctx）
t2i_rid = t2i.generate_async("<h1>hi</h1>", nil, { tag = "ctx-t2i", group = 10001 })
http_rid = http.get_async("{{HTTP_URL}}/echo", { tag = "ctx-http" })
-- 同步阶段错误：opts 含未知键
opts_rid, opts_err = t2i.generate_async("<h1>x</h1>", { bad_key = 1 })
-- 异步回调错误路径：连接拒绝
http.get_async("{{REFUSED_URL}}/")

function on_t2i_response(req_id, ctx, result, err)
    t2i_result = tostring(result)
    t2i_err = tostring(err)
    if ctx then
        t2i_ctx_tag = tostring(ctx.tag)
        t2i_ctx_group = ctx.group
    end
    t2i_done = (t2i_done or 0) + 1
end

function on_http_response(req_id, ctx, result, err)
    if err then
        http_fail_err = tostring(err)
    else
        http_status = result.status
        http_body = tostring(result.body)
        if ctx then http_ctx_tag = tostring(ctx.tag) end
    end
    http_done = (http_done or 0) + 1
end
`

func TestT2IAndHTTPAsync(t *testing.T) {
	// HTTP 测试 server（GET /echo 返回固定 JSON）
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/echo" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"path":%q}`, r.URL.Path)
	}))
	defer httpSrv.Close()

	// T2I 测试 server（模拟 POST /text2img/generate）
	t2iSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/text2img/generate" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"id":"data/rendered.png"}}`))
	}))
	defer t2iSrv.Close()
	t2iClient := &t2icaller.Client{
		Config:     t2icaller.Config{BaseURL: t2iSrv.URL, Timeout: 5 * time.Second},
		HttpClient: t2iSrv.Client(),
	}

	// 确定会被拒绝的端口：绑定后立即关闭（127.0.0.1:1 在部分网络栈（如 WSL）不会拒绝而是挂起）。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refusedURL := "http://" + ln.Addr().String()
	ln.Close()

	base := t.TempDir()
	pe := NewPluginEngine(base, &fakeAdapter{}, nil, nil, t2iClient, nil, nil, nil, &fakeLLM{reply: "x"})
	writeTestPlugin(t, base, "t2ihttp")
	main := strings.ReplaceAll(t2iHTTPAsyncTestMain, "{{HTTP_URL}}", httpSrv.URL)
	main = strings.ReplaceAll(main, "{{REFUSED_URL}}", refusedURL)
	if err := os.WriteFile(filepath.Join(base, "t2ihttp", "main.lua"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	L := lua.NewState()
	t.Cleanup(L.Close)
	pe.injectSDK(L, "t2ihttp")
	pe.injectBaseAPI(L, "t2ihttp", []string{"t2i", "http"})
	pe.mu.Lock()
	pe.plugins["t2ihttp"] = &LoadedPlugin{Manifest: Manifest{Name: "t2ihttp"}, State: L, Dir: filepath.Join(base, "t2ihttp")}
	pe.mu.Unlock()

	luaDoString(pe, L, fmt.Sprintf(`package.path = %q .. ";" .. package.path`, filepath.Join(base, "t2ihttp")+"/?.lua"))
	if err := luaDoFile(pe, L, filepath.Join(base, "t2ihttp", "main.lua")); err != nil {
		t.Fatalf("加载测试插件失败: %v", err)
	}

	// 1. t2i 异步成功：req_id > 0，回调拿到结果与 ctx
	rid := luaGetGlobal(pe, L, "t2i_rid")
	if rid.Type() != lua.LTNumber || float64(rid.(lua.LNumber)) <= 0 {
		t.Fatalf("t2i.generate_async 应返回 req_id > 0, got %v", rid)
	}
	waitFor(t, func() bool {
		v := luaGetGlobal(pe, L, "t2i_done")
		return v.Type() == lua.LTNumber && float64(v.(lua.LNumber)) >= 1
	})
	if v := luaGetGlobal(pe, L, "t2i_result"); v.String() != "data/rendered.png" {
		t.Fatalf("on_t2i_response result = %v, want data/rendered.png", v)
	}
	if v := luaGetGlobal(pe, L, "t2i_err"); v.String() != "nil" {
		t.Fatalf("on_t2i_response err = %v, want nil", v)
	}
	if v := luaGetGlobal(pe, L, "t2i_ctx_tag"); v.String() != "ctx-t2i" {
		t.Fatalf("ctx.tag = %v, want ctx-t2i", v)
	}
	if v := luaGetGlobal(pe, L, "t2i_ctx_group"); float64(v.(lua.LNumber)) != 10001 {
		t.Fatalf("ctx.group = %v, want 10001", v)
	}

	// 2. http 异步成功：req_id > 0，回调拿到 {status, body} 与 ctx
	rid = luaGetGlobal(pe, L, "http_rid")
	if rid.Type() != lua.LTNumber || float64(rid.(lua.LNumber)) <= 0 {
		t.Fatalf("http.get_async 应返回 req_id > 0, got %v", rid)
	}
	waitFor(t, func() bool {
		v := luaGetGlobal(pe, L, "http_done")
		return v.Type() == lua.LTNumber && float64(v.(lua.LNumber)) >= 2 // 成功 + 连接失败两个回调
	})
	if v := luaGetGlobal(pe, L, "http_status"); float64(v.(lua.LNumber)) != 200 {
		t.Fatalf("on_http_response status = %v, want 200", v)
	}
	if v := luaGetGlobal(pe, L, "http_body"); !strings.Contains(v.String(), "/echo") {
		t.Fatalf("on_http_response body = %v, want 包含 /echo", v)
	}
	if v := luaGetGlobal(pe, L, "http_ctx_tag"); v.String() != "ctx-http" {
		t.Fatalf("ctx.tag = %v, want ctx-http", v)
	}

	// 3. 异步回调错误路径：连接拒绝 → err 非 nil
	if v := luaGetGlobal(pe, L, "http_fail_err"); v.String() == "" || v.String() == "nil" {
		t.Fatalf("连接失败应回调 err, got %v", v)
	}

	// 4. 同步阶段错误：opts 含未知键 → 返回 (0, err)
	if v := luaGetGlobal(pe, L, "opts_rid"); float64(v.(lua.LNumber)) != 0 {
		t.Fatalf("opts 解析错误应返回 req_id 0, got %v", v)
	}
	if v := luaGetGlobal(pe, L, "opts_err"); !strings.Contains(v.String(), "未知 T2I 选项") {
		t.Fatalf("opts 解析错误串 = %v, want 包含 未知 T2I 选项", v)
	}
}

// ---------- 测试: sandbox 异步 API（create/exec_shell/exec_python 异步化） ----------

// sandboxAsyncTestMain 异步机制测试插件。
const sandboxAsyncTestMain = `-- sandbox 异步机制测试
sb_create_rid = sandbox.create_async({ tag = "ctx-create" })
sb_shell_rid = sandbox.exec_shell_async("sb-1", "echo hi", { tag = "ctx-shell" })
sb_py_rid = sandbox.exec_python_async("sb-1", "print('world')", { tag = "ctx-py" })
-- 异步回调错误路径：sb-9 在测试 server 返回 500
sandbox.exec_shell_async("sb-9", "boom")

function on_sandbox_response(req_id, ctx, result, err)
    if err then
        sb_fail_err = tostring(err)
    elseif ctx and ctx.tag == "ctx-shell" then
        sb_shell_output = result.output
        sb_shell_exit = result.exit_code
    elseif ctx and ctx.tag == "ctx-create" then
        sb_create_id = result.sandbox_id
        sb_create_status = result.status
    elseif ctx and ctx.tag == "ctx-py" then
        sb_py_output = result.output
    end
    sb_done = (sb_done or 0) + 1
end
`

func TestSandboxAsync(t *testing.T) {
	// 模拟 sandbox 服务：/v1/sandboxes（create）、/{sid}/shell/exec、/{sid}/python/exec
	sbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes":
			w.Write([]byte(`{"id":"sb-1","status":"running"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/shell/exec"):
			if strings.Contains(r.URL.Path, "sb-9") {
				w.WriteHeader(500)
				w.Write([]byte(`{"error":"boom"}`))
				return
			}
			w.Write([]byte(`{"output":"hello","exit_code":0}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/python/exec"):
			w.Write([]byte(`{"output":"world","error":null}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer sbSrv.Close()
	sbClient := &sandboxcaller.Client{
		Config:     sandboxcaller.Config{BaseURL: sbSrv.URL},
		HttpClient: sbSrv.Client(),
	}

	base := t.TempDir()
	pe := NewPluginEngine(base, &fakeAdapter{}, nil, nil, nil, sbClient, nil, nil, &fakeLLM{reply: "x"})
	writeTestPlugin(t, base, "sbxtest")
	if err := os.WriteFile(filepath.Join(base, "sbxtest", "main.lua"), []byte(sandboxAsyncTestMain), 0o644); err != nil {
		t.Fatal(err)
	}

	L := lua.NewState()
	t.Cleanup(L.Close)
	pe.injectSDK(L, "sbxtest")
	pe.injectBaseAPI(L, "sbxtest", []string{"sandbox"})
	pe.mu.Lock()
	pe.plugins["sbxtest"] = &LoadedPlugin{Manifest: Manifest{Name: "sbxtest"}, State: L, Dir: filepath.Join(base, "sbxtest")}
	pe.mu.Unlock()

	luaDoString(pe, L, fmt.Sprintf(`package.path = %q .. ";" .. package.path`, filepath.Join(base, "sbxtest")+"/?.lua"))
	if err := luaDoFile(pe, L, filepath.Join(base, "sbxtest", "main.lua")); err != nil {
		t.Fatalf("加载测试插件失败: %v", err)
	}

	// 三个异步调用均返回 req_id > 0
	for _, name := range []string{"sb_create_rid", "sb_shell_rid", "sb_py_rid"} {
		if v := luaGetGlobal(pe, L, name); v.Type() != lua.LTNumber || float64(v.(lua.LNumber)) <= 0 {
			t.Fatalf("%s 应返回 req_id > 0, got %v", name, v)
		}
	}

	// 等待全部 4 个回调（create/shell/py 成功 + sb-9 失败）
	waitFor(t, func() bool {
		v := luaGetGlobal(pe, L, "sb_done")
		return v.Type() == lua.LTNumber && float64(v.(lua.LNumber)) >= 4
	})

	// create_async → {sandbox_id, status}
	if v := luaGetGlobal(pe, L, "sb_create_id"); v.String() != "sb-1" {
		t.Fatalf("on_sandbox_response create sandbox_id = %v, want sb-1", v)
	}
	if v := luaGetGlobal(pe, L, "sb_create_status"); v.String() != "running" {
		t.Fatalf("on_sandbox_response create status = %v, want running", v)
	}
	// exec_shell_async → {output, exit_code}
	if v := luaGetGlobal(pe, L, "sb_shell_output"); v.String() != "hello" {
		t.Fatalf("on_sandbox_response shell output = %v, want hello", v)
	}
	if v := luaGetGlobal(pe, L, "sb_shell_exit"); float64(v.(lua.LNumber)) != 0 {
		t.Fatalf("on_sandbox_response shell exit_code = %v, want 0", v)
	}
	// exec_python_async → {output, error}
	if v := luaGetGlobal(pe, L, "sb_py_output"); v.String() != "world" {
		t.Fatalf("on_sandbox_response python output = %v, want world", v)
	}
	// 错误路径：HTTP 500 → 回调 err 非 nil
	if v := luaGetGlobal(pe, L, "sb_fail_err"); v.String() == "" || v.String() == "nil" {
		t.Fatalf("HTTP 500 应回调 err, got %v", v)
	}
}
