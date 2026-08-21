package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseT2IStyles(t *testing.T) {
	raw := `[
		{"name":"Alpha","vibe":"test A","palette":"pal A","accents":"acc A","layout":"lay A"},
		{"name":"Beta","vibe":"test B","palette":"pal B","accents":"acc B","layout":"lay B"}
	]`
	styles := parseT2IStyles(raw)
	if len(styles) != 2 {
		t.Fatalf("期望 2 个风格，实际 %d", len(styles))
	}
	if !strings.Contains(styles[0], "Alpha") || !strings.Contains(styles[1], "Beta") {
		t.Errorf("风格内容不正确：%q, %q", styles[0], styles[1])
	}
	// 验证格式化格式
	for _, s := range styles {
		if !strings.Contains(s, "底色/正文/标题") {
			t.Errorf("缺少 palette 字段：%q", s)
		}
		if !strings.Contains(s, "强调") {
			t.Errorf("缺少 accents 字段：%q", s)
		}
		if !strings.Contains(s, "版式") {
			t.Errorf("缺少 layout 字段：%q", s)
		}
	}
}

func TestParseT2IStylesInvalid(t *testing.T) {
	if s := parseT2IStyles("{bad json}"); s != nil {
		t.Error("非法 JSON 应返回 nil")
	}
	if s := parseT2IStyles("[]"); s != nil {
		t.Error("空数组应返回 nil")
	}
	if s := parseT2IStyles(`[{"name":"X"}]`); s == nil || len(s) != 1 {
		t.Error("缺失可选字段仍应解析为 1 条风格")
	}
}

func TestLoadT2IStylesFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "styles.json")
	content := `[{"name":"Custom","vibe":"test","palette":"pal","accents":"acc","layout":"lay"}]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	old := t2iStylesPath
	t2iStylesPath = path
	defer func() { t2iStylesPath = old }()

	styles := loadT2IStyles()
	if len(styles) != 1 {
		t.Fatalf("期望 1 个风格，实际 %d", len(styles))
	}
	if !strings.Contains(styles[0], "Custom") {
		t.Errorf("文件内容未正确加载：%q", styles[0])
	}
}

func TestLoadT2IStylesFallback(t *testing.T) {
	old := t2iStylesPath
	t2iStylesPath = "/nonexistent/style.json"
	defer func() { t2iStylesPath = old }()

	// 文件不存在 → 回退内置通用兜底（1 条）
	styles := loadT2IStyles()
	if len(styles) != 1 {
		t.Fatalf("回退后期望 1 条通用兜底，实际 %d", len(styles))
	}

	// 空文件 → 回退
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	_ = os.WriteFile(path, []byte("  \n\n"), 0o644)
	t2iStylesPath = path
	styles = loadT2IStyles()
	if len(styles) != 1 {
		t.Fatalf("空文件回退失败：%d", len(styles))
	}

	// 非法 JSON → 回退
	path2 := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(path2, []byte("not json"), 0o644)
	t2iStylesPath = path2
	styles = loadT2IStyles()
	if len(styles) != 1 {
		t.Fatalf("非法 JSON 回退失败：%d", len(styles))
	}
}

func TestPickT2IStyleAlwaysNonEmpty(t *testing.T) {
	old := t2iStylesPath
	t2iStylesPath = "/nonexistent/style.json"
	defer func() { t2iStylesPath = old }()

	for i := 0; i < 50; i++ {
		s := pickT2IStyle()
		if strings.TrimSpace(s) == "" {
			t.Fatal("随机风格不应为空")
		}
	}
}

func TestInjectT2IStyle(t *testing.T) {
	old := t2iStylesPath
	t2iStylesPath = "/nonexistent/style.json"
	defer func() { t2iStylesPath = old }()

	// 占位符被替换
	out := injectT2IStyle("前缀\n" + t2iStylePlaceholder + "\n后缀")
	if strings.Contains(out, t2iStylePlaceholder) {
		t.Fatal("占位符未被替换")
	}
	if !strings.Contains(out, "前缀") || !strings.Contains(out, "后缀") {
		t.Fatal("占位符替换破坏了相邻内容")
	}
	// 风格内容被插入
	styles := loadT2IStyles()
	matched := false
	for _, s := range styles {
		if strings.Contains(out, s) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("注入内容不是任一风格条目：%q", out)
	}

	// 无占位符 → 原样返回
	orig := "没有任何占位符的文本"
	if got := injectT2IStyle(orig); got != orig {
		t.Fatalf("无占位符时应原样返回，实际: %q", got)
	}
}

func TestFormatT2IStyle(t *testing.T) {
	s := &t2iStyle{
		Name:    "Test",
		Vibe:    "测试气质",
		Palette: "底色 #fff",
		Accents: "红 #f00",
		Layout:  "居中",
	}
	f := s.format()
	if !strings.HasPrefix(f, "Test") {
		t.Errorf("format 应以名字开头：%q", f)
	}
	if !strings.Contains(f, "底色/正文/标题") {
		t.Errorf("format 应包含 palette：%q", f)
	}
	if !strings.Contains(f, "红") {
		t.Errorf("format 应包含 accent：%q", f)
	}
}

func TestSetSelectedT2IStyle(t *testing.T) {
	// 重置
	SetSelectedT2IStyle("")
	if GetSelectedT2IStyle() != "" {
		t.Fatal("重置后应为空")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "styles.json")
	content := `[
		{"name":"Alpha","vibe":"a","palette":"p","accents":"c","layout":"l"},
		{"name":"Beta","vibe":"b","palette":"p","accents":"c","layout":"l"}
	]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	old := t2iStylesPath
	t2iStylesPath = path
	defer func() {
		t2iStylesPath = old
		SetSelectedT2IStyle("")
	}()

	// 选中 Alpha → 恒返回 Alpha
	SetSelectedT2IStyle("Alpha")
	for i := 0; i < 20; i++ {
		if s := pickT2IStyle(); !strings.HasPrefix(s, "Alpha") {
			t.Fatalf("选中 Alpha 后应恒返回 Alpha，实际 %q", s)
		}
	}

	// 选中不存在的风格 → 退回随机（不 panic、非空）
	SetSelectedT2IStyle("Nonexistent")
	if s := pickT2IStyle(); s == "" {
		t.Fatal("选中不存在的风格不应返回空")
	}

	// 空选择 → 随机（覆盖所有风格）
	SetSelectedT2IStyle("")
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		s := pickT2IStyle()
		for _, name := range []string{"Alpha", "Beta"} {
			if strings.HasPrefix(s, name) {
				seen[name] = true
			}
		}
	}
	if !seen["Alpha"] || !seen["Beta"] {
		t.Errorf("随机应覆盖两个风格，实际 %v", seen)
	}
}

func TestLoadT2IStyleList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "styles.json")
	content := `[{"name":"Alpha","category":"科技","tags":["a","b"],"vibe":"测试","palette":"p","accents":"c","layout":"l"}]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	old := t2iStylesPath
	t2iStylesPath = path
	defer func() { t2iStylesPath = old }()

	list := LoadT2IStyleList()
	if len(list) != 1 {
		t.Fatalf("期望 1 条，实际 %d", len(list))
	}
	if list[0].Name != "Alpha" || list[0].Category != "科技" || len(list[0].Tags) != 2 {
		t.Errorf("结构化字段错误：%+v", list[0])
	}
	if list[0].Vibe != "测试" {
		t.Errorf("vibe 字段错误：%+v", list[0])
	}
}
