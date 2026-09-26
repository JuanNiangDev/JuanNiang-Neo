package tool

import (
	"context"
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
