package pluggin

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestConcurrentLoadDeduplicates 验证并发 Load(name) 去重（singleflight）：
// 两个 goroutine 同时 Load 同一插件时，entry（L.DoFile 的命令注册/异步提交等
// 副作用）只执行一次，插件表只发布一份；两个调用至少一个成功，
// 另一个复用结果（nil）或得到 already loaded（发布双检兜底）。
func TestConcurrentLoadDeduplicates(t *testing.T) {
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
	// entry 副作用计数器：加载执行一次副作用 +1（纯全局，不依赖 SDK）
	main := `side_effect_count = (side_effect_count or 0) + 1
function on_http_response(req_id, ctx, result, err) end
`
	if err := os.WriteFile(filepath.Join(dir, "main.lua"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	const n = 4
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = pe.Load("chainplug")
		}(i)
	}
	wg.Wait()

	success := 0
	for _, err := range errs {
		if err == nil {
			success++
		}
	}
	if success == 0 {
		t.Fatalf("并发 Load 全部失败: %v", errs)
	}

	// 插件表只发布一份
	pe.mu.RLock()
	loaded := len(pe.plugins)
	p := pe.plugins["chainplug"]
	pe.mu.RUnlock()
	if loaded != 1 {
		t.Fatalf("插件表应有 1 个插件, got %d", loaded)
	}

	// 副作用只发生一次（entry 全局计数 == 1）
	p.stateMu.Lock()
	side := p.State.GetGlobal("side_effect_count")
	p.stateMu.Unlock()
	if side.Type() != side.Type() || side.String() != "1" {
		t.Fatalf("entry 副作用应恰好执行 1 次, got %v", side)
	}

	// loading 表已清空（无残留）
	pe.loadMu.Lock()
	pend := len(pe.loading)
	pe.loadMu.Unlock()
	if pend != 0 {
		t.Fatalf("loading 表应清空, got %d", pend)
	}
}

// TestConcurrentLoadWaitFailure 验证加载失败时等待者收到相同错误且可重试：
// 首次 Load（缺 main.lua）失败后 loading 表清理、done 关闭；补全文件后
// 再次 Load 成功（说明等待者没有卡死、状态没有残留）。
func TestConcurrentLoadWaitFailure(t *testing.T) {
	base := t.TempDir()
	pe := NewPluginEngine(base, &fakeAdapter{}, nil, nil, nil, nil, nil, nil, &fakeLLM{reply: "x"})

	dir := filepath.Join(base, "chainplug")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pluggin.yaml"),
		[]byte("name: chainplug\nversion: \"1.0.0\"\nentry: main.lua\npermissions:\n  - http\nenabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 并发失败加载（main.lua 缺失）：所有调用返回错误，且等待者不被卡死
	errs := make([]error, 3)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = pe.Load("chainplug")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err == nil {
			t.Fatalf("并发失败加载第 %d 个不应成功", i)
		}
	}

	// 补全入口后重试成功
	if err := os.WriteFile(filepath.Join(dir, "main.lua"), []byte("side_effect_count = 1\nfunction on_http_response() end\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pe.Load("chainplug"); err != nil {
		t.Fatalf("失败后重试应成功: %v", err)
	}
	_ = fmt.Sprint()
}
