package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"JuanNiang-Neo/internal/agent/provider"
)

// TestVisionURLLive 真实链路验证：下载图片 → 转码 JPEG → image_model Provider 描述。
// 图片下载按安全要求禁用代理、只允许公网直连，因此该测试需要在具备公网直连的
// 服务器上运行；默认跳过，需要真实端点时显式提供环境变量：
//
//	VISION_ENDPOINT=http://127.0.0.1:5276/v1 \
//	VISION_KEY=<wb-api token> \
//	VISION_IMG=https://daily.juya.uk/icon.png \
//	go test ./internal/agent/ -run TestVisionURLLive -v
func TestVisionURLLive(t *testing.T) {
	endpoint := os.Getenv("VISION_ENDPOINT")
	token := os.Getenv("VISION_KEY")
	imageURL := os.Getenv("VISION_IMG")
	if endpoint == "" || token == "" || imageURL == "" {
		t.Skip("未设置 VISION_ENDPOINT / VISION_KEY / VISION_IMG，跳过真实链路测试")
	}

	pg := provider.NewProviderGroup()
	pg.AddProvider(provider.NewProvider(provider.ProviderConfig{
		ID:       "wb-api-image",
		Name:     "wb-api-image",
		Type:     provider.ModelTypeImage,
		Endpoint: endpoint,
		Token:    token,
		Model:    "global:deepseek-v4.1-flash",
	}))
	h := &HagoCenter{Providers: pg}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	desc, err := h.VisionURL(ctx, "wb-api-image", imageURL, "用一句中文描述这张图片的内容")
	if err != nil {
		t.Fatalf("VisionURL 失败: %v", err)
	}
	if desc == "" {
		t.Fatal("描述为空")
	}
	t.Logf("图片描述: %s", desc)
}

// TestVisionURLLiveByName 空 provider 名时回退到启用的 image_model。
func TestVisionURLLiveByName(t *testing.T) {
	endpoint := os.Getenv("VISION_ENDPOINT")
	token := os.Getenv("VISION_KEY")
	imageURL := os.Getenv("VISION_IMG")
	if endpoint == "" || token == "" || imageURL == "" {
		t.Skip("未设置 VISION_ENDPOINT / VISION_KEY / VISION_IMG，跳过真实链路测试")
	}

	pg := provider.NewProviderGroup()
	pg.AddProvider(provider.NewProvider(provider.ProviderConfig{
		ID:       "wb-api-image",
		Name:     "wb-api-image",
		Type:     provider.ModelTypeImage,
		Endpoint: endpoint,
		Token:    token,
		Model:    "global:deepseek-v4.1-flash",
	}))
	h := &HagoCenter{Providers: pg}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	desc, err := h.VisionURL(ctx, "", imageURL, "这张图里有什么？一句话回答")
	if err != nil {
		t.Fatalf("VisionURL(空名回退) 失败: %v", err)
	}
	t.Logf("回退描述: %s", desc)
}
