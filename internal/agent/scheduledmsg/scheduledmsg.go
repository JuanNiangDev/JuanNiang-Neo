// Package scheduledmsg 定时消息：独立的调度器（不复用 CronJob 系统）。
//
// 一个任务含多段消息（text / image(t2i|url|图床) / face CQ 码），段间可自定义延迟。
// 触发后逐段发送，每段一条消息；图片段按来源渲染（t2i 用 T2I 服务生成 / url 直链 /
// 图床用 imgs:// 引用由发送层自动转 base64）。
package scheduledmsg

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"

	"github.com/robfig/cron/v3"
)

var log = logging.NewModule("schedmsg")

// Manager 定时消息调度器。
type Manager struct {
	mu      sync.Mutex
	cron    *cron.Cron
	entries map[string]cron.EntryID // 任务 ID → cron entry

	dao     *dao.ScheduledMessageDAO
	getT2I  func() *t2icaller.Client
	adapter *adapter.Adapter
}

// New 创建调度器。
func New(d *dao.ScheduledMessageDAO, getT2I func() *t2icaller.Client, adp *adapter.Adapter) *Manager {
	return &Manager{
		cron:    cron.New(cron.WithSeconds(), cron.WithLocation(time.Local)),
		entries: make(map[string]cron.EntryID),
		dao:     d, getT2I: getT2I, adapter: adp,
	}
}

// Reload 从 DB 重新加载所有启用的任务（先移除旧的再注册）。
func (m *Manager) Reload(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, eid := range m.entries {
		m.cron.Remove(eid)
		delete(m.entries, id)
	}

	tasks, err := m.dao.ListActive(ctx)
	if err != nil {
		log.Error("定时消息加载失败", "err", err)
		return
	}
	for i := range tasks {
		task := tasks[i]
		if len(task.Segments) == 0 {
			continue
		}
		eid, err := m.cron.AddFunc(task.CronExpr, func() {
			runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			if err := m.TriggerNow(runCtx, task.ID); err != nil {
				log.Error("定时消息执行失败", "name", task.Name, "err", err)
			} else {
				log.Info("定时消息执行完成", "name", task.Name, "segments", len(task.Segments))
			}
		})
		if err != nil {
			log.Error("定时消息 cron 注册失败", "name", task.Name, "cron_expr", task.CronExpr, "err", err)
			continue
		}
		m.entries[task.ID] = eid
		log.Info("定时消息已调度", "name", task.Name, "cron_expr", task.CronExpr, "target", task.TargetID, "segments", len(task.Segments))
	}
}

// Run 启动调度器并阻塞直到 ctx 结束。
func (m *Manager) Run(ctx context.Context) {
	m.Reload(ctx)
	m.cron.Start()
	log.Info("定时消息调度器已启动")
	<-ctx.Done()
	m.cron.Stop()
	log.Info("定时消息调度器已停止")
}

// TriggerNow 手动/定时触发单个任务：逐段发送，段间按配置延迟。
func (m *Manager) TriggerNow(ctx context.Context, id string) error {
	task, err := m.dao.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("任务不存在: %w", err)
	}
	if len(task.Segments) == 0 {
		return errors.New("任务没有配置消息段")
	}
	recordErr := func(runErr error) error {
		_ = m.dao.MarkRunResult(ctx, task.ID, time.Now(), runErr.Error())
		return runErr
	}

	for i := range task.Segments {
		seg := &task.Segments[i]
		msg, err := m.renderSegment(ctx, seg)
		if err != nil {
			return recordErr(fmt.Errorf("第 %d 段渲染失败: %w", i+1, err))
		}
		if err := m.send(task, msg); err != nil {
			return recordErr(fmt.Errorf("第 %d 段发送失败: %w", i+1, err))
		}
		log.Info("定时消息段已发送", "name", task.Name, "index", i+1, "type", seg.Type, "target", task.TargetID)
		// 段间延迟（最后一段之后不等待）
		if i < len(task.Segments)-1 && seg.DelaySeconds > 0 {
			select {
			case <-time.After(time.Duration(seg.DelaySeconds) * time.Second):
			case <-ctx.Done():
				return recordErr(ctx.Err())
			}
		}
	}
	_ = m.dao.MarkRunResult(ctx, task.ID, time.Now(), "")
	return nil
}

// renderSegment 把单个消息段渲染为 CQ 码/纯文本消息。
func (m *Manager) renderSegment(ctx context.Context, seg *models.ScheduledMessageSegment) (string, error) {
	switch seg.Type {
	case "text":
		if seg.Content == "" {
			return "", errors.New("文字内容为空")
		}
		return seg.Content, nil
	case "face":
		if seg.Content == "" {
			return "", errors.New("表情内容为空")
		}
		// Content 直接是 CQ 码文本，如 [CQ:face,id=66]
		return seg.Content, nil
	case "image":
		switch seg.Source {
		case "url":
			return fmt.Sprintf("[CQ:image,file=%s]", seg.Content), nil
		case "imgstore":
			// imgs:// 引用由 adapter 发送层自动解析为 base64
			return fmt.Sprintf("[CQ:image,file=%s]", seg.Content), nil
		case "t2i":
			t2i := m.getT2I()
			if t2i == nil {
				return "", errors.New("T2I 服务未启用，无法渲染图片")
			}
			img, err := t2i.GenerateImage(ctx, t2icaller.GenerateRequest{
				HTML: seg.Content,
				Options: &t2icaller.GenerateOptions{
					Type:    t2icaller.ImageTypeJPEG,
					Quality: 85,
				},
			})
			if err != nil {
				return "", fmt.Errorf("T2I 生成失败: %w", err)
			}
			return fmt.Sprintf("[CQ:image,file=base64://%s]", base64.StdEncoding.EncodeToString(img)), nil
		default:
			return "", fmt.Errorf("未知图片来源: %s", seg.Source)
		}
	default:
		return "", fmt.Errorf("未知消息段类型: %s", seg.Type)
	}
}

// send 按任务目标发送单段消息。
func (m *Manager) send(task *models.ScheduledMessage, msg string) error {
	if task.TargetType == "private" {
		_, err := m.adapter.SendPrivateMsg(task.TargetID, msg)
		return err
	}
	_, err := m.adapter.SendGroupMsg(task.TargetID, msg)
	return err
}
