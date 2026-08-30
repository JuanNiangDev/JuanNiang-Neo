package tool

import (
	"context"
	"testing"
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
