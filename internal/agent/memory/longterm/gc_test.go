package longterm

import (
	"context"
	"testing"
	"time"

	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
)

// TestSearchMarksRecalled 召回命中应更新 last_recalled_at（GC 判定未使用记忆用）。
func TestSearchMarksRecalled(t *testing.T) {
	lt, db := newTestLongTerm(t, RecallModeRecent)
	ctx := context.Background()

	item, err := lt.Add(ctx, "area1", "记忆：用户喜欢喝冰美式")
	if err != nil {
		t.Fatal(err)
	}
	// 初始 last_recalled_at 为空
	var raw struct{ LastRecalledAt *time.Time }
	if err := db.Raw("SELECT last_recalled_at FROM long_term_memory_items WHERE id = ?", item.ID).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	if raw.LastRecalledAt != nil {
		t.Fatalf("新条目 last_recalled_at 应为 NULL，got %v", raw.LastRecalledAt)
	}

	// 召回命中（recent 模式 → Search 空 query）
	if _, err := lt.Search(ctx, "area1", "", 5); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT last_recalled_at FROM long_term_memory_items WHERE id = ?", item.ID).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	if raw.LastRecalledAt == nil {
		t.Fatal("召回后 last_recalled_at 应被更新")
	}
}

// TestRemoveClearsHotArea GC 删除后热区不应残留已删条目。
func TestRemoveClearsHotArea(t *testing.T) {
	lt, _ := newTestLongTerm(t, RecallModeRecent)
	ctx := context.Background()

	ids := make([]string, 0, 3)
	for _, c := range []string{"记忆A", "记忆B", "记忆C"} {
		item, err := lt.Add(ctx, "area1", c)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
	}
	if got := lt.GetHot("area1"); len(got) != 2 {
		t.Fatalf("热区应有 2 条（HotAreaSize=2），got %d", len(got))
	}

	// 删除 记忆A、记忆C（GC 场景：删除 ID 集合中的条目）
	lt.Remove(map[string]bool{ids[0]: true, ids[2]: true})
	hot := lt.GetHot("area1")
	if len(hot) != 1 || hot[0].ID != ids[1] {
		t.Fatalf("热区应仅保留记忆B，got %v", hot)
	}
}

// TestListUnusedOrdering 未使用条目排序：NULL 最旧在前（优先清理从未召回的）。
func TestListUnusedOrdering(t *testing.T) {
	_, db := newTestLongTerm(t, RecallModeRecent)
	ctx := context.Background()
	dao := dao.NewBundle(db).LongTermMemItem

	idA := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	idB := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	for _, it := range []models.LongTermMemoryItem{
		{ID: idA, ChatAreaID: "area1", Content: "条目A（未召回）"},
		{ID: idB, ChatAreaID: "area1", Content: "条目B（更早召回）"},
	} {
		if err := dao.Create(ctx, &it); err != nil {
			t.Fatal(err)
		}
	}
	_ = dao.Touch(ctx, idB)
	old := time.Now().Add(-48 * time.Hour)
	_ = db.Model(&models.LongTermMemoryItem{}).Where("id = ?", idB).Update("last_recalled_at", old).Error

	since := time.Now().Add(-24 * time.Hour)
	list, err := dao.ListUnused(ctx, since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("窗口内应 2 条未使用，got %d", len(list))
	}
	if list[0].ID != idA {
		t.Fatalf("NULL last_recalled_at 应排最前（优先清理），got %s", list[0].ID)
	}
}
