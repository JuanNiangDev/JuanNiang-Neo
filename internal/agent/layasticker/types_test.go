package layasticker

import (
	"strings"
	"testing"
	"time"
)

func TestValidateCategoriesRequiresUniqueIDsAndEnabledNoSend(t *testing.T) {
	valid := []Category{
		{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
		{ID: "HAPPY", Description: "开心", StickerTags: []string{"开心"}, Enabled: true},
	}
	if err := ValidateCategories(valid); err != nil {
		t.Fatalf("valid categories rejected: %v", err)
	}

	duplicate := append([]Category(nil), valid...)
	duplicate[1].ID = "NONE"
	if err := ValidateCategories(duplicate); err == nil {
		t.Fatal("duplicate category IDs must be rejected")
	}

	noNoSend := []Category{{ID: "HAPPY", Description: "开心", Enabled: true}}
	if err := ValidateCategories(noNoSend); err == nil {
		t.Fatal("enabled categories without an enabled no-send category must be rejected")
	}
}

func TestValidateConfigRejectsUnsafeEndpointAndTimeout(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Endpoint:     "http://127.0.0.1:8080/v1/systemone",
		ProtocolMode: "json",
		Timeout:      0,
		Categories:   []Category{{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true}},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("zero timeout must be rejected")
	}
	cfg.Timeout = 5 * time.Second
	cfg.Endpoint = "file:///tmp/laya"
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("non-http endpoint must be rejected")
	}
	cfg.Endpoint = "http://laya.example/v1/systemone"
	cfg.ProtocolMode = "unknown.v9"
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("unknown protocol mode must be rejected, got %v", err)
	}
}

func TestClientRequiresExplicitJSONRequestTemplateBeforeDecision(t *testing.T) {
	cfg := Config{
		Enabled:         true,
		Endpoint:        "https://laya.example/v1/systemone",
		ProtocolMode:    ProtocolJSON,
		Timeout:         5 * time.Second,
		CategoryPointer: "/answers/sticker/choice",
		Categories:      []Category{{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true}},
	}
	if _, err := NewClient(cfg); err == nil {
		t.Fatal("JSON protocol must require an administrator-provided request template")
	}
}

func TestParseJSONPointer(t *testing.T) {
	parts, err := ParseJSONPointer("/answers/sticker/choice")
	if err != nil {
		t.Fatalf("parse pointer: %v", err)
	}
	want := []string{"answers", "sticker", "choice"}
	if len(parts) != len(want) {
		t.Fatalf("parts = %#v, want %#v", parts, want)
	}
	for i := range want {
		if parts[i] != want[i] {
			t.Fatalf("parts = %#v, want %#v", parts, want)
		}
	}
	parts, err = ParseJSONPointer("/a~1b/c~0d")
	if err != nil || len(parts) != 2 || parts[0] != "a/b" || parts[1] != "c~d" {
		t.Fatalf("pointer escaping failed: %#v, %v", parts, err)
	}
	if _, err := ParseJSONPointer("answers/sticker/choice"); err == nil {
		t.Fatal("non-pointer path must be rejected")
	}
}

// TestValidateConfigSystemOneNativeMode 原生模式：要求模型与启用类别，
// 不要求 JSON 请求模板，遗留的无效模板不得阻止切换；只允许 POST。
func TestValidateConfigSystemOneNativeMode(t *testing.T) {
	base := Config{
		Enabled:      true,
		Endpoint:     "http://laya.example/v1/systemone",
		ProtocolMode: ProtocolSystemOne,
		Model:        "multilingual",
		Timeout:      5 * time.Second,
		Categories: []Category{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心", Enabled: true},
		},
	}
	if err := ValidateConfig(base); err != nil {
		t.Fatalf("valid native config rejected: %v", err)
	}

	stale := base
	stale.RequestTemplate = "{not-json-at-all"
	if err := ValidateConfig(stale); err != nil {
		t.Fatalf("stale JSON template must not block native mode: %v", err)
	}

	noModel := base
	noModel.Model = ""
	if err := ValidateConfig(noModel); err == nil {
		t.Fatal("native mode without model must be rejected")
	}

	badMethod := base
	badMethod.HTTPMethod = "PUT"
	if err := ValidateConfig(badMethod); err == nil {
		t.Fatal("native mode must reject unverified custom methods")
	}

	jsonMode := base
	jsonMode.ProtocolMode = ProtocolJSON
	jsonMode.RequestTemplate = ""
	if err := ValidateConfig(jsonMode); err == nil {
		t.Fatal("switching back to JSON mode must re-validate the template")
	}
}
