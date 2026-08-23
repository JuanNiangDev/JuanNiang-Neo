package groupmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/ragtag"

	caller "JuanNiang-Neo/infrastructure/rag/handler"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestManager 构造测试 Manager：sqlite 内存库 + 可选 mock RAG server（score 可定制）。
func newTestManager(t *testing.T, ragScore *float64) (*Manager, *dao.GroupMgrDAO) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := core.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	gmdao := dao.NewBundle(db).GroupMgr
	if err := gmdao.InitConfig(context.Background()); err != nil {
		t.Fatalf("init config: %v", err)
	}

	var ragCli *caller.Client
	if ragScore != nil {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/tags/search" {
				http.NotFound(w, r)
				return
			}
			// 返回一个命中：tag = ragtag.Sample("1")（样本表首个自增 ID=1）
			_ = json.NewEncoder(w).Encode(caller.SearchResponse{Results: []caller.SearchHit{
				{Tag: ragtag.Sample("1"), Score: *ragScore},
			}})
		}))
		t.Cleanup(srv.Close)
		ragCli = &caller.Client{
			Config:     caller.Config{BaseURL: srv.URL, Timeout: 5 * time.Second},
			HttpClient: &http.Client{Timeout: 5 * time.Second},
		}
	}

	m := New(gmdao, adapter.New(adapter.Config{}), func() *caller.Client { return ragCli }, provider.NewProviderGroup())
	// 样本候选集：插入一条样本（ID=1，与 mock 命中 tag 对齐）
	if _, err := gmdao.SampleAdd(context.Background(), "办卡加群办套餐", "ad", "seed"); err != nil {
		t.Fatalf("seed sample: %v", err)
	}
	// Init：默认配置 + 种子词库导入 + 内存缓存加载
	if err := m.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	return m, gmdao
}

func TestViolationRAGUnavailableKeywordPath(t *testing.T) {
	m, _ := newTestManager(t, nil) // 无 RAG
	ctx := context.Background()

	// 黑词 → 高危复核（review）
	rep := m.TestViolation(ctx, "校园卡办卡免沸！低价流量卡，加裙114514")
	if rep.Verdict != "review" || rep.Word == "" {
		t.Fatalf("黑词应走 review，got verdict=%s word=%q", rep.Verdict, rep.Word)
	}
	// 无词 → 放行
	rep = m.TestViolation(ctx, "今天食堂的饭真好吃")
	if rep.Verdict != "pass" {
		t.Fatalf("无词应 pass，got %s", rep.Verdict)
	}
}

func TestViolationRAGHighScorePunish(t *testing.T) {
	score := 0.92
	m, _ := newTestManager(t, &score)
	rep := m.TestViolation(context.Background(), "0元购送福利，加我微信领流量卡")
	if !rep.RAGOK {
		t.Fatal("RAG 应可用")
	}
	if rep.Verdict != "punish" {
		t.Fatalf("高置信应 punish，got %s (%s)", rep.Verdict, rep.Reason)
	}
}

func TestViolationRAGMidScoreReview(t *testing.T) {
	score := 0.6
	m, _ := newTestManager(t, &score)
	rep := m.TestViolation(context.Background(), "低价流量卡办理")
	if rep.Verdict != "review" {
		t.Fatalf("模棱两可应 review，got %s (%s)", rep.Verdict, rep.Reason)
	}
}

func TestViolationRAGLowScoreNoSignalPass(t *testing.T) {
	score := 0.3
	m, _ := newTestManager(t, &score)
	rep := m.TestViolation(context.Background(), "明天要交作业了吗")
	if rep.Verdict != "pass" {
		t.Fatalf("低置信无词应 pass，got %s (%s)", rep.Verdict, rep.Reason)
	}
}

func TestViolationRAGLowScoreWithWordReview(t *testing.T) {
	score := 0.3
	m, _ := newTestManager(t, &score)
	rep := m.TestViolation(context.Background(), "校园卡办理，找我办卡")
	if rep.Verdict != "review" {
		t.Fatalf("低置信有词应 review，got %s (%s)", rep.Verdict, rep.Reason)
	}
}

func TestDAOFixtures(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	// 词条幂等：重复 upsert 不增加计数（种子已导入 2268 条）
	before, _ := gmdao.WordCount(ctx)
	if err := gmdao.WordUpsert(ctx, "校园卡", "gray", "import"); err != nil {
		t.Fatal(err)
	}
	if err := gmdao.WordUpsert(ctx, "校园卡", "gray", "import"); err != nil {
		t.Fatal(err)
	}
	after, _ := gmdao.WordCount(ctx)
	if after != before && after != before+1 {
		t.Fatalf("词条幂等失败，before=%d after=%d", before, after)
	}
	// 违规记录
	if err := gmdao.ViolationSet(ctx, 100, 200, 1); err != nil {
		t.Fatal(err)
	}
	c, _ := gmdao.ViolationGet(ctx, 100, 200)
	if c != 1 {
		t.Fatalf("违规次数 = %d", c)
	}
	if err := gmdao.ViolationSet(ctx, 100, 200, 0); err != nil {
		t.Fatal(err)
	}
	if c, _ = gmdao.ViolationGet(ctx, 100, 200); c != 0 {
		t.Fatalf("清零后违规次数 = %d", c)
	}
	// 统计
	if _, err := gmdao.StatIncr(ctx, "100:stats:warn"); err != nil {
		t.Fatal(err)
	}
	if v, _ := gmdao.StatGet(ctx, "100:stats:warn"); v != "1" {
		t.Fatalf("统计值 = %q", v)
	}
	// 词库热更新后命中（种子词库含办校园卡等黑词，任意类别命中即可）
	_ = m.Reload(ctx)
	hit, cat := m.wordHit(ctx, "帮我办校园卡")
	if hit == "" || cat == "" {
		t.Fatalf("词命中 = %q/%s", hit, cat)
	}
	_ = fmt.Sprint()
}
