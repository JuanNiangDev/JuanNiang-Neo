package fishcal

import (
	"strings"
	"testing"
	"time"
)

func TestBuildCalendarHTML(t *testing.T) {
	// 2025-07-25 为用户示例中的日期（农历六月初一，周五）
	now := time.Date(2025, 7, 25, 9, 0, 0, 0, time.Local)
	html := buildCalendarHTML(now, "招新部门：技术部 & 设计部")

	for _, want := range []string{
		"摸鱼人日历",
		"宜划水",  // 印章
		"忌内卷",
		"七月",    // 日历卡月
		">25<",   // 日历卡日（两位数字）
		"星期五 · 农历闰六月初一", // 星期 + 农历合并行
		"距周末还有", // 周末倒计时
		"今天就是周末啦", // 周五 5/5
		"距国庆节还有", // 节假日倒计时
		"金句",
		"招新部门", // 群务
		"@卷娘",
		"data:font/woff2;base64,", // 楷体内嵌（无需外网）
	} {
		if !strings.Contains(html, want) {
			t.Errorf("模板缺少 %q，实际:\n%s", want, html)
		}
	}
	// 群务为空时应有提示
	html2 := buildCalendarHTML(now, "")
	if !strings.Contains(html2, "今日无群务安排") {
		t.Errorf("空群务时缺少提示")
	}
	// 模板尺寸 682×757
	if !strings.Contains(html2, "width:682px") || !strings.Contains(html2, "height:757px") {
		t.Errorf("模板尺寸应为 682×757")
	}
}

func TestNextStatutoryHoliday(t *testing.T) {
	// 2025-07-25 之后最近的法定假日是 2025-10-01 国庆节
	name, days := nextStatutoryHoliday(time.Date(2025, 7, 25, 0, 0, 0, 0, time.Local))
	if name != "国庆节" {
		t.Errorf("最近假日应为国庆节，实际 %q", name)
	}
	// 7-25 到 10-01 相隔 68 天
	if days != 68 {
		t.Errorf("国庆节倒计时应为 68 天，实际 %d", days)
	}
	// 非午夜触发（如 09:30）也不应因时间截断少算天数
	name2, days2 := nextStatutoryHoliday(time.Date(2025, 7, 25, 9, 30, 0, 0, time.Local))
	if name2 != "国庆节" || days2 != 68 {
		t.Errorf("09:30 触发倒计时应为国庆节 68 天，实际 %q %d", name2, days2)
	}
}
