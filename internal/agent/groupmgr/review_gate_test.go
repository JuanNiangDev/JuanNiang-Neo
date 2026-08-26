package groupmgr

import (
	"context"
	"testing"
	"time"
)

// TestReviewGate 发送前审核闸门状态机：
// 无记录放行 / 已判 black 拦截 / 在途 pending / WaitReview 超时放行。
func TestReviewGate(t *testing.T) {
	m, _ := newTestManager(t, nil)
	ctx := context.Background()

	// 1. 无任何记录（未送审）→ 放行
	if b, p := m.ReviewGate(ctx, 100, 200, 999); b || p {
		t.Fatalf("无记录应放行，got blocked=%v pending=%v", b, p)
	}

	// 2. 已判 black → 拦截（不再 pending）
	m.llmMu.Lock()
	m.reviewVerdict[123] = "black"
	m.llmMu.Unlock()
	if b, p := m.ReviewGate(ctx, 100, 200, 123); !b || p {
		t.Fatalf("black 终态应 blocked，got blocked=%v pending=%v", b, p)
	}

	// 3. 已判 white/none → 放行
	m.llmMu.Lock()
	m.reviewVerdict[124] = "white"
	m.llmMu.Unlock()
	if b, p := m.ReviewGate(ctx, 100, 200, 124); b || p {
		t.Fatalf("white 终态应放行，got blocked=%v pending=%v", b, p)
	}

	// 4. 在途（批窗口/LLM 判断中）→ pending
	m.llmMu.Lock()
	m.llmPending[pkOf(100, 200)] = true
	m.llmMu.Unlock()
	if b, p := m.ReviewGate(ctx, 100, 200, 456); b || !p {
		t.Fatalf("在途应 pending，got blocked=%v pending=%v", b, p)
	}

	// 5. WaitReview 超时（在途一直无终态）→ 放行，且确实等待了一段时间
	start := time.Now()
	if b := m.WaitReview(ctx, 100, 200, 456, 250*time.Millisecond); b {
		t.Fatal("超时应按放行处理")
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("WaitReview 应轮询等待，实际仅 %v", elapsed)
	}

	// 6. 在途期间终态到达 → WaitReview 提前返回 blocked（保持 pending 状态，
	//    100ms 后终态写入；ReviewGate 优先返回 verdict）
	go func() {
		time.Sleep(100 * time.Millisecond)
		m.llmMu.Lock()
		m.reviewVerdict[456] = "black"
		m.llmMu.Unlock()
	}()
	if !m.WaitReview(ctx, 100, 200, 456, 2*time.Second) {
		t.Fatal("终态 black 到达后 WaitReview 应返回 blocked")
	}

	// 7. 非法 messageID / groupID → 直接放行
	if b, p := m.ReviewGate(ctx, 0, 200, 1); b || p {
		t.Fatalf("非法 groupID 应放行，got blocked=%v pending=%v", b, p)
	}
	if b, p := m.ReviewGate(ctx, 100, 200, 0); b || p {
		t.Fatalf("非法 messageID 应放行，got blocked=%v pending=%v", b, p)
	}
}
