package layasticker

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoverCapabilitiesValidatesFixedAndDynamicModesWithoutInventingLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
          "schema_version": 1,
          "service": "laya",
          "api": {"protocol": "systemone.v1", "decision_path": "/v1/systemone", "method": "POST"},
          "models": [{"id": "multilingual", "sticker": {"category_mode": "dynamic", "suggested_categories": [
            {"id": "NONE", "description": "不发送", "no_send": true},
            {"id": "THINKING", "description": "思考", "no_send": false}
          ]}}],
          "response": {"category_pointer": "/answers/sticker/choice", "confidence_pointer": "/answers/sticker/answer_confidence"}
        }`)
	}))
	defer srv.Close()
	cfg := testConfig("http://unused.example/v1/systemone")
	cfg.CapabilitiesEndpoint = srv.URL + "/capabilities"
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := client.DiscoverCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != 1 || snapshot.Service != "laya" || len(snapshot.Models) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Models[0].Sticker.CategoryMode != "dynamic" || len(snapshot.Models[0].Sticker.SuggestedCategories) != 2 {
		t.Fatalf("sticker capability = %#v", snapshot.Models[0].Sticker)
	}
	if snapshot.Models[0].Sticker.MaxCategories != nil {
		t.Fatal("client must not invent a max_categories value")
	}
}

func TestValidateCapabilitySnapshotRejectsMalformedSchema(t *testing.T) {
	bad := CapabilitySnapshot{
		SchemaVersion: 2,
		Service:       "laya",
		API:           CapabilityAPI{Protocol: "systemone.v1", DecisionPath: "/v1/systemone", Method: "POST"},
	}
	if err := ValidateCapabilitySnapshot(bad); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("bad schema should be rejected: %v", err)
	}
	bad.SchemaVersion = 1
	bad.Service = "other"
	if err := ValidateCapabilitySnapshot(bad); err == nil || !strings.Contains(err.Error(), "service") {
		t.Fatalf("bad service should be rejected: %v", err)
	}
}

func TestDiscoverCapabilitiesUsesBearerAndDoesNotOverwriteConfig(t *testing.T) {
	const secret = "cap-secret"
	gotAuth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"schema_version":1,"service":"laya","api":{"protocol":"future.v2","decision_path":"/decision","method":"POST"},"models":[{"id":"future","sticker":{"category_mode":"dynamic"}}],"response":{"category_pointer":"/category"}}`)
	}))
	defer srv.Close()
	cfg := testConfig("http://decision.example/v1/systemone")
	cfg.APIKey = secret
	cfg.CapabilitiesEndpoint = srv.URL + "/capabilities"
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DiscoverCapabilities(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer "+secret {
		t.Fatalf("capability auth = %q", gotAuth)
	}
	if cfg.Endpoint != "http://decision.example/v1/systemone" || cfg.ProtocolMode != "json" {
		t.Fatal("capability discovery must not mutate saved decision config")
	}
}

// Laya 的 capabilities 固定挂在服务根路径：无论决策路径是 /v1/systemone
// 还是带前缀的 /prefix/v1/systemone，默认推导都必须是根路径 /capabilities，
// 否则「检测连接并获取能力」会打到不存在的 /v1/capabilities 上得到 404。
func TestCapabilitiesEndpointDerivesFromRootPath(t *testing.T) {
	for _, tc := range []struct{ endpoint, want string }{
		{"https://laya.example/v1/systemone", "https://laya.example/capabilities"},
		{"https://laya.example/prefix/v1/systemone", "https://laya.example/capabilities"},
		{"https://laya.example", "https://laya.example/capabilities"},
	} {
		client, err := NewClient(Config{Endpoint: tc.endpoint})
		if err != nil {
			t.Fatal(err)
		}
		endpoint, err := client.capabilitiesEndpoint()
		if err != nil {
			t.Fatal(err)
		}
		if endpoint != tc.want {
			t.Fatalf("derived capabilities endpoint for %q = %q, want %q", tc.endpoint, endpoint, tc.want)
		}
	}
}
