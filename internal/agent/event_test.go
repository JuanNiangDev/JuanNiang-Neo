package agent

import (
	"strings"
	"testing"
)

// TestSplitMessagesCQCodeNotSplit 验证 CQ 码不会被切到两条消息里。
// 带 query 参数的图片 URL 含 "?"（断句符），若 CQ 码不保护会被从中间切开。
func TestSplitMessagesCQCodeNotSplit(t *testing.T) {
	content := "看图 [CQ:image,file=https://example.com/img.jpg?x=1&y=2]。再看看这张 [CQ:face,id=14]。最后一句话到这里结束。"
	parts := splitMessages(content)

	for i, p := range parts {
		if strings.Contains(p, "[CQ:") && !strings.Contains(p, "]") {
			t.Fatalf("段 %d 包含不完整的 CQ 码（被切开）: %q", i, p)
		}
	}

	// 拼接回去应保留所有 CQ 码完整
	joined := strings.Join(parts, "")
	for _, cq := range []string{"[CQ:image,file=https://example.com/img.jpg?x=1&y=2]", "[CQ:face,id=14]"} {
		if !strings.Contains(joined, cq) {
			t.Fatalf("CQ 码在拆分后丢失: %s\nparts: %v", cq, parts)
		}
	}
}

// TestSplitMessagesLongTextWithCQ 长文本中 CQ 码在断句处附近，拆分后 CQ 码仍完整。
func TestSplitMessagesLongTextWithCQ(t *testing.T) {
	content := "第一句话讲了一些内容，第二句也有点长需要被拆分处理看看效果如何。第三句带图[CQ:image,file=https://example.com/a.png?size=1]第四句继续说事情。第五句结束啦！"
	parts := splitMessages(content)

	if len(parts) == 0 {
		t.Fatal("拆分结果为空")
	}
	joined := strings.Join(parts, "")
	if !strings.Contains(joined, "[CQ:image,file=https://example.com/a.png?size=1]") {
		t.Fatalf("CQ 码在拆分后不完整\nparts: %v", parts)
	}
	for _, p := range parts {
		if strings.Contains(p, "[CQ:") && !strings.Contains(p, "]") {
			t.Fatalf("段包含被切开的 CQ 码: %q", p)
		}
	}
}

// TestSplitMessagesShortNoSplit 短内容（≤60 有效字）不拆分。
func TestSplitMessagesShortNoSplit(t *testing.T) {
	content := "你好[CQ:face,id=14]。今天天气不错！"
	parts := splitMessages(content)
	if len(parts) != 1 {
		t.Fatalf("短内容应不拆分，实际 %d 段: %v", len(parts), parts)
	}
}
