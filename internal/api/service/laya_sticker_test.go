package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"JuanNiang-Neo/internal/api/dto"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	"github.com/cloudwego/hertz/pkg/app"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestApplyLayaConfigPreservesAndClearsAPIKey(t *testing.T) {
	cfg := &models.ReplyStrategyConfig{LayaStickerAPIKey: "old-secret"}
	req := dto.UpdateReplyStrategyReq{
		LayaStickerEnabled:              true,
		LayaStickerEndpoint:             "https://laya.example/v1/systemone",
		LayaStickerModel:                "multilingual",
		LayaStickerProtocolMode:         "json",
		LayaStickerTimeout:              10,
		LayaStickerRequestTemplate:      `{"user":"{{user_message}}"}`,
		LayaStickerResponseCategoryPath: "/answers/sticker/choice",
		LayaStickerCategories: []dto.LayaCategoryReq{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心", StickerTags: []string{"开心"}, Enabled: true},
		},
	}
	if err := applyLayaConfig(cfg, req); err != nil {
		t.Fatal(err)
	}
	if cfg.LayaStickerAPIKey != "old-secret" {
		t.Fatalf("blank key should preserve old key, got %q", cfg.LayaStickerAPIKey)
	}
	if !cfg.LayaStickerEnabled || cfg.LayaStickerEndpoint == "" {
		t.Fatalf("config not applied: %+v", cfg)
	}
	req.LayaStickerClearAPIKey = true
	if err := applyLayaConfig(cfg, req); err != nil {
		t.Fatal(err)
	}
	if cfg.LayaStickerAPIKey != "" {
		t.Fatalf("explicit clear should remove key, got %q", cfg.LayaStickerAPIKey)
	}
}

func TestReplyStrategyResponseMasksLayaAPIKey(t *testing.T) {
	cfg := &models.ReplyStrategyConfig{
		LayaStickerAPIKey:     "do-not-return",
		LayaStickerCategories: `[{"id":"NONE","description":"不发送","no_send":true,"enabled":true}]`,
	}
	resp, err := replyStrategyResp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resp.LayaStickerAPIKeySet != true || resp.LayaStickerAPIKey != "" {
		t.Fatalf("API key should be masked: %+v", resp)
	}
	if len(resp.LayaStickerCategories) != 1 || resp.LayaStickerCategories[0].ID != "NONE" {
		t.Fatalf("categories not mapped: %+v", resp.LayaStickerCategories)
	}
}

func TestSanitizeLayaErrorRedactsBearerAndConfiguredKey(t *testing.T) {
	secret := "super-secret"
	message := sanitizeLayaError("upstream returned Bearer "+secret+" and api_key="+secret, secret)
	if strings.Contains(message, secret) {
		t.Fatalf("sanitized error contains API key: %q", message)
	}
	if strings.Contains(message, "Bearer "+secret) || strings.Contains(message, "api_key="+secret) {
		t.Fatalf("authorization was not redacted: %q", message)
	}
}

func TestApplyLayaConfigRejectsInvalidCategories(t *testing.T) {
	cfg := &models.ReplyStrategyConfig{}
	err := applyLayaConfig(cfg, dto.UpdateReplyStrategyReq{
		LayaStickerEnabled:      true,
		LayaStickerEndpoint:     "https://laya.example/decision",
		LayaStickerProtocolMode: "json",
		LayaStickerTimeout:      10,
		LayaStickerCategories:   []dto.LayaCategoryReq{{ID: "HAPPY", Description: "开心", Enabled: true}},
	})
	if err == nil {
		t.Fatal("enabled config without no-send category must be rejected")
	}
}

func TestApplyLayaConfigRejectsInvalidRequestTemplate(t *testing.T) {
	cfg := &models.ReplyStrategyConfig{}
	err := applyLayaConfig(cfg, dto.UpdateReplyStrategyReq{
		LayaStickerEnabled:         true,
		LayaStickerEndpoint:        "https://laya.example/decision",
		LayaStickerProtocolMode:    "json",
		LayaStickerTimeout:         10,
		LayaStickerRequestTemplate: `{"broken":`,
		LayaStickerCategories: []dto.LayaCategoryReq{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
		},
	})
	if err == nil {
		t.Fatal("invalid request template must be rejected")
	}
}

func TestRefreshLayaCapabilitiesRetainsLastSuccessOnFailure(t *testing.T) {
	responses := []string{
		`{"schema_version":1,"service":"laya","api":{"protocol":"future.v2","decision_path":"/decision","method":"POST"},"models":[{"id":"multilingual","sticker":{"category_mode":"dynamic"}}],"response":{"category_pointer":"/decision/category"}}`,
		`not-json`,
	}
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := responses[call]
		call++
		if call > len(responses) {
			body = `not-json`
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ReplyStrategyConfig{}); err != nil {
		t.Fatal(err)
	}
	bundle := dao.NewBundle(db)
	cfg, err := bundle.ReplyStrategy.GetOrCreate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.LayaStickerEnabled = true
	cfg.LayaStickerEndpoint = srv.URL + "/decision"
	cfg.LayaCapabilitiesEndpoint = srv.URL + "/capabilities"
	cfg.LayaStickerProtocolMode = "json"
	cfg.LayaStickerTimeout = 2
	cfg.LayaStickerCategories = `[{"id":"NONE","description":"不发送","no_send":true,"enabled":true}]`
	if err := bundle.ReplyStrategy.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	svc := &Service{DAO: bundle}
	if err := svc.refreshLayaCapabilities(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	first, err := bundle.ReplyStrategy.GetOrCreate(context.Background())
	if err != nil || first.LayaCapabilitySnapshot == "" || first.LayaCapabilityError != "" {
		t.Fatalf("successful capability state invalid: %+v err=%v", first, err)
	}
	firstSnapshot := first.LayaCapabilitySnapshot
	if err := svc.refreshLayaCapabilities(context.Background(), first); err == nil {
		t.Fatal("invalid second capability response should fail")
	}
	second, err := bundle.ReplyStrategy.GetOrCreate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.LayaCapabilitySnapshot != firstSnapshot || second.LayaCapabilityError == "" {
		t.Fatalf("failed refresh must retain snapshot and record error: first=%+v second=%+v", first, second)
	}
	if second.LayaStickerEndpoint != cfg.LayaStickerEndpoint || second.LayaStickerProtocolMode != "json" {
		t.Fatal("capability refresh must not overwrite decision config")
	}
}

func TestRefreshLayaCapabilitiesDoesNotOverwriteConcurrentDecisionConfig(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("capabilities method = %s, want GET", r.Method)
		}
		close(started)
		<-release
		_, _ = io.WriteString(w, `{"schema_version":1,"service":"laya","api":{"protocol":"future.v2","decision_path":"/decision","method":"POST"},"models":[{"id":"multilingual","sticker":{"category_mode":"dynamic"}}],"response":{"category_pointer":"/decision/category"}}`)
	}))
	defer srv.Close()

	db, err := gorm.Open(sqlite.Open("file:laya_capability_concurrency?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ReplyStrategyConfig{}); err != nil {
		t.Fatal(err)
	}
	bundle := dao.NewBundle(db)
	cfg, err := bundle.ReplyStrategy.GetOrCreate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.LayaStickerEnabled = true
	cfg.LayaStickerEndpoint = srv.URL + "/decision"
	cfg.LayaCapabilitiesEndpoint = srv.URL + "/capabilities"
	cfg.LayaStickerProtocolMode = "json"
	cfg.LayaStickerTimeout = 2
	cfg.LayaStickerCategories = `[{"id":"NONE","description":"不发送","no_send":true,"enabled":true}]`
	if err := bundle.ReplyStrategy.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	svc := &Service{DAO: bundle}
	result := make(chan error, 1)
	go func() { result <- svc.refreshLayaCapabilities(context.Background(), cfg) }()
	<-started

	latest, err := bundle.ReplyStrategy.GetOrCreate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	latest.LayaStickerEndpoint = "https://new-admin.example/decision"
	latest.LayaStickerModel = "new-model"
	latest.LayaStickerCategories = `[{"id":"NEW","description":"新类别","no_send":true,"enabled":true}]`
	if err := bundle.ReplyStrategy.Update(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-result; err == nil || !strings.Contains(err.Error(), "配置已变化，请重新检测") {
		t.Fatalf("obsolete refresh must report configuration change, got %v", err)
	}

	after, err := bundle.ReplyStrategy.GetOrCreate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after.LayaStickerEndpoint != "https://new-admin.example/decision" || after.LayaStickerModel != "new-model" {
		t.Fatalf("capability refresh overwrote concurrent decision settings: endpoint=%q model=%q", after.LayaStickerEndpoint, after.LayaStickerModel)
	}
	if after.LayaStickerCategories != latest.LayaStickerCategories || after.LayaCapabilitySnapshot != "" {
		t.Fatalf("stale capability refresh changed categories or saved an obsolete snapshot: categories=%q snapshot=%q", after.LayaStickerCategories, after.LayaCapabilitySnapshot)
	}
}

const loadedCapabilityJSON = `{"schema_version":1,"service":"laya","api":{"protocol":"systemone.v1","decision_path":"/v1/systemone","method":"POST"},"models":[{"id":"ready","loaded":true,"sticker":{"category_mode":"dynamic"}},{"id":"cold","loaded":false,"sticker":{"category_mode":"dynamic"}},{"id":"unknown","sticker":{"category_mode":"dynamic"}}],"response":{"category_pointer":"/answers/sticker/choice"}}`

func newLayaCapabilityTestService(t *testing.T) (*Service, *models.ReplyStrategyConfig) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.ReplyStrategyConfig{}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{DAO: dao.NewBundle(db)}
	cfg, err := svc.DAO.ReplyStrategy.GetOrCreate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return svc, cfg
}

func TestLayaCapabilityLoadedRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, loadedCapabilityJSON)
	}))
	defer srv.Close()
	svc, cfg := newLayaCapabilityTestService(t)
	cfg.LayaStickerEndpoint = srv.URL + "/v1/systemone"
	if err := svc.DAO.ReplyStrategy.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	refresh := app.NewContext(0)
	svc.RefreshLayaCapabilities(context.Background(), refresh)
	var refreshed struct {
		Status uint `json:"status"`
	}
	if err := json.Unmarshal(refresh.Response.Body(), &refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.Status != dto.OK.Status {
		t.Fatalf("refresh failed: %s", refresh.Response.Body())
	}

	// Read via GET after persistence, rather than inspecting only the discovery result.
	read := app.NewContext(0)
	svc.GetReplyStrategy(context.Background(), read)
	var response struct {
		Data struct {
			Snapshot struct {
				SourceEndpoint string           `json:"source_endpoint"`
				Models         []map[string]any `json:"models"`
			} `json:"laya_capability_snapshot"`
		} `json:"data"`
	}
	if err := json.Unmarshal(read.Response.Body(), &response); err != nil {
		t.Fatal(err)
	}
	got := response.Data.Snapshot
	if len(got.Models) != 3 {
		t.Fatalf("models = %#v", got.Models)
	}
	if got.Models[0]["loaded"] != true || got.Models[1]["loaded"] != false {
		t.Errorf("loaded true/false lost in round trip: %#v", got.Models)
	}
	if _, present := got.Models[2]["loaded"]; present {
		t.Errorf("missing loaded must remain absent: %#v", got.Models[2])
	}
	if got.SourceEndpoint != cfg.LayaStickerEndpoint {
		t.Errorf("source endpoint = %q", got.SourceEndpoint)
	}
}

func TestLayaCapabilityPreviewDoesNotPersist(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "success", false: "failure"}[valid], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if !valid {
					_, _ = io.WriteString(w, "not-json")
					return
				}
				_, _ = io.WriteString(w, loadedCapabilityJSON)
			}))
			defer srv.Close()
			svc, cfg := newLayaCapabilityTestService(t)
			cfg.LayaStickerEndpoint = "https://saved.example/v1/systemone"
			cfg.LayaCapabilitySnapshot = loadedCapabilityJSON
			cfg.LayaCapabilityError = "previous error"
			fetched := time.Now().UTC().Truncate(time.Second)
			cfg.LayaCapabilityFetchedAt = &fetched
			if err := svc.DAO.ReplyStrategy.Update(context.Background(), cfg); err != nil {
				t.Fatal(err)
			}
			req, err := json.Marshal(dto.UpdateReplyStrategyReq{LayaStickerEndpoint: srv.URL + "/v1/systemone"})
			if err != nil {
				t.Fatal(err)
			}
			c := app.NewContext(0)
			c.Request.SetBody(req)
			svc.RefreshLayaCapabilities(context.Background(), c)
			var response struct {
				Status uint `json:"status"`
				Data   struct {
					Preview  bool           `json:"laya_capability_preview"`
					Snapshot map[string]any `json:"laya_capability_snapshot"`
				} `json:"data"`
			}
			if err := json.Unmarshal(c.Response.Body(), &response); err != nil {
				t.Fatal(err)
			}
			if valid {
				if response.Status != dto.OK.Status || !response.Data.Preview || response.Data.Snapshot["source_endpoint"] != srv.URL+"/v1/systemone" {
					t.Errorf("preview response = %s", c.Response.Body())
				}
			} else if response.Status == dto.OK.Status {
				t.Error("invalid response should fail")
			}
			after, err := svc.DAO.ReplyStrategy.GetOrCreate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if after.LayaStickerEndpoint != cfg.LayaStickerEndpoint || after.LayaCapabilitySnapshot != cfg.LayaCapabilitySnapshot || after.LayaCapabilityError != cfg.LayaCapabilityError || after.LayaCapabilityFetchedAt == nil || !after.LayaCapabilityFetchedAt.Equal(fetched) {
				t.Errorf("preview changed persisted configuration or capability state: %+v", after)
			}
		})
	}
}

func TestApplyLayaConfigInvalidatesCapabilityForChangedSource(t *testing.T) {
	for _, field := range []string{"endpoint", "capabilities_endpoint", "api_key", "unchanged"} {
		t.Run(field, func(t *testing.T) {
			now := time.Now()
			cfg := &models.ReplyStrategyConfig{
				LayaStickerEndpoint:      "https://saved.example/v1/systemone",
				LayaCapabilitiesEndpoint: "https://saved.example/capabilities",
				LayaStickerAPIKey:        "saved-key",
				LayaCapabilitySnapshot:   loadedCapabilityJSON,
				LayaCapabilityFetchedAt:  &now,
				LayaCapabilityError:      "previous error",
			}
			req := dto.UpdateReplyStrategyReq{LayaStickerEndpoint: cfg.LayaStickerEndpoint, LayaCapabilitiesEndpoint: cfg.LayaCapabilitiesEndpoint}
			switch field {
			case "endpoint":
				req.LayaStickerEndpoint = "https://new.example/v1/systemone"
			case "capabilities_endpoint":
				req.LayaCapabilitiesEndpoint = "https://new.example/capabilities"
			case "api_key":
				req.LayaStickerAPIKey = "new-key"
			}
			if err := applyLayaConfig(cfg, req); err != nil {
				t.Fatal(err)
			}
			if field == "unchanged" {
				if cfg.LayaCapabilitySnapshot != loadedCapabilityJSON || cfg.LayaCapabilityFetchedAt == nil || cfg.LayaCapabilityError != "previous error" {
					t.Fatal("unchanged source lost capability state")
				}
			} else if cfg.LayaCapabilitySnapshot != "" || cfg.LayaCapabilityFetchedAt != nil || cfg.LayaCapabilityError != "" {
				t.Fatal("changed source retained old capability state")
			}
		})
	}
}

func TestRefreshLayaCapabilitiesReportsConcurrentSourceChange(t *testing.T) {
	for _, upstreamValid := range []bool{true, false} {
		for _, changed := range []string{"endpoint", "capabilities_endpoint", "api_key"} {
			t.Run(fmt.Sprintf("%s/valid=%t", changed, upstreamValid), func(t *testing.T) {
				started, release := make(chan struct{}), make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					close(started)
					<-release
					if upstreamValid {
						_, _ = io.WriteString(w, loadedCapabilityJSON)
					} else {
						_, _ = io.WriteString(w, "not-json")
					}
				}))
				defer srv.Close()
				defer unblock()
				svc, cfg := newLayaCapabilityTestService(t)
				cfg.LayaStickerEndpoint = srv.URL + "/v1/systemone"
				cfg.LayaStickerAPIKey = "old-key"
				if err := svc.DAO.ReplyStrategy.Update(context.Background(), cfg); err != nil {
					t.Fatal(err)
				}
				c := app.NewContext(0)
				done := make(chan struct{})
				go func() { defer close(done); svc.RefreshLayaCapabilities(context.Background(), c) }()
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("discovery did not start")
				}
				latest, err := svc.DAO.ReplyStrategy.GetOrCreate(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				switch changed {
				case "endpoint":
					latest.LayaStickerEndpoint = "https://new.example/v1/systemone"
				case "capabilities_endpoint":
					latest.LayaCapabilitiesEndpoint = "https://new.example/capabilities"
				case "api_key":
					latest.LayaStickerAPIKey = "new-key"
				}
				latest.LayaCapabilitySnapshot = loadedCapabilityJSON
				latest.LayaCapabilityError = "new source status"
				if err := svc.DAO.ReplyStrategy.Update(context.Background(), latest); err != nil {
					t.Fatal(err)
				}
				unblock()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Fatal("discovery did not finish")
				}
				var response struct {
					Status uint            `json:"status"`
					Info   string          `json:"info"`
					Data   dto.ErrorDetail `json:"data"`
				}
				if err := json.Unmarshal(c.Response.Body(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Status != 40901 || response.Info != "配置已变化，请重新检测" || !strings.Contains(response.Data.ErrorDetail, "配置已变化，请重新检测") {
					t.Errorf("obsolete refresh must report conflict, got %s", c.Response.Body())
				}
				after, err := svc.DAO.ReplyStrategy.GetOrCreate(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if after.LayaCapabilitySnapshot != latest.LayaCapabilitySnapshot || after.LayaCapabilityError != latest.LayaCapabilityError {
					t.Fatal("obsolete request overwrote new source capability state")
				}
			})
		}
	}
}
