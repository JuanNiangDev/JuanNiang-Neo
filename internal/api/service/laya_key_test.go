package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	"JuanNiang-Neo/internal/api/dto"
	"JuanNiang-Neo/internal/core/models"
)

func TestApplyLayaConfigKeyOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, oldEndpoint, oldCaps, endpoint, caps string
		newKey                                     string
		clear, reject                              bool
	}{
		{name: "decision host", oldEndpoint: "https://old.example/decision", endpoint: "https://new.example/decision", reject: true},
		{name: "capability host", oldEndpoint: "https://old.example/decision", endpoint: "https://old.example/decision", caps: "https://new.example/capabilities", reject: true},
		{name: "scheme", oldEndpoint: "https://old.example/decision", endpoint: "http://old.example/decision", reject: true},
		{name: "port", oldEndpoint: "https://old.example/decision", endpoint: "https://old.example:8443/decision", reject: true},
		{name: "same source paths", oldEndpoint: "https://old.example/old", oldCaps: "https://old.example/old-cap", endpoint: "https://old.example/new", caps: "https://old.example/new-cap"},
		{name: "default https port and host case", oldEndpoint: "https://OLD.example/old", endpoint: "https://old.example:443/new"},
		{name: "default http port", oldEndpoint: "http://old.example:80/old", endpoint: "http://old.example/new"},
		{name: "explicit to derived same source", oldEndpoint: "https://old.example/decision", oldCaps: "https://old.example:443/cap", endpoint: "https://old.example/new"},
		{name: "explicit to derived new source", oldEndpoint: "https://old.example/decision", oldCaps: "https://cap.example/cap", endpoint: "https://old.example/new", reject: true},
		{name: "decision changes even if cap remains", oldEndpoint: "https://old.example/decision", oldCaps: "https://cap.example/cap", endpoint: "https://new.example/decision", caps: "https://cap.example/cap", reject: true},
		{name: "new key", oldEndpoint: "https://old.example/decision", endpoint: "https://new.example/decision", newKey: "test-new-key"},
		{name: "clear key", oldEndpoint: "https://old.example/decision", endpoint: "https://new.example/decision", clear: true},
		{name: "clear takes priority", oldEndpoint: "https://old.example/decision", endpoint: "https://new.example/decision", clear: true, newKey: "test-new-key"},
		{name: "unknown prior source", endpoint: "https://new.example/decision", reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &models.ReplyStrategyConfig{LayaStickerEndpoint: tc.oldEndpoint, LayaCapabilitiesEndpoint: tc.oldCaps, LayaStickerAPIKey: "test-old-key"}
			before := *cfg
			err := applyLayaConfig(cfg, dto.UpdateReplyStrategyReq{LayaStickerEndpoint: tc.endpoint, LayaCapabilitiesEndpoint: tc.caps, LayaStickerAPIKey: tc.newKey, LayaStickerClearAPIKey: tc.clear})
			if tc.reject {
				if err == nil || !strings.Contains(err.Error(), "重新输入 API Key") {
					t.Fatalf("expected clear source-change rejection, got %v", err)
				}
				if strings.Contains(err.Error(), "test-old-key") {
					t.Fatal("error exposed key")
				}
				if *cfg != before {
					t.Fatal("rejected configuration mutated saved state")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := "test-old-key"
			if tc.newKey != "" {
				want = tc.newKey
			}
			if tc.clear {
				want = ""
			}
			if cfg.LayaStickerAPIKey != want {
				t.Fatal("key preserve/replace/clear semantics changed")
			}
		})
	}
}

func TestLayaHandlersRejectInheritedKeyBeforeNetwork(t *testing.T) {
	for _, preview := range []bool{false, true} {
		for _, capabilityOnly := range []bool{false, true} {
			for _, keyAction := range []string{"inherit", "replace", "clear"} {
				name := keyAction
				if preview {
					name += "/preview"
				} else {
					name += "/save"
				}
				if capabilityOnly {
					name += "/capability"
				} else {
					name += "/decision"
				}
				t.Run(name, func(t *testing.T) {
					var calls atomic.Int32
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						want := ""
						if keyAction == "replace" {
							want = "Bearer test-new-key"
						}
						if r.Header.Get("Authorization") != want {
							t.Error("unexpected credential sent to new origin")
						}
						_, _ = w.Write([]byte(loadedCapabilityJSON))
					}))
					defer srv.Close()
					svc, cfg := newLayaCapabilityTestService(t)
					cfg.LayaStickerEndpoint = "https://old.example/decision"
					cfg.LayaStickerAPIKey = "test-old-key"
					if err := svc.DAO.ReplyStrategy.Update(context.Background(), cfg); err != nil {
						t.Fatal(err)
					}
					req := dto.UpdateReplyStrategyReq{LayaStickerEndpoint: srv.URL + "/decision"}
					if capabilityOnly {
						req.LayaStickerEndpoint = cfg.LayaStickerEndpoint
						req.LayaCapabilitiesEndpoint = srv.URL + "/capabilities"
					}
					if keyAction == "replace" {
						req.LayaStickerAPIKey = "test-new-key"
					}
					req.LayaStickerClearAPIKey = keyAction == "clear"
					body, err := json.Marshal(req)
					if err != nil {
						t.Fatal(err)
					}
					c := app.NewContext(0)
					c.Request.Header.Set("Content-Type", "application/json")
					c.Request.SetBody(body)
					if preview {
						svc.RefreshLayaCapabilities(context.Background(), c)
					} else {
						svc.UpdateReplyStrategy(context.Background(), c)
					}
					var result dto.FinalResponse
					if err := json.Unmarshal(c.Response.Body(), &result); err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(c.Response.Body()), "test-old-key") || strings.Contains(string(c.Response.Body()), "test-new-key") {
						t.Fatal("response exposed credential")
					}
					if keyAction == "inherit" {
						if result.Status == dto.OK.Status || !strings.Contains(result.Info, "重新输入 API Key") {
							t.Fatalf("expected rejection: %s", c.Response.Body())
						}
						if calls.Load() != 0 {
							t.Fatal("rejected preview contacted external service")
						}
					} else {
						if result.Status != dto.OK.Status {
							t.Fatalf("explicit key action failed: %s", c.Response.Body())
						}
						if preview && calls.Load() != 1 {
							t.Fatal("preview did not contact candidate service")
						}
					}
					after, err := svc.DAO.ReplyStrategy.GetOrCreate(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if preview || keyAction == "inherit" {
						if after.LayaStickerEndpoint != cfg.LayaStickerEndpoint || after.LayaStickerAPIKey != cfg.LayaStickerAPIKey || after.LayaCapabilitiesEndpoint != cfg.LayaCapabilitiesEndpoint {
							t.Fatal("preview or rejected save modified saved config")
						}
					} else {
						want := "test-new-key"
						if keyAction == "clear" {
							want = ""
						}
						if after.LayaStickerAPIKey != want {
							t.Fatal("saved key action incorrect")
						}
						// The next formal refresh uses only the newly authorized credential.
						if err := svc.refreshLayaCapabilities(context.Background(), after); err != nil {
							t.Fatal(err)
						}
						if calls.Load() != 1 {
							t.Fatal("saved discovery did not use new endpoint")
						}
					}
				})
			}
		}
	}
}
