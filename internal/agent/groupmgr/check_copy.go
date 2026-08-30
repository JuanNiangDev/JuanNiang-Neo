package groupmgr

import (
	"context"
	"strings"
	"time"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/stats"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/metrics"
)

// checkCopySpam +1 复读检测：N 人连续发相同纯文本消息触发警告。
// 开关/阈值来自面板配置。返回 true = 已触发（消费消息）。
// 排除：命令消息（/ 前缀，插件/系统命令统一形态）不参与复读。
func (m *Manager) checkCopySpam(ctx context.Context, ev adapter.Event, cfg *models.GroupMgrConfig) bool {
	raw := strings.TrimSpace(ev.Message.RawMessage)
	// 命令消息（/qd、/签到 等插件/系统命令）不参与复读检测，
	// 否则群友连发同一条命令（如每日签到）会被误判刷屏
	if raw == "" || strings.HasPrefix(raw, "/") || strings.Contains(raw, "[CQ:") {
		return false
	}
	// 复读检测开关（面板配置）
	if !cfg.EnableCopyCheck {
		return false
	}
	threshold := cfg.CopyThreshold
	if threshold <= 0 {
		threshold = 3
	}
	groupID, userID := ev.Message.GroupID, ev.Message.UserID

	m.cpMu.Lock()
	defer m.cpMu.Unlock()
	st := m.cpState[groupID]
	if st == nil {
		m.cpState[groupID] = &copyState{lastMsg: raw, count: 1, users: map[int64]bool{userID: true}}
		return false
	}
	if st.lastMsg != raw {
		st.lastMsg = raw
		st.count = 1
		st.users = map[int64]bool{userID: true}
		st.trig = false
		return false
	}
	if st.users[userID] {
		return false // 同一用户不算复读
	}
	st.users[userID] = true
	st.count++
	if st.count < threshold {
		return false
	}
	if st.trig {
		return true // 已触发过警告，冷却中
	}
	st.trig = true
	const copyWarnText = "你们这群人机能不能别刷屏了"
	_, _ = m.adp.SendGroupMsg(groupID, []adapter.Segment{
		{Type: "text", Data: map[string]any{"text": copyWarnText}},
		{Type: "image", Data: map[string]any{"file": imgShuaping2B64}},
	})
	// 复读警告统计事件（Loki+Promtail 通道；无触发者单一身份，user_id 记 0）
	if m.stats != nil {
		if !m.stats.Emit(stats.Event{
			Timestamp: time.Now(),
			GroupID:   groupID,
			Direction: stats.DirectionReply,
			Source:    stats.SourceGroupMgr,
			Text:      stats.Truncate(copyWarnText, statsReplyTextMax),
			ReplyTo:   stats.Truncate(stripCQ(raw), statsReplyTextMax),
		}) {
			metrics.ChatStatsDroppedTotal.WithLabelValues("reply").Inc()
		}
	}
	_, _ = m.dao.StatIncr(ctx, gkey(groupID, "stats:copy_warn"))
	metrics.GroupMgrSpamTotal.WithLabelValues("copy").Inc()
	log.Info("复读触发", "group", groupID, "count", st.count)
	return true
}
