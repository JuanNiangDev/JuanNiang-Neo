package groupmgr

import (
	"context"
	"testing"
)

func TestCleanWord(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  校园卡  ", "校园卡"},
		{"# 注释行", ""},
		{"", ""},
		{"AV", ""},         // 短 ASCII 过滤
		{"av", ""},         // 短 ASCII 过滤
		{"sb", "sb"},       // 显式保留
		{"Apple", "apple"}, // 小写
		{" 加群 ", "加群"},
		{"Vx", ""},         // 2 字符被过滤
		{"广告词A1", "广告词a1"}, // 混合大小写
	}
	for _, c := range cases {
		if got := cleanWord(c.in); got != c.want {
			t.Errorf("cleanWord(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLoadSeedWords(t *testing.T) {
	words := loadSeedWords()
	for _, cat := range []string{"black", "gray", "sensitive"} {
		if len(words[cat]) == 0 {
			t.Fatalf("种子词库 %s 为空", cat)
		}
	}
	// 黑色优先：灰色集合不得与黑色重叠
	black := map[string]bool{}
	for _, w := range words["black"] {
		black[w] = true
	}
	for _, w := range words["gray"] {
		if black[w] {
			t.Errorf("灰色词库包含黑色词 %q（应剔除）", w)
		}
	}
}

func TestDetectGroupCard(t *testing.T) {
	if !detectGroupCard(`[CQ:json,data={"app":"com.tencent.troopsharecard","prompt":"xxx"}]`) {
		t.Error("troopsharecard 应命中")
	}
	if !detectGroupCard(`[CQ:json,data={"app":"com.tencent.contact.lua"}]`) {
		t.Error("contact.lua 应命中")
	}
	// 段外普通文本不命中
	if detectGroupCard("今天聊了 com.tencent.troopsharecard 这个应用") {
		t.Error("段外文本不应命中")
	}
	if detectGroupCard("普通消息") {
		t.Error("普通消息不应命中")
	}
}

func TestStripCQ(t *testing.T) {
	if got := stripCQ("[CQ:image,file=abc]你好[CQ:at,qq=123]"); got != " 你好 " {
		t.Errorf("stripCQ = %q", got)
	}
}

func TestParseTargetQQ(t *testing.T) {
	if got := ParseTargetQQ([]string{"[CQ:at,qq=114514]"}); got != 114514 {
		t.Errorf("at 解析 = %d", got)
	}
	if got := ParseTargetQQ([]string{"123456"}); got != 123456 {
		t.Errorf("纯数字解析 = %d", got)
	}
	if got := ParseTargetQQ(nil); got != 0 {
		t.Errorf("空参数 = %d", got)
	}
	if got := ParseTargetQQ([]string{"abc"}); got != 0 {
		t.Errorf("非法参数 = %d", got)
	}
}

// TestWordSoftDeleteRebuild 词条软删后重建同名：不得报唯一索引冲突（DAO 层回归）。
// 回归：GroupMgrWord.Word 曾为普通 uniqueIndex，WordDelete 软删后再次
// WordUpsert 同名报 UNIQUE constraint failed（词库面板删词后再加同词直接失败）。
// 注：词条表已废弃（关键词改内存兜底），本测试仅验证 DAO 兼容旧行为。
func TestWordSoftDeleteRebuild(t *testing.T) {
	_, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	id, err := gmdao.WordUpsert(ctx, "测试重建词条", "gray", "import")
	if err != nil {
		t.Fatal(err)
	}
	if err := gmdao.WordDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	// 软删后重建同名：不报错，且复用同一条记录（复活）
	id2, err := gmdao.WordUpsert(ctx, "测试重建词条", "black", "import")
	if err != nil {
		t.Fatalf("软删后重建同名应成功，got %v", err)
	}
	if id2 != id {
		t.Fatalf("复活应复用原记录 ID=%d，got %d", id, id2)
	}
}

// TestWordHitFromSeedTxt 关键词词库从 go:embed txt 加载到内存：
// Reload 后 wordHit 能命中种子词（不入 DB，仅内存兜底）。
func TestWordHitFromSeedTxt(t *testing.T) {
	m, _ := newTestManager(t, nil)
	ctx := context.Background()

	// 种子词库含校园卡等黑词（black.txt），Reload 后应命中
	_ = m.Reload(ctx)
	hit, cat := m.wordHit(ctx, "帮我办校园卡")
	if hit == "" || cat == "" {
		t.Fatalf("种子词命中 = %q/%s", hit, cat)
	}
}

// TestDeleteWordRemovesSamples 词条删除对账（DAO 层回归，词条表已废弃）。
// 回归：此前只软删 group_mgr_words，seed 样本与 RAG 向量仍活跃，删词后 RAG 照常命中。
// 注：Manager.DeleteWord 已弃用（关键词改内存兜底），本测试仅验证 DAO 双删路径的旧契约。
func TestDeleteWordRemovesSamples(t *testing.T) {
	_, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	id, err := gmdao.WordUpsert(ctx, "测试删词清理", "black", "import")
	if err != nil {
		t.Fatal(err)
	}
	// 模拟词条派生样本（source=seed，WordID 关联）
	if _, err := gmdao.SampleAddWithWord(ctx, "测试删词清理", "ad", "seed", id); err != nil {
		t.Fatal(err)
	}

	// 直接走 DAO 软删 + 派生样本清理（Manager.DeleteWord 已弃用，此处复现其内部逻辑）
	if err := gmdao.WordDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	samples, err := gmdao.SampleListByWord(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range samples {
		if err := gmdao.SampleDelete(ctx, s.ID); err != nil {
			t.Fatal(err)
		}
	}
	w, err := gmdao.WordGet(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !w.DeletedAt.Valid {
		t.Fatal("词条应为软删状态")
	}
	list, err := gmdao.SampleListByText(ctx, "测试删词清理")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("词条派生样本应被删除，剩余 %d 条", len(list))
	}
}

func TestReasonAndCategory(t *testing.T) {
	if got := categoryByWordOrCard("x", "sensitive", false, "ad"); got != "sensitive" {
		t.Errorf("敏感词类别 = %s", got)
	}
	// 回归：敏感词 + 推荐卡片同时命中时，敏感红线优先（card 分支曾提前返回 ad）
	if got := categoryByWordOrCard("台独", "sensitive", true, "ad"); got != "sensitive" {
		t.Errorf("敏感词+卡片类别 = %s, want sensitive", got)
	}
	if got := categoryByWordOrCard("x", "black", false, "sensitive"); got != "ad" {
		t.Errorf("黑词类别 = %s", got)
	}
	if got := categoryByWordOrCard("", "", true, "sensitive"); got != "ad" {
		t.Errorf("卡片类别 = %s", got)
	}
	if got := categoryByWordOrCard("", "", false, "sensitive"); got != "sensitive" {
		t.Errorf("样本类别回退 = %s", got)
	}
	if got := reasonByWord("", "", true); got != "广告违规：推荐群聊卡片" {
		t.Errorf("卡片 reason = %s", got)
	}
	if got := reasonByWord("台独", "sensitive", false); got != "敏感违规：台独" {
		t.Errorf("敏感 reason = %s", got)
	}
}
