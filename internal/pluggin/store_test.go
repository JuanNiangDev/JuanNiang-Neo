package pluggin

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// zipEntry 描述一个待写入 zip 的条目。
type zipEntry struct {
	name    string
	content string
	mode    uint64 // zip 文件头 mode（0 = 默认普通文件）
	isDir   bool
}

// buildZip 构造测试用 zip 字节流。
func buildZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name}
		if e.mode != 0 {
			hdr.SetMode(os.FileMode(e.mode))
		}
		if e.isDir {
			hdr.SetMode(os.ModeDir | 0o755)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("创建 zip 条目失败: %v", err)
		}
		if !e.isDir && e.content != "" {
			if _, err := w.Write([]byte(e.content)); err != nil {
				t.Fatalf("写入 zip 条目失败: %v", err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	return buf.Bytes()
}

// validManifest 最小可加载的 pluggin.yaml。
const validManifest = "name: t\nentry: main.lua\npermissions:\n  - onebot11\nenabled: true\n"

// sentinelPath 探针文件：逃逸写入会覆盖它。
func sentinelPath(t *testing.T, pe *PluginEngine) string {
	t.Helper()
	// basePath 上一级（逃逸目标）：data/pluggins -> data/pwned.txt
	return filepath.Join(filepath.Dir(pe.basePath), "pwned.txt")
}

// TestInstallPluginZipEscapeEntry 恶意条目 "../../../pwned.txt" 必须被拒绝，
// 且不得写出插件目录外。
func TestInstallPluginZipEscapeEntry(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	sentinel := sentinelPath(t, pe)
	if err := os.WriteFile(sentinel, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatalf("写探针失败: %v", err)
	}
	data := buildZip(t, []zipEntry{
		{name: "evil/pluggin.yaml", content: validManifest},
		{name: "evil/../../../pwned.txt", content: "PWNED"},
	})
	if _, err := InstallPluginZip(pe, "evil", data); err == nil {
		t.Fatal("逃逸条目必须被拒绝")
	} else if !strings.Contains(err.Error(), "逃逸") {
		t.Fatalf("错误信息应指明逃逸: %v", err)
	}
	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("读探针失败: %v", err)
	}
	if string(got) != "ORIGINAL" {
		t.Fatalf("探针被覆盖，逃逸成功: %q", got)
	}
}

// TestInstallPluginZipDirNameTraversal zip 推断出 ".." 目录名（entry
// "x/../../pluggin.yaml"）必须被白名单拒绝，basePath 上级目录不得被删。
func TestInstallPluginZipDirNameTraversal(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	upper := filepath.Dir(pe.basePath) // t.TempDir() 根
	marker := filepath.Join(upper, "survivor.txt")
	if err := os.WriteFile(marker, []byte("KEEP"), 0o644); err != nil {
		t.Fatalf("写标记失败: %v", err)
	}
	data := buildZip(t, []zipEntry{
		{name: "x/../../pluggin.yaml", content: validManifest},
	})
	if _, err := InstallPluginZip(pe, "any", data); err == nil {
		t.Fatal("'..' 目录名必须被拒绝")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("上级目录被删（RemoveAll 逃逸）: %v", err)
	}
	// basePath 自身也不应被清掉
	if _, err := os.Stat(pe.basePath); err != nil {
		t.Fatalf("basePath 被删: %v", err)
	}
}

// TestInstallPluginZipSymlinkEntry symlink 条目必须被拒绝（防写穿透）。
func TestInstallPluginZipSymlinkEntry(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	data := buildZip(t, []zipEntry{
		{name: "evil/pluggin.yaml", content: validManifest},
		{name: "evil/link", mode: uint64(os.ModeSymlink | 0o777)},
	})
	if _, err := InstallPluginZip(pe, "evil", data); err == nil {
		t.Fatal("symlink 条目必须被拒绝")
	}
}

// TestInstallPluginZipBadNameParam name 参数末段非法（"plugins/.."）必须拒绝。
func TestInstallPluginZipBadNameParam(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	data := buildZip(t, []zipEntry{
		{name: "pluggin.yaml", content: validManifest}, // 根 yaml → 用 name 兜底
	})
	for _, bad := range []string{"plugins/..", "..", "a/b/..", "."} {
		if _, err := InstallPluginZip(pe, bad, data); err == nil {
			t.Fatalf("非法 name %q 必须被拒绝", bad)
		}
	}
}

// TestInstallPluginZipValidRootYaml 合法包（根 pluggin.yaml）正常安装，
// 且目录名来自 name 末段（"plugins/welcome" → "welcome"）。
func TestInstallPluginZipValidRootYaml(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	data := buildZip(t, []zipEntry{
		{name: "pluggin.yaml", content: validManifest},
		{name: "main.lua", content: "-- entry"},
	})
	name, err := InstallPluginZip(pe, "plugins/welcome", data)
	if err != nil {
		t.Fatalf("合法包安装失败: %v", err)
	}
	if name != "welcome" {
		t.Fatalf("目录名应为 name 末段 welcome，得到 %q", name)
	}
	if _, err := os.Stat(filepath.Join(pe.basePath, "welcome", "main.lua")); err != nil {
		t.Fatalf("插件文件未落盘: %v", err)
	}
}

// TestInstallPluginZipSubdirYaml 合法包（子目录 pluggin.yaml + rootPrefix 剥离）正常安装。
func TestInstallPluginZipSubdirYaml(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	data := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/main.lua", content: "-- entry"},
		{name: "welcome/assets/", isDir: true},
	})
	name, err := InstallPluginZip(pe, "whatever", data)
	if err != nil {
		t.Fatalf("合法子目录包安装失败: %v", err)
	}
	if name != "welcome" {
		t.Fatalf("目录名应为 zip 内目录名 welcome，得到 %q", name)
	}
	if _, err := os.Stat(filepath.Join(pe.basePath, "welcome", "main.lua")); err != nil {
		t.Fatalf("插件文件未落盘: %v", err)
	}
}

// TestInstallPluginZipKeepsOldOnBadZip 安装失败（条目非法）时旧插件目录必须原样保留。
func TestInstallPluginZipKeepsOldOnBadZip(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	// 先装一个合法版本
	good := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/main.lua", content: "-- v1"},
	})
	if _, err := InstallPluginZip(pe, "plugins/welcome", good); err != nil {
		t.Fatalf("初装失败: %v", err)
	}
	// 再装一个恶意升级包：条目逃逸 → 整体拒绝，旧版不动
	bad := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/../../../pwned.txt", content: "PWNED"},
	})
	if _, err := InstallPluginZip(pe, "plugins/welcome", bad); err == nil {
		t.Fatal("逃逸条目必须被拒绝")
	}
	got, err := os.ReadFile(filepath.Join(pe.basePath, "welcome", "main.lua"))
	if err != nil {
		t.Fatalf("旧版被删除: %v", err)
	}
	if string(got) != "-- v1" {
		t.Fatalf("旧版本文件内容被改动: %q", got)
	}
	// 不残留 .staging-* 暂存目录
	entries, _ := os.ReadDir(pe.basePath)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Fatalf("暂存目录未清理: %s", e.Name())
		}
	}
}

// TestInstallPluginZipUpgradesSuccessfully 合法升级包正常替换旧版。
func TestInstallPluginZipUpgradesSuccessfully(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	v1 := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/main.lua", content: "-- v1"},
	})
	if _, err := InstallPluginZip(pe, "plugins/welcome", v1); err != nil {
		t.Fatalf("v1 安装失败: %v", err)
	}
	v2 := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/main.lua", content: "-- v2"},
	})
	if _, err := InstallPluginZip(pe, "plugins/welcome", v2); err != nil {
		t.Fatalf("v2 升级失败: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(pe.basePath, "welcome", "main.lua"))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(got) != "-- v2" {
		t.Fatalf("升级后应为 v2 内容: %q", got)
	}
	// 备份目录不残留
	entries, _ := os.ReadDir(pe.basePath)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Fatalf("备份/暂存目录未清理: %s", e.Name())
		}
	}
}

// TestInstallPluginZipRollbackOnLoadFail 升级包解压成功但加载失败时，回滚旧版。
func TestInstallPluginZipRollbackOnLoadFail(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	v1 := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/main.lua", content: "-- v1"},
	})
	if _, err := InstallPluginZip(pe, "plugins/welcome", v1); err != nil {
		t.Fatalf("v1 安装失败: %v", err)
	}
	// v2 的 main.lua 语法错误 → Load 失败 → 回滚 v1
	v2 := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/main.lua", content: "!!语法错误(("},
	})
	if _, err := InstallPluginZip(pe, "plugins/welcome", v2); err == nil {
		t.Fatal("加载失败必须报错")
	}
	got, err := os.ReadFile(filepath.Join(pe.basePath, "welcome", "main.lua"))
	if err != nil {
		t.Fatalf("回滚失败，旧版丢失: %v", err)
	}
	if string(got) != "-- v1" {
		t.Fatalf("应回滚到 v1 内容: %q", got)
	}
	entries, _ := os.ReadDir(pe.basePath)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Fatalf("回滚后暂存/备份目录未清理: %s", e.Name())
		}
	}
	// 引擎状态断言：回滚后 welcome 应重新注册为已加载（v1 重载）
	if _, loaded := pe.plugins["welcome"]; !loaded {
		t.Error("回滚后 welcome 未在引擎中重新注册（v1 重载失败）")
	}
}

// TestInstallPluginZipFirstInstallLoadFailCleans 首次安装加载失败时删除
// 已提交的新目录（不遗留未加载插件）。
func TestInstallPluginZipFirstInstallLoadFailCleans(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	data := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/main.lua", content: "!!语法错误(("},
	})
	if _, err := InstallPluginZip(pe, "plugins/welcome", data); err == nil {
		t.Fatal("加载失败必须报错")
	}
	if _, err := os.Stat(filepath.Join(pe.basePath, "welcome")); err == nil {
		t.Fatal("首次安装加载失败应删除新插件目录")
	}
	if _, loaded := pe.plugins["welcome"]; loaded {
		t.Error("加载失败的插件不应在引擎中注册")
	}
	entries, _ := os.ReadDir(pe.basePath)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Fatalf("暂存/备份目录未清理: %s", e.Name())
		}
	}
}

// TestExtractZipToSizeLimit zip 炸弹：单文件或总量超过上限必须被拒绝。
func TestExtractZipToSizeLimit(t *testing.T) {
	dir := t.TempDir()
	// 超过单文件上限（32MiB）的压缩条目，用高压缩比重复字节
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("big.bin")
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("A"), 1<<20)
	for i := 0; i < 33; i++ {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	err = extractZipTo(reader, "", dir)
	if err == nil {
		t.Fatal("超限条目必须被拒绝")
	}
	if !strings.Contains(err.Error(), "超过单文件解压上限") {
		t.Fatalf("错误信息应指明单文件超限: %v", err)
	}
}

// TestInstallStagedPluginConcurrentSameName 同一插件并发安装串行执行：
// 两个请求同时到达时，最终目录状态一致且无 .staging-* 残留。
func TestInstallStagedPluginConcurrentSameName(t *testing.T) {
	pe, _ := newMiniTestEngine(t, nil)
	v1 := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/main.lua", content: "-- v1"},
	})
	v2 := buildZip(t, []zipEntry{
		{name: "welcome/pluggin.yaml", content: validManifest},
		{name: "welcome/main.lua", content: "-- v2"},
	})
	r1, _ := zip.NewReader(bytes.NewReader(v1), int64(len(v1)))
	r2, _ := zip.NewReader(bytes.NewReader(v2), int64(len(v2)))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = InstallStagedPlugin(pe, pe.basePath, "welcome", r1, "welcome/") }()
	go func() { defer wg.Done(); _ = InstallStagedPlugin(pe, pe.basePath, "welcome", r2, "welcome/") }()
	wg.Wait()

	// 最终目录存在且内容完整（v1 或 v2 任一），无暂存残留
	got, err := os.ReadFile(filepath.Join(pe.basePath, "welcome", "main.lua"))
	if err != nil {
		t.Fatalf("并发安装后目录缺失: %v", err)
	}
	if string(got) != "-- v1" && string(got) != "-- v2" {
		t.Fatalf("目录内容应为 v1 或 v2，实际 %q", got)
	}
	if _, loaded := pe.plugins["welcome"]; !loaded {
		t.Error("并发安装后 welcome 应已加载")
	}
	entries, _ := os.ReadDir(pe.basePath)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Fatalf("并发安装后暂存/备份目录未清理: %s", e.Name())
		}
	}
}
