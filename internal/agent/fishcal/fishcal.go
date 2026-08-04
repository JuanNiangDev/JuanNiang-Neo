// Package fishcal 摸鱼人日历：独立的每日定时任务（不复用 CronJob 系统）。
//
// 流程：每天按配置的 cron 表达式触发 → 用模板组装日历内容（日期/农历/本周进度/
// 法定假日倒计时/金句/群务）→ 通过 T2I 服务渲染成图片 → 发送到目标群。
package fishcal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/logging"

	"github.com/6tail/lunar-go/calendar"
	"github.com/robfig/cron/v3"
)

var log = logging.NewModule("fishcal")

// Scheduler 摸鱼人日历调度器（独立 cron，不依赖 CronJob Manager）。
type Scheduler struct {
	mu      sync.Mutex
	cron    *cron.Cron
	entryID cron.EntryID

	dao     *dao.FishCalendarDAO
	getT2I  func() *t2icaller.Client
	adapter *adapter.Adapter
}

// New 创建调度器。
func New(d *dao.FishCalendarDAO, getT2I func() *t2icaller.Client, adp *adapter.Adapter) *Scheduler {
	return &Scheduler{
		cron: cron.New(cron.WithSeconds(), cron.WithLocation(time.Local)),
		dao:  d, getT2I: getT2I, adapter: adp,
	}
}

// Reload 从 DB 读取配置并（重新）注册 cron 任务；未启用则移除。
func (s *Scheduler) Reload(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.entryID = 0
	}
	cfg, err := s.dao.GetConfig(ctx)
	if err != nil {
		log.Warn("摸鱼日历配置读取失败", "err", err)
		return
	}
	if !cfg.Enabled {
		log.Info("摸鱼日历未启用，跳过调度")
		return
	}
	eid, err := s.cron.AddFunc(cfg.CronExpr, func() {
		runCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if err := s.TriggerNow(runCtx); err != nil {
			log.Error("摸鱼日历执行失败", "err", err)
		} else {
			log.Info("摸鱼日历发送成功", "group", cfg.TargetGroupID)
		}
	})
	if err != nil {
		log.Error("摸鱼日历 cron 注册失败", "expr", cfg.CronExpr, "err", err)
		return
	}
	s.entryID = eid
	log.Info("摸鱼日历已调度", "expr", cfg.CronExpr, "group", cfg.TargetGroupID)
}

// Run 启动调度器并阻塞直到 ctx 结束。
func (s *Scheduler) Run(ctx context.Context) {
	s.cron.Start()
	log.Info("摸鱼日历调度器已启动")
	<-ctx.Done()
	s.cron.Stop()
	log.Info("摸鱼日历调度器已停止")
}

// TriggerNow 立即执行一次：生成日历图片并发送到目标群（Web 手动触发与 cron 共用）。
func (s *Scheduler) TriggerNow(ctx context.Context) error {
	cfg, err := s.dao.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	recordErr := func(runErr error) error {
		_ = s.dao.MarkRunResult(ctx, time.Now(), runErr.Error())
		return runErr
	}
	if cfg.TargetGroupID == 0 {
		return recordErr(errors.New("未配置目标群号"))
	}
	t2i := s.getT2I()
	if t2i == nil {
		return recordErr(errors.New("T2I 服务未启用，无法生成图片"))
	}

	html := buildCalendarHTML(time.Now(), cfg.GroupAffairs)
	img, err := t2i.GenerateImage(ctx, t2icaller.GenerateRequest{
		HTML: html,
		Options: &t2icaller.GenerateOptions{
			Type:    t2icaller.ImageTypeJPEG,
			Quality: 85,
		},
	})
	if err != nil {
		return recordErr(fmt.Errorf("T2I 生成失败: %w", err))
	}

	b64 := "base64://" + base64.StdEncoding.EncodeToString(img)
	msg := fmt.Sprintf("[CQ:image,file=%s]", b64)
	if _, err := s.adapter.SendGroupMsg(cfg.TargetGroupID, msg); err != nil {
		return recordErr(fmt.Errorf("发送群消息失败: %w", err))
	}
	_ = s.dao.MarkRunResult(ctx, time.Now(), "")
	return nil
}

// ---------- 日历内容 ----------

// holiday 法定节假日。
type holiday struct {
	name       string
	month, day int
}

// holidaysByYear 法定节假日表（每年元旦/春节/清明/劳动/端午/中秋/国庆）。
var holidaysByYear = map[int][]holiday{
	2025: {
		{"元旦", 1, 1}, {"春节", 1, 29}, {"清明节", 4, 4},
		{"劳动节", 5, 1}, {"端午节", 5, 31}, {"中秋节", 10, 6}, {"国庆节", 10, 1},
	},
	2026: {
		{"元旦", 1, 1}, {"春节", 2, 17}, {"清明节", 4, 5},
		{"劳动节", 5, 1}, {"端午节", 6, 19}, {"中秋节", 9, 25}, {"国庆节", 10, 1},
	},
}

var weekdayCN = []string{"日", "一", "二", "三", "四", "五", "六"}

// buildCalendarHTML 组装摸鱼日历 HTML 模板（T2I 渲染成图片）。
func buildCalendarHTML(t time.Time, groupAffairs string) string {
	// 农历
	lunarStr := ""
	if solar := calendar.NewSolarFromYmd(t.Year(), int(t.Month()), t.Day()); solar != nil {
		ld := solar.GetLunar()
		lunarStr = fmt.Sprintf("%s月%s", ld.GetMonthInChinese(), ld.GetDayInChinese())
	}

	// 本周进度：周一=1 … 周五=5，周末按 5/5
	passed := int(t.Weekday())
	if passed == 0 {
		passed = 7
	}
	if passed > 5 {
		passed = 5
	}
	weekLine := fmt.Sprintf("⏰ 本周已过去 [%d/5] 天！再坚持 %d 天就周末啦！", passed, 5-passed)
	if passed >= 5 {
		weekLine = "⏰ 本周已过去 [5/5] 天！今天就是周末啦！"
	}

	// 最近法定假日
	holidayName, holidayDays := nextStatutoryHoliday(t)
	holidayLine := fmt.Sprintf("🎉 距离下一个法定假日（%s）还有：[%d] 天", holidayName, holidayDays)
	if holidayName == "" {
		holidayLine = "🎉 暂无后续法定假日数据，专心摸鱼吧！"
	}

	// 金句
	quote := fetchHitokoto(context.Background())

	affairs := strings.TrimSpace(groupAffairs)
	if affairs != "" {
		affairs = "✨ " + affairs + " ✨"
	} else {
		affairs = "（今日群务未设置，请管理员在 Web 面板配置）"
	}

	return fmt.Sprintf(`<div style="font-family:'Microsoft YaHei',sans-serif;background:linear-gradient(135deg,#5b6ee1 0%%,#8e5bd4 100%%);padding:28px;width:620px;color:#fff;border-radius:18px;box-sizing:border-box;">
<h2 style="text-align:center;margin:0 0 6px;font-size:26px;letter-spacing:4px;">🐟 摸鱼人日历 🐟</h2>
<p style="text-align:center;color:#ffd86e;margin:0 0 18px;font-size:16px;">今日宜划水 · 忌内卷</p>
<div style="background:rgba(255,255,255,0.14);border-radius:14px;padding:18px 20px;font-size:15px;line-height:1.9;">
<p style="margin:0;">📅 今天是 %d年%d月%d日，星期%s</p>
<p style="margin:0;">🌙 农历 %s</p>
<p style="margin:0;">%s</p>
<p style="margin:0;">%s</p>
<p style="margin:0;">🔥 今日金句：<br/>&nbsp;&nbsp;“%s”</p>
<p style="margin:10px 0 0;">🚩 今日群务：<br/>&nbsp;&nbsp;%s</p>
</div>
<p style="text-align:center;color:rgba(255,255,255,0.75);margin:14px 0 0;font-size:13px;">🤖 想我了？随时 @卷娘 🤖</p>
</div>`,
		t.Year(), int(t.Month()), t.Day(), weekdayCN[int(t.Weekday())],
		lunarStr, weekLine, holidayLine, quote, affairs)
}

// nextStatutoryHoliday 返回距离今天最近的未来法定假日名称与天数。
func nextStatutoryHoliday(t time.Time) (name string, days int) {
	best := -1
	for year := t.Year(); year <= t.Year()+1; year++ {
		for _, h := range holidaysByYear[year] {
			d := time.Date(year, time.Month(h.month), h.day, 0, 0, 0, 0, t.Location())
			if d.After(t) {
				diff := int(d.Sub(t).Hours() / 24)
				if best == -1 || diff < best {
					best = diff
					name = h.name
				}
			}
		}
	}
	return name, best
}

// fetchHitokoto 从一言 API 获取金句；失败时回退内置句子。
func fetchHitokoto(ctx context.Context) string {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://v1.hitokoto.cn/?encode=json", nil)
	if err != nil {
		return fallbackQuote()
	}
	resp, err := client.Do(req)
	if err != nil {
		return fallbackQuote()
	}
	defer resp.Body.Close()
	var data struct {
		Hitokoto string `json:"hitokoto"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil || strings.TrimSpace(data.Hitokoto) == "" {
		return fallbackQuote()
	}
	return data.Hitokoto
}

var fallbackQuotes = []string{
	"工作嘛，躺平是态度，摸鱼是技术，准时下班是原则。",
	"人生苦短，摸鱼及时；该摸就摸，绝不加班。",
	"今天也要元气满满地摸鱼呀，毕竟周末在前面等着我们！",
	"工位是租来的，快乐是自己的，摸鱼是对生活的基本尊重。",
	"不想上班，只想放假；既然放假还早，那就先摸一会儿鱼。",
}

// fallbackQuote 随机返回一条内置金句。
func fallbackQuote() string {
	return fallbackQuotes[rand.Intn(len(fallbackQuotes))]
}
