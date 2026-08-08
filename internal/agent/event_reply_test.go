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

// TestSplitMessagesBlankLineStrongSplit 验证空行被当作强分段信号，
// 无论字数多少都拆分（修复"@A 回复1\n\n@B 回复2"被合并成一条的问题）。
func TestSplitMessagesBlankLineStrongSplit(t *testing.T) {
	// 用户场景：LLM 输出两个独立回复，分别 @ 不同的人，用空行分隔。
	// 整体有效字数 < 60，原逻辑会合并成一条发送；强分段后应拆为 2 条。
	content := "[CQ:at,qq=111] 不行喵～卷娘不能随便禁言群友的[CQ:face,id=66]\n\n[CQ:at,qq=222] 卷娘只喜欢红岩的大家喵～[CQ:face,id=67]"
	parts := splitMessages(content)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts split by blank line, got %d: %+v", len(parts), parts)
	}
	if strings.Contains(parts[0], "qq=222") {
		t.Errorf("first part leaked second @: %q", parts[0])
	}
	if !strings.Contains(parts[1], "qq=222") {
		t.Errorf("second part missing second @: %q", parts[1])
	}
	if !strings.Contains(parts[0], "qq=111") {
		t.Errorf("first part missing first @: %q", parts[0])
	}
}

// TestSplitMessagesBlankLineWithWhitespace 验证空行中含空白字符也能触发强分段。
func TestSplitMessagesBlankLineWithWhitespace(t *testing.T) {
	content := "第一段回复\n  \n第二段回复"
	parts := splitMessages(content)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts split by whitespace-only blank line, got %d: %+v", len(parts), parts)
	}
	if strings.TrimSpace(parts[0]) != "第一段回复" {
		t.Errorf("first part mismatch: %q", parts[0])
	}
	if strings.TrimSpace(parts[1]) != "第二段回复" {
		t.Errorf("second part mismatch: %q", parts[1])
	}
}

// TestSplitMessagesSingleNewlineNotSplit 验证单换行不触发强分段（保持原行为）。
func TestSplitMessagesSingleNewlineNotSplit(t *testing.T) {
	// 单换行（无空行）的短内容仍应原样返回一条
	short := "第一行\n第二行\n第三行"
	parts := splitMessages(short)
	if len(parts) != 1 || parts[0] != short {
		t.Errorf("single-newline short content should be returned as-is: %+v", parts)
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
