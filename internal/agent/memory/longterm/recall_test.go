package longterm

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"JuanNiang-Neo/internal/core"
	"JuanNiang-Neo/internal/core/dao"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestLongTerm 用 SQLite 内存库构造真实 DAO 的 LongTermMemory。
// 注意：SQLite 下 DAO.SemanticSearch 自动回退单关键词 ILIKE——
// 整段消息作为关键词几乎不命中，天然制造"语义候选为空"场景，
// 正好验证 Recall 的空候选回退路径（与 PG 生产行为一致）。
func newTestLongTerm(t *testing.T, mode RecallMode) (*LongTermMemory, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := core.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	bundle := dao.NewBundle(db)
	lt := New(Config{HotAreaSize: 2, RecallMode: mode}, bundle.LongTermMemItem)
	return lt, db
}

func TestRecallSemanticEmptyCandidateFallsBackToRecent(t *testing.T) {
	lt, _ := newTestLongTerm(t, RecallModeSemantic)
	ctx := context.Background()

	// 写入两条记忆（最近在后，回退路径按 created_at DESC 取最新）
	lt.Add(ctx, "area1", "旧记忆：用户喜欢喝咖啡")
	lt.Add(ctx, "area1", "新记忆：用户是 Go 开发者")

	// 语义候选为空（SQLite 环境整段 ILIKE 必不命中）→ 应回退最近条目
	items, err := lt.Recall(ctx, "area1", []string{"量子力学"}, "量子力学讲座", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("空候选应回退最近条目，实际为空")
	}
	if items[0].Content != "新记忆：用户是 Go 开发者" {
		t.Fatalf("应优先回退最新条目，实际: %q", items[0].Content)
	}
}

func TestRecallRecentModeSkipsSemantic(t *testing.T) {
	lt, _ := newTestLongTerm(t, RecallModeRecent)
	ctx := context.Background()

	lt.Add(ctx, "area1", "记忆A")
	lt.Add(ctx, "area1", "记忆B")

	// recent 模式：即使给了 gram 也只走最近路径
	items, err := lt.Recall(ctx, "area1", []string{"记忆A"}, "记忆A", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("recent 模式应返回最近 2 条，实际 %d 条", len(items))
	}
}

func TestRecallNoGramsFallsBackToRecent(t *testing.T) {
	lt, _ := newTestLongTerm(t, RecallModeSemantic)
	ctx := context.Background()

	lt.Add(ctx, "area1", "纯表情聊天的上下文")
	lt.Add(ctx, "area1", "第二条")

	// gram 为空（短消息/纯表情）→ 直接最近路径
	items, err := lt.Recall(ctx, "area1", nil, "", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("gram 为空应回退最近，实际 %d 条", len(items))
	}
	if items[0].Content != "第二条" {
		t.Fatalf("最新条目应在前，实际: %q", items[0].Content)
	}
}

func TestRecallGramHitUsesSemanticPath(t *testing.T) {
	// SQLite 语义回退是整段 ILIKE：消息很短且与内容一致时可命中，
	// 验证语义路径确实返回了命中的条目（而非最近）。
	lt, _ := newTestLongTerm(t, RecallModeSemantic)
	ctx := context.Background()

	lt.Add(ctx, "area1", "用户喜欢摸鱼")
	lt.Add(ctx, "area1", "最新无关条目")

	items, err := lt.Recall(ctx, "area1", []string{"摸鱼"}, "摸鱼", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("短查询应命中单条，实际 %d 条: %v", len(items), items)
	}
	if items[0].Content != "用户喜欢摸鱼" {
		t.Fatalf("应命中语义相关条目，实际: %q", items[0].Content)
	}
}

// TestRecallSemanticHitMarksRecalled 回归：语义召回命中路径也必须刷新 last_recalled_at，
// 否则 GC 按最近召回时间误判"长期未召回"并清理刚被召回的条目。
func TestRecallSemanticHitMarksRecalled(t *testing.T) {
	lt, db := newTestLongTerm(t, RecallModeSemantic)
	ctx := context.Background()

	item, err := lt.Add(ctx, "area1", "用户喜欢摸鱼")
	if err != nil {
		t.Fatal(err)
	}
	// 初始 last_recalled_at 为空（从未召回）
	var raw struct{ LastRecalledAt *time.Time }
	if err := db.Raw("SELECT last_recalled_at FROM long_term_memory_items WHERE id = ?", item.ID).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	if raw.LastRecalledAt != nil {
		t.Fatalf("新条目 last_recalled_at 应为 NULL，got %v", raw.LastRecalledAt)
	}

	// 语义路径命中（SQLite 回退整段 ILIKE，短查询可命中）
	if _, err := lt.Recall(ctx, "area1", []string{"摸鱼"}, "摸鱼", 5); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if err := db.Raw("SELECT last_recalled_at FROM long_term_memory_items WHERE id = ?", item.ID).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	if raw.LastRecalledAt == nil {
		t.Fatal("语义召回命中后 last_recalled_at 应被更新（防 GC 误清理）")
	}
}
