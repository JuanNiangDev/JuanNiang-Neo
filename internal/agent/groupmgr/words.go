package groupmgr

import (
	"embed"
	"strings"
)

// 词库文件（go:embed 内置，仅作首次启动种子导入 group_mgr_words；
// 之后词条管理全部走 Web/数据库，本文件不再参与热更新）。
// 来源：black.txt（campus-ad-detection-words/样本.md 抽取）+ cn_advertisement.txt（CN 广告词库）
//
//	all.txt（campus-ad-detection-words 灰色词）、cn_pornographic/cn_politics/cn_general（CN 敏感词库）
//
// 许可与出处见 JuanNiang-Plugins/plugins/redrock_group_manager/README.md。
//
//go:embed words/*.txt
var wordFiles embed.FS

// seedWordFiles 种子词库文件 → 分类映射。
var seedWordFiles = map[string]string{
	"words/black.txt":            "black",
	"words/cn_advertisement.txt": "black",
	"words/all.txt":              "gray",
	"words/cn_pornographic.txt":  "sensitive",
	"words/cn_politics.txt":      "sensitive",
	"words/cn_general.txt":       "sensitive",
}

// shortASCIIAllow 显式保留的短 ASCII 词（如 "sb" 灰色辱骂词审查用）。
var shortASCIIAllow = map[string]bool{"sb": true}

// cleanWord 规范化单行词条：去空白 → 跳过注释/空行 → 小写。
// 纯 ASCII 且短于 3 字符的 token（如 "av"）丢弃，防误命中 have/save 等正常单词。
func cleanWord(line string) string {
	w := strings.ToLower(strings.TrimSpace(line))
	if w == "" || strings.HasPrefix(w, "#") {
		return ""
	}
	if isASCIIAlnum(w) && len(w) < 3 && !shortASCIIAllow[w] {
		return ""
	}
	return w
}

func isASCIIAlnum(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// loadSeedWords 解析全部种子词库文件：去注释/去空白/小写/跨文件去重，
// 返回 分类 → 词条列表。
func loadSeedWords() map[string][]string {
	result := map[string][]string{"black": nil, "gray": nil, "sensitive": nil}
	seen := map[string]bool{}
	for path, category := range seedWordFiles {
		data, err := wordFiles.ReadFile(path)
		if err != nil {
			log.Warn("种子词库读取失败", "path", path, "err", err)
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			w := cleanWord(line)
			if w == "" || seen[w] {
				continue
			}
			seen[w] = true
			result[category] = append(result[category], w)
		}
	}
	// 黑色优先：从灰色集合剔除与黑色重叠的词条
	black := map[string]bool{}
	for _, w := range result["black"] {
		black[w] = true
	}
	var gray []string
	for _, w := range result["gray"] {
		if !black[w] {
			gray = append(gray, w)
		}
	}
	result["gray"] = gray
	log.Info("种子词库加载完成", "black", len(result["black"]), "gray", len(result["gray"]), "sensitive", len(result["sensitive"]))
	return result
}
