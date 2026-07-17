package dao

import (
	"context"
	"testing"

	"JuanNiang-Neo/internal/core/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.AdminUser{},
		&models.Provider{},
		&models.MCPServer{},
		&models.Skill{},
		&models.ToolConfig{},
		&models.Prompt{},
		&models.ChatArea{},
		&models.Session{},
		&models.ShortTermMemory{},
		&models.LongTermMemory{},
		&models.LongTermMemoryItem{},
		&models.BackgroundTask{},
		&models.ChatRecord{},
		&models.Plugin{},
		&models.ACLRule{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestUserDAO_CreateAndGet(t *testing.T) {
	db := testDB(t)
	dao := NewUserDAO(db)
	ctx := context.Background()

	u := &models.AdminUser{Username: "admin", PasswordHash: "hash", Role: "admin"}
	if err := dao.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	got, err := dao.GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "admin" || got.Role != "admin" {
		t.Errorf("unexpected user: %+v", got)
	}
}

func TestProviderDAO_CRUD(t *testing.T) {
	db := testDB(t)
	dao := NewProviderDAO(db)
	ctx := context.Background()

	p := &models.Provider{
		ID:          newUUID(),
		Name:        "test-provider",
		Type:        models.ModelTypeText,
		Endpoint:    "https://api.example.com",
		Token:       "sk-test",
		Model:       "gpt-4",
		Temperature: 0.7,
		IsActive:    true,
	}
	if err := dao.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	got, err := dao.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "test-provider" {
		t.Errorf("unexpected name: %s", got.Name)
	}

	list, err := dao.List(ctx, models.ModelTypeText)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 provider, got %d", len(list))
	}

	if err := dao.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	_, err = dao.GetByID(ctx, p.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestChatAreaDAO_GetOrCreate(t *testing.T) {
	db := testDB(t)
	dao := NewChatAreaDAO(db)
	ctx := context.Background()

	area, err := dao.GetOrCreate(ctx, models.AreaTypeGroup, 12345)
	if err != nil {
		t.Fatal(err)
	}
	if area.TargetID != 12345 || area.AreaType != models.AreaTypeGroup {
		t.Errorf("unexpected area: %+v", area)
	}

	area2, err := dao.GetOrCreate(ctx, models.AreaTypeGroup, 12345)
	if err != nil {
		t.Fatal(err)
	}
	if area2.ID != area.ID {
		t.Error("GetOrCreate should return same record")
	}

	count, err := dao.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 chat area, got %d", count)
	}
}

func TestSessionDAO_GetOrCreate(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	areaDAO := NewChatAreaDAO(db)
	area, _ := areaDAO.GetOrCreate(ctx, models.AreaTypePrivate, 999)

	sessionDAO := NewSessionDAO(db)
	sess, err := sessionDAO.GetOrCreate(ctx, area.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ChatAreaID != area.ID {
		t.Errorf("unexpected chat_area_id: %s", sess.ChatAreaID)
	}

	if err := sessionDAO.AddTokenUsage(ctx, sess.ID, 100); err != nil {
		t.Fatal(err)
	}

	updated, _ := sessionDAO.GetByID(ctx, sess.ID)
	if updated.TokenUsage != 100 {
		t.Errorf("expected token_usage 100, got %d", updated.TokenUsage)
	}
}

func TestBundle(t *testing.T) {
	db := testDB(t)
	b := NewBundle(db)

	if b.User == nil || b.Provider == nil || b.ChatArea == nil || b.Session == nil {
		t.Error("bundle fields should not be nil")
	}
}
