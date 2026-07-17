package acl

import (
	"context"
	"testing"

	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testACL(t *testing.T) *ACL {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.AutoMigrate(&models.ACLRule{})
	return NewACL(dao.NewACLDAO(db))
}

func TestACL_DefaultAllow(t *testing.T) {
	acl := testACL(t)
	ctx := context.Background()

	if !acl.Check(ctx, 123, "area-1", "chat") {
		t.Error("default should allow")
	}
}

func TestACL_Deny(t *testing.T) {
	acl := testACL(t)
	ctx := context.Background()

	rule := &models.ACLRule{
		UserID:     123,
		ChatAreaID: "area-1",
		Permission: models.ACLPermissionDenied,
	}
	if err := acl.AddRule(ctx, rule); err != nil {
		t.Fatal(err)
	}

	if acl.Check(ctx, 123, "area-1", "chat") {
		t.Error("should be denied")
	}

	if !acl.Check(ctx, 456, "area-1", "chat") {
		t.Error("other user should still be allowed")
	}
}

func TestACL_AllowWithActions(t *testing.T) {
	acl := testACL(t)
	ctx := context.Background()

	rule := &models.ACLRule{
		UserID:     123,
		ChatAreaID: "area-2",
		Permission: models.ACLPermissionAllowed,
		Actions:    []string{"chat", "tool"},
	}
	acl.AddRule(ctx, rule)

	if !acl.Check(ctx, 123, "area-2", "chat") {
		t.Error("chat action should be allowed")
	}
	if !acl.Check(ctx, 123, "area-2", "tool") {
		t.Error("tool action should be allowed")
	}
	if acl.Check(ctx, 123, "area-2", "admin") {
		t.Error("admin action should not be in allow list")
	}
}

func TestACL_RemoveRule(t *testing.T) {
	acl := testACL(t)
	ctx := context.Background()

	rule := &models.ACLRule{UserID: 1, ChatAreaID: "z", Permission: models.ACLPermissionDenied}
	acl.AddRule(ctx, rule)

	if !acl.Check(ctx, 1, "z", "chat") {
		_ = "denied"
	}

	acl.RemoveRule(ctx, rule.ID)

	if !acl.Check(ctx, 1, "z", "chat") {
		t.Error("should be allowed after removing rule")
	}
}
