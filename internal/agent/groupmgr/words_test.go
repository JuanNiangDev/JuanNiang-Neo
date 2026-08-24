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

// TestWordSoftDeleteRebuild 词条软删后重建同名：不得报唯一索引冲突。
// 回归：GroupMgrWord.Word 曾为普通 uniqueIndex，WordDelete 软删后再次
// WordUpsert 同名报 UNIQUE constraint failed（词库面板删词后再加同词直接失败）。
func TestWordSoftDeleteRebuild(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
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
	// 重建后应参与命中（内存缓存 Reload 后生效）
	_ = m.Reload(ctx)
	hit, cat := m.wordHit(ctx, "这是一个测试重建词条")
	if hit != "测试重建词条" || cat != "black" {
		t.Fatalf("重建词条应命中 black，got %q/%s", hit, cat)
	}
}

func TestReasonAndCategory(t *testing.T) {
	if got := categoryByWordOrCard("x", "sensitive", false, "ad"); got != "sensitive" {
		t.Errorf("敏感词类别 = %s", got)
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
