package agent

import (
	"context"
	"testing"
	"time"

	"JuanNiang-Neo/internal/adapter"
)

// partMsg 构造一条走参与窗口的群聊消息：非 @/命令/名字（不进 mustKeep），且非噪音。
func partMsg(id int64, user int64) adapter.Event {
	return adapter.Event{
		PostType: "message",
		Message: &adapter.MessageEvent{
			MessageType: "group",
			MessageID:   id,
			UserID:      user,
			GroupID:     456,
			RawMessage:  "今天天气怎么样大家聊聊",
			Message:     []adapter.Segment{{Type: "text", Data: map[string]any{"text": "今天天气怎么样大家聊聊"}}},
		},
	}
}

// partRS 返回确定性参与参数（关闭随机），便于窗口状态机测试。
func partRS() ReplySettings {
	return ReplySettings{
		QuietGapSeconds:        60,
		ForceCount:             100,
		MaxAgeSeconds:          60,
		WindowMaxMsgs:          20,
		JitterSeconds:          0,
		ForceCountJitter:       0,
		ParticipateProbability: 1.0,
		TypingDelayMaxMs:       0,
	}
}

// TestParticipationWindowForceCount 插话计数强发：攒够 force_count 条即释放整窗，
// 两条消息都被一次 handleMessage 消费（写 user chat_records）。
func TestParticipationWindowForceCount(t *testing.T) {
	h, db := newDedupTestHago(t)
	ctx := context.Background()
	rs := partRS()
	rs.ForceCount = 2

	h.dispatchToAgent(ctx, partMsg(1, 111), rs)
	h.dispatchToAgent(ctx, partMsg(2, 222), rs)

	time.Sleep(300 * time.Millisecond) // 让 runAgent goroutine 落库
	if got := countUserRecords(t, db); got != 2 {
		t.Fatalf("计数强发应消费整窗 2 条，实际 %d", got)
	}
}

// TestParticipationWindowQuietRelease 安静释放：窗口消息后无人插话，安静间隔到期触发释放。
func TestParticipationWindowQuietRelease(t *testing.T) {
	h, db := newDedupTestHago(t)
	ctx := context.Background()
	rs := partRS()
	rs.QuietGapSeconds = 1

	h.dispatchToAgent(ctx, partMsg(1, 111), rs)

	time.Sleep(1600 * time.Millisecond)
	if got := countUserRecords(t, db); got != 1 {
		t.Fatalf("安静释放应消费 1 条，实际 %d", got)
	}
}

// TestParticipationWindowMaxAge 最迟必发：窗口创建 max_age 到期后即使无新消息也强制释放。
func TestParticipationWindowMaxAge(t *testing.T) {
	h, db := newDedupTestHago(t)
	ctx := context.Background()
	rs := partRS()
	rs.QuietGapSeconds = 60 // 安静间隔很长，只有 max_age 会触发
	rs.MaxAgeSeconds = 1

	h.dispatchToAgent(ctx, partMsg(1, 111), rs)

	time.Sleep(1600 * time.Millisecond)
	if got := countUserRecords(t, db); got != 1 {
		t.Fatalf("max_age 强制释放应消费 1 条，实际 %d", got)
	}
}

// TestParticipationWindowProbabilitySkip 安静释放参与概率：probability=0 时安静路径静默放弃本窗。
func TestParticipationWindowProbabilitySkip(t *testing.T) {
	h, db := newDedupTestHago(t)
	ctx := context.Background()
	rs := partRS()
	rs.QuietGapSeconds = 1
	rs.ParticipateProbability = 0.0

	h.dispatchToAgent(ctx, partMsg(1, 111), rs)

	time.Sleep(1600 * time.Millisecond)
	if got := countUserRecords(t, db); got != 0 {
		t.Fatalf("参与概率 0 应静默放弃本窗，实际消费 %d", got)
	}
}

// TestParticipationMustKeepDiscardsWindow mustKeep 立即回复并丢弃当前参与窗口：
// 窗口内消息不再释放（等过安静间隔仍不消费），只有 mustKeep 消息被处理。
func TestParticipationMustKeepDiscardsWindow(t *testing.T) {
	h, db := newDedupTestHago(t)
	ctx := context.Background()
	rs := partRS()
	rs.QuietGapSeconds = 1

	h.dispatchToAgent(ctx, partMsg(1, 111), rs) // 进参与窗口
	h.dispatchToAgent(ctx, groupMsg(2), rs)     // 含"机器人"→ mustKeep，丢弃窗口并立即回

	time.Sleep(1600 * time.Millisecond) // 覆盖安静间隔
	if got := countUserRecords(t, db); got != 1 {
		t.Fatalf("mustKeep 应立即回复且丢弃窗口（窗口消息不应被消费），实际消费 %d", got)
	}
}

// TestParticipationMustKeepImmediate mustKeep 私聊/@/名字路径不攒窗，立即处理。
func TestParticipationMustKeepImmediate(t *testing.T) {
	h, db := newDedupTestHago(t)
	ctx := context.Background()
	rs := partRS()

	h.dispatchToAgent(ctx, groupMsg(1), rs) // "你好机器人" → isDefinitelyRelevant

	time.Sleep(300 * time.Millisecond)
	if got := countUserRecords(t, db); got != 1 {
		t.Fatalf("mustKeep 应立即消费 1 条，实际 %d", got)
	}
}

// TestReplySettingsDefaults 参与窗口参数缺省（0）时回退默认值。
func TestReplySettingsDefaults(t *testing.T) {
	var rs ReplySettings
	if got := rs.quietGap(); got != 5*time.Second {
		t.Errorf("quietGap 默认应 5s，实际 %v", got)
	}
	if got := rs.forceCount(); got != 5 {
		t.Errorf("forceCount 默认应 5，实际 %d", got)
	}
	if got := rs.maxAge(); got != 20*time.Second {
		t.Errorf("maxAge 默认应 20s，实际 %v", got)
	}
	if got := rs.windowMaxMsgs(); got != 20 {
		t.Errorf("windowMaxMsgs 默认应 20，实际 %d", got)
	}
}

// TestJitterSecDeterministic 抖动关闭时恒为 0（确定性模式）。
func TestJitterSecDeterministic(t *testing.T) {
	for i := 0; i < 100; i++ {
		if got := jitterSec(0); got != 0 {
			t.Fatalf("jitterSec(0) 应恒为 0，实际 %d", got)
		}
	}
	// 上限内取值
	for i := 0; i < 100; i++ {
		if got := jitterSec(2); got < 0 || got > 2 {
			t.Fatalf("jitterSec(2) 应在 [0,2]，实际 %d", got)
		}
	}
}

// TestParticipationShortSymbolNotNoise ≤2 字与纯 emoji/符号不算噪音：进参与窗口
// 等待安静释放（而非被 isDefinitelyIrrelevant 规则丢弃）。
func TestParticipationShortSymbolNotNoise(t *testing.T) {
	h, db := newDedupTestHago(t)
	ctx := context.Background()
	rs := partRS()
	rs.QuietGapSeconds = 1

	shortMsg := partMsg(1, 111)
	shortMsg.Message.RawMessage = "哈哈"
	shortMsg.Message.Message = []adapter.Segment{{Type: "text", Data: map[string]any{"text": "哈哈"}}}
	symMsg := partMsg(2, 222)
	symMsg.Message.RawMessage = "😂😂😂"
	symMsg.Message.Message = []adapter.Segment{{Type: "text", Data: map[string]any{"text": "😂😂😂"}}}

	h.dispatchToAgent(ctx, shortMsg, rs)
	h.dispatchToAgent(ctx, symMsg, rs)

	time.Sleep(1600 * time.Millisecond) // 覆盖安静间隔，窗口整窗释放
	if got := countUserRecords(t, db); got != 2 {
		t.Fatalf("≤2 字/纯符号应进窗口并在安静释放时消费，实际消费 %d", got)
	}
}

// TestParticipationNoTextNoise 剥离 CQ 码/URL 后无任何文字（纯图片/纯 sticker）仍算噪音，
// 不进入参与窗口。
func TestParticipationNoTextNoise(t *testing.T) {
	h, db := newDedupTestHago(t)
	ctx := context.Background()
	rs := partRS()
	rs.QuietGapSeconds = 1

	imgMsg := partMsg(1, 111)
	imgMsg.Message.RawMessage = "[CQ:image,file=abc.jpg]"
	imgMsg.Message.Message = []adapter.Segment{{Type: "image", Data: map[string]any{"file": "abc.jpg"}}}

	h.dispatchToAgent(ctx, imgMsg, rs)

	time.Sleep(1600 * time.Millisecond)
	if got := countUserRecords(t, db); got != 0 {
		t.Fatalf("无文字纯图应作为噪音丢弃，实际消费 %d", got)
	}
}
