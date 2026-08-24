package pluggin

import (
	"strings"
	"testing"
)

// TestOneBot11ForwardMsgAndReplyTo 插件发消息能力（feat-reply-acl-plugin）：
//   - 合并转发 send_group_forward_msg_sync 节点正确传递
//   - 引用回复 reply_to 在消息前插入 Reply 段
//   - 非法 reply_to 参数被忽略（消息原样发送，不 panic）
func TestOneBot11ForwardMsgAndReplyTo(t *testing.T) {
	pe, adp := newMiniTestEngine(t, nil)
	L := loadMiniPlugin(t, pe, "msgtest", `
local jn = require("jn")
function on_message(event)
    -- ① 引用回复：字符串消息 + reply_to=12345 → 前插 Reply 段
    jn.onebot11.send_group_msg(1001, "hello", 12345)
    -- ② 合并转发：2 节点（1 构造 + 1 引用）
    jn.onebot11.send_group_forward_msg_sync(1001, {
        { user_id = 1, nickname = "a", content = "hi" },
        { id = 99 },
    })
    -- ③ 非法 reply_to（非数字字符串）→ 忽略引用，消息原样
    jn.onebot11.send_group_msg(1001, "no-reply", "invalid-id")
    -- ④ 段数组消息 + reply_to=678 → 段数组前插 Reply 段
    jn.onebot11.send_group_msg(1001, { { type = "text", data = { text = "seg" } } }, 678)
    return false, nil
end
`)

	runOnMessage(t, pe, L, 1001, 2001, "trigger", "90001")

	// send_group_msg 为异步入队，等待全部 4 条送达
	waitFor(t, func() bool {
		adp.mu.Lock()
		defer adp.mu.Unlock()
		return len(adp.groups) == 4
	})

	adp.mu.Lock()
	groups := append([]string{}, adp.groups...)
	adp.mu.Unlock()
	if len(groups) != 4 {
		t.Fatalf("应发送 4 条消息，实际 %d: %v", len(groups), groups)
	}

	// 异步发送顺序不定，按内容断言
	has := func(sub string) bool {
		for _, g := range groups {
			if strings.Contains(g, sub) {
				return true
			}
		}
		return false
	}
	// ① 引用回复（字符串）：消息变为段数组且含 Reply 段与消息 ID
	if !has("12345") {
		t.Fatalf("引用回复应含被引用消息 ID 12345: %v", groups)
	}
	// ② 合并转发：2 个节点
	if !has("forward:2") {
		t.Fatalf("合并转发应传递 2 节点: %v", groups)
	}
	// ③ 非法 reply_to：原样字符串（不含 reply 段）
	if !has("no-reply") {
		t.Fatalf("非法 reply_to 应忽略引用原样发送: %v", groups)
	}
	// ④ 段数组 + reply_to：前插 Reply 段（678）
	if !has("678") {
		t.Fatalf("段数组引用回复应含被引用消息 ID 678: %v", groups)
	}
	// 引用段类型出现于 ①④（Reply 段序列化为 {reply map[id:...]}）
	replyCount := 0
	for _, g := range groups {
		if strings.Contains(g, "reply map") {
			replyCount++
		}
	}
	if replyCount != 2 {
		t.Fatalf("应有两处 reply 段，实际 %d: %v", replyCount, groups)
	}
}

// TestOneBot11ForwardMsgEmptyNodes 空节点数组返回错误（不 panic）。
func TestOneBot11ForwardMsgEmptyNodes(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	L := loadMiniPlugin(t, pe, "msgtest2", `
local jn = require("jn")
forward_err = ""
function on_message(event)
    local ok, err = jn.onebot11.send_group_forward_msg_sync(1001, {})
    if not ok then forward_err = tostring(err) end
    return false, nil
end
`)

	runOnMessage(t, pe, L, 1001, 2001, "trigger", "90002")
	err := luaGetGlobal(pe, L, "forward_err")
	if err.Type() != 3 /* LString */ || !strings.Contains(err.String(), "不能为空") {
		t.Fatalf("空节点应返回错误，got %q", err.String())
	}
}
