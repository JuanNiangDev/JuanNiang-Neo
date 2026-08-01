package splitter

import (
	"fmt"
	"regexp"
)

// Protector 保护 CQ 码和颜文字不被切分破坏。
type Protector struct {
	cqPattern  *regexp.Regexp
	kaoPattern *regexp.Regexp
}

// NewProtector 创建保护器。
func NewProtector() *Protector {
	return &Protector{
		cqPattern:  regexp.MustCompile(`\[CQ:[^\]]+\]`),
		kaoPattern: regexp.MustCompile(`\([^\)]*[\u3040-\u309f\u30a0-\u30ff\u4e00-\u9fff][^\)]*\)`),
	}
}

// Protect 将 CQ 码和颜文字替换为占位符，返回保护后文本和映射表。
func (p *Protector) Protect(text string) (string, map[string]string) {
	mapping := make(map[string]string)
	idx := 0

	// 保护 CQ 码
	text = p.cqPattern.ReplaceAllStringFunc(text, func(match string) string {
		key := fmt.Sprintf("__CQ_%d__", idx)
		mapping[key] = match
		idx++
		return key
	})

	// 保护颜文字
	text = p.kaoPattern.ReplaceAllStringFunc(text, func(match string) string {
		key := fmt.Sprintf("__KAO_%d__", idx)
		mapping[key] = match
		idx++
		return key
	})

	return text, mapping
}

// Restore 将占位符还原。
func (p *Protector) Restore(text string, mapping map[string]string) string {
	for placeholder, original := range mapping {
		text = regexp.MustCompile(regexp.QuoteMeta(placeholder)).ReplaceAllString(text, original)
	}
	return text
}
