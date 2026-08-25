package groupmgr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	caller "JuanNiang-Neo/infrastructure/rag/handler"
)

func TestWhiteGCRemovesUnusedOnly(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	// 3 条白名单语录：2 条从未命中（将删除）+ 1 条最近命中（保留）
	ids := make([]uint, 0, 3)
	for _, text := range []string{"明天一起吃饭吗", "周末去爬山吗", "新版本出了吗"} {
		id, err := gmdao.SampleAddPhrase(ctx, text, "ok", "seed", "white")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	// 第 3 条最近命中（LastUsedAt=now，应保留）
	if err := gmdao.SampleTouch(ctx, ids[2]); err != nil {
		t.Fatal(err)
	}

	removed := mustRunWhiteGC(m, ctx, 1) // 周期 1 天：删除 1 天前未命中的（前 2 条）
	if removed != 2 {
		t.Fatalf("应删除 2 条未使用语录，got %d", removed)
	}
	list, err := gmdao.SampleListByList(ctx, "white")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != ids[2] {
		t.Fatalf("应仅保留最近命中语录，got %v", list)
	}
}

func TestWhiteGCRespectsPageSizeAndNoop(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	// 8 条未使用白名单语录：单次最多清理 5 条
	for i := 0; i < 8; i++ {
		if _, err := gmdao.SampleAddPhrase(ctx, "常见问题示例"+string(rune('a'+i)), "ok", "seed", "white"); err != nil {
			t.Fatal(err)
		}
	}
	if removed := mustRunWhiteGC(m, ctx, 1); removed != 5 {
		t.Fatalf("单次 GC 应清理 5 条，got %d", removed)
	}
	// 剩余 3 条仍从未命中，二次执行继续清理，直到删完
	if removed := mustRunWhiteGC(m, ctx, 1); removed != 3 {
		t.Fatalf("二次 GC 应清理剩余 3 条，got %d", removed)
	}
	// 全部已清理后再次执行：无操作不报错
	if removed := mustRunWhiteGC(m, ctx, 1); removed != 0 {
		t.Fatalf("无未使用语录时应清理 0 条，got %d", removed)
	}
}

func TestWhiteGCSkipsBlackPhrases(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	// 黑名单语录未命中 → 不应被白名单 GC 清理
	if _, err := gmdao.SampleAdd(ctx, "办卡加群", "ad", "seed"); err != nil {
		t.Fatal(err)
	}
	if removed := mustRunWhiteGC(m, ctx, 1); removed != 0 {
		t.Fatalf("白名单 GC 不应清理黑名单语录，got %d", removed)
	}
}

// TestWhiteGCRAGDeleteFailureKeepsRow 回归：RAG 向量删除失败时不得删 PG 行，
// 并标记 rag_synced=false 供下次 GC 重试（先删向量成功再删主库，防孤儿向量）。
func TestWhiteGCRAGDeleteFailureKeepsRow(t *testing.T) {
	// mock RAG-Service：DELETE 一律 500（向量删除失败）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			http.Error(w, "simulated failure", http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m, gmdao := newTestManager(t, nil)
	ragCli := &caller.Client{
		Config:     caller.Config{BaseURL: srv.URL, Timeout: 5 * time.Second},
		HttpClient: &http.Client{Timeout: 5 * time.Second},
	}
	m.getRAG = func() *caller.Client { return ragCli }
	ctx := context.Background()

	id, err := gmdao.SampleAddPhrase(ctx, "从未命中的白名单语录", "ok", "seed", "white")
	if err != nil {
		t.Fatal(err)
	}

	if removed := mustRunWhiteGC(m, ctx, 1); removed != 0 {
		t.Fatalf("RAG 删除失败时不应删除 PG 行，got removed=%d", removed)
	}
	// PG 行保留且标记未同步（下次 GC 重试）
	list, err := gmdao.SampleListByList(ctx, "white")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != id {
		t.Fatalf("PG 行应保留，got %v", list)
	}
	if list[0].RAGSynced {
		t.Fatalf("RAG 删除失败后应标记 rag_synced=false，got true")
	}
}

// TestWhiteGCRAGDeleteOKRemovesRow RAG 删除成功后正常删 PG 行（对照：向量删除成功不阻碍主库清理）。
func TestWhiteGCRAGDeleteOKRemovesRow(t *testing.T) {
	// mock RAG-Service：DELETE 返回 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m, gmdao := newTestManager(t, nil)
	ragCli := &caller.Client{
		Config:     caller.Config{BaseURL: srv.URL, Timeout: 5 * time.Second},
		HttpClient: &http.Client{Timeout: 5 * time.Second},
	}
	m.getRAG = func() *caller.Client { return ragCli }
	ctx := context.Background()

	if _, err := gmdao.SampleAddPhrase(ctx, "从未命中的白名单语录", "ok", "seed", "white"); err != nil {
		t.Fatal(err)
	}
	if removed := mustRunWhiteGC(m, ctx, 1); removed != 1 {
		t.Fatalf("RAG 删除成功应删除 PG 行，got removed=%d", removed)
	}
}

func mustRunWhiteGC(m *Manager, ctx context.Context, days int) int {
	return mustRunWhiteGCWithDB(m, ctx, days)
}

func mustRunWhiteGCWithDB(m *Manager, ctx context.Context, days int) int {
	cfg, _ := m.dao.GetConfig(ctx)
	cfg.WhiteGCIntervalDays = days
	_ = m.dao.UpdateConfig(ctx, cfg)
	before, _ := m.dao.SampleCountByList(ctx, "white")
	if err := m.runWhiteGC(ctx, days); err != nil {
		panic(err)
	}
	after, _ := m.dao.SampleCountByList(ctx, "white")
	return int(before - after)
}
