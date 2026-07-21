package cronjob

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
	"github.com/robfig/cron/v3"
)

// Manager 管理定时任务的生命周期与向 Agent 注入事件。
type Manager struct {
	cron      *cron.Cron
	dao       *dao.CronJobDAO
	eventChan chan adapter.Event // 向 Agent 事件循环发送合成事件

	mu      sync.RWMutex
	entries map[string]cron.EntryID // cronJobID → entryID
}

// New 创建 Manager。eventChan 用于将 cron 触发后的合成事件发送给 Agent 事件循环。
func New(d *dao.CronJobDAO, eventChan chan adapter.Event) *Manager {
	return &Manager{
		cron: cron.New(
			cron.WithSeconds(),
			cron.WithLocation(time.Local),
		),
		dao:       d,
		eventChan: eventChan,
		entries:   make(map[string]cron.EntryID),
	}
}

// Run 加载所有启用的定时任务并启动调度器。
// ctx 用于监听退出信号；直到 ctx 结束前本函数会 block。
func (m *Manager) Run(ctx context.Context) {
	m.reloadAll()
	m.cron.Start()
	slog.Info("CronJob 调度器已启动")

	<-ctx.Done()
	m.cron.Stop()
	slog.Info("CronJob 调度器已停止")
}

// Reload 重新从 DB 加载所有启用的任务，移除已停用/删除的，添加新启用的。
func (m *Manager) Reload(ctx context.Context) error {
	m.reloadAll()
	return nil
}

func (m *Manager) reloadAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 移除已有的所有 entry
	for _, eid := range m.entries {
		m.cron.Remove(eid)
	}
	m.entries = make(map[string]cron.EntryID)

	// 加载所有启用的任务
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jobs, err := m.dao.ListActive(ctx)
	if err != nil {
		slog.Error("CronJob: 加载任务失败", "err", err)
		return
	}

	for i := range jobs {
		job := jobs[i]
		// 注册 cron entry
		fn := m.makeJobFunc(&job)
		eid, err := m.cron.AddFunc(job.CronExpr, fn)
		if err != nil {
			slog.Error("CronJob: 注册任务失败", "name", job.Name, "cron_expr", job.CronExpr, "err", err)
			continue
		}
		m.entries[job.ID] = eid
		slog.Info("CronJob 任务已注册", "name", job.Name, "cron_expr", job.CronExpr, "msg_type", job.MessageType, "target", job.TargetID)
	}
}

// makeJobFunc 返回一个闭包：触发时构造 Event 并注入到 Agent 事件循环。
func (m *Manager) makeJobFunc(job *models.CronJob) func() {
	return func() {
		slog.Info("CronJob 触发", "name", job.Name, "msg_type", job.MessageType, "target", job.TargetID)

		// 更新最后执行时间
		if err := m.dao.UpdateLastRun(context.Background(), job.ID, time.Now(), ""); err != nil {
			slog.Warn("CronJob: 更新 last_run_at 失败", "name", job.Name, "err", err)
		}

		// 构造 MessageEvent（模拟 OneBot11 消息事件）
		msg := &adapter.MessageEvent{
			MessageType: job.MessageType,
			RawMessage:  job.Message,
		}

		if job.MessageType == "group" {
			msg.GroupID = job.TargetID
		} else {
			msg.UserID = job.TargetID
		}

		ev := adapter.Event{
			PostType:    "cronjob",
			IsCronJob:   true,
			Message:     msg,
			Time:        time.Now().Unix(),
		}

		// 非阻塞发送到 Agent 事件循环
		select {
		case m.eventChan <- ev:
			slog.Info("CronJob 事件已注入 Agent 事件循环", "name", job.Name)
		default:
			slog.Warn("CronJob: 事件通道已满，丢弃事件", "name", job.Name)
		}
	}
}
