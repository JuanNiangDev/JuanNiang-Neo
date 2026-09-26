package layasticker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testConfig(endpoint string) Config {
	return Config{
		Enabled:           true,
		Endpoint:          endpoint,
		Model:             "multilingual",
		ProtocolMode:      "json",
		Timeout:           2 * time.Second,
		RequestTemplate:   `{"model":"{{model}}","user":"{{user_message}}","categories":{{enabled_categories}}}`,
		CategoryPointer:   "/answers/sticker/choice",
		ConfidencePointer: "/answers/sticker/confidence",
		Categories:        []Category{{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true}, {ID: "HAPPY", Description: "开心", Enabled: true}},
	}
}

func TestClientDecideParsesKnownAndAlternateResponsePointers(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if calls == 1 {
			_, _ = io.WriteString(w, `{"answers":{"sticker":{"type":"choice","choice":"HAPPY","confidence":0.687}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"decision":{"category":"THINKING","confidence":0.93}}`)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Decide(context.Background(), DecisionRequest{UserMessage: "你好", AssistantReply: "回复", MessageType: "group"})
	if err != nil || result.Category != "HAPPY" || result.Confidence == nil || *result.Confidence != 0.687 {
		t.Fatalf("first result = %#v, err=%v", result, err)
	}
	cfg.CategoryPointer = "/decision/category"
	cfg.ConfidencePointer = "/decision/confidence"
	client, err = NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	result, err = client.Decide(context.Background(), DecisionRequest{UserMessage: "你好", AssistantReply: "回复", MessageType: "group"})
	if err != nil || result.Category != "THINKING" || result.Confidence == nil || *result.Confidence != 0.93 {
		t.Fatalf("alternate result = %#v, err=%v", result, err)
	}
}

func TestClientProvidesDynamicCriteriaFromEnabledCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read decision body: %v", err)
		}
		var request struct {
			Choice struct {
				Criteria []string `json:"criteria"`
			} `json:"choice"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode decision body: %v", err)
		}
		if len(request.Choice.Criteria) != 2 || request.Choice.Criteria[0] != "NONE" || request.Choice.Criteria[1] != "HAPPY" {
			t.Fatalf("criteria = %#v, want configured enabled IDs", request.Choice.Criteria)
		}
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"NONE"}}}`)
	}))
	defer srv.Close()
	cfg := testConfig(srv.URL)
	cfg.RequestTemplate = `{"choice":{"criteria":{{criteria}}}}`
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Decide(context.Background(), DecisionRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestClientSendsBearerAndDoesNotLogOrReturnKey(t *testing.T) {
	const secret = "super-secret-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"NONE"}}}`)
	}))
	defer srv.Close()
	cfg := testConfig(srv.URL)
	cfg.APIKey = secret
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Decide(context.Background(), DecisionRequest{})
	if err != nil {
		t.Fatalf("decision error leaked secret or was unexpected: %v", err)
	}
}

func TestClientRejectsNon2xxInvalidJSONAndOversizedResponse(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
		want string
	}{
		{"status", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "bad", http.StatusBadGateway) }, "status"},
		{"json", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "not-json") }, "JSON"},
		{"oversize", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, strings.Repeat("x", 200)) }, "过大"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.h)
			defer srv.Close()
			cfg := testConfig(srv.URL)
			cfg.MaxResponseBytes = 64
			client, err := NewClient(cfg)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Decide(context.Background(), DecisionRequest{})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"NONE"}}}`)
	}))
	defer srv.Close()
	cfg := testConfig(srv.URL)
	cfg.Timeout = 20 * time.Millisecond
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Decide(context.Background(), DecisionRequest{})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}

// TestClientUnknownProtocolRejected 未知协议在构造与决策两处都必须明确报错。
func TestClientUnknownProtocolRejected(t *testing.T) {
	cfg := testConfig("http://laya.example/v1/systemone")
	cfg.ProtocolMode = "future.v9"
	if _, err := NewClient(cfg); err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("unknown protocol must be rejected at construction, err=%v", err)
	}
}

func TestClientDoesNotForwardBearerAcrossHostRedirect(t *testing.T) {
	var redirectedAuth string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"NONE"}}}`)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	cfg := testConfig(redirect.URL)
	cfg.APIKey = "secret"
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Decide(context.Background(), DecisionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if redirectedAuth != "" {
		t.Fatalf("authorization forwarded across host redirect: %q", redirectedAuth)
	}
}

func TestBuiltRequestIsValidJSON(t *testing.T) {
	body, err := BuildJSONTemplate(`{"value":"{{user_message}}"}`, DecisionRequest{UserMessage: "quoted \"text\""})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(body, &decoded); err != nil || decoded["value"] != "quoted \"text\"" {
		t.Fatalf("decoded body = %#v, err=%v", decoded, err)
	}
}

func TestClientProvidesCriteriaMapFromEnabledCategories(t *testing.T) {
	want := map[string]string{
		"NONE":    "不发送表情包",
		"HAPPY":   "开心、快乐、庆祝",
		"COMFORT": "安慰、鼓励、关心\n包含\"引号\"与 {{model}}",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Questions struct {
				Sticker struct {
					Criteria map[string]string `json:"criteria"`
				} `json:"sticker"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if !reflect.DeepEqual(request.Questions.Sticker.Criteria, want) {
			t.Errorf("criteria = %#v, want %#v", request.Questions.Sticker.Criteria, want)
		}
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"HAPPY"}}}`)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.RequestTemplate = `{"model":"{{model}}","state":{"user_message":"{{user_message}}","assistant_reply":"{{assistant_reply}}"},"questions":{"sticker":{"type":"choice","instructions":"判断是否发送表情包并选择类别","criteria":{{criteria_map}}}}}`
	cfg.Categories = []Category{
		{ID: "NONE", Description: "不发送表情包", NoSend: true, Enabled: true},
		{ID: "HAPPY", Description: "开心、快乐、庆祝", Enabled: true},
		{ID: "COMFORT", Description: "安慰、鼓励、关心\n包含\"引号\"与 {{model}}", Enabled: true},
		{ID: "DISABLED", Description: "不应发送", Enabled: false},
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Decide(context.Background(), DecisionRequest{UserMessage: "你好", AssistantReply: "回复"}); err != nil {
		t.Fatal(err)
	}
}
