package pluggin

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallAllRealStoreZips 批量：Plugins/dist 全部真实商店包都能正常安装。
func TestInstallAllRealStoreZips(t *testing.T) {
	distDir := filepath.Join("..", "..", "..", "Plugins", "dist")
	entries, err := os.ReadDir(distDir)
	if os.IsNotExist(err) {
		t.Skipf("商店 dist 不存在（需 Plugins 仓库同级检出）: %s", distDir)
	}
	if err != nil {
		t.Fatalf("读取 dist 失败: %v", err)
	}
	installed := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(distDir, e.Name()))
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", e.Name(), err)
		}
		reader, rerr := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if rerr != nil {
			t.Errorf("真实包 %s 无法解析 zip: %v", e.Name(), rerr)
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".zip")
		// 与 InstallPluginZip 一致：按 pluggin.yaml 所在目录推导 rootPrefix，
		// 覆盖前缀剥离逻辑（含顶层目录的真实包不会被误判为解压不完整）。
		var rootPrefix string
		for _, f := range reader.File {
			if filepath.Base(f.Name) == "pluggin.yaml" {
				if d := filepath.ToSlash(filepath.Dir(f.Name)); d != "." {
					rootPrefix = d + "/"
				}
				break
			}
		}
		// 本测试只验证解压安全与完整性（StageZipExtract 全量校验解压），
		// 不触发引擎 Load——nil-db 测试引擎缺 database 表，依赖该权限的
		// 插件 Lua 必然执行失败，且首装失败会回滚删除目录，无法据此断言。
		// 运行时依赖交给专用 e2e 测试。
		pe, _ := newMiniTestEngine(t, nil)
		destDir := filepath.Join(pe.basePath, name)
		staging, serr := StageZipExtract(reader, rootPrefix, destDir)
		if serr != nil {
			t.Errorf("真实包 %s 解压失败: %v", e.Name(), serr)
			continue
		}
		if _, merr := os.Stat(filepath.Join(staging, "pluggin.yaml")); merr != nil {
			t.Errorf("真实包 %s 解压不完整（manifest 缺失）: %v", e.Name(), merr)
			_ = os.RemoveAll(staging)
			continue
		}
		_ = os.RemoveAll(staging)
		installed++
	}
	t.Logf("批量安装成功 %d 个真实商店包", installed)
}
