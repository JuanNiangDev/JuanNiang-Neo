package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"JuanNiang-Neo/internal/agent/memory"
	"JuanNiang-Neo/internal/core"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	caller "JuanNiang-Neo/infrastructure/rag/handler"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newGCTestHago 构造最小 HagoCenter（真实 DAO + 可注入 RAG client），供长期记忆 GC 测试。
func newGCTestHago(t *testing.T, ragURL string) (*HagoCenter, *dao.Bundle, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := core.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	bundle := dao.NewBundle(db)
	h := NewHagoCenter()
	h.DAO = bundle
	h.Memory = memory.NewMemoryGroup(nil, nil, nil)
	if ragURL != "" {
		cli := &caller.Client{
			Config:     caller.Config{BaseURL: ragURL, Timeout: 5 * time.Second},
			HttpClient: &http.Client{Timeout: 5 * time.Second},
		}
		h.RAGClient.Store(cli)
	}
	return h, bundle, db
}

func seedUnusedMemory(t *testing.T, bundle *dao.Bundle, id string) {
	t.Helper()
	if err := bundle.LongTermMemItem.Create(context.Background(), &models.LongTermMemoryItem{
		ID: id, ChatAreaID: "area1", Content: "未被召回的长期记忆",
	}); err != nil {
		t.Fatal(err)
	}
}

func countMemoryRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&models.LongTermMemoryItem{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// TestLongTermMemoryGCRAGDeleteFailureKeepsRow 回归：RAG 向量删除失败时不得删 PG 行，
// 并标记 rag_synced=false 供下次 GC 重试（先删向量成功再删主库，防孤儿向量）。
func TestLongTermMemoryGCRAGDeleteFailureKeepsRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			http.Error(w, "simulated failure", http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	h, bundle, db := newGCTestHago(t, srv.URL)
	ctx := context.Background()
	const id = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	seedUnusedMemory(t, bundle, id)

	if err := h.runLongTermMemoryGC(ctx, 1); err != nil {
		t.Fatalf("runLongTermMemoryGC: %v", err)
	}
	if n := countMemoryRows(t, db); n != 1 {
		t.Fatalf("RAG 删除失败时 PG 行应保留，got %d 行", n)
	}
	var item models.LongTermMemoryItem
	if err := db.Where("id = ?", id).First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.RAGSynced {
		t.Fatal("RAG 删除失败后应标记 rag_synced=false")
	}
}

// TestLongTermMemoryGCRAGDeleteOKRemovesRow 对照：RAG 向量删除成功时正常删 PG 行。
func TestLongTermMemoryGCRAGDeleteOKRemovesRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	h, bundle, db := newGCTestHago(t, srv.URL)
	ctx := context.Background()
	seedUnusedMemory(t, bundle, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	if err := h.runLongTermMemoryGC(ctx, 1); err != nil {
		t.Fatalf("runLongTermMemoryGC: %v", err)
	}
	if n := countMemoryRows(t, db); n != 0 {
		t.Fatalf("RAG 删除成功应删除 PG 行，got %d 行", n)
	}
}

// TestLongTermMemoryGCRAGNilRemovesRow RAG 未配置（client 为 nil）时直接删 PG 行（无向量可删）。
func TestLongTermMemoryGCRAGNilRemovesRow(t *testing.T) {
	h, bundle, db := newGCTestHago(t, "")
	ctx := context.Background()
	seedUnusedMemory(t, bundle, "cccccccc-cccc-cccc-cccc-cccccccccccc")

	if err := h.runLongTermMemoryGC(ctx, 1); err != nil {
		t.Fatalf("runLongTermMemoryGC: %v", err)
	}
	if n := countMemoryRows(t, db); n != 0 {
		t.Fatalf("RAG 未配置应直接删除 PG 行，got %d 行", n)
	}
}

// TestLongTermMemoryGCRAGNilKeepsSyncedRow 回归：RAG 不可用（client 为 nil）时，
// 已同步条目不得删 PG 行（向量仍在 RAG 库，待 RAG 恢复后先删向量再删主库，防孤儿向量）。
func TestLongTermMemoryGCRAGNilKeepsSyncedRow(t *testing.T) {
	h, bundle, db := newGCTestHago(t, "")
	ctx := context.Background()
	const id = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	seedUnusedMemory(t, bundle, id)
	if err := bundle.LongTermMemItem.MarkRAGSynced(ctx, id, true); err != nil {
		t.Fatal(err)
	}

	if err := h.runLongTermMemoryGC(ctx, 1); err != nil {
		t.Fatalf("runLongTermMemoryGC: %v", err)
	}
	if n := countMemoryRows(t, db); n != 1 {
		t.Fatalf("RAG 不可用时已同步条目应保留，got %d 行", n)
	}
	var item models.LongTermMemoryItem
	if err := db.Where("id = ?", id).First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if !item.RAGSynced {
		t.Fatal("保留的条目应保持 rag_synced=true（向量仍存在，待 RAG 恢复后重试删除）")
	}
}
