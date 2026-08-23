package longterm

import (
	"regexp"
	"strings"
	"unicode"
)

// 长期记忆语义召回的消息预处理（纯函数，无外部依赖）：
//  1. 清洗：只保留 CJK 与 ASCII 字母数字（去 CQ 码/URL/标点/表情）
//  2. 截断：超长文本截到 recallMaxChars 字（控制 similarity 计算与 SQL 大小）
//  3. 切 gram：3-gram 优先（与 pg_trgm 粒度对齐），不足退 2-gram；去重 + 限 top-N
//
// 返回（清洗后的查询文本, gram 列表）。gram 用于 GIN trgm 的 LIKE OR 候选召回，
// 查询文本用于候选内 similarity() 精确排序。

const (
	// recallMaxGrams 单次召回送出的 gram 数上限（同时控制 SQL OR 子句数量）
	recallMaxGrams = 10
	// recallMaxChars 参与匹配的文本长度上限（字）
	recallMaxChars = 100
)

// recallStopChars 常见停用字（虚词/代词/语气词）：切 gram 时剔除，
// 避免"的了在"这类无信息量 gram 拉低召回精度。
var recallStopChars = map[rune]bool{}

func init() {
	for _, r := range "的了是在有和与就都而及或我你他她它们这那吗呢吧啊哦嗯么着过不也很最把被从向对跟为由于" {
		recallStopChars[r] = true
	}
}

// recallCQCodeRe / recallURLRe：清洗前先剥离 Q 码与 URL，避免其 ASCII 字符串
// （CQ/face/id/https/examplecom）污染 gram 与相似度计算。
var recallCQCodeRe = regexp.MustCompile(`\[CQ:[^\]]*\]`)
var recallURLRe = regexp.MustCompile(`https?://\S+`)

// RecallTerms 清洗文本并切 gram。gram/文本为空时返回 (cleanText, nil)。
func RecallTerms(text string) (string, []string) {
	// 0. 剥离 CQ 码与 URL（剩余 ASCII 结构字符串无召回价值）
	text = recallCQCodeRe.ReplaceAllString(text, " ")
	text = recallURLRe.ReplaceAllString(text, " ")

	// 1. 清洗：保留 CJK / ASCII 字母数字，剔除停用字
	var runes []rune
	for _, r := range text {
		if !recallStopChars[r] && isRecallRune(r) {
			runes = append(runes, r)
		}
	}
	if len(runes) > recallMaxChars {
		runes = runes[:recallMaxChars]
	}
	clean := strings.TrimSpace(string(runes))
	if clean == "" {
		return "", nil
	}

	// 2. 切 gram：3-gram 优先，不足 3 字退 2-gram，1 字无召回价值
	n := 3
	if len(runes) < 3 {
		n = 2
	}
	if len(runes) < n {
		return clean, nil
	}

	seen := make(map[string]bool, len(runes))
	grams := make([]string, 0, recallMaxGrams)
	for i := 0; i+n <= len(runes) && len(grams) < recallMaxGrams; i++ {
		g := string(runes[i : i+n])
		if !seen[g] {
			seen[g] = true
			grams = append(grams, g)
		}
	}
	return clean, grams
}

// isRecallRune 判断字符是否值得参与匹配：CJK 表意文字或 ASCII 字母数字。
func isRecallRune(r rune) bool {
	if r >= 0x4E00 && r <= 0x9FFF {
		return true
	}
	if r <= 0x7F {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	return false
}
