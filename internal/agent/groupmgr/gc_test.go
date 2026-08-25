package groupmgr

import (
	"context"
	"testing"
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
