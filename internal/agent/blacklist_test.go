package agent

import (
	"context"
	"testing"

	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/core/acl"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newBlacklistTestHago 构造带真实 ACL（sqlite 内存）的最小 HagoCenter。
func newBlacklistTestHago(t *testing.T, rules []models.ACLRule) *HagoCenter {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.ACLRule{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	mgr := acl.NewACL(dao.NewACLDAO(db))
	ctx := context.Background()
	for _, r := range rules {
		if err := mgr.AddRule(ctx, &r); err != nil {
			t.Fatalf("add rule: %v", err)
		}
	}
	return &HagoCenter{ACL: mgr}
}

// TestFilterBlockedEventsNoAdminExemption 黑名单移除管理员豁免：
// 命中黑名单的管理员消息同样被丢弃（feat-reply-acl-plugin 语义）。
func TestFilterBlockedEventsNoAdminExemption(t *testing.T) {
	ctx := context.Background()
	chatArea := &models.ChatArea{ID: "area-1"}
	denyList := []models.ACLRule{{
		ChatAreaID: "area-1",
		Scope:      models.ACLScopeChat,
		Permission: models.ACLPermissionDeny,
		TargetType: models.ACLTargetList,
		UserIDs:    models.JSONSlice{"200"},
	}}
	h := newBlacklistTestHago(t, denyList)

	// 被 ban 的 QQ 号在 Admins 列表内 → 仍被丢弃（管理员不豁免）
	adminEv := adapter.Event{
		Admins:  []string{"200"},
		Message: &adapter.MessageEvent{UserID: 200, GroupID: 100, MessageType: "group"},
	}
	if kept := h.filterBlockedEvents(ctx, []adapter.Event{adminEv}, chatArea); len(kept) != 0 {
		t.Fatalf("黑名单内管理员消息应被丢弃，实际保留 %d 条", len(kept))
	}

	// 黑名单内普通用户 → 丢弃
	userEv := adapter.Event{
		Message: &adapter.MessageEvent{UserID: 200, GroupID: 100, MessageType: "group"},
	}
	if kept := h.filterBlockedEvents(ctx, []adapter.Event{userEv}, chatArea); len(kept) != 0 {
		t.Fatalf("黑名单内用户消息应被丢弃，实际保留 %d 条", len(kept))
	}
}

// TestFilterBlockedEventsAllowNormal 不在黑名单的用户消息保留。
func TestFilterBlockedEventsAllowNormal(t *testing.T) {
	ctx := context.Background()
	chatArea := &models.ChatArea{ID: "area-1"}
	h := newBlacklistTestHago(t, []models.ACLRule{{
		ChatAreaID: "area-1",
		Scope:      models.ACLScopeChat,
		Permission: models.ACLPermissionDeny,
		TargetType: models.ACLTargetList,
		UserIDs:    models.JSONSlice{"200"},
	}})

	ev := adapter.Event{
		Admins:  []string{"300"},
		Message: &adapter.MessageEvent{UserID: 300, GroupID: 100, MessageType: "group"},
	}
	if kept := h.filterBlockedEvents(ctx, []adapter.Event{ev}, chatArea); len(kept) != 1 {
		t.Fatalf("非黑名单用户消息应保留，实际保留 %d 条", len(kept))
	}

	// 其它 ChatArea 不受影响（规则按 chat_area_id 隔离）
	other := adapter.Event{
		Message: &adapter.MessageEvent{UserID: 200, GroupID: 100, MessageType: "group"},
	}
	otherArea := &models.ChatArea{ID: "area-2"}
	if kept := h.filterBlockedEvents(ctx, []adapter.Event{other}, otherArea); len(kept) != 1 {
		t.Fatalf("其它 ChatArea 不应受影响，实际保留 %d 条", len(kept))
	}
}
