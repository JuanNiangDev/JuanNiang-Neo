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
		"2025年7月25日",
		"星期五",
		"六月",    // 农历
		"本周已过去", // 周进度
		"法定假日",  // 节假日倒计时
		"金句",
		"招新部门", // 群务
		"@卷娘",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("模板缺少 %q，实际:\n%s", want, html)
		}
	}
	// 群务为空时应有提示
	html2 := buildCalendarHTML(now, "")
	if !strings.Contains(html2, "今日群务未设置") {
		t.Errorf("空群务时缺少提示")
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
}
