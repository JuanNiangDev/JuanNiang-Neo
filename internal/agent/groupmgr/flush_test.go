package groupmgr

import (
	"context"
	"strings"
	"testing"
	"time"

	"JuanNiang-Neo/internal/agent/provider"
)

// mockLLMProvider 实现 provider.Provider 的测试替身：Chat 固定返回 black 裁决 JSON，
// 便于验证批量审查链路（submitReview → flushBatch → handleReviewBatch → 学习闭环）。
type mockLLMProvider struct{}

func (mockLLMProvider) ID() string               { return "mock-llm" }
func (mockLLMProvider) Name() string             { return "Mock LLM" }
func (mockLLMProvider) Type() provider.ModelType { return provider.ModelTypeText }
func (mockLLMProvider) Model() string            { return "mock-model" }
func (mockLLMProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	return &provider.ChatResponse{
		Message: provider.ChatMessage{
			Role:    "assistant",
			Content: `{"results":[{"index":0,"verdict":"black","reason":"mock 判定广告"}]}`,
		},
	}, nil
}
func (mockLLMProvider) ChatStream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatStreamChunk, error) {
	ch := make(chan provider.ChatStreamChunk)
	close(ch)
	return ch, nil
}
func (mockLLMProvider) Vision(ctx context.Context, imageData []byte, prompt string) (string, error) {
	return "", nil
}

// TestFlushBatchAsyncFullTrigger 回归：满批触发 flushBatch 必须异步——submitReview 立即返回
// （此前同步调用会阻塞消息处理主循环至 LLM 返回，最长 90s），裁决经 channel 串行消费。
func TestFlushBatchAsyncFullTrigger(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()
	cfg, _ := gmdao.GetConfig(ctx)
	cfg.LLMReview = true
	_ = gmdao.UpdateConfig(ctx, cfg)
	m.providers.AddProvider(mockLLMProvider{})

	// 填满批队列（llmBatchMax=20 条），最后一条触发满批异步 flush
	start := time.Now()
	for i := 0; i < llmBatchMax; i++ {
		ev := groupEv(int64(100+i), int64(200+i), "低价流量卡办理")
		ev.Message.MessageID = int64(1000 + i) // 唯一，避开 llmReviewed 去重
		if !m.submitReview(ctx, ev, reviewCtx{text: "低价流量卡办理"}) {
			t.Fatalf("第 %d 条 submitReview 应入批", i)
		}
	}
	// 满批触发为异步：submitReview 必须很快返回（不阻塞至 LLM 返回）
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("满批触发不应同步阻塞，耗时 %v", elapsed)
	}
	// 裁决异步到达 llmResults 通道（mock Chat 立即返回）
	select {
	case out := <-m.llmResults:
		if len(out.items) != llmBatchMax {
			t.Fatalf("批应含 %d 条，got %d", llmBatchMax, len(out.items))
		}
		if out.err != nil {
			t.Fatalf("批裁决不应报错，got %v", out.err)
		}
		// 串行消费：handleReviewBatch → punish + 学习闭环异步写入黑名单语录
		m.handleReviewBatch(ctx, out)
	case <-time.After(5 * time.Second):
		t.Fatal("批裁决应在超时内到达 llmResults")
	}
	// 学习闭环异步写库：轮询等待黑名单语录出现
	deadline := time.Now().Add(3 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		samples, _ := gmdao.SampleListByList(ctx, "black")
		for _, s := range samples {
			if strings.Contains(s.Text, "流量卡") {
				found = true
				break
			}
		}
		if found {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !found {
		t.Error("LLM 判 black 后应异步写入黑名单语录（学习闭环）")
	}
	// 违规记录应落库（handleReviewBatch 内 punish 已执行）
	if c, _ := gmdao.ViolationGet(ctx, 100, 200); c < 1 {
		t.Fatalf("批裁决 black 后违规计数应 ≥1，got %d", c)
	}
}
