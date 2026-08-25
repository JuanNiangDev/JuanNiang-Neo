package groupmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
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
// 默认 mock 命中黑名单语录（ragtag.Sample）；white=true 时命中白名单语录（ragtag.WhitePhrase）。
func newTestManager(t *testing.T, ragScore *float64) (*Manager, *dao.GroupMgrDAO) {
	return newTestManagerEx(t, ragScore, false)
}

func newTestManagerEx(t *testing.T, ragScore *float64, white bool) (*Manager, *dao.GroupMgrDAO) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// :memory: 库每连接独立：固定单连接（含空闲池），保证并发测试共享同一库
	if sqlDB, derr := db.DB(); derr == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
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
			// 返回一个命中：tag 与下方插入的语录行（首个自增 ID=1）对齐
			tag := ragtag.Sample("1")
			if white {
				tag = ragtag.WhitePhrase("1")
			}
			_ = json.NewEncoder(w).Encode(caller.SearchResponse{Results: []caller.SearchHit{
				{Tag: tag, Score: *ragScore},
			}})
		}))
		t.Cleanup(srv.Close)
		ragCli = &caller.Client{
			Config:     caller.Config{BaseURL: srv.URL, Timeout: 5 * time.Second},
			HttpClient: &http.Client{Timeout: 5 * time.Second},
		}
	}

	m := New(gmdao, adapter.New(adapter.Config{}), func() *caller.Client { return ragCli }, provider.NewProviderGroup())
	// 语录候选集：插入一条语录（ID=1，与 mock 命中 tag 对齐）
	if white {
		if _, err := gmdao.SampleAddPhrase(context.Background(), "明天一起食堂吃饭吗", "ok", "seed", "white"); err != nil {
			t.Fatalf("seed white phrase: %v", err)
		}
	} else if _, err := gmdao.SampleAdd(context.Background(), "办卡加群办套餐", "ad", "seed"); err != nil {
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
		t.Fatalf("黑名单高置信应 punish，got %s (%s)", rep.Verdict, rep.Reason)
	}
	if rep.BlackScore < 0.9 {
		t.Fatalf("black_score 应上报最高分，got %f", rep.BlackScore)
	}
}

func TestViolationRAGMidScoreReview(t *testing.T) {
	score := 0.6
	m, _ := newTestManager(t, &score)
	rep := m.TestViolation(context.Background(), "低价流量卡办理")
	if !rep.RAGOK {
		t.Fatal("RAG 应可用")
	}
	if rep.Verdict != "review" {
		t.Fatalf("未达黑白阈值应 review（LLM 判定），got %s (%s)", rep.Verdict, rep.Reason)
	}
}

func TestViolationRAGLowScoreReview(t *testing.T) {
	score := 0.3
	m, _ := newTestManager(t, &score)
	rep := m.TestViolation(context.Background(), "明天要交作业了吗")
	if rep.Verdict != "review" {
		t.Fatalf("低分未命中黑白 → LLM 判定，got %s (%s)", rep.Verdict, rep.Reason)
	}
}

func TestViolationRAGWhitePhrasePass(t *testing.T) {
	score := 0.92
	m, _ := newTestManagerEx(t, &score, true) // mock 命中白名单语录
	rep := m.TestViolation(context.Background(), "明天一起食堂吃饭吗")
	if !rep.RAGOK {
		t.Fatal("RAG 应可用")
	}
	if rep.WhiteScore < 0.9 {
		t.Fatalf("white_score 应上报最高分，got %f", rep.WhiteScore)
	}
	if rep.Verdict != "pass" {
		t.Fatalf("白名单高置信应放行，got %s (%s)", rep.Verdict, rep.Reason)
	}
}

// TestViolationIncrConcurrent 违规计数原子自增：并发 N 次自增最终计数 = N。
// 回归：punish 曾 ViolationGet → count++ → ViolationSet 非原子，事件循环与
// Run 循环双 goroutine 竞争会丢计数（双重处罚/错档惩罚）。
func TestViolationIncrConcurrent(t *testing.T) {
	_, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = gmdao.ViolationIncr(ctx, 100, 200, dao.ViolationMeta{Username: "并发", DetectionPath: "keyword", LLMReason: "r"})
		}()
	}
	wg.Wait()

	c, err := gmdao.ViolationGet(ctx, 100, 200)
	if err != nil {
		t.Fatal(err)
	}
	if c != n {
		t.Fatalf("并发自增后计数应为 %d，实际 %d（read-modify-write 丢计数）", n, c)
	}
	// 现场信息为最后一次处罚覆盖
	list, _ := gmdao.ViolationList(ctx)
	if len(list) != 1 {
		t.Fatalf("违规记录应 1 条，got %d", len(list))
	}
	if list[0].Count != n {
		t.Fatalf("DB 内 count 应为 %d，got %d", n, list[0].Count)
	}
}

// TestPunishTiersConcurrent 双 goroutine 并发处罚：同一用户两条违规消息同时处理，
// 最终 count 精确为 2（ViolationIncr 单条 UPSERT 原子自增不丢级）；-race 下同时验证无数据竞争。
func TestPunishTiersConcurrent(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()
	ev := groupEv(100, 200, "广告")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		m.punish(ev, "广告违规：并发路径1", "ad", "keyword")
	}()
	go func() {
		defer wg.Done()
		m.punish(ev, "广告违规：并发路径2", "ad", "llm")
	}()
	wg.Wait()

	c, err := gmdao.ViolationGet(ctx, 100, 200)
	if err != nil {
		t.Fatal(err)
	}
	if c != 2 {
		t.Fatalf("并发处罚后 count 应为 2，实际 %d（read-modify-write 丢级）", c)
	}
}

func TestDAOFixtures(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	// 词条幂等：重复 upsert 不增加计数（种子已导入 2268 条）
	before, _ := gmdao.WordCount(ctx)
	if _, err := gmdao.WordUpsert(ctx, "校园卡", "gray", "import"); err != nil {
		t.Fatal(err)
	}
	if _, err := gmdao.WordUpsert(ctx, "校园卡", "gray", "import"); err != nil {
		t.Fatal(err)
	}
	after, _ := gmdao.WordCount(ctx)
	if after != before && after != before+1 {
		t.Fatalf("词条幂等失败，before=%d after=%d", before, after)
	}
	// 违规记录
	if err := gmdao.ViolationSet(ctx, 100, 200, 1, dao.ViolationMeta{}); err != nil {
		t.Fatal(err)
	}
	c, _ := gmdao.ViolationGet(ctx, 100, 200)
	if c != 1 {
		t.Fatalf("违规次数 = %d", c)
	}
	if err := gmdao.ViolationSet(ctx, 100, 200, 0, dao.ViolationMeta{}); err != nil {
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
