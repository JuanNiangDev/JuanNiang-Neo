// Package fishcal 摸鱼人日历：独立的每日定时任务（不复用 CronJob 系统）。
//
// 流程：每天按配置的 cron 表达式触发 → 用模板组装日历内容（日期/农历/本周进度/
// 法定假日倒计时/金句/群务）→ 通过 T2I 服务渲染成图片 → 发送到目标群。
package fishcal

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
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

//go:embed fonts/lxgwwenkai-regular.woff2
var wenkaiFonts embed.FS

// 内嵌霞鹜文楷 woff2（全量汉字 U+4E00-9FFF + 西文/全角/CJK 标点），以 data URI 注入模板，
// T2I 渲染器无需外网或共享文件系统即可使用楷体；子集外字形回退 font-family 后续字体。

// buildFontFaces 返回 @font-face 定义（base64 data URI）。
func buildFontFaces() string {
	b, err := wenkaiFonts.ReadFile("fonts/lxgwwenkai-regular.woff2")
	if err != nil {
		return ""
	}
	return "@font-face{font-family:'LXGW WenKai';font-weight:400;font-style:normal;" +
		"src:url(data:font/woff2;base64," + base64.StdEncoding.EncodeToString(b) + ")}"
}

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
			log.Info("摸鱼日历发送成功", "groups", strings.Join(cfg.TargetGroups, ","))
		}
	})
	if err != nil {
		log.Error("摸鱼日历 cron 注册失败", "expr", cfg.CronExpr, "err", err)
		return
	}
	s.entryID = eid
	log.Info("摸鱼日历已调度", "expr", cfg.CronExpr, "groups", strings.Join(cfg.TargetGroups, ","))
}

// Run 启动调度器并阻塞直到 ctx 结束（启动前先加载一次配置）。
func (s *Scheduler) Run(ctx context.Context) {
	s.Reload(ctx)
	s.cron.Start()
	log.Info("摸鱼日历调度器已启动")
	<-ctx.Done()
	s.cron.Stop()
	log.Info("摸鱼日历调度器已停止")
}

// TriggerNow 立即执行一次：生成日历图片并发送到所有目标群（Web 手动触发与 cron 共用）。
func (s *Scheduler) TriggerNow(ctx context.Context) error {
	cfg, err := s.dao.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	recordErr := func(runErr error) error {
		_ = s.dao.MarkRunResult(ctx, time.Now(), runErr.Error())
		return runErr
	}
	groups := parseGroups(cfg.TargetGroups)
	if len(groups) == 0 {
		return recordErr(errors.New("未配置目标群"))
	}
	t2i := s.getT2I()
	if t2i == nil {
		return recordErr(errors.New("T2I 服务未启用，无法生成图片"))
	}

	// 当天群务（无配置时用默认提示）
	now := time.Now()
	affair := "今日无群务安排，安心摸鱼～"
	if a, err := s.dao.AffairGet(ctx, now.Format("2006-01-02")); err == nil && a != nil {
		affair = a.Content
	}

	html := buildCalendarHTML(now, affair)
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

	// 富文本消息：文案 + 图片（不 @全体成员）
	b64 := "base64://" + base64.StdEncoding.EncodeToString(img)
	msg := fmt.Sprintf("今日份摸鱼人日历来了~\n[CQ:image,file=%s]", b64)
	for _, gid := range groups {
		if _, err := s.adapter.SendGroupMsg(gid, msg); err != nil {
			log.Error("摸鱼日历发送群失败", "group", gid, "err", err)
			return recordErr(fmt.Errorf("发送群 %d 失败: %w", gid, err))
		}
		log.Info("摸鱼日历已发送到群", "group", gid)
	}
	_ = s.dao.MarkRunResult(ctx, now, "")
	return nil
}

// parseGroups 把 JSONSlice（字符串群号）解析为 int64 列表。
func parseGroups(groups []string) []int64 {
	out := make([]int64, 0, len(groups))
	seen := make(map[int64]struct{})
	for _, g := range groups {
		gid, err := strconv.ParseInt(strings.TrimSpace(g), 10, 64)
		if err != nil || gid == 0 {
			continue
		}
		if _, ok := seen[gid]; ok {
			continue
		}
		seen[gid] = struct{}{}
		out = append(out, gid)
	}
	return out
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

// solarMonthCN 公历月份中文（1-12 月 → 一~十二）。
var solarMonthCN = []string{"一", "二", "三", "四", "五", "六", "七", "八", "九", "十", "十一", "十二"}

// buildProgressSegs 生成周进度 5 格横条（已过实心，未过空心）。
func buildProgressSegs(passed int) string {
	var b strings.Builder
	for i := 1; i <= 5; i++ {
		if i <= passed {
			b.WriteString(`<span class="seg"></span>`)
		} else {
			b.WriteString(`<span class="seg empty"></span>`)
		}
	}
	return b.String()
}

// buildCalendarHTML 组装摸鱼日历 HTML 模板（682×757，纸张朱印质感 + 洛谷式日历卡，T2I 渲染成图片）。
// 楷体以 woff2 data URI 内嵌（fonts/lxgwwenkai-regular.woff2），渲染器无需外网；子集外字形回退 KaiTi/SimSun。
func buildCalendarHTML(t time.Time, affair string) string {
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
	progressTxt := fmt.Sprintf("再坚持 %d 天就周末啦！", 5-passed)
	if passed >= 5 {
		progressTxt = "今天就是周末啦！"
	}
	weekendLine := fmt.Sprintf("距周末还有 %d 天", 5-passed)

	// 最近法定假日
	holidayName, holidayDays := nextStatutoryHoliday(t)
	holidayLine := fmt.Sprintf("距%s还有 %d 天", holidayName, holidayDays)
	if holidayName == "" {
		holidayLine = "暂无后续法定假日数据，专心摸鱼吧！"
	}

	// 金句
	quote := fetchHitokoto(context.Background())

	affair = strings.TrimSpace(affair)
	if affair == "" {
		affair = "今日无群务安排，安心摸鱼～"
	}

	// 纸张朱印质感：米白底 + 墨色 + 唯一朱红强调 + 楷体；日历卡采用洛谷式「月/超大日/星期」纵向堆叠。
	// 字体以 data URI 内嵌（buildFontFaces），渲染器无需外网；子集外字形回退 KaiTi/SimSun。
	return fmt.Sprintf(`<meta charset="utf-8">
<style>
%s
html,body{margin:0;padding:0;}
.page{position:relative;width:682px;height:757px;box-sizing:border-box;background:#f7f4ec;overflow:hidden;font-family:'LXGW WenKai','KaiTi','STKaiti','SimSun',serif;color:#2b2b2b;}
.frame{position:absolute;inset:22px;border:1px solid #2b2b2b;padding:26px 40px 18px;display:flex;flex-direction:column;}
.header{text-align:center;}
h1{margin:0;font-size:46px;font-weight:700;letter-spacing:12px;}
.seals{display:flex;justify-content:center;gap:30px;margin-top:12px;}
.seal{background:#c0392b;color:#f7f4ec;font-size:22px;font-weight:700;letter-spacing:4px;padding:5px 18px;border-radius:5px;}
.seal.a{transform:rotate(-4deg);}
.seal.b{transform:rotate(3deg);}
.rule{margin:12px 0 12px;border-top:1px solid #2b2b2b;}
.cal-card{margin:0 auto;border:1px solid #2b2b2b;border-top:4px solid #c0392b;padding:10px 30px 8px;text-align:center;}
.cal-month{font-size:22px;font-weight:700;letter-spacing:8px;}
.cal-day{font-size:118px;font-weight:700;line-height:1.06;margin:0;}
.cal-week{font-size:22px;font-weight:700;letter-spacing:6px;}
.cal-lines{margin-top:10px;}
.cal-lines div{font-size:19px;color:#8a8578;line-height:1.6;}
.mid{flex:1;display:flex;flex-direction:column;justify-content:space-evenly;padding:6px 2px 0;}
.row-label{font-size:20px;color:#8a8578;letter-spacing:3px;}
.progress{display:flex;align-items:center;gap:10px;margin-top:8px;}
.seg{width:34px;height:16px;background:#2b2b2b;}
.seg.empty{background:transparent;border:1px solid #2b2b2b;}
.progress .txt{font-size:26px;font-weight:700;color:#c0392b;margin-left:8px;}
.quote p,.affair p{margin:4px 0 0 22px;font-size:23px;line-height:1.5;}
.footer{text-align:center;border-top:1px solid #2b2b2b;padding-top:9px;font-size:18px;color:#8a8578;letter-spacing:5px;}
</style>
<div class="page">
<div class="frame">
<div class="header">
<h1>摸鱼人日历</h1>
<div class="seals">
<span class="seal a">宜划水</span>
<span class="seal b">忌内卷</span>
</div>
</div>
<div class="rule"></div>
<div class="cal-card">
<div class="cal-month">%s月</div>
<div class="cal-day">%s</div>
<div class="cal-week">星期%s · 农历%s</div>
<div class="cal-lines">
<div>%s</div>
<div>%s</div>
</div>
</div>
<div class="mid">
<div>
<div class="row-label">本周进度</div>
<div class="progress">
%s
<span class="txt">%s</span>
</div>
</div>
<div class="quote">
<div class="row-label">今日金句</div>
<p>“%s”</p>
</div>
<div class="affair">
<div class="row-label">今日群务</div>
<p>%s</p>
</div>
</div>
<div class="footer">想我了？随时 @卷娘</div>
</div>
</div>`,
		buildFontFaces(),
		solarMonthCN[int(t.Month())-1], fmt.Sprintf("%02d", t.Day()),
		weekdayCN[int(t.Weekday())], lunarStr,
		weekendLine, holidayLine,
		buildProgressSegs(passed), progressTxt,
		quote, affair)
}

// nextStatutoryHoliday 返回距离今天最近的未来法定假日名称与天数。
func nextStatutoryHoliday(t time.Time) (name string, days int) {
	// 按日期归一化再求差，避免当天时间戳导致的截断误差（如 09:00 触发少算 1 天）。
	today := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	best := -1
	for year := t.Year(); year <= t.Year()+1; year++ {
		for _, h := range holidaysByYear[year] {
			d := time.Date(year, time.Month(h.month), h.day, 0, 0, 0, 0, t.Location())
			if d.After(today) {
				diff := int(d.Sub(today).Hours() / 24)
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
	return fallbackQuotes[rand.IntN(len(fallbackQuotes))]
}
