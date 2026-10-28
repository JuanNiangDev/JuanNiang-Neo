package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"JuanNiang-Neo/internal/agent/tool"
	"JuanNiang-Neo/internal/api/dto"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/openai/openai-go/v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type permissionTestTool struct{}

func (permissionTestTool) ID() string                                               { return "fixture" }
func (permissionTestTool) Name() string                                             { return "fixture" }
func (permissionTestTool) Description() string                                      { return "test" }
func (permissionTestTool) Parameters() openai.FunctionParameters                    { return nil }
func (permissionTestTool) Execute(context.Context, json.RawMessage) (string, error) { return "", nil }
func (permissionTestTool) IsBuiltin() bool                                          { return true }
func (permissionTestTool) IsLongRunning() bool                                      { return false }

// TestToolPermissionPersistsByReturnedID 验证内置工具首次保存后，使用接口返回的数据库 ID
// 继续修改权限仍能正确持久化，并同步刷新运行时权限。
// 同时覆盖自定义工具、无效标识和数据库异常，防止后续更新误创建工具配置记录。
func TestToolPermissionPersistsByReturnedID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = db.AutoMigrate(&models.ToolConfig{}); err != nil {
		t.Fatal(err)
	}
	s := &Service{DAO: dao.NewBundle(db), ToolRegistry: tool.NewToolRegistry()}
	s.ToolRegistry.Register(permissionTestTool{})
	refreshed := 0
	s.OnUpdateToolAdminOnly = func() error { refreshed++; return nil }
	h := server.New()
	h.PUT("/tools/:id/admin-only", s.UpdateToolAdminOnly)
	h.GET("/tools", s.ListTools)
	ctx := context.Background()
	id := "builtin:fixture"
	// First creation, then repeated true/false updates using the ID returned by GET.
	for i, value := range []bool{true, false, true, false} {
		body := `{"admin_only":false}`
		if value {
			body = `{"admin_only":true}`
		}
		res := ut.PerformRequest(h.Engine, "PUT", "/tools/"+id+"/admin-only", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})
		var envelope struct {
			Status int `json:"status"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil || envelope.Status != 0 {
			t.Fatalf("update: %s", res.Body.String())
		}
		list := ut.PerformRequest(h.Engine, "GET", "/tools", nil)
		var got struct {
			Data []struct {
				ID        string `json:"id"`
				AdminOnly bool   `json:"admin_only"`
			} `json:"data"`
		}
		if err := json.Unmarshal(list.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Data) != 1 || got.Data[0].AdminOnly != value {
			t.Fatalf("list after %d: %s", i, list.Body.String())
		}
		id = got.Data[0].ID
		if strings.HasPrefix(id, "builtin:") {
			t.Fatal("ID not persisted")
		}
		runtime, err := s.DAO.ToolConfig.ListAdminOnly(ctx)
		if err != nil || runtime["fixture"] != value || refreshed != i+1 {
			t.Fatal("runtime permissions not refreshed", runtime, err)
		}
	}
	// Custom tools also use their actual ID. Invalid IDs/names never create rows.
	custom := &models.ToolConfig{Name: "custom", Description: "fixture"}
	if err = s.DAO.ToolConfig.Create(ctx, custom); err != nil {
		t.Fatal(err)
	}
	if err = s.setToolAdminOnly(ctx, custom.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"fixture", "missing-id", "builtin:missing", "builtin:custom"} {
		if err = s.setToolAdminOnly(ctx, bad, true); err == nil {
			t.Fatal("accepted unknown tool", bad)
		}
	}
	rows, _ := s.DAO.ToolConfig.List(ctx)
	if len(rows) != 2 {
		t.Fatal("created phantom tools", len(rows))
	}
	// Database errors must propagate rather than falling through to creation.
	sqlDB.Close()
	if err = s.setToolAdminOnly(ctx, "builtin:fixture", true); err == nil {
		t.Fatal("ignored DB failure")
	}
}

// TestToolPermissionRefreshFailure 验证数据库写入成功但运行时刷新失败时，接口返回错误。
// 刷新恢复后可使用同一标识重试，且不会重复创建工具配置。
func TestToolPermissionRefreshFailure(t *testing.T) {
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
	s := &Service{DAO: dao.NewBundle(db), ToolRegistry: tool.NewToolRegistry()}
	s.ToolRegistry.Register(permissionTestTool{})
	refreshErr := errors.New("runtime refresh unavailable")
	refreshCalls := 0
	s.OnUpdateToolAdminOnly = func() error {
		refreshCalls++
		return refreshErr
	}
	h := server.New()
	h.PUT("/tools/:id/admin-only", s.UpdateToolAdminOnly)
	body := `{"admin_only":true}`
	for _, wantStatus := range []uint{dto.ServerInternalErr.Status, dto.OK.Status} {
		res := ut.PerformRequest(h.Engine, "PUT", "/tools/builtin:fixture/admin-only",
			&ut.Body{Body: strings.NewReader(body), Len: len(body)},
			ut.Header{Key: "Content-Type", Value: "application/json"})
		var envelope struct {
			Status uint            `json:"status"`
			Data   dto.ErrorDetail `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Status != wantStatus {
			t.Fatalf("want status %d, got %s", wantStatus, res.Body.String())
		}
		if refreshErr != nil && (!strings.Contains(envelope.Data.ErrorDetail, "已保存") ||
			!strings.Contains(envelope.Data.ErrorDetail, refreshErr.Error())) {
			t.Fatalf("missing refresh failure details: %s", res.Body.String())
		}
		rows, err := s.DAO.ToolConfig.List(context.Background())
		if err != nil || len(rows) != 1 || !rows[0].AdminOnly {
			t.Fatalf("persisted permission missing or duplicated: rows=%v err=%v", rows, err)
		}
		refreshErr = nil
	}
	if refreshCalls != 2 {
		t.Fatalf("expected refresh on failure and retry, got %d calls", refreshCalls)
	}
	res := ut.PerformRequest(h.Engine, "PUT", "/tools/missing-id/admin-only",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"})
	var envelope struct {
		Status uint `json:"status"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil || envelope.Status != dto.ServerInternalErr.Status {
		t.Fatalf("expected persistence failure: %s", res.Body.String())
	}
	if refreshCalls != 2 {
		t.Fatal("refresh called despite persistence failure")
	}
}
