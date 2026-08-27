// Package stats 群消息 / Agent 回复统计事件写入器。
//
// 用途：为 Loki + Promtail 提供「每个群的消息与对应回复」明细统计。
// 关键设计——独立于主日志 pipeline：
//   - 不走 slog / internal/logging（主日志带 ANSI 颜色 + Web Hub 推送，不适合采集）；
//   - 事件以 NDJSON 逐行追加到独立文件（默认 data/stats/chat-events.log），
//     Promtail 单独配置一个 scrape job 采集，互不干扰；
//   - 文件轮转由 lumberjack 负责（按大小 + 保留份数 + gzip 压缩），
//     Promtail 通过 __path__ 通配 + inode 跟随无缝衔接。
//
// 写入异步化：Emit 仅投递到 channel（非阻塞），后台 goroutine 攒批写文件，
// 不阻塞消息处理主循环；队列满时丢弃并计数（dropped），绝不影响主流程。
package stats

import (
	"bufio"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Direction 事件方向。
const (
	DirectionMsg   = "msg"   // 群消息
	DirectionReply = "reply" // Agent/群管理回复
)

// Source 回复来源（direction=reply 时区分谁发出的）。
const (
	SourceAgent    = "agent"    // Agent 回复（sendReply）
	SourceGroupMgr = "groupmgr" // 群管理处罚/刷屏/复读警告
)

// Event 一条统计事件（NDJSON 一行，字段与 Promtail json 解析对齐）。
type Event struct {
	Timestamp time.Time `json:"ts"`
	GroupID   int64     `json:"group_id"`
	UserID    int64     `json:"user_id"`
	MessageID int64     `json:"message_id,omitempty"`
	Direction string    `json:"direction"`          // msg / reply
	Source    string    `json:"source,omitempty"`   // 回复来源（agent/groupmgr…）；消息方向可空
	Text      string    `json:"text"`               // 消息原文 / 回复内容（截断由调用方控制）
	ReplyTo   string    `json:"reply_to,omitempty"` // reply 事件携带触发消息原文（对应关系）
}

// Config 写入器配置。
type Config struct {
	Enabled    bool   // 总开关（默认 false）
	Path       string // NDJSON 文件路径（默认 data/stats/chat-events.log）
	MaxSizeMB  int    // 单文件轮转大小 MB（默认 100）
	MaxBackups int    // 保留归档份数（默认 10）
	MaxAgeDays int    // 归档保留天数（默认 7）
	QueueSize  int    // 异步队列容量（默认 1024）
}

// defaultPath 默认统计文件路径。
const defaultPath = "data/stats/chat-events.log"

// Writer 异步统计事件写入器（Loki+Promtail 专用通道，独立于主日志 pipeline）。
type Writer struct {
	cfg  Config
	ch   chan Event
	w    *bufio.Writer
	file *lumberjack.Logger
	wg   sync.WaitGroup

	closed  atomic.Bool
	dropped atomic.Uint64
}

// New 创建写入器（未启动；调用方需 Start 开启后台写盘 goroutine）。
func New(cfg Config) *Writer {
	if cfg.Path == "" {
		cfg.Path = defaultPath
	}
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 100
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = 10
	}
	if cfg.MaxAgeDays <= 0 {
		cfg.MaxAgeDays = 7
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	return &Writer{
		cfg: cfg,
		ch:  make(chan Event, cfg.QueueSize),
		file: &lumberjack.Logger{
			Filename:   cfg.Path,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   true,
		},
	}
}

// Start 启动后台写盘 goroutine。
func (w *Writer) Start() {
	w.wg.Add(1)
	go w.run()
}

// run 消费事件队列：bufio 攒批，每批或定时 flush（lumberjack 负责轮转）。
func (w *Writer) run() {
	defer w.wg.Done()
	w.w = bufio.NewWriterSize(w.file, 64*1024)
	flush := time.NewTicker(2 * time.Second)
	defer flush.Stop()
	for {
		select {
		case ev, ok := <-w.ch:
			if !ok {
				w.flush()
				return
			}
			w.write(ev)
		case <-flush.C:
			w.flush()
		}
	}
}

// write 单条事件写入缓冲（失败计数，不阻塞）。
func (w *Writer) write(ev Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		w.dropped.Add(1)
		return
	}
	if _, err := w.w.Write(append(b, '\n')); err != nil {
		w.dropped.Add(1)
	}
}

// flush 冲刷缓冲到文件。
func (w *Writer) flush() {
	if w.w == nil {
		return
	}
	if err := w.w.Flush(); err != nil {
		w.dropped.Add(1)
	}
}

// Truncate 规范化统计文本：折叠空白 + 截断到 max rune（超长加省略号）。
// 各埋点统一调用，避免超长消息撑爆 Loki 单行。
func Truncate(s string, max int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// Emit 投递一条事件（非阻塞）。返回是否成功入队；队列满/已关闭返回 false（内部已计数）。
func (w *Writer) Emit(ev Event) bool {
	if w == nil || w.closed.Load() {
		return false
	}
	select {
	case w.ch <- ev:
		return true
	default:
		w.dropped.Add(1)
		return false
	}
}

// Dropped 返回被丢弃的事件数（队列满 / 写入失败）。nil 接收者返回 0。
func (w *Writer) Dropped() uint64 {
	if w == nil {
		return 0
	}
	return w.dropped.Load()
}

// Close 冲刷缓冲并关闭后台 goroutine（幂等）。
func (w *Writer) Close() {
	if w == nil || !w.closed.CompareAndSwap(false, true) {
		return
	}
	close(w.ch)
	w.wg.Wait()
	_ = w.file.Close()
}
