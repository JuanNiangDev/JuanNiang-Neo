package layasticker

import (
	"encoding/json"
	"testing"
)

func TestBuildJSONTemplatePreservesStructuredVariables(t *testing.T) {
	template := `{"user":"{{user_message}}","reply":"{{assistant_reply}}","type":"{{message_type}}","model":"{{model}}","categories":{{categories}},"enabled":{{enabled_categories}},"choice":{"criteria":{{criteria}}}}`
	req := DecisionRequest{
		UserMessage:    "你好",
		AssistantReply: "回复",
		MessageType:    "group",
		Model:          "multilingual",
		Categories: []Category{
			{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true},
			{ID: "HAPPY", Description: "开心", StickerTags: []string{"开心"}, Enabled: true},
		},
		EnabledCategories: []Category{{ID: "HAPPY", Description: "开心", Enabled: true}},
		Criteria:          []string{"NONE", "HAPPY"},
	}
	body, err := BuildJSONTemplate(template, req)
	if err != nil {
		t.Fatalf("build template: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("built body is not JSON: %v", err)
	}
	if got["user"] != "你好" || got["model"] != "multilingual" {
		t.Fatalf("scalar placeholders changed type/value: %#v", got)
	}
	if _, ok := got["categories"].([]any); !ok {
		t.Fatalf("categories must remain an array, got %T", got["categories"])
	}
	if _, ok := got["enabled"].([]any); !ok {
		t.Fatalf("enabled_categories must remain an array, got %T", got["enabled"])
	}
	choice, ok := got["choice"].(map[string]any)
	if !ok {
		t.Fatalf("choice must remain an object, got %T", got["choice"])
	}
	if _, ok := choice["criteria"].([]any); !ok {
		t.Fatalf("criteria must remain an array, got %T", choice["criteria"])
	}
}

func TestBuildJSONTemplateRejectsStructuredInlineInterpolation(t *testing.T) {
	_, err := BuildJSONTemplate(`{"text":"categories={{categories}}"}`, DecisionRequest{
		Categories: []Category{{ID: "NONE", Description: "不发送", NoSend: true, Enabled: true}},
	})
	if err == nil {
		t.Fatal("structured placeholder in inline string must be rejected")
	}
}

func TestValidateJSONTemplateRejectsInvalidJSONAndUnknownVariables(t *testing.T) {
	if err := ValidateJSONTemplate(`{"broken":`); err == nil {
		t.Fatal("invalid JSON template must be rejected")
	}
	if err := ValidateJSONTemplate(`{"value":"{{unknown}}"}`); err == nil {
		t.Fatal("unknown template variable must be rejected")
	}
	if err := ValidateJSONTemplate(`{"choice":{"criteria":{{criteria}}}}`); err != nil {
		t.Fatalf("valid structured template rejected: %v", err)
	}
}

func TestExtractJSONPointerValues(t *testing.T) {
	var response any
	if err := json.Unmarshal([]byte(`{"decision":{"category":"THINKING","confidence":0.93}}`), &response); err != nil {
		t.Fatal(err)
	}
	category, err := ExtractJSONPointer(response, "/decision/category")
	if err != nil || category != "THINKING" {
		t.Fatalf("category = %#v, err=%v", category, err)
	}
	confidence, err := ExtractJSONPointer(response, "/decision/confidence")
	if err != nil || confidence.(float64) != 0.93 {
		t.Fatalf("confidence = %#v, err=%v", confidence, err)
	}
}
