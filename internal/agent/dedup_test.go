package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// 内存去重器测试。

func TestMemoryDedupSeenBefore(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDedup(time.Minute)

	if d.SeenBefore(ctx, "group:10086") {
		t.Fatal("首次出现不应判为重复")
	}
	if !d.SeenBefore(ctx, "group:10086") {
		t.Fatal("去重窗口内再次出现应判为重复")
	}
}

func TestMemoryDedupDistinctKeys(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDedup(time.Minute)

	// 不同 message_id 互不影响
	if d.SeenBefore(ctx, "group:10086") || d.SeenBefore(ctx, "group:10087") {
		t.Fatal("不同 message_id 不应互斥")
	}
	// 群/私聊 message_id 空间独立，同号不互扰
	if d.SeenBefore(ctx, "private:10086") {
		t.Fatal("私聊与群聊的同号 message_id 不应互斥")
	}
	// 各自第二次出现才算重复
	if !d.SeenBefore(ctx, "group:10086") || !d.SeenBefore(ctx, "private:10086") {
		t.Fatal("各自的第二次出现应判为重复")
	}
}

func TestMemoryDedupExpiry(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDedup(50 * time.Millisecond)

	if d.SeenBefore(ctx, "group:10086") {
		t.Fatal("首次出现不应判为重复")
	}
	time.Sleep(80 * time.Millisecond)
	if d.SeenBefore(ctx, "group:10086") {
		t.Fatal("TTL 过期后应可再次处理")
	}
}

func TestMemoryDedupConcurrent(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDedup(time.Minute)

	const workers = 16
	var wg sync.WaitGroup
	// 并发投递同一 key：仅一个应返回 false（放行），其余全部判重
	first := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			first <- !d.SeenBefore(ctx, "group:10086")
		}()
	}
	wg.Wait()
	close(first)

	allowed := 0
	for v := range first {
		if v {
			allowed++
		}
	}
	if allowed != 1 {
		t.Fatalf("并发投递同一 key 应有且仅有 1 个放行，实际 %d", allowed)
	}
}

// TestDeduperInterface 编译期断言：memoryDedup / redisDedup 都实现 Deduper 接口。
func TestDeduperInterface(t *testing.T) {
	var _ Deduper = (*memoryDedup)(nil)
	var _ Deduper = (*redisDedup)(nil)
}

// ---------- redisDedup 测试 ----------
//
// 用 fakeDedupStore 模拟 Redis SetNX 行为，避免引入 miniredis 等外部依赖。
// fakeDedupStore 维护内存 map 模拟"已存在"判断，可注入错误验证降级路径。

type fakeDedupStore struct {
	mu      sync.Mutex
	entries map[string]struct{}
	err     error // 非 nil 时所有 SetNX 调用返回此错误
}

func newFakeDedupStore() *fakeDedupStore {
	return &fakeDedupStore{entries: make(map[string]struct{})}
}

func (f *fakeDedupStore) SetNX(ctx context.Context, key string, val any, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	if _, exists := f.entries[key]; exists {
		return false, nil
	}
	f.entries[key] = struct{}{}
	return true, nil
}

// TestRedisDedup_FirstSeenPasses 首次出现的 key 应放行（SeenBefore 返回 false）。
func TestRedisDedup_FirstSeenPasses(t *testing.T) {
	ctx := context.Background()
	d := newRedisDedup(newFakeDedupStore(), time.Minute)
	if d.SeenBefore(ctx, "group:10086") {
		t.Error("首次出现不应判为重复")
	}
}

// TestRedisDedup_SecondSeenBlocked 同一 key 第二次出现应拦截（SeenBefore 返回 true）。
func TestRedisDedup_SecondSeenBlocked(t *testing.T) {
	ctx := context.Background()
	d := newRedisDedup(newFakeDedupStore(), time.Minute)
	d.SeenBefore(ctx, "group:10086")
	if !d.SeenBefore(ctx, "group:10086") {
		t.Error("去重窗口内再次出现应判为重复")
	}
}

// TestRedisDedup_RedisErrorDegrades Redis 调用失败时应降级放行（SeenBefore 返回 false），
// 避免去重器故障导致 Agent 整体停摆——下游 Layer 2（短期记忆 Lua 幂等）仍能兜住部分重复。
func TestRedisDedup_RedisErrorDegrades(t *testing.T) {
	ctx := context.Background()
	store := newFakeDedupStore()
	store.err = errors.New("redis down")
	d := newRedisDedup(store, time.Minute)
	if d.SeenBefore(ctx, "group:10086") {
		t.Error("Redis 错误时应降级放行，而非阻塞")
	}
}

// TestRedisDedup_DifferentKeysIndependent 不同 key 互不影响。
func TestRedisDedup_DifferentKeysIndependent(t *testing.T) {
	ctx := context.Background()
	d := newRedisDedup(newFakeDedupStore(), time.Minute)
	d.SeenBefore(ctx, "group:10086")
	if d.SeenBefore(ctx, "group:10087") {
		t.Error("不同 message_id 不应互斥")
	}
	if d.SeenBefore(ctx, "private:10086") {
		t.Error("私聊与群聊的同号 message_id 不应互斥")
	}
}
