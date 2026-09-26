package layasticker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// systemOneTestConfig 原生模式配置：不带 JSON 请求模板。
func systemOneTestConfig(endpoint string) Config {
	return Config{
		Enabled:      true,
		Endpoint:     endpoint,
		Model:        "multilingual",
		ProtocolMode: ProtocolSystemOne,
		Timeout:      2 * time.Second,
		Categories: []Category{
			{ID: "NONE", Description: "不需要追加表情", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心、庆祝", Enabled: true},
			{ID: "THINKING", Description: "思考、疑惑", Enabled: false},
		},
	}
}

type systemOneCaptured struct {
	Model string `json:"model"`
	State struct {
		UserMessage    string `json:"user_message"`
		AssistantReply string `json:"assistant_reply"`
	} `json:"state"`
	Questions struct {
		Sticker struct {
			Type         string            `json:"type"`
			Instructions string            `json:"instructions"`
			Criteria     map[string]string `json:"criteria"`
		} `json:"sticker"`
	} `json:"questions"`
}

// TestSystemOneDecideBuildsNativeRequest 验证原生请求的路径、方法、模型、
// state 与 questions.sticker 结构；criteria 必须是包含全部启用类别
// （含 no_send、不含禁用类别）的 JSON 对象；响应从
// /answers/sticker/choice 与 /answers/sticker/answer_confidence 解析。
func TestSystemOneDecideBuildsNativeRequest(t *testing.T) {
	var captured systemOneCaptured
	var rawBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("path = %s, want /v1/systemone", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("request body is not valid JSON: %v", err)
			return
		}
		if err := json.Unmarshal(body, &rawBody); err != nil {
			t.Errorf("request body is not valid JSON: %v", err)
			return
		}
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"type":"choice","choice":"HAPPY","answer_confidence":0.88,"confidence":0.5}}}`)
	}))
	defer srv.Close()

	// 原生模式不需要（也未配置）JSON 请求模板。
	cfg := systemOneTestConfig(srv.URL + "/v1/systemone")
	if cfg.RequestTemplate != "" {
		t.Fatal("native test config must not carry a JSON template")
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("native mode must not require a JSON request template: %v", err)
	}
	result, err := client.Decide(context.Background(), DecisionRequest{
		UserMessage:    "今天终于把项目跑通了！",
		AssistantReply: "太好了，终于可以放心了！",
		MessageType:    "group",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Category != "HAPPY" {
		t.Fatalf("category = %q, want HAPPY", result.Category)
	}
	if result.Confidence == nil || *result.Confidence != 0.88 {
		t.Fatalf("confidence = %v, want 0.88 from answer_confidence", result.Confidence)
	}

	if captured.Model != "multilingual" {
		t.Fatalf("model = %q", captured.Model)
	}
	if captured.State.UserMessage != "今天终于把项目跑通了！" || captured.State.AssistantReply != "太好了，终于可以放心了！" {
		t.Fatalf("state = %#v", captured.State)
	}
	sticker := captured.Questions.Sticker
	if sticker.Type != "choice" || strings.TrimSpace(sticker.Instructions) == "" {
		t.Fatalf("questions.sticker = %#v", sticker)
	}
	// criteria 必须是 JSON 对象而不是数组或转义字符串。
	if _, isMap := rawBody["questions"].(map[string]any)["sticker"].(map[string]any)["criteria"].(map[string]any); !isMap {
		t.Fatalf("criteria must be a JSON object: %#v", rawBody)
	}
	want := map[string]string{"NONE": "不需要追加表情", "HAPPY": "开心、庆祝"}
	if len(sticker.Criteria) != len(want) {
		t.Fatalf("criteria = %#v, want %#v", sticker.Criteria, want)
	}
	for id, desc := range want {
		if sticker.Criteria[id] != desc {
			t.Fatalf("criteria[%q] = %q, want %q", id, sticker.Criteria[id], desc)
		}
	}
	if _, ok := sticker.Criteria["THINKING"]; ok {
		t.Fatal("disabled category must not appear in criteria")
	}
}

// TestSystemOneCriteriaWithSpecialCharacters 类别描述中的引号、换行与
// 反斜杠必须被 json.Marshal 安全转义，不破坏请求 JSON。
func TestSystemOneCriteriaWithSpecialCharacters(t *testing.T) {
	const tricky = "开心\"引号\"\n换行\\反斜杠{\"嵌套\":true}"
	var captured systemOneCaptured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("request body is not valid JSON: %v", err)
		}
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"NONE"}}}`)
	}))
	defer srv.Close()
	cfg := systemOneTestConfig(srv.URL + "/v1/systemone")
	cfg.Categories[1].Description = tricky
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Decide(context.Background(), DecisionRequest{UserMessage: "用户", AssistantReply: "回复"}); err != nil {
		t.Fatal(err)
	}
	if captured.Questions.Sticker.Criteria["HAPPY"] != tricky {
		t.Fatalf("special characters corrupted: %q", captured.Questions.Sticker.Criteria["HAPPY"])
	}
}

// TestSystemOneAndJSONModesShareCategoryMapping 两种协议使用同一份类别映射：
// 切换模式不改变启用/禁用/no_send 语义，都能正常完成决策。
func TestSystemOneAndJSONModesShareCategoryMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"HAPPY","answer_confidence":0.9}}}`)
	}))
	defer srv.Close()

	nativeCfg := systemOneTestConfig(srv.URL)
	nativeClient, err := NewClient(nativeCfg)
	if err != nil {
		t.Fatal(err)
	}
	nativeResult, err := nativeClient.Decide(context.Background(), DecisionRequest{UserMessage: "用户", AssistantReply: "回复"})
	if err != nil || nativeResult.Category != "HAPPY" {
		t.Fatalf("native result = %#v, err=%v", nativeResult, err)
	}

	jsonCfg := nativeCfg
	jsonCfg.ProtocolMode = ProtocolJSON
	jsonCfg.RequestTemplate = `{"model":"{{model}}","state":{"user_message":"{{user_message}}","assistant_reply":"{{assistant_reply}}"},"questions":{"sticker":{"type":"choice","instructions":"判断是否发送表情包并选择类别","criteria":{{criteria_map}}}}}`
	jsonClient, err := NewClient(jsonCfg)
	if err != nil {
		t.Fatal(err)
	}
	jsonResult, err := jsonClient.Decide(context.Background(), DecisionRequest{UserMessage: "用户", AssistantReply: "回复"})
	if err != nil || jsonResult.Category != "HAPPY" {
		t.Fatalf("json result = %#v, err=%v", jsonResult, err)
	}
}

// TestSystemOneNoSendCategoryStillDecides no_send 类别（如 NONE）仍作为
// 普通候选项参与决策，由调用方依据类别配置决定不发送。
func TestSystemOneNoSendCategoryStillDecides(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"NONE"}}}`)
	}))
	defer srv.Close()
	client, err := NewClient(systemOneTestConfig(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Decide(context.Background(), DecisionRequest{UserMessage: "用户", AssistantReply: "回复"})
	if err != nil || result.Category != "NONE" {
		t.Fatalf("result = %#v, err=%v", result, err)
	}
}

// TestSystemOneErrorsFallBackCleanly Laya 返回错误、非法 JSON 或超时时
// Decide 必须返回错误，由调用方回退原版文字回复。
func TestSystemOneErrorsFallBackCleanly(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad", http.StatusBadGateway)
		}))
		defer srv.Close()
		client, err := NewClient(systemOneTestConfig(srv.URL))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Decide(context.Background(), DecisionRequest{UserMessage: "用户", AssistantReply: "回复"}); err == nil {
			t.Fatal("HTTP error must surface")
		}
	})
	t.Run("invalid-json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "not-json")
		}))
		defer srv.Close()
		client, err := NewClient(systemOneTestConfig(srv.URL))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Decide(context.Background(), DecisionRequest{UserMessage: "用户", AssistantReply: "回复"}); err == nil {
			t.Fatal("invalid JSON must surface")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"NONE"}}}`)
		}))
		defer srv.Close()
		cfg := systemOneTestConfig(srv.URL)
		cfg.Timeout = 20 * time.Millisecond
		client, err := NewClient(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Decide(context.Background(), DecisionRequest{UserMessage: "用户", AssistantReply: "回复"}); err == nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout error = %v", err)
		}
	})
	t.Run("missing-model-or-categories", func(t *testing.T) {
		// 模型为空或没有启用类别时必须在发起 HTTP 前返回明确错误。
		if _, err := BuildSystemOneRequest(DecisionRequest{
			EnabledCategories: []Category{{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true}},
		}); err == nil {
			t.Fatal("empty model must fail before HTTP")
		}
		if _, err := BuildSystemOneRequest(DecisionRequest{Model: "multilingual"}); err == nil {
			t.Fatal("empty enabled categories must fail before HTTP")
		}
		if _, err := BuildSystemOneRequest(DecisionRequest{
			Model:             "multilingual",
			EnabledCategories: []Category{{ID: "HAPPY", Description: "开心", Enabled: false}},
		}); err == nil {
			t.Fatal("all-disabled categories must fail before HTTP")
		}
	})
}

// TestSystemOneWorksWithoutCapabilities /capabilities 不可用时，已保存的
// 有效原生配置必须继续可用；决策不得触发任何能力发现请求。
func TestSystemOneWorksWithoutCapabilities(t *testing.T) {
	capCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/capabilities" {
			capCalls++
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"answers":{"sticker":{"choice":"HAPPY"}}}`)
	}))
	defer srv.Close()
	cfg := systemOneTestConfig(srv.URL + "/v1/systemone")
	cfg.CapabilitiesEndpoint = srv.URL + "/capabilities"
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		result, err := client.Decide(context.Background(), DecisionRequest{UserMessage: "用户", AssistantReply: "回复"})
		if err != nil || result.Category != "HAPPY" {
			t.Fatalf("decision %d failed: %#v err=%v", i, result, err)
		}
	}
	if capCalls != 0 {
		t.Fatalf("decision must not trigger capability discovery, capCalls=%d", capCalls)
	}
}
