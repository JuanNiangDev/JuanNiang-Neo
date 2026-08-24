package groupmgr

import (
	"context"
	"strings"
	"testing"

	"JuanNiang-Neo/internal/adapter"
)

// groupEv 构造群消息事件（raw 含图片用）。
func groupEv(groupID, userID int64, raw string) adapter.Event {
	return adapter.Event{
		PostType: "message",
		Admins:   []string{},
		Message: &adapter.MessageEvent{
			MessageType: "group",
			GroupID:     groupID,
			UserID:      userID,
			MessageID:   int64(userID), // 简单唯一
			RawMessage:  raw,
		},
	}
}

// TestCheckImageSpamWarnThenMute 图片刷屏：首次达阈值警告；警告后继续刷 → 禁言。
func TestCheckImageSpamWarnThenMute(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()
	ev := groupEv(100, 200, "[CQ:image,file=abc]")

	// 1 张图：不触发
	if m.checkImageSpam(ctx, ev) {
		t.Fatal("单张图片不应触发")
	}
	// 2 张：不触发
	if m.checkImageSpam(ctx, ev) {
		t.Fatal("2 张图片不应触发")
	}
	// 3 张：触发警告（发送配图话术）
	if !m.checkImageSpam(ctx, ev) {
		t.Fatal("3 张图片应触发警告")
	}
	m.imgMu.Lock()
	warned := m.imgWarn[gkey(100, "ims:200")]
	m.imgMu.Unlock()
	if !warned {
		t.Fatal("警告后 imgWarn 应为 true")
	}
	if v, _ := gmdao.StatGet(ctx, gkey(100, "stats:warn")); v != "1" {
		t.Fatalf("警告统计 = %q", v)
	}

	// 已警告状态下再连续发图 → 禁言（stats:mute 递增）
	if !m.checkImageSpam(ctx, ev) {
		t.Fatal("警告后达阈值应触发禁言")
	}
	if v, _ := gmdao.StatGet(ctx, gkey(100, "stats:mute")); v != "1" {
		t.Fatalf("禁言统计 = %q", v)
	}
	// 禁言后刷屏状态已重置：单张图不再触发
	if m.checkImageSpam(ctx, ev) {
		t.Fatal("禁言后状态应重置，单张图不应触发")
	}
}

// TestCheckCopySpamTrigger 复读：3 人连续相同纯文本 → 触发警告。
func TestCheckCopySpamTrigger(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	// 用户 1、2 发相同消息：未达阈值不触发；用户 3 触发警告
	for _, uid := range []int64{1, 2} {
		if m.checkCopySpam(ctx, groupEv(100, uid, "你们这群人机")) {
			t.Fatalf("用户 %d 不应提前触发", uid)
		}
	}
	if !m.checkCopySpam(ctx, groupEv(100, 3, "你们这群人机")) {
		t.Fatal("第 3 人应触发复读警告")
	}
	if v, _ := gmdao.StatGet(ctx, gkey(100, "stats:copy_warn")); v != "1" {
		t.Fatalf("复读警告统计 = %q", v)
	}

	// 同一用户重复发言不算复读（新消息重置）
	m.checkCopySpam(ctx, groupEv(100, 4, "别的内容"))
	for i := 0; i < 5; i++ {
		if m.checkCopySpam(ctx, groupEv(100, 4, "别的内容")) {
			t.Fatal("同一用户重复不应触发")
		}
	}
	// 含 CQ 码的消息不参与复读检测
	if m.checkCopySpam(ctx, groupEv(100, 5, "[CQ:face,id=14]")) {
		t.Fatal("CQ 消息不应参与复读")
	}
}

// TestPunishTiers 三级惩罚：计数递增；第 3 次踢人失败（adapter 未启动）时计数保留。
func TestPunishTiers(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()
	ev := groupEv(100, 200, "广告")

	// 第 1 次：警告（violation count 1）
	m.punish(ev, "广告违规：测试", "ad")
	if c, _ := gmdao.ViolationGet(ctx, 100, 200); c != 1 {
		t.Fatalf("第 1 次后 count = %d", c)
	}
	// 第 2 次：禁言（count 2）
	m.punish(ev, "广告违规：测试", "ad")
	if c, _ := gmdao.ViolationGet(ctx, 100, 200); c != 2 {
		t.Fatalf("第 2 次后 count = %d", c)
	}
	// 第 3 次：踢出（adapter 未启动 → 踢人失败，count 保留 3 供下次仍按第 3 级）
	m.punish(ev, "广告违规：测试", "ad")
	if c, _ := gmdao.ViolationGet(ctx, 100, 200); c != 3 {
		t.Fatalf("踢人失败后 count 应保留 = 3，got %d", c)
	}
}

// TestWhitelistCommands 白名单命令：加白名单后豁免 + 解豁免恢复 + 豁免清违规。
func TestWhitelistCommands(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()

	// 违规记录前置
	_ = gmdao.ViolationSet(ctx, 100, 300, 2)

	// 加入白名单（含清违规 + 解禁言尝试）
	reply := m.CommandWhitelist(100, 300)
	if reply == "" {
		t.Fatal("白名单回复为空")
	}
	if !m.isWhitelisted(ctx, 300) {
		t.Fatal("白名单后应豁免")
	}
	if c, _ := gmdao.ViolationGet(ctx, 100, 300); c != 0 {
		t.Fatalf("白名单应清违规，count = %d", c)
	}

	// 重复加白名单：already 话术
	_ = m.CommandWhitelist(100, 300)

	// 豁免命令（不加入白名单，清违规 + 解禁言）
	_ = gmdao.ViolationSet(ctx, 100, 400, 1)
	reply = m.CommandPardon(100, 400)
	if reply == "" {
		t.Fatal("豁免回复为空")
	}
	if m.isWhitelisted(ctx, 400) {
		t.Fatal("豁免不应加入白名单")
	}
	if c, _ := gmdao.ViolationGet(ctx, 100, 400); c != 0 {
		t.Fatalf("豁免应清违规，count = %d", c)
	}

	// 解豁免：移出白名单恢复检测
	reply = m.CommandUnexempt(300)
	if reply == "" {
		t.Fatal("解豁免回复为空")
	}
	if m.isWhitelisted(ctx, 300) {
		t.Fatal("解豁免后不应豁免")
	}
}

// TestLearnSampleDedup 学习闭环：同一文本重复入库只产生一条样本（幂等）。
func TestLearnSampleDedup(t *testing.T) {
	m, gmdao := newTestManager(t, nil)
	ctx := context.Background()
	ev := groupEv(100, 200, "办卡加群")

	m.learnSample(ctx, `{"violation":"ad","reason":"广告"}`, ev, "ad")
	m.learnSample(ctx, `{"violation":"ad","reason":"广告"}`, ev, "ad")

	list, err := gmdao.SampleListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	learned := 0
	for _, s := range list {
		if s.Source == "learn" {
			learned++
		}
	}
	if learned != 1 {
		t.Fatalf("学习样本应幂等去重为 1 条，实际 %d", learned)
	}
}

// TestStatsText 统计文本组装（/groupstats 与 Web 共用）。
func TestStatsText(t *testing.T) {
	m, _ := newTestManager(t, nil)
	ctx := context.Background()
	_ = m.dao.StatSet(ctx, gkey(100, "stats:warn"), "2")
	_ = m.dao.StatSet(ctx, gkey(100, "stats:mute"), "1")
	text := m.StatsText(ctx, 100)
	for _, want := range []string{"群管理统计", "刷屏警告: 2 次", "刷屏禁言: 1 次", "广告违规: 0 次"} {
		if !strings.Contains(text, want) {
			t.Errorf("统计文本缺少 %q: %s", want, text)
		}
	}
}
