package longterm

import "testing"

func TestRecallTermsCleansAndTrims(t *testing.T) {
	// CQ 码/URL/标点/表情被剔除，中文保留
	clean, grams := RecallTerms("[CQ:face,id=66] 摸鱼人日历 https://example.com 好用！")
	if clean != "摸鱼人日历好用" {
		t.Fatalf("清洗结果非法: %q", clean)
	}
	if len(grams) == 0 {
		t.Fatalf("应切出 gram")
	}
	for _, g := range grams {
		if len([]rune(g)) != 3 {
			t.Fatalf("应切 3-gram，实际 %q", g)
		}
	}
}

func TestRecallTermsShortTextFallsBackToBigram(t *testing.T) {
	// 2 字退 2-gram（3 字仍走 3-gram）
	clean, grams := RecallTerms("摸鱼")
	if clean != "摸鱼" {
		t.Fatalf("清洗结果非法: %q", clean)
	}
	if len(grams) != 1 || grams[0] != "摸鱼" {
		t.Fatalf("2-gram 切分非法: %v", grams)
	}
}

func TestRecallTermsSingleCharNoGrams(t *testing.T) {
	clean, grams := RecallTerms("摸")
	if clean != "摸" || grams != nil {
		t.Fatalf("单字不应切 gram: clean=%q grams=%v", clean, grams)
	}
}

func TestRecallTermsEmptyOrStopwordOnly(t *testing.T) {
	if _, grams := RecallTerms(""); grams != nil {
		t.Fatalf("空文本不应有 gram: %v", grams)
	}
	// 全停用字（如纯表情/语气词）
	if _, grams := RecallTerms("的了么嗯"); grams != nil {
		t.Fatalf("停用字不应切出 gram: %v", grams)
	}
}

func TestRecallTermsDedupAndCap(t *testing.T) {
	// 重复文本（如刷屏长文）gram 去重
	clean, grams := RecallTerms("摸鱼摸鱼摸鱼摸鱼摸鱼摸鱼摸鱼摸鱼摸鱼摸鱼摸鱼摸鱼摸鱼")
	if clean == "" {
		t.Fatalf("清洗不应为空")
	}
	if len(grams) > recallMaxGrams {
		t.Fatalf("gram 数量超上限: %d", len(grams))
	}
	seen := map[string]bool{}
	for _, g := range grams {
		if seen[g] {
			t.Fatalf("gram 重复: %q", g)
		}
		seen[g] = true
	}
}

func TestRecallTermsTruncatesLongText(t *testing.T) {
	long := ""
	for i := 0; i < 50; i++ {
		long += "摸鱼人日历真好用"
	}
	clean, _ := RecallTerms(long)
	if len([]rune(clean)) > recallMaxChars {
		t.Fatalf("文本未截断: %d 字", len([]rune(clean)))
	}
}

func TestRecallTermsMixedAscii(t *testing.T) {
	clean, grams := RecallTerms("Linux 部署指南！")
	if clean != "Linux部署指南" {
		t.Fatalf("ASCII 应保留: %q", clean)
	}
	if len(grams) == 0 {
		t.Fatalf("应切出 gram: %v", grams)
	}
}
