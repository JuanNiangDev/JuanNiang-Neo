package agent

import (
	"testing"

	"JuanNiang-Neo/internal/core/models"
)

func TestKnowledgeLRU(t *testing.T) {
	lru := newKnowledgeLRU(2)
	item := func(s string) models.KnowledgeItem { return models.KnowledgeItem{ID: s, Content: s} }

	lru.Put("a", []models.KnowledgeItem{item("a")})
	lru.Put("b", []models.KnowledgeItem{item("b")})
	if lru.Len() != 2 {
		t.Fatalf("期望 2 条, got %d", lru.Len())
	}

	// 访问 a → a 变最近使用；插入 c → 淘汰最久未用的 b
	if _, ok := lru.Get("a"); !ok {
		t.Fatal("a 应命中")
	}
	lru.Put("c", []models.KnowledgeItem{item("c")})
	if lru.Len() != 2 {
		t.Fatalf("超过容量应淘汰, got %d", lru.Len())
	}
	if _, ok := lru.Get("b"); ok {
		t.Error("b 应被淘汰（最久未用）")
	}
	if _, ok := lru.Get("a"); !ok {
		t.Error("a 应保留")
	}
	if _, ok := lru.Get("c"); !ok {
		t.Error("c 应保留")
	}

	// 更新已存在的 key：不增加长度
	lru.Put("a", []models.KnowledgeItem{item("a2")})
	if lru.Len() != 2 {
		t.Fatalf("更新不应增加长度, got %d", lru.Len())
	}

	lru.Clear()
	if lru.Len() != 0 {
		t.Fatal("Clear 后应为空")
	}
}

func TestParseKeywordsFromLLM(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"标准JSON数组", `["K8s","Pod","命名空间"]`, []string{"K8s", "Pod", "命名空间"}},
		{"带代码块围栏", "```json\n[\"longhorn\",\"kube-system\"]\n```", []string{"longhorn", "kube-system"}},
		{"引号容错", "关键词：\"部署\"、\"镜像\"、\"Helm\"", []string{"部署", "镜像", "Helm"}},
		{"空内容", "", nil},
	}
	for _, c := range cases {
		got := parseKeywordsFromLLM(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%s: 期望 %d 个关键词, got %d (%v)", c.name, len(c.want), len(got), got)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%s: 第 %d 个关键词 = %q, want %q", c.name, i, got[i], c.want[i])
			}
		}
	}
}

func TestFormatKnowledgeContext(t *testing.T) {
	if got := formatKnowledgeContext(nil); got != "" {
		t.Errorf("空列表应返回空串, got %q", got)
	}
	items := []models.KnowledgeItem{
		{Content: "第一条知识"},
		{Content: "第二条知识"},
	}
	got := formatKnowledgeContext(items)
	if got == "" {
		t.Fatal("非空列表应返回内容")
	}
	if !contains(got, "第一条知识") || !contains(got, "第二条知识") {
		t.Errorf("应包含两条知识内容, got %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
