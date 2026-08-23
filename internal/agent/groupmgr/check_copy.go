package groupmgr

import (
	"context"
	"strings"

	"JuanNiang-Neo/internal/adapter"
)

// checkCopySpam +1 复读检测：N 人连续发相同纯文本消息触发警告。
// 返回 true = 已触发（消费消息）。
func (m *Manager) checkCopySpam(ctx context.Context, ev adapter.Event) bool {
	raw := strings.TrimSpace(ev.Message.RawMessage)
	if raw == "" || strings.Contains(raw, "[CQ:") {
		return false
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
	if st.count < copyThreshold {
		return false
	}
	if st.trig {
		return true // 已触发过警告，冷却中
	}
	st.trig = true
	_, _ = m.adp.SendGroupMsg(groupID, []adapter.Segment{
		{Type: "text", Data: map[string]any{"text": "你们这群人机能不能别刷屏了"}},
		{Type: "image", Data: map[string]any{"file": imgShuaping2B64}},
	})
	_, _ = m.dao.StatIncr(ctx, gkey(groupID, "stats:copy_warn"))
	log.Info("复读触发", "group", groupID, "count", st.count)
	return true
}
