package tool

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"JuanNiang-Neo/internal/adapter"
)

// TestDropDelivery 审核闸门丢弃交付消息：只移除投递到指定会话的 Delivery 项，
// 私聊 / 其他群 / 非交付消息保留。
func TestDropDelivery(t *testing.T) {
	q := NewDeferredSendQueue()
	q.Add(DeferredSend{MessageType: "group", TargetID: 123, Message: "投递当前群（应移除）", Delivery: true})
	q.Add(DeferredSend{MessageType: "group", TargetID: 123, Message: "非交付工具消息（保留）"})
	q.Add(DeferredSend{MessageType: "private", TargetID: 456, Message: "私聊交付（保留）", Delivery: true})
	q.Add(DeferredSend{MessageType: "group", TargetID: 789, Message: "其他群交付（保留）", Delivery: true})

	q.DropDelivery("group", 123)
	if q.Len() != 3 {
		t.Fatalf("DropDelivery 后应剩 3 条，got %d", q.Len())
	}

	// 通过 Flush 确认内容（fakeAdapter 记录发送；其余 3 条都会发出）
	a := &fakeAdapter{}
	sent := q.Flush(context.Background(), a)
	if len(sent) != 3 {
		t.Fatalf("Flush 应发送 3 条，got %d", len(sent))
	}
	want := map[string]bool{
		"非交付工具消息（保留）": true,
		"私聊交付（保留）":    true,
		"其他群交付（保留）":   true,
	}
	for _, s := range sent {
		if !want[s.Text()] {
			t.Fatalf("发送了不应发送的内容: %q", s.Text())
		}
	}

	// 空队列 / 无匹配时安全
	q2 := NewDeferredSendQueue()
	q2.DropDelivery("group", 1)
	if q2.Len() != 0 {
		t.Fatal("空队列 DropDelivery 应无副作用")
	}
}

func TestDeferredQueueHasExpressionToUsesTargetAndMessageStructure(t *testing.T) {
	q := NewDeferredSendQueue()
	q.Add(DeferredSend{MessageType: "group", TargetID: 123, Message: "普通文字 [CQ:image,file=stk://sticker-1]", Delivery: true})
	q.Add(DeferredSend{MessageType: "group", TargetID: 123, Message: "[CQ:face,id=14]", Delivery: false})
	q.Add(DeferredSend{MessageType: "group", TargetID: 123, Message: []adapter.Segment{
		adapter.Text("文本"),
		adapter.Sticker("stk://sticker-2"),
	}, Delivery: false})
	q.Add(DeferredSend{MessageType: "private", TargetID: 123, Message: "[CQ:face,id=66]", Delivery: false})

	if !q.HasExpressionTo("group", 123) {
		t.Fatal("当前群的 stk:///CQ face/消息段表达式应被检测到")
	}
	if q.HasExpressionTo("group", 456) {
		t.Fatal("其他群的表达式不应影响当前目标")
	}
	if q.HasExpressionTo("private", 123) == false {
		t.Fatal("私聊目标自己的表达式应被检测到")
	}
	if NewDeferredSendQueue().HasExpressionTo("group", 123) {
		t.Fatal("空队列不应报告表达式")
	}
}

func TestDeferredQueueHasExpressionToDoesNotTreatPlainMessageAsExpression(t *testing.T) {
	q := NewDeferredSendQueue()
	q.Add(DeferredSend{MessageType: "group", TargetID: 123, Message: "用户提到了 stk:// 但没有可发送消息段", Delivery: true})
	if q.HasExpressionTo("group", 123) {
		t.Fatal("普通文本中的非 CQ 表达式内容不应被误判")
	}
}

func TestMessageHasExpressionUsesFinalMessageStructure(t *testing.T) {
	if !MessageHasExpression("[CQ:face,id=14]") {
		t.Fatal("CQ face should be detected in final string")
	}
	if !MessageHasExpression([]adapter.Segment{adapter.Sticker("stk://short-id")}) {
		t.Fatal("stk image segment should be detected")
	}
	if MessageHasExpression("文本里提到 stk://short-id，但没有表达式") {
		t.Fatal("plain text should not be detected as expression")
	}
}

func TestDropReviewOnlyRemovesNewAutomaticItems(t *testing.T) {
	q := NewDeferredSendQueue()
	q.Add(DeferredSend{MessageType: "group", TargetID: 123, Message: "原版 face", Delivery: false})
	q.Add(DeferredSend{MessageType: "group", TargetID: 123, Message: "Laya 自动表情", Origin: "laya_auto", DropOnReview: true})
	q.Add(DeferredSend{MessageType: "group", TargetID: 456, Message: "其他群自动表情", Origin: "laya_auto", DropOnReview: true})
	q.Add(DeferredSend{MessageType: "group", TargetID: 123, Message: "原版交付", Delivery: true})

	q.DropOnReview("group", 123)
	if q.Len() != 3 {
		t.Fatalf("审核拒绝后只应移除当前群自动项，got %d", q.Len())
	}
	for _, s := range q.Flush(context.Background(), &fakeAdapter{}) {
		if s.Message == "Laya 自动表情" {
			t.Fatal("DropOnReview 项不应被发送")
		}
	}
}

// TestMessageHasGalleryStickerIgnoresPlainFaces 图库表情检测只认 stk:// 图片段，
// 普通 QQ 内联小表情（face 段）不算图库表情。
func TestMessageHasGalleryStickerIgnoresPlainFaces(t *testing.T) {
	if MessageHasGallerySticker("[CQ:face,id=14]") {
		t.Fatal("plain QQ face must not count as gallery sticker")
	}
	if MessageHasGallerySticker("文本里提到 stk://short-id，但不是表达式") {
		t.Fatal("plain text mention must not count as gallery sticker")
	}
	if !MessageHasGallerySticker("[CQ:image,file=stk://short-id,subType=1]") {
		t.Fatal("stk:// image CQ code must count as gallery sticker")
	}
	if !MessageHasGallerySticker([]adapter.Segment{adapter.Sticker("stk://short-id")}) {
		t.Fatal("stk:// image segment must count as gallery sticker")
	}
	if MessageHasGallerySticker([]adapter.Segment{{Type: "image", Data: map[string]any{"file": "https://example.com/pic.png"}}}) {
		t.Fatal("ordinary image must not count as gallery sticker")
	}
}

// deferredRecordingAdapter replaces only the external send boundary.
type deferredRecordingAdapter struct {
	AdapterProvider
	calls   []DeferredSend
	sendErr error
	onSend  func()
}

func (a *deferredRecordingAdapter) record(kind string, target int64, message any) (int64, error) {
	a.calls = append(a.calls, DeferredSend{MessageType: kind, TargetID: target, Message: message})
	if a.onSend != nil {
		a.onSend()
	}
	return 42, a.sendErr
}

func (a *deferredRecordingAdapter) SendPrivateMsg(target int64, message any) (int64, error) {
	return a.record("private", target, message)
}

func (a *deferredRecordingAdapter) SendGroupMsg(target int64, message any) (int64, error) {
	return a.record("group", target, message)
}

// Both entry points must agree on filtering and acknowledgement, while preserving
// the adapter's message payload and target exactly.
func TestDeferredSendDispatch(t *testing.T) {
	for _, tc := range []struct {
		name          string
		send          DeferredSend
		err           error
		wantCall      bool
		wantDelivered bool
	}{
		{"private success", DeferredSend{MessageType: "private", TargetID: 101, Message: "hello", Delivery: true}, nil, true, true},
		{"group success", DeferredSend{MessageType: "group", TargetID: 202, Message: []adapter.Segment{adapter.Sticker("stk://sample")}, Origin: "laya_auto", DropOnReview: true}, nil, true, true},
		{"private failure", DeferredSend{MessageType: "private", TargetID: 101, Message: "hello"}, errors.New("send failed"), true, false},
		{"group failure", DeferredSend{MessageType: "group", TargetID: 202, Message: "hello"}, errors.New("send failed"), true, false},
		{"zero target", DeferredSend{MessageType: "group", TargetID: 0, Message: "hello"}, nil, false, false},
		{"negative target", DeferredSend{MessageType: "private", TargetID: -1, Message: "hello"}, nil, false, false},
		{"unknown type", DeferredSend{MessageType: "other", TargetID: 101, Message: "hello"}, nil, false, false},
		{"silence token", DeferredSend{MessageType: "group", TargetID: 202, Message: "__NO_REPLY__"}, nil, false, false},
		{"silence phrase", DeferredSend{MessageType: "private", TargetID: 101, Message: "保持静默"}, nil, false, false},
		{"silence segments", DeferredSend{MessageType: "group", TargetID: 202, Message: []adapter.Segment{adapter.Text("__NO_"), adapter.Text("REPLY__")}}, nil, false, false},
	} {
		for _, mode := range []string{"SendNow", "Flush"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				q := NewDeferredSendQueue()
				a := &deferredRecordingAdapter{sendErr: tc.err}
				if mode == "SendNow" {
					if got := q.SendNow(context.Background(), a, tc.send); got != tc.wantDelivered {
						t.Fatalf("SendNow = %v, want %v", got, tc.wantDelivered)
					}
				} else {
					q.Add(tc.send)
					got := q.Flush(context.Background(), a)
					if tc.wantDelivered {
						if !reflect.DeepEqual(got, []DeferredSend{tc.send}) {
							t.Fatalf("delivered = %#v, want %#v", got, tc.send)
						}
					} else if len(got) != 0 {
						t.Fatalf("undelivered send returned: %#v", got)
					}
					if q.Len() != 0 {
						t.Fatal("Flush must consume failed and filtered entries too")
					}
				}
				var want []DeferredSend
				if tc.wantCall {
					want = []DeferredSend{{MessageType: tc.send.MessageType, TargetID: tc.send.TargetID, Message: tc.send.Message}}
				}
				if !reflect.DeepEqual(a.calls, want) {
					t.Fatalf("adapter calls = %#v, want %#v", a.calls, want)
				}
			})
		}
	}
}

func TestDeferredFlushDrainsInOrderBeforeUnlockedSending(t *testing.T) {
	q := NewDeferredSendQueue()
	first := DeferredSend{MessageType: "private", TargetID: 101, Message: "first"}
	failed := DeferredSend{MessageType: "group", TargetID: 202, Message: "failed"}
	last := DeferredSend{MessageType: "group", TargetID: 303, Message: "last"}
	next := DeferredSend{MessageType: "private", TargetID: 404, Message: "next batch"}
	for _, s := range []DeferredSend{first, failed, last} {
		q.Add(s)
	}
	a := &deferredRecordingAdapter{}
	a.onSend = func() {
		// TryLock fails immediately if Flush accidentally holds the mutex across I/O.
		if !q.mu.TryLock() {
			t.Fatal("Flush holds the queue mutex during adapter send")
		}
		q.mu.Unlock()
		if len(a.calls) == 1 {
			if q.Len() != 0 {
				t.Fatal("queue was not drained before sending")
			}
			q.Add(next)
		}
		a.sendErr = nil
		if len(a.calls) == 2 {
			a.sendErr = errors.New("send failed")
		}
	}
	if got := q.Flush(context.Background(), a); !reflect.DeepEqual(got, []DeferredSend{first, last}) {
		t.Fatalf("delivered = %#v, want first and last", got)
	}
	if !reflect.DeepEqual(a.calls, []DeferredSend{first, failed, last}) {
		t.Fatalf("adapter calls out of order: %#v", a.calls)
	}
	a.onSend = nil
	if got := q.Flush(context.Background(), a); !reflect.DeepEqual(got, []DeferredSend{next}) {
		t.Fatalf("newly queued send was lost or replayed: %#v", got)
	}
	if got := q.Flush(context.Background(), a); got != nil {
		t.Fatalf("empty Flush = %#v, want nil", got)
	}
}

func TestDeferredSendNowLeavesQueueAndSupportsNilReceiver(t *testing.T) {
	q := NewDeferredSendQueue()
	queued := DeferredSend{MessageType: "group", TargetID: 202, Message: "queued"}
	immediate := DeferredSend{MessageType: "private", TargetID: 101, Message: "immediate"}
	q.Add(queued)
	a := &deferredRecordingAdapter{}
	if !q.SendNow(context.Background(), a, immediate) {
		t.Fatal("SendNow should succeed")
	}
	if !reflect.DeepEqual(a.calls, []DeferredSend{immediate}) || q.Len() != 1 {
		t.Fatal("SendNow must send only its argument and leave queued entries untouched")
	}
	if got := q.Flush(context.Background(), a); !reflect.DeepEqual(got, []DeferredSend{queued}) {
		t.Fatalf("queued send changed: %#v", got)
	}
	var nilQueue *DeferredSendQueue
	if !nilQueue.SendNow(context.Background(), a, immediate) {
		t.Fatal("SendNow does not require a queue receiver")
	}
	if got := nilQueue.Flush(context.Background(), a); got != nil {
		t.Fatalf("nil queue Flush = %#v", got)
	}
}

func TestDeferredNilAdapterPreservesQueue(t *testing.T) {
	q := NewDeferredSendQueue()
	s := DeferredSend{MessageType: "private", TargetID: 101, Message: "retry later"}
	q.Add(s)
	if q.SendNow(context.Background(), nil, s) {
		t.Fatal("nil adapter must not report success")
	}
	if got := q.Flush(context.Background(), nil); got != nil || q.Len() != 1 {
		t.Fatal("nil adapter Flush must leave queued entries untouched")
	}
	if got := q.Flush(context.Background(), &deferredRecordingAdapter{}); !reflect.DeepEqual(got, []DeferredSend{s}) {
		t.Fatalf("retained send = %#v, want %#v", got, s)
	}
}
