package agent

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
)

// TestRefreshToolAdminOnly 验证运行时权限刷新会返回查询错误，且失败时不会清空原有名单。
// 查询恢复后重新加载数据库配置，使新增和取消的管理员限制均能生效。
func TestRefreshToolAdminOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.ToolConfig{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	h := &HagoCenter{DAO: dao.NewBundle(db)}
	oldTool := &models.ToolConfig{Name: "old", AdminOnly: true}
	newTool := &models.ToolConfig{Name: "new", AdminOnly: true}
	if err := h.DAO.ToolConfig.Create(ctx, oldTool); err != nil {
		t.Fatal(err)
	}
	if err := h.RefreshToolAdminOnly(ctx); err != nil || !h.isToolAdminOnly("old") {
		t.Fatalf("initial refresh failed: %v", err)
	}
	if err := h.DAO.ToolConfig.SetAdminOnly(ctx, oldTool.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := h.DAO.ToolConfig.Create(ctx, newTool); err != nil {
		t.Fatal(err)
	}
	queryErr := errors.New("permission query unavailable")
	if err := db.Callback().Query().Before("gorm:query").Register("test:permission_query_failure", func(tx *gorm.DB) {
		tx.AddError(queryErr)
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.RefreshToolAdminOnly(ctx); !errors.Is(err, queryErr) {
		t.Fatalf("refresh did not propagate query error: %v", err)
	}
	if !h.isToolAdminOnly("old") || h.isToolAdminOnly("new") {
		t.Fatal("failed refresh replaced cached permissions")
	}
	if err := db.Callback().Query().Remove("test:permission_query_failure"); err != nil {
		t.Fatal(err)
	}
	if err := h.RefreshToolAdminOnly(ctx); err != nil {
		t.Fatalf("refresh did not recover: %v", err)
	}
	if h.isToolAdminOnly("old") || !h.isToolAdminOnly("new") {
		t.Fatal("recovered refresh did not replace cached permissions")
	}
}
