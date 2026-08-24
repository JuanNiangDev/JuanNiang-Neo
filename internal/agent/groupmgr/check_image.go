package groupmgr

import (
	"context"
	"strconv"
	"strings"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/metrics"
)

// hasImage 消息中是否包含图片/表情（CQ 码或文本占位符）。
func hasImage(raw string) bool {
	return strings.Contains(raw, "[CQ:image") ||
		strings.Contains(raw, "[CQ:face") ||
		strings.Contains(raw, "[CQ:mface") ||
		strings.Contains(raw, "[图片]") ||
		strings.Contains(raw, "[表情]")
}

// checkImageSpam 图片刷屏检测：窗口内 ≥ 阈值触发警告；已警告仍刷 → 禁言。
// 参数（窗口/阈值/禁言时长）来自面板配置。返回 true = 已触发（消费消息）。
func (m *Manager) checkImageSpam(ctx context.Context, ev adapter.Event, cfg *models.GroupMgrConfig) bool {
	if !hasImage(ev.Message.RawMessage) {
		return false
	}
	window := cfg.ImgSpamWindow
	if window <= 0 {
		window = 2
	}
	threshold := cfg.ImgSpamThreshold
	if threshold <= 0 {
		threshold = 3
	}
	duration := cfg.ImgMuteDuration
	if duration <= 0 {
		duration = 60
	}
	groupID, userID := ev.Message.GroupID, ev.Message.UserID
	key := gkey(groupID, "ims:"+itoa(userID))
	now := time.Now().Unix()

	m.imgMu.Lock()
	times := m.imgState[key]
	cutoff := now - int64(window)
	recent := make([]int64, 0, len(times)+1)
	for _, ts := range times {
		if ts >= cutoff {
			recent = append(recent, ts)
		}
	}
	recent = append(recent, now)
	m.imgState[key] = recent
	warned := m.imgWarn[key]
	m.imgMu.Unlock()

	// kv 持久化兜底（重启不丢窗口）
	_ = m.dao.StatSet(ctx, key, joinInts(recent))

	if len(recent) >= threshold {
		if warned {
			// 已警告仍刷 → 禁言
			_ = m.adp.BanGroupMember(groupID, userID, duration)
			_, _ = m.dao.StatIncr(ctx, gkey(groupID, "stats:mute"))
			metrics.GroupMgrSpamTotal.WithLabelValues("image").Inc()
			log.Info("图片刷屏禁言", "user", userID, "group", groupID, "duration", duration)
			m.notifyAdmins(ev, itoa(userID)+" 因图片刷屏被禁言 "+strconv.FormatInt(int64(duration), 10)+"s")
			m.imgMu.Lock()
			delete(m.imgState, key)
			delete(m.imgWarn, key)
			m.imgMu.Unlock()
		} else {
			m.imgMu.Lock()
			m.imgWarn[key] = true
			m.imgMu.Unlock()
			m.replyGroupImage(ev, "做文明群友，杜绝刷屏哦！", imgShuapingB64)
			_, _ = m.dao.StatIncr(ctx, gkey(groupID, "stats:warn"))
		}
		return true
	}
	// 未达阈值：解除警告标记（重新计数）
	if len(recent) <= 1 {
		m.imgMu.Lock()
		delete(m.imgWarn, key)
		m.imgMu.Unlock()
	}
	return false
}

// restoreImgState 启动时从 kv 恢复图片刷屏窗口（内存态重建）。
func (m *Manager) restoreImgState(ctx context.Context) {
	list, err := m.dao.StatListPrefix(ctx, "")
	if err != nil {
		return
	}
	m.imgMu.Lock()
	defer m.imgMu.Unlock()
	for _, s := range list {
		if !strings.Contains(s.Key, ":ims:") {
			continue
		}
		times := parseInts(s.Value)
		if len(times) > 0 {
			m.imgState[s.Key] = times
		}
	}
}

func joinInts(v []int64) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = itoa(n)
	}
	return strings.Join(parts, ",")
}

func parseInts(s string) []int64 {
	var out []int64
	for _, p := range strings.Split(s, ",") {
		if p == "" {
			continue
		}
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}
