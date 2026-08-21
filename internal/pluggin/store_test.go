package pluggin

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
