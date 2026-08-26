package groupmgr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	caller "JuanNiang-Neo/infrastructure/rag/handler"
)

// TestSyncRAG 手动全量同步向量库：词条+样本批量 upsert（幂等，逐批 50）。
func TestSyncRAG(t *testing.T) {
	// mock RAG-Service：/scoops/groupmgr/tags/batch 接收批量 upsert，统计请求次数与条数
	var mu sync.Mutex
	batchItems := 0
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/scoops/groupmgr/tags/batch") {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Items []caller.BatchItem `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode batch: %v", err)
		}
		mu.Lock()
		requests++
		batchItems += len(body.Items)
		mu.Unlock()
		resp := caller.BatchResponse{}
		for _, it := range body.Items {
			resp.Results = append(resp.Results, caller.BatchItemResponse{Tag: it.Tag, ChunkCount: 1})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()
	// 替换 getRAG 为 mock client（newTestManager 的 RAG 为 nil）
	ragCli := &caller.Client{
		Config:     caller.Config{BaseURL: srv.URL, Timeout: 5 * time.Second},
		HttpClient: &http.Client{Timeout: 5 * time.Second},
	}
	m.getRAG = func() *caller.Client { return ragCli }

	// 词条 + 样本各一条
	_, _ = gmdao.WordUpsert(ctx, "同步测试词", "gray", "import")
	_, _ = gmdao.SampleAdd(ctx, "同步测试样本", "ad", "learn")

	total, failed, err := m.SyncRAG(ctx)
	if err != nil {
		t.Fatalf("SyncRAG 失败: %v", err)
	}
	// 同步范围 = 全部词条（含 2268 种子）+ 样本，失败必须为 0
	if total < 2 || failed != 0 {
		t.Fatalf("同步失败数应为 0，got total=%d failed=%d", total, failed)
	}
	mu.Lock()
	defer mu.Unlock()
	if batchItems != total {
		t.Fatalf("mock 收到条数应与 total 一致，got mock=%d total=%d（请求数 %d）", batchItems, total, requests)
	}
}

// TestSyncRAGUnconfigured RAG 未配置时返回明确错误（Web 面板展示用）。
func TestSyncRAGUnconfigured(t *testing.T) {
	m, _ := newTestManager(t, nil) // getRAG 返回 nil
	_, _, err := m.SyncRAG(context.Background())
	if err == nil || !strings.Contains(err.Error(), "未配置") {
		t.Fatalf("RAG 未配置应返回明确错误，got %v", err)
	}
}
