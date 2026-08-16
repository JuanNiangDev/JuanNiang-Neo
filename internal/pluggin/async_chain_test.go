package pluggin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"

	"JuanNiang-Neo/internal/adapter"
)

// 回归测试：插件在异步回调（on_http_response）内再次提交异步任务（链式调用，
// repo-intro 插件的 meta → README 流水线模式），不得死锁。
//
// 曾因 runAsyncCallbacks 持 pe.mu 写锁执行 Lua 回调时，submitAsync 查表再取
// pe.mu 读锁（Go RWMutex 不可重入）导致永久死锁：pe.mu 被永远占用后，
// 事件派发（Dispatch/OnNotice 取读锁）与其它异步回调（取写锁）全部冻结——
// 表现为戳一戳插件与 LLM Agent 对后续消息一律无响应，只能重启恢复。
const asyncChainTestMain = `-- 异步链式调用回归测试
-- 加载期提交第一个异步任务（覆盖 Load 持锁范围）
http.get_async("{{STEP1}}", { tag = "first" })

function on_http_response(req_id, ctx, result, err)
    if ctx and ctx.tag == "first" then
        first_done = (first_done or 0) + 1
        -- 回调内再次提交异步任务（关键回归点）
        http.get_async("{{STEP2}}", { tag = "second" })
    elseif ctx and ctx.tag == "second" then
        second_done = true
    end
end
`

func TestAsyncChainInsideCallback(t *testing.T) {
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"path":%q}`, r.URL.Path)
	}))
	defer httpSrv.Close()

	base := t.TempDir()
	pe := NewPluginEngine(base, &fakeAdapter{}, nil, nil, nil, nil, nil, nil, &fakeLLM{reply: "x"})

	// 与生产一致的插件目录结构，走 pe.Load 真实加载路径
	dir := filepath.Join(base, "chainplug")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pluggin.yaml"),
		[]byte("name: chainplug\nversion: \"1.0.0\"\nentry: main.lua\npermissions:\n  - http\nenabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := strings.ReplaceAll(asyncChainTestMain, "{{STEP1}}", httpSrv.URL+"/step1")
	main = strings.ReplaceAll(main, "{{STEP2}}", httpSrv.URL+"/step2")
	if err := os.WriteFile(filepath.Join(dir, "main.lua"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pe.Load("chainplug"); err != nil {
		t.Fatalf("加载插件失败: %v", err)
	}

	// 回调链必须完整执行：first 回调里再提交 second，全部在 5s 内完成
	waitFor(t, func() bool {
		v := luaGetGlobal(pe, pe.plugins["chainplug"].State, "second_done")
		return v.Type() == lua.LTBool && bool(v.(lua.LBool))
	})
	if v := luaGetGlobal(pe, pe.plugins["chainplug"].State, "first_done"); v.Type() != lua.LTNumber || float64(v.(lua.LNumber)) != 1 {
		t.Fatalf("first 回调应恰好执行 1 次, got %v", v)
	}

	// 死锁判定面：pe.mu 未被永久占用——事件派发（读锁）与卸载（写锁）都必须能完成
	done := make(chan struct{})
	go func() {
		defer close(done)
		pe.Dispatch(adapter.Event{})
		_ = pe.Unload("chainplug")
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("回调链完成后 pe.mu 仍被占用：事件派发/卸载阻塞，死锁未修复")
	}
}
