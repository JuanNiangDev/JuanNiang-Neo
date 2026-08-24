package agent

import (
	"testing"

	"github.com/google/uuid"
)

// TestFilterRagHits 过滤非本集合命中并按分数降序（知识/记忆/群管理样本共用的过滤逻辑）。
func TestFilterRagHits(t *testing.T) {
	owned := map[uuid.UUID]string{
		uuid.MustParse("11111111-1111-1111-1111-111111111111"): "a",
		uuid.MustParse("22222222-2222-2222-2222-222222222222"): "b",
	}
	hits := []ragHitWithTag{
		{tag: uuid.MustParse("33333333-3333-3333-3333-333333333333"), score: 0.99}, // 非本集合
		{tag: uuid.MustParse("22222222-2222-2222-2222-222222222222"), score: 0.5},
		{tag: uuid.MustParse("11111111-1111-1111-1111-111111111111"), score: 0.9},
	}
	got := filterRagHits(hits, owned)
	if len(got) != 2 {
		t.Fatalf("应过滤出 2 条，got %d: %v", len(got), got)
	}
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("应按分数降序 a(0.9) > b(0.5)，got %v", got)
	}
}

// TestFilterRagHitsEmpty 无命中返回 nil。
func TestFilterRagHitsEmpty(t *testing.T) {
	owned := map[uuid.UUID]string{uuid.New(): "x"}
	hits := []ragHitWithTag{{tag: uuid.New(), score: 0.9}}
	if got := filterRagHits(hits, owned); got != nil {
		t.Fatalf("无本集合命中应返回 nil，got %v", got)
	}
	if got := filterRagHits(nil, owned); got != nil {
		t.Fatalf("空命中应返回 nil，got %v", got)
	}
}
