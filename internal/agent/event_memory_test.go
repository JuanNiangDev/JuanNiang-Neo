package agent

import (
	"testing"

	"JuanNiang-Neo/internal/adapter"
)

// TestBuildMemorySpeaker 验证记忆发言人标识的格式（昵称+QQ+群号）。
func TestBuildMemorySpeaker(t *testing.T) {
	sender := func(card, nickname string) struct {
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

	// 群聊：群名片优先
	group := &adapter.MessageEvent{
		MessageType: "group",
		UserID:      1483915073,
		GroupID:     1076723599,
		Sender:      sender("TuF3i", "nick"),
	}
	if got := buildMemorySpeaker(group); got != "[TuF3i(QQ:1483915073) 在群1076723599] " {
		t.Errorf("group speaker mismatch: %q", got)
	}

	// 群聊：无群名片时回退昵称
	groupNoCard := &adapter.MessageEvent{
		MessageType: "group",
		UserID:      1483915073,
		GroupID:     1076723599,
		Sender:      sender("", "Color"),
	}
	if got := buildMemorySpeaker(groupNoCard); got != "[Color(QQ:1483915073) 在群1076723599] " {
		t.Errorf("group no-card speaker mismatch: %q", got)
	}

	// 私聊：无群号
	priv := &adapter.MessageEvent{
		MessageType: "private",
		UserID:      42,
		Sender:      sender("", "Alice"),
	}
	if got := buildMemorySpeaker(priv); got != "[Alice(QQ:42)] " {
		t.Errorf("private speaker mismatch: %q", got)
	}

	// 完全无名字：回退 QQ 号
	anon := &adapter.MessageEvent{
		MessageType: "group",
		UserID:      7,
		GroupID:     1,
		Sender:      sender("", ""),
	}
	if got := buildMemorySpeaker(anon); got != "[QQ7(QQ:7) 在群1] " {
		t.Errorf("anonymous speaker mismatch: %q", got)
	}
}

// TestSenderDisplayName 验证展示名优先级：群名片 > 昵称。
func TestSenderDisplayName(t *testing.T) {
	msg := &adapter.MessageEvent{Sender: struct {
		UserID   int64  `json:"user_id"`
		Nickname string `json:"nickname"`
		Sex      string `json:"sex"`
		Age      int    `json:"age"`
		Card     string `json:"card"`
	}{Card: "CardName", Nickname: "Nick"}}
	if got := senderDisplayName(msg); got != "CardName" {
		t.Errorf("expected CardName, got %q", got)
	}
	msg.Sender.Card = ""
	if got := senderDisplayName(msg); got != "Nick" {
		t.Errorf("expected Nick, got %q", got)
	}
}
