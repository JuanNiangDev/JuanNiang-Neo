package pluggin

import (
	"sync/atomic"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// TestTimerAfterDispatch 验证 jn.timer.after 异步 API 端到端：
//  1. 声明 timer 权限后 jn.timer 表被注入
//  2. after(秒, ctx) 立即返回有效 req_id
//  3. 到点后引擎经异步注册表（kind "timer"）派发 on_timer_response，且 ctx 现场原样取回
func TestTimerAfterDispatch(t *testing.T) {
	pe, _, _ := newTestEngine(t, nil)
	L := loadPluginManual(t, pe, "redrock_group_manager")

	// jn.timer 应已注入（loadPluginManual 权限含 timer）
	timer := luaGetGlobal(pe, L, "timer")
	if timer.Type() != lua.LTTable {
		t.Fatalf("jn.timer 未注入, got %s", timer.Type())
	}

	// 覆盖全局 on_timer_response，捕获派发与 ctx 传递
	var fired atomic.Bool
	pe.mu.RLock()
	L.SetGlobal("on_timer_response", L.NewFunction(func(LL *lua.LState) int {
		ctxv := LL.Get(2)
		if ctxv.Type() == lua.LTTable {
			L.SetGlobal("timer_ctx_tag", ctxv.(*lua.LTable).RawGetString("tag"))
		}
		fired.Store(true)
		return 0
	}))

	// 调用 jn.timer.after(0.05, {tag="x"})
	after := L.GetField(timer.(*lua.LTable), "after")
	L.Push(after)
	L.Push(lua.LNumber(0.05))
	ctx := L.NewTable()
	ctx.RawSetString("tag", lua.LString("x"))
	L.Push(ctx)
	if err := L.PCall(2, 1, nil); err != nil {
		pe.mu.RUnlock()
		t.Fatalf("jn.timer.after 调用失败: %v", err)
	}
	reqID := L.Get(-1)
	L.Pop(1)
	pe.mu.RUnlock()
	if reqID == lua.LNil || reqID == lua.LNumber(0) {
		t.Fatalf("after 返回无效 req_id: %v", reqID)
	}

	// 等待异步派发
	deadline := time.After(3 * time.Second)
	for !fired.Load() {
		select {
		case <-deadline:
			t.Fatal("timer 回调未在超时内派发")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// ctx 现场应原样取回
	if v := luaGetGlobal(pe, L, "timer_ctx_tag"); v != lua.LString("x") {
		t.Fatalf("on_timer_response 未取回 ctx.tag, got %v", v)
	}
}

// TestTimerRequiresPermission 验证未声明 timer 权限时 jn.timer 不注入。
func TestTimerRequiresPermission(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	pe, _, _ := newTestEngine(t, nil)
	pe.injectBaseAPI(L, "noperm", []string{"onebot11"}) // 无 timer 权限
	if v := L.GetGlobal("timer"); v.Type() != lua.LTNil {
		t.Fatalf("未声明 timer 权限时 jn.timer 不应注入, got %s", v.Type())
	}
}
