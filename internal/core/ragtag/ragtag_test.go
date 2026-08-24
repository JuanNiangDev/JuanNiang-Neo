package ragtag

import (
	"testing"

	"github.com/google/uuid"
)

// TestDerivedTagsDeterministic 派生 tag 确定性：同输入同输出（跨实例/重启稳定）。
func TestDerivedTagsDeterministic(t *testing.T) {
	id := "abc-123"
	if Knowledge(id) != Knowledge(id) {
		t.Error("Knowledge 派生应确定")
	}
	if Memory(id) != Memory(id) {
		t.Error("Memory 派生应确定")
	}
	if Word(id) != Word(id) {
		t.Error("Word 派生应确定")
	}
	if Sample(id) != Sample(id) {
		t.Error("Sample 派生应确定")
	}
}

// TestDerivedTagsUniquePerNamespace 不同前缀/条目派生结果互不相同（集合隔离基础）。
func TestDerivedTagsUniquePerNamespace(t *testing.T) {
	const id = "same-id"
	tags := []uuid.UUID{Knowledge(id), Memory(id), Word(id), Sample(id)}
	seen := map[uuid.UUID]bool{}
	for _, tag := range tags {
		if seen[tag] {
			t.Errorf("派生 tag 冲突: %s", tag)
		}
		seen[tag] = true
	}
	// 不同条目 ID 派生不同
	if Knowledge("a") == Knowledge("b") {
		t.Error("不同 ID 派生应不同")
	}
}

// TestDerivedTagsValidUUID 派生结果是合法 UUID。
func TestDerivedTagsValidUUID(t *testing.T) {
	for _, tag := range []uuid.UUID{Knowledge("1"), Memory("1"), Word("1"), Sample("1")} {
		if tag == uuid.Nil {
			t.Error("派生 tag 不能为 Nil")
		}
		if tag.String() == "" {
			t.Error("派生 tag 字符串不能为空")
		}
	}
}
