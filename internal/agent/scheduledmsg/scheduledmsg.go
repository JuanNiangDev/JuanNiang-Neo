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
	"strings"
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
		if len(task.Blocks) == 0 {
			continue
		}
		eid, err := m.cron.AddFunc(task.CronExpr, func() {
			runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			if err := m.TriggerNow(runCtx, task.ID); err != nil {
				log.Error("定时消息执行失败", "name", task.Name, "err", err)
			} else {
				log.Info("定时消息执行完成", "name", task.Name, "blocks", len(task.Blocks))
			}
		})
		if err != nil {
			log.Error("定时消息 cron 注册失败", "name", task.Name, "cron_expr", task.CronExpr, "err", err)
			continue
		}
		m.entries[task.ID] = eid
		log.Info("定时消息已调度", "name", task.Name, "cron_expr", task.CronExpr, "target", task.TargetID, "blocks", len(task.Blocks))
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

// TriggerNow 手动/定时触发单个任务：沿块链顺序执行（消息块发一条消息，延时块等待）。
func (m *Manager) TriggerNow(ctx context.Context, id string) error {
	task, err := m.dao.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("任务不存在: %w", err)
	}
	if len(task.Blocks) == 0 {
		return errors.New("任务没有配置编排块")
	}
	recordErr := func(runErr error) error {
		_ = m.dao.MarkRunResult(ctx, task.ID, time.Now(), runErr.Error())
		return runErr
	}

	for i := range task.Blocks {
		block := &task.Blocks[i]
		switch block.Type {
		case "delay":
			if block.DelaySeconds <= 0 {
				continue
			}
			log.Info("定时消息延时等待", "name", task.Name, "seconds", block.DelaySeconds)
			select {
			case <-time.After(time.Duration(block.DelaySeconds) * time.Second):
			case <-ctx.Done():
				return recordErr(ctx.Err())
			}
		case "message":
			if len(block.Segments) == 0 {
				return recordErr(fmt.Errorf("第 %d 块（消息）没有内容", i+1))
			}
			msg, err := m.renderMessage(ctx, block.Segments)
			if err != nil {
				return recordErr(fmt.Errorf("第 %d 块渲染失败: %w", i+1, err))
			}
			if err := m.send(task, msg); err != nil {
				return recordErr(fmt.Errorf("第 %d 块发送失败: %w", i+1, err))
			}
			log.Info("定时消息块已发送", "name", task.Name, "block", i+1, "segments", len(block.Segments), "target", task.TargetID)
		default:
			return recordErr(fmt.Errorf("第 %d 块未知类型: %s", i+1, block.Type))
		}
	}
	_ = m.dao.MarkRunResult(ctx, task.ID, time.Now(), "")
	return nil
}

// renderMessage 把一个消息块的所有段拼成一条富文本消息（CQ 码字符串）。
//
// 每个消息段（text/face/image）独占一行：除第一段外，每段前自动插入换行符。
// 用户在 Web 后台配多个段时，意图是分行显示，无需手动在段内容里加 \n。
func (m *Manager) renderMessage(ctx context.Context, segs models.ScheduledSegments) (string, error) {
	var sb strings.Builder
	for i := range segs {
		seg := &segs[i]
		// 除第一段外，每段前加换行（一个消息段就是一行）
		if i > 0 {
			sb.WriteString("\n")
		}
		switch seg.Type {
		case "text":
			if seg.Content == "" {
				return "", errors.New("文字内容为空")
			}
			sb.WriteString(seg.Content)
		case "face":
			if seg.Content == "" {
				return "", errors.New("表情内容为空")
			}
			sb.WriteString(seg.Content) // CQ 码，如 [CQ:face,id=66]
		case "image":
			switch seg.Source {
			case "url":
				sb.WriteString(fmt.Sprintf("[CQ:image,file=%s]", seg.Content))
			case "imgstore":
				// imgs:// 引用由 adapter 发送层自动解析为 base64
				sb.WriteString(fmt.Sprintf("[CQ:image,file=%s]", seg.Content))
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
				sb.WriteString(fmt.Sprintf("[CQ:image,file=base64://%s]", base64.StdEncoding.EncodeToString(img)))
			default:
				return "", fmt.Errorf("未知图片来源: %s", seg.Source)
			}
		default:
			return "", fmt.Errorf("未知消息段类型: %s", seg.Type)
		}
	}
	return sb.String(), nil
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
