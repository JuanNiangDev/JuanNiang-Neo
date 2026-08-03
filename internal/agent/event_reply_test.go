package agent

import (
	"strings"
	"testing"
)

// TestSplitMessagesPreservesNewlines 验证拆分后换行不被吞掉。
func TestSplitMessagesPreservesNewlines(t *testing.T) {
	// 短内容（≤60 字）原样返回，换行完整保留
	short := "第一行\n第二行\n第三行"
	if got := splitMessages(short); len(got) != 1 || got[0] != short {
		t.Errorf("short content should be returned as-is: %+v", got)
	}

	// 长内容（>60 字）中的换行在拆分合并后仍应保留
	long := strings.Repeat("这是一行内容。", 4) + "\n" + strings.Repeat("这是另一行内容。", 4)
	parts := splitMessages(long)
	if len(parts) == 0 {
		t.Fatal("splitMessages returned empty")
	}
	if joined := strings.Join(parts, ""); !strings.Contains(joined, "\n") {
		t.Errorf("newlines lost in splitMessages: %q", joined)
	}

	// 纯换行内容（无标点）不拆分，整体返回
	linesOnly := strings.Repeat("一行\n", 20)
	if got := splitMessages(linesOnly); len(got) != 1 || got[0] != linesOnly {
		t.Errorf("newline-only content should be returned as-is: %+v", got)
	}
}

// TestStripMarkdownPreservesNewlines 验证去除 Markdown 时保留空行与换行。
func TestStripMarkdownPreservesNewlines(t *testing.T) {
	in := "第一行\n\n第二行\n第三行"
	out := stripMarkdown(in)
	if !strings.Contains(out, "\n\n") {
		t.Errorf("blank lines lost in stripMarkdown: %q", out)
	}
	if out != in {
		t.Errorf("plain text should be unchanged, got %q", out)
	}

	// 含 markdown 语法时，行结构（含空行）也应保留
	md := "# 标题\n\n- 项目一\n- 项目二"
	out = stripMarkdown(md)
	if !strings.Contains(out, "\n\n") {
		t.Errorf("blank lines lost after markdown strip: %q", out)
	}
	if !strings.Contains(out, "项目一\n项目二") {
		t.Errorf("line breaks lost after markdown strip: %q", out)
	}
}
