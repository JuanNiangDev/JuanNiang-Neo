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

	if !acl.Check(ctx, 123, "area-1", models.ACLScopeChat) {
		t.Error("default should allow")
	}
}

func TestACL_DenyAll(t *testing.T) {
	acl := testACL(t)
	ctx := context.Background()

	rule := &models.ACLRule{
		ChatAreaID: "area-1",
		Scope:      models.ACLScopeChat,
		Permission: models.ACLPermissionDeny,
		TargetType: models.ACLTargetAll,
	}
	if err := acl.AddRule(ctx, rule); err != nil {
		t.Fatal(err)
	}

	if acl.Check(ctx, 123, "area-1", models.ACLScopeChat) {
		t.Error("should be denied for all users")
	}
	if acl.Check(ctx, 456, "area-1", models.ACLScopeChat) {
		t.Error("should be denied for all users")
	}
}

func TestACL_DenyList(t *testing.T) {
	acl := testACL(t)
	ctx := context.Background()

	rule := &models.ACLRule{
		ChatAreaID: "area-1",
		Scope:      models.ACLScopeChat,
		Permission: models.ACLPermissionDeny,
		TargetType: models.ACLTargetList,
		UserIDs:    models.JSONSlice{"123"},
	}
	acl.AddRule(ctx, rule)

	if acl.Check(ctx, 123, "area-1", models.ACLScopeChat) {
		t.Error("user 123 should be denied")
	}
	if !acl.Check(ctx, 456, "area-1", models.ACLScopeChat) {
		t.Error("user 456 should still be allowed")
	}
}

func TestACL_AllowList(t *testing.T) {
	acl := testACL(t)
	ctx := context.Background()

	rule := &models.ACLRule{
		ChatAreaID: "area-2",
		Scope:      models.ACLScopeTool,
		Permission: models.ACLPermissionAllow,
		TargetType: models.ACLTargetList,
		UserIDs:    models.JSONSlice{"123"},
	}
	acl.AddRule(ctx, rule)

	if !acl.Check(ctx, 123, "area-2", models.ACLScopeTool) {
		t.Error("user 123 should be allowed")
	}
	if acl.Check(ctx, 456, "area-2", models.ACLScopeTool) {
		t.Error("user 456 should be denied (whitelist)")
	}
}

func TestACL_RemoveRule(t *testing.T) {
	acl := testACL(t)
	ctx := context.Background()

	rule := &models.ACLRule{
		ChatAreaID: "z",
		Scope:      models.ACLScopeChat,
		Permission: models.ACLPermissionDeny,
		TargetType: models.ACLTargetAll,
	}
	acl.AddRule(ctx, rule)

	if acl.Check(ctx, 1, "z", models.ACLScopeChat) {
		t.Error("should be denied")
	}

	acl.RemoveRule(ctx, rule.ID)

	if !acl.Check(ctx, 1, "z", models.ACLScopeChat) {
		t.Error("should be allowed after removing rule")
	}
}
