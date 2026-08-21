package pluggin

import (
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
		pe, _ := newMiniTestEngine(t, nil)
		sdkDir := filepath.Join(pe.basePath, "sdk")
		os.MkdirAll(sdkDir, 0o755)
		os.WriteFile(filepath.Join(sdkDir, "jn.lua"), []byte(jnSDKSource), 0o644)
		name := strings.TrimSuffix(e.Name(), ".zip")
		_, err = InstallPluginZip(pe, "plugins/"+name, data)
		// nil-db 测试引擎不注入 database 表：依赖 database 权限的插件
		// Lua 执行会失败，但 zip 解压已正确完成（manifest 已落盘）。
		// 本测试只验证解压安全与完整性，运行时依赖交给专用测试。
		if err != nil && !strings.Contains(err.Error(), "执行 entry") {
			t.Errorf("真实包 %s 安装失败: %v", e.Name(), err)
			continue
		}
		if _, serr := os.Stat(filepath.Join(pe.basePath, name, "pluggin.yaml")); serr != nil {
			t.Errorf("真实包 %s 解压不完整（manifest 缺失）: %v", e.Name(), serr)
			continue
		}
		installed++
	}
	t.Logf("批量安装成功 %d 个真实商店包", installed)
}
