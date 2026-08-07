package shortterm

import "testing"

// TestContainsMsgID 验证幂等去重判定：MsgID 相同即重复；空 MsgID / 无匹配不重复。
func TestContainsMsgID(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "[A(QQ:1)] hi", MsgID: "100"},
		{Role: "assistant", Content: "hello", MsgID: ""},
		{Role: "user", Content: "[B(QQ:2)] hi", MsgID: "200"},
	}
	cases := []struct {
		name   string
		msgID  string
		expect bool
	}{
		{"命中已存在消息", "100", true},
		{"命中另一条消息", "200", true},
		{"未命中", "999", false},
		{"空 MsgID 不匹配", "", false},
	}
	for _, c := range cases {
		if got := containsMsgID(msgs, c.msgID); got != c.expect {
			t.Errorf("%s: containsMsgID(%v, %q) = %v, want %v", c.name, msgs, c.msgID, got, c.expect)
		}
	}
}

// TestContainsMsgIDEmptyList 验证空窗口时永不重复。
func TestContainsMsgIDEmptyList(t *testing.T) {
	if containsMsgID(nil, "100") {
		t.Error("空列表不应命中任何 MsgID")
	}
}
