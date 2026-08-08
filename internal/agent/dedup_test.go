package agent

import (
	"context"
	"sync"
	"testing"
	"time"
)

// 内存去重器测试（redisDedup 需真实 Redis，由集成测试覆盖）。

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
