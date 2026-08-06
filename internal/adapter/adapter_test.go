package adapter

import (
	"context"
	"testing"
	"time"
)

// newTestAdapter 创建一个监听随机端口的 adapter（Port=0 → ":0"）。
func newTestAdapter() *Adapter {
	return New(Config{Addr: "", Port: 0, Enable: true})
}

// TestStartCreatesServer 首次 Start 必须真正创建 WS server（回归：曾因
// closed 初始为 true 导致刚创建的 server 被误关，Start 返回 nil 但无监听）。
func TestStartCreatesServer(t *testing.T) {
	p := newTestAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("首次 Start 失败: %v", err)
	}
	defer p.Stop(context.Background())

	st := p.Status()
	if !st.Running {
		t.Fatal("首次 Start 后应处于运行状态，但 Status().Running = false")
	}
	if p.server == nil {
		t.Fatal("首次 Start 后 p.server 为 nil，WS server 未创建")
	}
}

// TestStartIdempotent 已在运行时重复 Start 应返回 nil 且不替换现有 server。
func TestStartIdempotent(t *testing.T) {
	p := newTestAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("首次 Start 失败: %v", err)
	}
	defer p.Stop(context.Background())

	first := p.server
	if err := p.Start(ctx); err != nil {
		t.Fatalf("重复 Start 应返回 nil，实际: %v", err)
	}
	if p.server != first {
		t.Fatal("重复 Start 不应替换现有 server")
	}
}

// TestStopThenRestart Stop 后重新 Start 应能再次创建 server（SyncConfig 重启路径）。
func TestStopThenRestart(t *testing.T) {
	p := newTestAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		t.Fatalf("首次 Start 失败: %v", err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	if p.server != nil {
		t.Fatal("Stop 后 p.server 应置 nil")
	}

	if err := p.Start(ctx); err != nil {
		t.Fatalf("Stop 后重新 Start 失败: %v", err)
	}
	defer p.Stop(context.Background())
	if p.server == nil {
		t.Fatal("重新 Start 后 p.server 为 nil")
	}
	if !p.Status().Running {
		t.Fatal("重新 Start 后应处于运行状态")
	}
}
