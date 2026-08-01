// Package splitter 提供 LLM 回复的消息切分、Markdown去除、错别字注入等功能。
// 学习 MaiBot 的五阶段流水线设计。
package splitter

import (
	"math/rand"
	"regexp"
	"strings"
	"unicode"
)

// Options 切分配置。
type Options struct {
	MaxLength     int     // 单段最大长度（中文占比>90%时生效，0=不限）
	MaxSentences  int     // 切割后最大句子数（超限触发截断或默认回复）
	MaxSegments   int     // 最终输出最大段数（合并到此数量）
	StripMarkdown bool    // 去除 Markdown 格式
	EnableTypo    bool    // 注入中文错别字
	TypoRate      float64 // 错别字概率 (0-1)
	AutoSplit     bool    // 是否启用切分
}

// DefaultOptions 返回默认配置。
func DefaultOptions() Options {
	return Options{
		MaxLength:     400,
		MaxSentences:  10,
		MaxSegments:   5,
		StripMarkdown: false,
		EnableTypo:    false,
		TypoRate:      0.03,
		AutoSplit:     true,
	}
}

// Splitter 消息切分器。
type Splitter struct {
	opts      Options
	protector *Protector
}

// New 创建 Splitter。
func New(opts Options) *Splitter {
	if opts.MaxSegments <= 0 {
		opts.MaxSegments = 5
	}
	if opts.MaxSentences <= 0 {
		opts.MaxSentences = 10
	}
	return &Splitter{
		opts:      opts,
		protector: NewProtector(),
	}
}

// UpdateOptions 运行时更新配置。
func (s *Splitter) UpdateOptions(opts Options) {
	s.opts = opts
}

// Process 完整处理流水线。
func (s *Splitter) Process(text string) []string {
	if text == "" {
		return nil
	}

	// [1] Markdown 去除
	if s.opts.StripMarkdown {
		text = StripMarkdown(text)
	}

	// [2] CQ码/颜文字保护
	protected, mapping := s.protector.Protect(text)

	// [3] 中文占比过滤
	if chineseRatio(protected) > 0.9 && len([]rune(protected)) > s.opts.MaxLength*2 {
		return []string{"呃呃"}
	}

	// [4] 句子分割 + 概率合并
	var sentences []string
	if s.opts.AutoSplit {
		sentences = splitAndMerge(protected)
	} else {
		sentences = []string{protected}
	}

	// [5] 恢复保护
	for i, sent := range sentences {
		sentences[i] = s.protector.Restore(sent, mapping)
	}

	// [6] 数量限制
	if len(sentences) > s.opts.MaxSentences {
		sentences = sentences[:s.opts.MaxSentences]
	}

	// [7] 合并到 MaxSegments
	sentences = mergeToMax(sentences, s.opts.MaxSegments)

	// [8] 错别字注入
	if s.opts.EnableTypo {
		for i, sent := range sentences {
			sentences[i] = injectTypo(sent, s.opts.TypoRate)
		}
	}

	return sentences
}

// chineseRatio 计算中文字符占比。
func chineseRatio(s string) float64 {
	runes := []rune(s)
	if len(runes) == 0 {
		return 0
	}
	chinese := 0
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) {
			chinese++
		}
	}
	return float64(chinese) / float64(len(runes))
}

// splitAndMerge 句子分割 + 概率合并。
func splitAndMerge(text string) []string {
	runes := []rune(text)
	n := len(runes)
	if n < 3 {
		if rand.Float64() < 0.01 {
			return []string{string(runes[0]), string(runes[1:])}
		}
		return []string{text}
	}

	// 分割点：中英文标点 + 空格 + 换行
	separators := map[rune]bool{
		'，': true, ',': true, ' ': true, '。': true, ';': true,
		'！': true, '？': true, '\n': true, '.': true, '!': true, '?': true,
	}

	type segment struct {
		content string
		sep     string
	}
	var segments []segment
	current := ""

	for i := 0; i < n; i++ {
		ch := runes[i]
		if separators[ch] {
			canSplit := true
			// 空格不切数字/英文之间
			if ch == ' ' && i > 0 && i < n-1 {
				prev := runes[i-1]
				next := runes[i+1]
				if isAlnum(prev) && isAlnum(next) {
					canSplit = false
				}
			}
			if canSplit {
				if current != "" {
					segments = append(segments, segment{current, string(ch)})
				}
				current = ""
			} else {
				current += string(ch)
			}
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		segments = append(segments, segment{current, ""})
	}
	if len(segments) == 0 {
		return []string{text}
	}

	// 概率合并
	splitStrength := 0.6
	if n < 12 {
		splitStrength = 0.2
	} else if n < 32 {
		splitStrength = 0.6
	} else {
		splitStrength = 0.8
	}
	mergeProb := 1.0 - splitStrength

	merged := []string{}
	i := 0
	for i < len(segments) {
		cur := segments[i].content
		// 检查是否可以与下一段合并
		for i+1 < len(segments) && cur != "" && rand.Float64() < mergeProb {
			cur += segments[i].sep + segments[i+1].content
			i++
		}
		if cur != "" {
			merged = append(merged, cur)
		}
		i++
	}

	if len(merged) == 0 {
		return []string{text}
	}
	return merged
}

func isAlnum(r rune) bool {
	return unicode.IsDigit(r) || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}

// mergeToMax 将 sentences 合并到最多 maxSegments 条。
func mergeToMax(sentences []string, maxSegments int) []string {
	if maxSegments <= 0 || len(sentences) <= maxSegments {
		return sentences
	}
	// 均匀合并
	chunkSize := (len(sentences) + maxSegments - 1) / maxSegments
	result := make([]string, 0, maxSegments)
	for i := 0; i < len(sentences); i += chunkSize {
		end := i + chunkSize
		if end > len(sentences) {
			end = len(sentences)
		}
		result = append(result, strings.Join(sentences[i:end], ""))
	}
	return result
}

// —————— Markdown 去除 ——————

// StripMarkdown 去除 Markdown 格式。
func StripMarkdown(text string) string {
	// **bold** → bold
	bold := regexp.MustCompile(`\*\*(.+?)\*\*`)
	text = bold.ReplaceAllString(text, "$1")

	// *italic* → italic
	italic := regexp.MustCompile(`\*(.+?)\*`)
	text = italic.ReplaceAllString(text, "$1")

	// `code` → code
	code := regexp.MustCompile("`([^`]+)`")
	text = code.ReplaceAllString(text, "$1")

	// [text](url) → text
	link := regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	text = link.ReplaceAllString(text, "$1")

	// ### heading
	heading := regexp.MustCompile(`(?m)^#{1,6}\s+`)
	text = heading.ReplaceAllString(text, "")

	// - list item → · list item
	listItem := regexp.MustCompile(`(?m)^[-*+]\s+`)
	text = listItem.ReplaceAllString(text, "· ")

	// > blockquote
	blockquote := regexp.MustCompile(`(?m)^>\s+`)
	text = blockquote.ReplaceAllString(text, "")

	// ~~strikethrough~~ → strikethrough
	strike := regexp.MustCompile(`~~(.+?)~~`)
	text = strike.ReplaceAllString(text, "$1")

	// ___ → 空
	hr := regexp.MustCompile(`(?m)^_{3,}\s*$`)
	text = hr.ReplaceAllString(text, "")

	return text
}

// —————— 错别字注入 ——————

var typoTable = map[rune]rune{
	'在': '再', '的': '得', '地': '的',
	'己': '已', '已': '己', '未': '末',
	'人': '入', '千': '干', '土': '士',
	'白': '百', '目': '日', '大': '太',
}

func injectTypo(text string, rate float64) string {
	if rate <= 0 {
		return text
	}
	runes := []rune(text)
	for i, r := range runes {
		if repl, ok := typoTable[r]; ok && rand.Float64() < rate {
			runes[i] = repl
		}
	}
	return string(runes)
}
