package pluggin

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPluginFilePath(t *testing.T) {
	base := filepath.Join(t.TempDir(), "pluggins")
	plugin := "demo"

	valid := []string{
		"data.txt",
		"data/notes.txt",
		"./data/a.txt",
		"a/b/c.txt",
	}
	for _, p := range valid {
		full, err := pluginFilePath(base, plugin, p)
		if err != nil {
			t.Errorf("pluginFilePath(%q) 不应报错: %v", p, err)
			continue
		}
		rel, _ := filepath.Rel(filepath.Join(base, plugin), full)
		if rel == ".." || len(rel) > 2 && rel[:3] == ".."+string(filepath.Separator) {
			t.Errorf("pluginFilePath(%q) 越界: %q", p, full)
		}
	}

	invalid := []string{
		"",
		"../secret.txt",
		"a/../../secret.txt",
		"/etc/passwd",
		"..",
	}
	for _, p := range invalid {
		if _, err := pluginFilePath(base, plugin, p); err == nil {
			t.Errorf("pluginFilePath(%q) 应报错但未报错", p)
		}
	}
}

func TestReadWriteTextLines(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{"普通行", "a\nb\nc\n", []string{"a", "b", "c"}},
		{"无末尾换行", "a\nb", []string{"a", "b"}},
		{"空行", "a\n\nb\n", []string{"a", "", "b"}},
		{"CRLF", "a\r\nb\r\n", []string{"a", "b"}},
		{"空文件", "", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full := filepath.Join(dir, "f.txt")
			if err := os.WriteFile(full, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := readTextLines(full)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("readTextLines 结果 = %#v, want %#v", got, tc.want)
			}
		})
	}

	// 写回后重新读取应保持一致
	full := filepath.Join(dir, "w.txt")
	lines := []string{"x", "", "y"}
	if err := writeTextLines(full, lines); err != nil {
		t.Fatal(err)
	}
	got, err := readTextLines(full)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, lines) {
		t.Errorf("写入后重读 = %#v, want %#v", got, lines)
	}

	// 空行数组写入 → 空文件
	if err := writeTextLines(full, []string{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(full); len(got) != 0 {
		t.Errorf("空行数组写入后文件应为空, got %q", string(got))
	}
}
