package pluggin

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInstallRealStoreZipE2E 端到端：真实商店 dist/welcome.zip 完整安装加载。
func TestInstallRealStoreZipE2E(t *testing.T) {
	zipPath := filepath.Join("..", "..", "..", "Plugins", "dist", "welcome.zip")
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		t.Skipf("商店包不存在（需 Plugins 仓库同级检出）: %s", zipPath)
	}
	data, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("读取商店包失败: %v", err)
	}
	pe, _ := newMiniTestEngine(t, nil)
	// SDK 落盘：生产由 ensureEmbeddedAssets 启动时写入 basePath/sdk，
	// 测试环境手动补齐（welcome 插件 require("jn") 依赖）
	sdkDir := filepath.Join(pe.basePath, "sdk")
	if err := os.MkdirAll(sdkDir, 0o755); err != nil {
		t.Fatalf("创建 sdk 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sdkDir, "jn.lua"), []byte(jnSDKSource), 0o644); err != nil {
		t.Fatalf("写入 sdk/jn.lua 失败: %v", err)
	}
	name, err := InstallPluginZip(pe, "plugins/welcome", data)
	if err != nil {
		t.Fatalf("真实商店包安装失败: %v", err)
	}
	if name != "welcome" {
		t.Fatalf("目录名应为 welcome，得到 %q", name)
	}
	// 验证落盘完整：manifest + 入口存在
	if _, err := os.Stat(filepath.Join(pe.basePath, "welcome", "pluggin.yaml")); err != nil {
		t.Fatalf("manifest 未落盘: %v", err)
	}
	// 验证引擎已注册该插件
	pe.mu.RLock()
	_, registered := pe.plugins[name]
	pe.mu.RUnlock()
	if !registered {
		t.Fatal("插件未注册进引擎")
	}
}
