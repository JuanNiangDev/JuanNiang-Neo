package agent

import (
	"regexp"
	"strings"
)

var (
	mdCodeBlock   = regexp.MustCompile("(?s)```[^`]*```")
	mdInlineCode  = regexp.MustCompile("`([^`]+)`")
	mdImage       = regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	mdLink        = regexp.MustCompile(`\[([^\]]*)\]\(.*?\)`)
	mdBoldItalic  = regexp.MustCompile(`\*{3}(.+?)\*{3}`)
	mdBold        = regexp.MustCompile(`\*{2}(.+?)\*{2}`)
	mdItalic      = regexp.MustCompile(`\*([^*\n]+)\*`)
	mdStrike      = regexp.MustCompile(`~~(.+?)~~`)
	mdUnder       = regexp.MustCompile(`__([^_\n]+)__`)
	mdHeading     = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdQuote       = regexp.MustCompile(`(?m)^>\s?`)
	mdUnordered   = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	mdOrdered     = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	mdHR          = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`)
	mdTableSep    = regexp.MustCompile(`(?m)^\|?[-:| ]+\|[-:| ]+\|?$`)
)

// stripMarkdown 去除 markdown 格式，保留纯文本。
// 顺序很重要：代码块和图片要先于其他格式处理。
func stripMarkdown(s string) string {
	// 代码块：去掉围栏，保留内容
	s = mdCodeBlock.ReplaceAllStringFunc(s, func(match string) string {
		// 去掉第一行和最后一行的 ``` 围栏
		lines := strings.Split(match, "\n")
		if len(lines) <= 2 {
			return ""
		}
		return strings.Join(lines[1:len(lines)-1], "\n")
	})

	// 行内代码：保留内容
	s = mdInlineCode.ReplaceAllString(s, "$1")

	// 图片：移除
	s = mdImage.ReplaceAllString(s, "[图片]")

	// 链接：保留文字
	s = mdLink.ReplaceAllString(s, "$1")

	// 粗斜体
	s = mdBoldItalic.ReplaceAllString(s, "$1")
	// 粗体
	s = mdBold.ReplaceAllString(s, "$1")
	// 斜体
	s = mdItalic.ReplaceAllString(s, "$1")
	// 删除线
	s = mdStrike.ReplaceAllString(s, "$1")
	// 下划线
	s = mdUnder.ReplaceAllString(s, "$1")

	// 标题：去掉 # 前缀
	s = mdHeading.ReplaceAllString(s, "")
	// 引用：去掉 > 前缀
	s = mdQuote.ReplaceAllString(s, "")
	// 无序列表：去掉 - * + 前缀
	s = mdUnordered.ReplaceAllString(s, "")
	// 有序列表：去掉数字前缀
	s = mdOrdered.ReplaceAllString(s, "")

	// 表格分隔行：移除
	s = mdTableSep.ReplaceAllString(s, "")
	// 表格竖线：替换为空格
	s = strings.ReplaceAll(s, "|", " ")
	// 水平线：移除
	s = mdHR.ReplaceAllString(s, "")

	// 清理多余空白
	lines := strings.Split(s, "\n")
	var clean []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			clean = append(clean, line)
		}
	}
	return strings.TrimSpace(strings.Join(clean, "\n"))
}
