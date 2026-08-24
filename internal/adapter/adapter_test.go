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

// TestGetGroupMemberInfoCached 群成员信息缓存：命中缓存不再打底层 API，失败走负缓存。
// 回归：isGroupAdmin 曾直接调 GetGroupMemberInfo（无缓存），每条非白名单群消息
// 都同步触发 OneBot11 请求；且退群/失败成员无负缓存，每条消息重试。
func TestGetGroupMemberInfoCached(t *testing.T) {
	p := newTestAdapter() // 未启动：底层 GetGroupMemberInfo 快速返回 "adapter 未启动"

	key := "100:200"
	// 预置一条未过期的正缓存记录：命中时应直接返回，不触发底层
	info := &GroupMemberInfo{Role: "admin"}
	p.memberInfoMu.Lock()
	p.memberInfoCache[key] = memberInfoCacheEntry{info: info, expiresAt: time.Now().Add(time.Hour)}
	p.memberInfoMu.Unlock()

	got, err := p.GetGroupMemberInfoCached(100, 200)
	if err != nil {
		t.Fatalf("命中缓存不应报错: %v", err)
	}
	if got != info {
		t.Fatalf("命中缓存应返回预置记录，got %+v", got)
	}
	p.memberInfoMu.RLock()
	if _, ok := p.memberInfoCache[key]; !ok {
		p.memberInfoMu.RUnlock()
		t.Fatal("正缓存命中后应保留缓存条目")
	}
	p.memberInfoMu.RUnlock()

	// 未命中 + 底层失败 → 负缓存（60s），再次调用不重复打底层
	_, err = p.GetGroupMemberInfoCached(100, 999)
	if err == nil {
		t.Fatal("adapter 未启动时查询应报错")
	}
	p.memberInfoMu.RLock()
	e, ok := p.memberInfoCache["100:999"]
	p.memberInfoMu.RUnlock()
	if !ok {
		t.Fatal("失败查询应写入负缓存")
	}
	if got := time.Until(e.expiresAt); got > memberInfoCacheTTL || got <= 0 {
		t.Fatalf("负缓存 TTL 应在 (0, %s]，实际 %s", memberInfoCacheTTL, got)
	}
	if e.info != nil {
		t.Fatalf("负缓存条目应为 nil info，got %+v", e.info)
	}

	// 再次调用同一 key：命中负缓存，快速返回同一错误，不新增底层调用
	_, err2 := p.GetGroupMemberInfoCached(100, 999)
	if err2 == nil {
		t.Fatal("负缓存命中后仍应返回错误")
	}
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
