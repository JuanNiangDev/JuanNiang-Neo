package agent

import (
	"context"
	"strings"
	"testing"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/memory"
	"JuanNiang-Neo/internal/agent/memory/longterm"
	"JuanNiang-Neo/internal/agent/memory/shortterm"
	"JuanNiang-Neo/internal/core/cache"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// quoteSender 构造带昵称的发送者字段。
func quoteSender(card, nickname string) struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Sex      string `json:"sex"`
	Age      int    `json:"age"`
	Card     string `json:"card"`
} {
	return struct {
		UserID   int64  `json:"user_id"`
		Nickname string `json:"nickname"`
		Sex      string `json:"sex"`
		Age      int    `json:"age"`
		Card     string `json:"card"`
	}{Card: card, Nickname: nickname}
}

// TestPlainRuneLen 验证纯文本字数：剥离 CQ 码 / URL / [msgid:N]，rune 计数。
func TestPlainRuneLen(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"普通中文", "你好世界", 4},
		{"剥离 CQ 码", "看图[CQ:image,file=https://a.com/b.png]后的文字", 6},
		{"剥离 URL", "看这个 https://multimedia.nt.qq.com.cn/x/y?z=1 然后", 7},
		{"剥离 msgid", "[msgid:123]长消息正文", 5},
		{"CQ+URL+msgid 混合", "[msgid:9][CQ:face,id=14]https://a.com/x 正文", 3},
		{"空串", "", 0},
	}
	for _, c := range cases {
		if got := plainRuneLen(c.in); got != c.want {
			t.Errorf("%s: plainRuneLen(%q) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}

// TestMemoryMsgLongGetsMsgid 验证 memoryMsg 的长消息标记规则：
// >阈值（纯文本>100）且带真实 message_id → 前缀 [msgid:N]；≤阈值 / 空 / 无 ID 不加。
func TestMemoryMsgLongGetsMsgid(t *testing.T) {
	base := func(id int64, raw string) *adapter.MessageEvent {
		return &adapter.MessageEvent{
			MessageType: "group",
			MessageID:   id,
			UserID:      7,
			GroupID:     9,
			RawMessage:  raw,
			Sender:      quoteSender("TuF3i", "nick"),
		}
	}
	speaker := "[TuF3i(QQ:7) 在群9] "

	long := strings.Repeat("长", 101)
	if got := memoryMsg(base(42, long)); !strings.HasPrefix(got, "[msgid:42]") {
		t.Errorf(">100 字长消息应带 [msgid:42]，实际: %q", got)
	} else if !strings.Contains(got, speaker) {
		t.Errorf("长消息仍应带发言人标识，实际: %q", got)
	}

	// 恰好 100 字（阈值不含）→ 不加标记
	if got := memoryMsg(base(43, strings.Repeat("短", 100))); strings.Contains(got, "[msgid:43]") {
		t.Errorf("恰好 100 字不应带 [msgid:43]，实际: %q", got)
	}

	// 空消息 → ""
	if got := memoryMsg(base(44, "   ")); got != "" {
		t.Errorf("空消息应返回空串，实际: %q", got)
	}

	// 无 message_id（cronjob/webhook 注入）→ 即使长也不带标记
	if got := memoryMsg(base(0, long)); strings.Contains(got, "[msgid:") {
		t.Errorf("无 message_id 不应带 [msgid:]，实际: %q", got)
	}

	// 长消息但纯文本其实被 URL 撑长 → 剥离 URL 后 ≤ 阈值，不加标记
	urlLong := "看这个链接 https://multimedia.nt.qq.com.cn/very/long/path?x=" + strings.Repeat("a", 200)
	if got := memoryMsg(base(45, urlLong)); strings.Contains(got, "[msgid:45]") {
		t.Errorf("URL 撑长不计入纯文本，不应带 [msgid:45]，实际: %q", got)
	}
}

// newQuoteTestHago 构造带真实短期记忆（miniredis）的 HagoCenter，供 enrichQuote 反查。
func newQuoteTestHago(t *testing.T) (*HagoCenter, *memory.MemoryGroup) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	cc := cache.NewCache(rc, "juan-test:")
	mg := memory.NewMemoryGroup(
		shortterm.New(shortterm.Config{WindowSize: 100}, cc),
		longterm.New(longterm.Config{RecallMode: longterm.RecallModeRecent}, nil),
		nil,
	)
	return &HagoCenter{Memory: mg}, mg
}

// quoteMsg 构造一条带 reply 段的用户消息。
func quoteMsg(id int64, replyData map[string]any) *adapter.MessageEvent {
	segs := []adapter.Segment{{Type: "text", Data: map[string]any{"text": "我回复一下"}}}
	if replyData != nil {
		segs = append([]adapter.Segment{{Type: "reply", Data: replyData}}, segs...)
	}
	return &adapter.MessageEvent{
		MessageType: "group",
		MessageID:   id,
		UserID:      7,
		GroupID:     9,
		RawMessage:  "我回复一下",
		Message:     segs,
		Sender:      quoteSender("Bob", "nick"),
	}
}

// TestEnrichQuoteShortInMemory 短引用（记忆内且无 [msgid:N] 标记）→ 注入引用原文。
func TestEnrichQuoteShortInMemory(t *testing.T) {
	h, mg := newQuoteTestHago(t)
	ctx := context.Background()
	areaID := "area-short"
	if err := mg.AddShortTermMessage(ctx, areaID, shortterm.ChatMessage{
		Role: "user", MsgID: "123", Content: "[Alice(QQ:11)] 你好呀",
	}); err != nil {
		t.Fatalf("add shortterm: %v", err)
	}
	m := quoteMsg(999, map[string]any{"id": "123"})
	got := h.enrichQuote(ctx, "[CQ:reply,id=123] 我回复一下", m, areaID)
	if !strings.Contains(got, "【引用原文】") || !strings.Contains(got, "[Alice(QQ:11)] 你好呀") {
		t.Errorf("短引用应注入引用原文，实际: %q", got)
	}
}

// TestEnrichQuoteLongInMemoryPassIDOnly 长引用且在被引用消息在记忆窗口（带 [msgid:N]）
// → 只传 messageid，不注入原文（历史 [msgid:N] 可关联）。
func TestEnrichQuoteLongInMemoryPassIDOnly(t *testing.T) {
	h, mg := newQuoteTestHago(t)
	ctx := context.Background()
	areaID := "area-long"
	longContent := "[msgid:456][Bob(QQ:22)] " + strings.Repeat("长", 120)
	if err := mg.AddShortTermMessage(ctx, areaID, shortterm.ChatMessage{
		Role: "user", MsgID: "456", Content: longContent,
	}); err != nil {
		t.Fatalf("add shortterm: %v", err)
	}
	mu := "[CQ:reply,id=456] 这条太长我只引用一下"
	m := quoteMsg(999, map[string]any{"id": "456"})
	if got := h.enrichQuote(ctx, mu, m, areaID); got != mu {
		t.Errorf("长引用在记忆窗口应只传 messageid（不注入），实际: %q", got)
	}
	if strings.Contains(mu, "【引用原文】") {
		t.Errorf("原始 mu 不应含引用标记，构造错误")
	}
}

// TestEnrichQuoteLongNotInMemoryInject 长引用但不在记忆窗口 → 回退内嵌内容注入引用原文。
func TestEnrichQuoteLongNotInMemoryInject(t *testing.T) {
	h, _ := newQuoteTestHago(t)
	ctx := context.Background()
	areaID := "area-empty"
	embedded := "一条已经不在记忆窗口里的被引用长消息"
	m := quoteMsg(999, map[string]any{"id": "789", "content": embedded})
	got := h.enrichQuote(ctx, "[CQ:reply,id=789] 回复", m, areaID)
	if !strings.Contains(got, "【引用原文】") || !strings.Contains(got, embedded) {
		t.Errorf("长引用不在记忆应回退注入内嵌内容，实际: %q", got)
	}
}

// TestEnrichQuoteNoIDEmbeddedInject 无 reply id 但有内嵌内容 → 回退注入引用原文。
func TestEnrichQuoteNoIDEmbeddedInject(t *testing.T) {
	h, _ := newQuoteTestHago(t)
	ctx := context.Background()
	embedded := "内嵌引用内容（无 id）"
	m := quoteMsg(999, map[string]any{"content": embedded})
	got := h.enrichQuote(ctx, "[CQ:reply] 回复", m, "area-no-id")
	if !strings.Contains(got, "【引用原文】") || !strings.Contains(got, embedded) {
		t.Errorf("无 id 有内嵌内容应注入引用原文，实际: %q", got)
	}
}

// TestEnrichQuoteIDNoContentKeepCQ 有 reply id 但拿不到内容（记忆未命中且无内嵌）
// → 保持 CQ 码原样（现状）。
func TestEnrichQuoteIDNoContentKeepCQ(t *testing.T) {
	h, _ := newQuoteTestHago(t)
	ctx := context.Background()
	mu := "[CQ:reply,id=99999] 这条引用查不到"
	m := quoteMsg(1000, map[string]any{"id": "99999"})
	if got := h.enrichQuote(ctx, mu, m, "area-unknown"); got != mu {
		t.Errorf("有 id 无内容应保持 CQ 码原样，实际: %q", got)
	}
}

// TestEnrichQuoteNoReplySegment 无 reply 段 → 原样返回。
func TestEnrichQuoteNoReplySegment(t *testing.T) {
	h, _ := newQuoteTestHago(t)
	ctx := context.Background()
	m := quoteMsg(1001, nil)
	mu := "没有引用的普通消息"
	if got := h.enrichQuote(ctx, mu, m, "area-none"); got != mu {
		t.Errorf("无 reply 段应原样返回，实际: %q", got)
	}
}

// TestReplySegmentID 验证 reply 段 id 类型兼容（string/int64/float64）。
func TestReplySegmentID(t *testing.T) {
	for _, c := range []struct {
		name string
		data map[string]any
		want int64
	}{
		{"string", map[string]any{"id": "123"}, 123},
		{"int64", map[string]any{"id": int64(456)}, 456},
		{"int", map[string]any{"id": 789}, 789},
		{"float64", map[string]any{"id": float64(1011)}, 1011},
		{"无 id", map[string]any{"content": "x"}, 0},
		{"无 reply 段", nil, 0},
	} {
		m := quoteMsg(1, c.data)
		if got := replySegmentID(m); got != c.want {
			t.Errorf("%s: replySegmentID = %d, want %d", c.name, got, c.want)
		}
	}
}
