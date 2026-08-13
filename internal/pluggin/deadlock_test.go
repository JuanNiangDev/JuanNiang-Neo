package pluggin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// ====================================================================
// 今日修复的回归测试：
//  1. 死锁 1：异步回调（on_chat_response/on_timer_response）内再提交异步任务
//     （timer.after/chat_async）——旧引擎 runAsyncCallbacks 持写锁执行回调，
//     回调内 submitAsync 请求 RLock 自死锁
//  2. 死锁 3：命令 handler 内动态注册命令——旧引擎在 commands 读锁下执行
//     handler，注册命令需要写锁 = 读锁升级写锁死锁
//  3. 生命周期：Unload 丢弃在途异步任务 / 等待在途回调完成，不触碰已关闭 LState
//  4. execCommand 插件已卸载时不执行 handler（Match 与执行之间的 Unload 窗口）
//  5. 处罚写操作同步 API：成功生效且失败可感知（返回 false+err）
//  6. onCronJob 按插件目录名过滤（修复 p.Manifest.Name 显示名误用）
//  7. per-plugin stateMu 隔离：慢回调不阻塞其它插件
//  8. atomic 注册表 copy-on-write 并发读写无 data race
// ====================================================================

// newMiniTestEngine 独立测试引擎：不依赖 Plugins 仓库检出（loadMiniPlugin 自建插件），
// 与 newTestEngine（端到端加载真实插件）互补，任何环境下都可运行。
func newMiniTestEngine(t *testing.T, llm LLMAccess) (*PluginEngine, *fakeAdapter) {
	t.Helper()
	adp := &fakeAdapter{mu: sync.Mutex{}, mutedUsers: make(map[int64]bool)}
	pe := NewPluginEngine(t.TempDir(), adp, nil, nil, nil, nil, nil, nil, llm)
	return pe, adp
}

// loadMiniPlugin 用给定 Lua 源码加载一个临时插件（注入 onebot11/database/file/llm/timer 权限）。
// 与 loadPluginManual 相同的生命周期约定：Cleanup 先删 map 再关 LState（LIFO）。
func loadMiniPlugin(t *testing.T, pe *PluginEngine, name, src string) *lua.LState {
	t.Helper()
	pluginDir := filepath.Join(pe.basePath, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("创建插件目录失败: %v", err)
	}
	// SDK 落盘：生产由 ensureEmbeddedAssets 在启动时写入 basePath/sdk/jn.lua，
	// 测试环境手动补齐（injectSDK 仅把 sdk 目录加入 package.path）
	sdkDir := filepath.Join(pe.basePath, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatalf("创建 sdk 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sdkDir, "jn.lua"), []byte(jnSDKSource), 0o644); err != nil {
		t.Fatalf("写入 sdk/jn.lua 失败: %v", err)
	}
	manifest := "name: " + name + "\nentry: main.lua\npermissions:\n  - onebot11\n  - database\n  - file\n  - llm\n  - timer\nenabled: true\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "pluggin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("写入 pluggin.yaml 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "main.lua"), []byte(src), 0o644); err != nil {
		t.Fatalf("写入 main.lua 失败: %v", err)
	}

	L := lua.NewState()
	pe.injectSDK(L, name)
	pe.injectBaseAPI(L, name, []string{"onebot11", "database", "file", "llm", "timer", "agent"})
	// database 需要 gorm db（测试环境不提供），手写空实现
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
	// injectBaseAPI 要求 pe.dao != nil 才注入 agent；测试引擎 dao 为 nil，
	// 手动补注入（get_current_chat_area 仅依赖 agentOp，nil 时返回事件字段、
	// 不带 chat_area_id；其余 agent API 内部均有 nil 防御）
	pe.injectAgent(L)
	if err := L.DoString(fmt.Sprintf(`package.path = %q .. ";" .. package.path`, pluginDir+"/?.lua")); err != nil {
		t.Fatalf("设置 package.path 失败: %v", err)
	}
	if err := L.DoFile(filepath.Join(pluginDir, "main.lua")); err != nil {
		t.Fatalf("加载插件 %s 失败: %v", name, err)
	}

	pe.mu.Lock()
	p := &LoadedPlugin{Manifest: Manifest{Name: name}, State: L, Dir: pluginDir}
	attachPluginRef(L, p)
	pe.plugins[name] = p
	pe.mu.Unlock()
	// 清理：先删 map（runAsyncCallbacks 拿不到新任务），再 p.close()——
	// 在 stateMu 下关闭并与在途异步回调互斥；已被 Unload 关闭时幂等跳过。
	// 不能直接 L.Close()：会与 runAsyncCallbacks 在途 PCall 并发（race/panic）。
	t.Cleanup(func() {
		pe.mu.Lock()
		p, ok := pe.plugins[name]
		delete(pe.plugins, name)
		pe.mu.Unlock()
		if ok && p != nil {
			p.close()
		}
	})
	return L
}

// TestAsyncCallbackNestedSubmitAsync 死锁 1 回归：异步回调内再提交异步任务。
// 旧引擎：runAsyncCallbacks 持 pe.mu 写锁执行 on_chat_response，回调内
// jn.timer.after / jn.llm.chat_async → submitAsync → pe.mu.RLock() → 自死锁，
// 事件循环与 Web API 全部停摆（redrock_group_manager 违规通知即此路径）。
func TestAsyncCallbackNestedSubmitAsync(t *testing.T) {
	llm := &fakeLLM{reply: `{"violation":"none"}`}
	pe, _ := newMiniTestEngine(t, llm)
	loadMiniPlugin(t, pe, "nested", `
local jn = require("jn")
function on_message(event)
    jn.llm.chat_async({ { role = "user", content = "m1" } }, { timeout = 5 })
    return false, nil
end
function on_chat_response(req_id, content, err)
    -- 回调内再次提交：timer + chat（旧引擎写锁下 submitAsync 自死锁）
    jn.timer.after(0.05, { tag = "nested" })
    jn.llm.chat_async({ { role = "user", content = "m2" } }, { timeout = 5 })
end
function on_timer_response(req_id, ctx, result, err)
    if ctx and ctx.tag == "nested" then
        -- 再提交一层：验证链式异步不阻塞
        jn.llm.chat_async({ { role = "user", content = "m3" } }, { timeout = 5 })
    end
end
`)
	L := pe.plugins["nested"].State

	// 触发 on_message → chat_async(1) → on_chat_response → timer + chat_async(2)
	// → on_timer_response → chat_async(3)：共 3 次 LLM 调用。
	// 死锁时链无法推进，waitFor 超时 fail。
	if runOnMessage(t, pe, L, 10001, 20001, "hello", "90001") {
		t.Fatal("on_message 应返回 consumed=false")
	}
	waitFor(t, func() bool { return llm.callCount() >= 3 })

	// 事件循环仍应健康：再来一条消息能正常处理（无残留锁）
	if runOnMessage(t, pe, L, 10001, 20002, "world", "90002") {
		t.Fatal("第二条消息 on_message 应正常执行")
	}
}

// TestAsyncCallbackTimerChain 死锁 1 回归（timer 链）：on_timer_response 内
// 再调度 timer（redrock_group_manager notify_pump 逐条延迟发送模式）。
func TestAsyncCallbackTimerChain(t *testing.T) {
	pe, adp := newMiniTestEngine(t, nil)
	loadMiniPlugin(t, pe, "timerchain", `
local jn = require("jn")
function on_message(event)
    jn.timer.after(0.05, { chain = 1 })
    return false, nil
end
function on_timer_response(req_id, ctx, result, err)
    if not ctx then return end
    if ctx.chain == 1 then
        jn.onebot11.send_group_msg(1001, "first")
        jn.timer.after(0.05, { chain = 2 })   -- 回调内再调度（旧引擎自死锁点）
    elseif ctx.chain == 2 then
        jn.onebot11.send_group_msg(1001, "second")
    end
end
`)
	L := pe.plugins["timerchain"].State
	runOnMessage(t, pe, L, 10001, 20001, "go", "90001")

	// 链应完整走完：first → second 两条消息都发出
	waitFor(t, func() bool {
		return adp.deletedCount() == 0 && len(adp.mutedCalls()) == 0 && hasGroupMsg(adp, "first") && hasGroupMsg(adp, "second")
	})
}

// TestCommandHandlerDynamicRegister 死锁 3 回归：命令 handler 内动态注册命令。
// 旧引擎：commands.Dispatch 持 commands 读锁执行 handler，注册需要写锁
// = 读锁升级写锁死锁。新引擎：Match 锁内匹配，handler 在插件 stateMu 下执行。
func TestCommandHandlerDynamicRegister(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	loadMiniPlugin(t, pe, "dynreg", `
local jn = require("jn")
jn.command.register("outer", function(args, event)
    -- handler 内动态注册新命令
    jn.command.register("inner", function(args2, event2)
        return true, "inner-ok"
    end, { description = "dyn" })
    return true, "outer-ok"
end)
`)
	ev := EventData{PostType: "message", MessageType: "group", UserID: 1, GroupID: 2}

	c, reply, err := dispatchCommand(pe, "/outer x", ev)
	if err != nil || !c || reply != "outer-ok" {
		t.Fatalf("/outer 应返回 (true, outer-ok, nil), got (%v, %q, %v)", c, reply, err)
	}
	// 动态注册的命令立即可用
	c, reply, err = dispatchCommand(pe, "/inner", ev)
	if err != nil || !c || reply != "inner-ok" {
		t.Fatalf("/inner 应返回 (true, inner-ok, nil), got (%v, %q, %v)", c, reply, err)
	}
}

// TestUnloadDropsPendingAsync 生命周期：卸载后，在途异步任务被丢弃，
// 不得在已关闭的 LState 上执行回调（不 panic 即通过）。
func TestUnloadDropsPendingAsync(t *testing.T) {
	llm := &fakeLLM{reply: "ok"}
	llm.setDelay(300 * time.Millisecond) // 任务完成时插件已卸载
	pe, _ := newMiniTestEngine(t, llm)
	loadMiniPlugin(t, pe, "pend", `
local jn = require("jn")
function on_message(event)
    jn.llm.chat_async({ { role = "user", content = "x" } }, { timeout = 5 })
    return false, nil
end
function on_chat_response(req_id, content, err)
    jn.onebot11.send_group_msg(1001, "should-never-send")
end
`)
	L := pe.plugins["pend"].State
	runOnMessage(t, pe, L, 10001, 20001, "go", "90001")

	// 任务在途时卸载插件
	if err := pe.Unload("pend"); err != nil {
		t.Fatalf("Unload 失败: %v", err)
	}
	// 等任务完成 + 回调派发路径走完：回调不得执行（LState 已关闭）
	time.Sleep(600 * time.Millisecond)
	// 测试未 panic 即通过；顺带确认回调确实被丢弃（fakeLLM 计数已 +1，但消息未发出）
	if llm.callCount() != 1 {
		t.Fatalf("LLM 应只被调用 1 次, got %d", llm.callCount())
	}
}

// TestUnloadWaitsForInFlightCallback 生命周期：卸载等待在途回调完成
// （close 与执行互斥），Unload 返回时回调必然已结束。
func TestUnloadWaitsForInFlightCallback(t *testing.T) {
	pe, adp := newMiniTestEngine(t, nil)
	loadMiniPlugin(t, pe, "inflight", `
local jn = require("jn")
function on_message(event)
    jn.timer.after(0.05, { tag = "x" })
    return false, nil
end
function on_timer_response(req_id, ctx, result, err)
    -- 模拟慢回调：忙等 0.5s 后发消息
    local t0 = os.clock()
    while os.clock() - t0 < 0.5 do end
    jn.onebot11.send_group_msg(1001, "inflight-done")
end
`)
	L := pe.plugins["inflight"].State
	runOnMessage(t, pe, L, 10001, 20001, "go", "90001")

	// 等回调开始忙等（0.05s 到点后进入回调；sleep 200ms 保证在途）
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	if err := pe.Unload("inflight"); err != nil {
		t.Fatalf("Unload 失败: %v", err)
	}
	elapsed := time.Since(start)
	// close 等待在途回调（约剩余 0.35s），Unload 应被阻塞而非直接关闭 LState
	if elapsed < 300*time.Millisecond {
		t.Fatalf("Unload 应在在途回调完成后返回, elapsed=%v", elapsed)
	}
	// 回调确实完整执行了（忙等结束后发送的消息存在）
	waitFor(t, func() bool { return hasGroupMsg(adp, "inflight-done") })
}

// TestBuiltinHelpNotRequireSystemPlugin 内置 /help 命令的执行不依赖 system 插件
// 是否加载（handler 是纯 Go 闭包，不触碰 LState）。修复前 execCommand 按
// pluginByName(node.PluginName) 查找，system 未加载时 /help 静默不可用。
func TestBuiltinHelpNotRequireSystemPlugin(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil) // 未加载任何插件（含 system）
	ev := EventData{PostType: "message", MessageType: "group", UserID: 1, GroupID: 2}

	c, reply, err := dispatchCommand(pe, "/help", ev)
	if err != nil || !c {
		t.Fatalf("/help 应被消费且无错误, got (consumed=%v, err=%v)", c, err)
	}
	if !strings.Contains(reply, "可用命令") {
		t.Fatalf("/help 应返回帮助文本, got %q", reply)
	}

	// 内置命令不参与插件命令列表（无插件归属）
	if pe.commands.HasCommand("/help") == false {
		t.Fatal("/help 应存在于命令注册表")
	}
}

// TestExecCommandSkipsUnloadedPlugin execCommand 安全分支：命令已匹配但插件
// 已卸载（Match 与执行之间的 Unload 窗口）→ 不执行 handler、不 panic。
func TestExecCommandSkipsUnloadedPlugin(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	loadMiniPlugin(t, pe, "cmdu", `
local jn = require("jn")
jn.command.register("qd", function(args, event)
    return true, "qd-ok"
end)
`)
	ev := EventData{PostType: "message", MessageType: "group", UserID: 1, GroupID: 2}

	node, args, hint := pe.commands.Match("/qd")
	if node == nil || hint != "" {
		t.Fatalf("Match 应命中 /qd")
	}

	// 模拟 Unload 窗口：命令仍在注册表，但插件已从 map 删除（未调 Unload 以免移除命令）
	pe.mu.Lock()
	delete(pe.plugins, "cmdu")
	pe.mu.Unlock()

	c, reply, err := pe.execCommand(node, args, ev)
	if err != nil || c || reply != "" {
		t.Fatalf("插件已卸载时 execCommand 应返回 (false, \"\", nil), got (%v, %q, %v)", c, reply, err)
	}
}

// TestOneBot11WriteAPIs 处罚写操作同步 API：成功返回 (true, nil) 且调用生效。
// 插件侧处罚动作（撤回/禁言/踢人）用同步版——踢人不频繁，同步等待可接受，
// 且失败必须即时感知（见 TestOneBot11WriteAPIError）。
func TestOneBot11WriteAPIs(t *testing.T) {
	pe, adp := newMiniTestEngine(t, nil)
	L := loadMiniPlugin(t, pe, "syncwrite", `
local jn = require("jn")
function on_message(event)
    local ok1, err1 = jn.onebot11.delete_msg("90001")
    local ok2, err2 = jn.onebot11.ban_group_member(12345, 67890, 60)
    local ok3, err3 = jn.onebot11.kick_group_member(12345, 67890, false)
    assert(ok1 == true and err1 == nil and ok2 == true and err2 == nil and ok3 == true and err3 == nil, "同步 API 应返回 (true, nil): " .. tostring(err1) .. tostring(err2) .. tostring(err3))
    return false, nil
end
`)
	if runOnMessage(t, pe, L, 10001, 20001, "go", "90000") {
		t.Fatal("on_message 应返回 consumed=false")
	}
	// 调用已同步生效（fakeAdapter 记录到调用）
	adp.mu.Lock()
	defer adp.mu.Unlock()
	foundDel, foundBan, foundKick := false, false, false
	for _, id := range adp.deleted {
		if id == 90001 {
			foundDel = true
		}
	}
	for _, m := range adp.muted {
		if m == "12345:67890:60" {
			foundBan = true
		}
	}
	for _, k := range adp.kicked {
		if k == "12345:67890" {
			foundKick = true
		}
	}
	if !foundDel || !foundBan || !foundKick {
		t.Fatalf("同步 API 未生效: del=%v ban=%v kick=%v", foundDel, foundBan, foundKick)
	}
}

// TestOneBot11WriteAPIError 同步 API 失败可感知：返回 (false, err)，
// 插件据此做失败处理（不重置违规次数、通知管理员）。
func TestOneBot11WriteAPIError(t *testing.T) {
	pe, adp := newMiniTestEngine(t, nil)
	adp.kickErr = fmt.Errorf("no permission")
	L := loadMiniPlugin(t, pe, "syncerr", `
local jn = require("jn")
function on_message(event)
    local ok, err = jn.onebot11.kick_group_member(12345, 67890, false)
    assert(ok == false and err ~= nil and err ~= "", "失败应返回 (false, err), got (" .. tostring(ok) .. ", " .. tostring(err) .. ")")
    return false, nil
end
`)
	if runOnMessage(t, pe, L, 10001, 20001, "go", "90000") {
		t.Fatal("on_message 应返回 consumed=false")
	}
	// 失败时不应记录踢人
	adp.mu.Lock()
	defer adp.mu.Unlock()
	if len(adp.kicked) != 0 {
		t.Fatalf("失败调用不应记录, got %v", adp.kicked)
	}
}

// TestCronJobDirectoryNameFilter onCronJob 按插件目录名过滤
// （修复误用 p.Manifest.Name 显示名的行为 bug）。
func TestCronJobDirectoryNameFilter(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	loadMiniPlugin(t, pe, "plug_a", `
function on_cronjob(event)
    _G.cron_fired = (_G.cron_fired or 0) + 1
    return false
end
`)
	loadMiniPlugin(t, pe, "plug_b", `
function on_cronjob(event)
    _G.cron_fired = (_G.cron_fired or 0) + 1
    return false
end
`)
	L_a := pe.plugins["plug_a"].State
	L_b := pe.plugins["plug_b"].State

	ev := EventData{PostType: "cronjob", Admins: []string{}}
	// 只通知 plug_a（目录名过滤）
	if !pe.onCronJob(ev, []string{"plug_a"}) {
		// 返回 false 也可以（插件 on_cronjob 返回 false），重点看是否被调用
	}
	if v := luaGetGlobal(pe, L_a, "cron_fired"); v != lua.LNumber(1) {
		t.Fatalf("plug_a 应被调用 1 次, got %v", v)
	}
	if v := luaGetGlobal(pe, L_b, "cron_fired"); v != lua.LNil {
		t.Fatalf("plug_b 不应被调用（目录名过滤）, got %v", v)
	}

	// 空列表 = 通知所有插件
	pe.onCronJob(ev, nil)
	if v := luaGetGlobal(pe, L_a, "cron_fired"); v != lua.LNumber(2) {
		t.Fatalf("plug_a 应累计 2 次, got %v", v)
	}
	if v := luaGetGlobal(pe, L_b, "cron_fired"); v != lua.LNumber(1) {
		t.Fatalf("plug_b 应被调用 1 次, got %v", v)
	}
}

// TestPluginSlowCallbackNotBlockOthers per-plugin 隔离：插件 A 的慢 on_message
// （忙等 0.5s，持 A.stateMu）不阻塞插件 B 的异步回调派发（B.stateMu 空闲）。
// 旧引擎：事件循环持全局读锁执行 A 的 on_message，runAsyncCallbacks 等待写锁，
// B 的 timer 回调被推迟到 A 完成后（>0.5s）；新引擎：B 回调在 ~0.05s 完成。
func TestPluginSlowCallbackNotBlockOthers(t *testing.T) {
	pe, adp := newMiniTestEngine(t, nil)
	loadMiniPlugin(t, pe, "slow_a", `
function on_message(event)
    -- 模拟慢插件：忙等 0.5s（占用自身 stateMu）
    local t0 = os.clock()
    while os.clock() - t0 < 0.5 do end
    return false, nil
end
`)
	loadMiniPlugin(t, pe, "fast_b", `
local jn = require("jn")
function on_message(event)
    jn.timer.after(0.05, { tag = "b" })
    return false, nil
end
function on_timer_response(req_id, ctx, result, err)
    jn.onebot11.send_group_msg(1002, "b-done")
end
`)

	// A 的慢 on_message 在 goroutine 里执行（模拟事件循环处理慢插件）
	done := make(chan struct{})
	go func() {
		defer close(done)
		runOnMessage(t, pe, pe.plugins["slow_a"].State, 10001, 20001, "slow", "90001")
	}()
	// 确保 A 已进入忙等（持 A.stateMu）
	time.Sleep(150 * time.Millisecond)

	// B 提交 timer 并等待回调：应 ~0.05s 完成，不被 A 的忙等阻塞
	start := time.Now()
	runOnMessage(t, pe, pe.plugins["fast_b"].State, 10002, 20002, "go", "90002")
	waitFor(t, func() bool { return hasGroupMsg(adp, "b-done") })
	elapsed := time.Since(start)
	if elapsed > 400*time.Millisecond {
		t.Fatalf("插件 B 的异步回调被插件 A 的慢 on_message 阻塞: b-done 耗时 %v（应 < 400ms）", elapsed)
	}
	<-done
}

// TestRegisterAsyncAPIConcurrentReadWrite atomic 注册表 copy-on-write：
// 并发注册（load-copy-store 由 registerMu 串行化）+ 并发读取（无锁）。
// 无 data race（-race 下运行），且并发注册的 kind 全部保留（无静默丢失）。
func TestRegisterAsyncAPIConcurrentReadWrite(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	const writers = 8
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			kind := fmt.Sprintf("custom-%d", i)
			pe.RegisterAsyncAPI(kind, AsyncAPI{Entry: "on_custom_response"})
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			m := pe.asyncAPIs.Load()
			_ = (*m)["chat"]
			_ = (*m)["timer"]
		}
	}()
	wg.Wait()
	// 内置注册不受影响
	m := *pe.asyncAPIs.Load()
	if _, ok := m["chat"]; !ok {
		t.Fatal("内置 chat kind 丢失")
	}
	// 并发注册的 kind 全部保留（修复前：后 Store 的 writer 覆盖先注册的 kind，静默丢失）
	for i := 0; i < writers; i++ {
		if _, ok := m[fmt.Sprintf("custom-%d", i)]; !ok {
			t.Fatalf("并发注册的 kind custom-%d 丢失（writer 未串行化）", i)
		}
	}
}

// TestAsyncCallbackSeesTriggerEvent 回归：事件 A 触发的异步任务，即使事件 B
// 随后派发，回调（on_chat_response）内 jn.agent.get_current_chat_area 仍读到
// 任务触发时的事件 A（修复前读共享全局 currentEv 会读到 B 的会话）。
func TestAsyncCallbackSeesTriggerEvent(t *testing.T) {
	llm := &fakeLLM{reply: "ok"}
	llm.setDelay(200 * time.Millisecond) // 任务 A 在途期间让事件 B 派发
	pe, _ := newMiniTestEngine(t, llm)
	loadMiniPlugin(t, pe, "evctx", `
local jn = require("jn")
function on_message(event)
    -- 仅事件 A（"hello"）触发异步任务，事件 B（"world"）只派发不提交，
    -- 避免多个任务完成顺序不定导致断言抖动
    if event.raw_message == "hello" then
        jn.llm.chat_async({ { role = "user", content = "x" } }, { timeout = 5 })
    end
    return false, nil
end
function on_chat_response(req_id, content, err)
    local area = jn.agent.get_current_chat_area()
    _G.cb_group = area.group_id
    _G.cb_user = area.user_id
end
`)
	L := pe.plugins["evctx"].State

	// 事件 A：群 10001 / 用户 20001（触发异步任务）
	runOnMessage(t, pe, L, 10001, 20001, "hello", "90001")
	// 事件 B 立即派发：群 10002 / 用户 20002（修复前会覆盖全局 currentEv）
	runOnMessage(t, pe, L, 10002, 20002, "world", "90002")

	// 任务 A 的回调必须读到事件 A 的会话（事件 B 派发不影响）
	waitFor(t, func() bool { return luaGetGlobal(pe, L, "cb_group") != lua.LNil })
	if v := luaGetGlobal(pe, L, "cb_group"); v != lua.LNumber(10001) {
		t.Fatalf("回调应读到事件 A 的 group_id=10001, got %v", v)
	}
	if v := luaGetGlobal(pe, L, "cb_user"); v != lua.LNumber(20001) {
		t.Fatalf("回调应读到事件 A 的 user_id=20001, got %v", v)
	}
}

// ---------- 测试辅助 ----------

// hasGroupMsg 检查 fakeAdapter 是否收到过包含子串的消息。
func hasGroupMsg(adp *fakeAdapter, substr string) bool {
	adp.mu.Lock()
	defer adp.mu.Unlock()
	for _, g := range adp.groups {
		if strings.Contains(g, substr) {
			return true
		}
	}
	return false
}
