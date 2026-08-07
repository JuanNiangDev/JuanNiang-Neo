package pluggin

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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
	filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
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
	})
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
	pe.injectBaseAPI(L, name, []string{"onebot11", "database", "file", "llm"})
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
	return L
}

// runOnMessage 构造群消息事件并调用插件 on_message，返回是否被消费。
func runOnMessage(t *testing.T, L *lua.LState, groupID, userID int64, raw, msgID string) bool {
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
	L.DoString(fmt.Sprintf(`package.path = %q .. ";" .. package.path`, filepath.Join(base, "llmtest")+"/?.lua"))
	if err := L.DoFile(filepath.Join(base, "llmtest", "main.lua")); err != nil {
		t.Fatalf("加载测试插件失败: %v", err)
	}
	pe.mu.Lock()
	pe.plugins["llmtest"] = &LoadedPlugin{Manifest: Manifest{Name: "llmtest"}, State: L, Dir: filepath.Join(base, "llmtest")}
	pe.mu.Unlock()

	if v := L.GetGlobal("avail"); v.Type() != lua.LTBool || !bool(v.(lua.LBool)) {
		t.Fatalf("llm.available() 应为 true, got %v", v)
	}
	if v := L.GetGlobal("synced"); v.Type() != lua.LTString || v.String() != `{"violation":"none"}` {
		t.Fatalf("llm.chat 同步结果 = %v", v)
	}
	// 异步：chat_async 立即返回 req_id > 0，完成后引擎派发 on_chat_response
	submitted := L.GetGlobal("cb_submitted")
	if submitted.Type() != lua.LTNumber || float64(submitted.(lua.LNumber)) <= 0 {
		t.Fatalf("chat_async 应返回 req_id > 0, got %v", submitted)
	}
	waitFor(t, func() bool {
		v := L.GetGlobal("done_count")
		return v.Type() == lua.LTNumber && float64(v.(lua.LNumber)) >= 1
	})
	if v := L.GetGlobal("last_rid"); v.String() != submitted.String() {
		t.Fatalf("on_chat_response req_id = %v, want 与提交返回值一致 %v", v, submitted)
	}
	if v := L.GetGlobal("last_content"); v.String() != `{"violation":"none"}` {
		t.Fatalf("异步回调 content = %v", v.String())
	}
	if v := L.GetGlobal("last_err"); v.String() != "nil" {
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
	L.DoString(`err_submitted = llm.chat_async("hi", {})`)
	waitFor(t, func() bool {
		v := L.GetGlobal("done_count")
		return v.Type() == lua.LTNumber && float64(v.(lua.LNumber)) >= 2
	})
	if !strings.Contains(L.GetGlobal("last_err").String(), "boom") {
		t.Fatalf("错误回调 err = %v, want 包含 boom", L.GetGlobal("last_err"))
	}
}

// ---------- 测试: 真实 redrock_group_manager 插件行为 ----------

func TestRedrockBlackGraySensitive(t *testing.T) {
	llm := &fakeLLM{reply: `{"violation":"none"}`}
	pe, adp, _ := newTestEngine(t, llm)
	L := loadPluginManual(t, pe, "redrock_group_manager")

	// 1. 黑色地带：无本续贷（words/black.txt）→ 消费 + 撤回（第一次违规），不触发 LLM
	consumed := runOnMessage(t, L, 10001, 20001, "对接全国 无本续贷 一手收单", "90001")
	if !consumed {
		t.Fatal("黑色词消息应被消费")
	}
	waitFor(t, func() bool { return adp.deletedCount() >= 1 })
	if llm.callCount() != 0 {
		t.Fatalf("黑色词不应触发 LLM, calls=%d", llm.callCount())
	}

	// 2. 敏感地带：操逼（cn_pornographic）→ 消费 + 撤回，不触发 LLM
	before := adp.deletedCount()
	consumed = runOnMessage(t, L, 10001, 20002, "你个操逼的", "90002")
	if !consumed {
		t.Fatal("敏感词消息应被消费")
	}
	waitFor(t, func() bool { return adp.deletedCount() > before })
	if llm.callCount() != 0 {
		t.Fatalf("敏感词不应触发 LLM, calls=%d", llm.callCount())
	}

	// 3. 灰色地带：命中 all.txt 灰色词（考研/加群）→ 不消费、不立即处罚，异步触发 LLM
	llm.setReply(`{"violation":"ad","reason":"以资料分享为幌子拉群引流"}`)
	consumed = runOnMessage(t, L, 10001, 20003, "考研上岸的学长学姐，加群一起交流经验", "90003")
	if consumed {
		t.Fatal("灰色词消息不应被消费（先放行）")
	}
	waitFor(t, func() bool { return llm.callCount() >= 1 })
	// 回调（LLM 判 ad）→ 撤回追罚
	waitFor(t, func() bool { return adp.deletedCount() >= 3 })

	// 4. 灰色地带：LLM 判 none → 不处罚
	llm.setReply(`{"violation":"none","reason":"正常讨论考研"}`)
	before = adp.deletedCount()
	consumed = runOnMessage(t, L, 10001, 20004, "有没有考研上岸的学长学姐", "90004")
	if consumed {
		t.Fatal("灰色词消息不应被消费")
	}
	time.Sleep(300 * time.Millisecond)
	if adp.deletedCount() != before {
		t.Fatalf("LLM 判 none 不应撤回: before=%d after=%d", before, adp.deletedCount())
	}

	// 5. 豁免：/豁免 20005（管理员）→ 黑色词放行；被禁言用户豁免时自动解除禁言
	adp.mu.Lock()
	adp.mutedUsers[20005] = true
	adp.mu.Unlock()
	cmdEvent := EventData{
		PostType: "message", MessageType: "group",
		GroupID: 10001, UserID: 99999, RawMessage: "/豁免 20005",
		Admins: []string{"99999"}, MessageID: 1,
	}
	consumed, reply, err := pe.commands.Dispatch("/豁免 20005", cmdEvent)
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

	consumed = runOnMessage(t, L, 10001, 20005, "对接全国 无本续贷", "90005")
	if consumed {
		t.Fatal("豁免用户发黑色词不应被消费")
	}
	waitFor(t, func() bool { return adp.deletedCount() >= 3 })
	if adp.deletedCount() != 3 {
		t.Fatalf("豁免用户消息不应被撤回: deleted=%d", adp.deletedCount())
	}

	// 6. 解除豁免（/取消豁免 别名同样注册）→ 恢复检测
	cmdEvent.RawMessage = "/解除豁免 20005"
	consumed, _, err = pe.commands.Dispatch("/解除豁免 20005", cmdEvent)
	if err != nil || !consumed {
		t.Fatalf("/解除豁免 派发失败 consumed=%v err=%v", consumed, err)
	}
	consumed = runOnMessage(t, L, 10001, 20005, "无本续贷 全国一手收单", "90006")
	if !consumed {
		t.Fatal("解除豁免后黑色词应恢复被消费")
	}
	waitFor(t, func() bool { return adp.deletedCount() >= 4 })

	// 7. 命令注册完整性
	for _, cmd := range []string{"/豁免", "/解除豁免", "/取消豁免", "/groupstats"} {
		if !pe.commands.HasCommand(cmd) {
			t.Fatalf("命令 %s 未注册", cmd)
		}
	}
}

// 防未使用告警（io 保留给后续扩展）
var _ = io.Discard
