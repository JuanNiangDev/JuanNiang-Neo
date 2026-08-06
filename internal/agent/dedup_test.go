package agent

import (
	"sync"
	"testing"
	"time"
)

func TestMsgDedupSeenBefore(t *testing.T) {
	d := newMsgDedup(time.Minute)

	if d.seenBefore("group:10086") {
		t.Fatal("首次出现不应判为重复")
	}
	if !d.seenBefore("group:10086") {
		t.Fatal("去重窗口内再次出现应判为重复")
	}
}

func TestMsgDedupDistinctKeys(t *testing.T) {
	d := newMsgDedup(time.Minute)

	// 不同 message_id 互不影响
	if d.seenBefore("group:10086") || d.seenBefore("group:10087") {
		t.Fatal("不同 message_id 不应互斥")
	}
	// 群/私聊 message_id 空间独立，同号不互扰
	if d.seenBefore("private:10086") {
		t.Fatal("私聊与群聊的同号 message_id 不应互斥")
	}
	// 各自第二次出现才算重复
	if !d.seenBefore("group:10086") || !d.seenBefore("private:10086") {
		t.Fatal("各自的第二次出现应判为重复")
	}
}

func TestMsgDedupExpiry(t *testing.T) {
	d := newMsgDedup(50 * time.Millisecond)

	if d.seenBefore("group:10086") {
		t.Fatal("首次出现不应判为重复")
	}
	time.Sleep(80 * time.Millisecond)
	if d.seenBefore("group:10086") {
		t.Fatal("TTL 过期后应可再次处理")
	}
}

func TestMsgDedupConcurrent(t *testing.T) {
	d := newMsgDedup(time.Minute)

	const workers = 16
	var wg sync.WaitGroup
	// 并发投递同一 key：仅一个应返回 false（放行），其余全部判重
	first := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			first <- !d.seenBefore("group:10086")
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
