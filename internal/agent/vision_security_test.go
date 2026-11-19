package agent

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.1.1", "100.64.0.1", "::1"} {
		if isPublicIP(net.ParseIP(raw)) {
			t.Fatalf("非公网地址被放行: %s", raw)
		}
	}
	if !isPublicIP(net.ParseIP("1.1.1.1")) || !isPublicIP(net.ParseIP("2606:4700:4700::1111")) {
		t.Fatal("公网地址被误拦截")
	}
}

func TestToJPEGRejectsNonImage(t *testing.T) {
	if _, err := toJPEG([]byte("<html>error</html>")); err == nil {
		t.Fatal("非图片内容应返回错误，不能原样送给 Provider")
	}
}

func TestToJPEGConvertsPNG(t *testing.T) {
	// 现场生成 1x1 PNG，避免测试素材失效
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var src bytes.Buffer
	if err := png.Encode(&src, img); err != nil {
		t.Fatal(err)
	}
	raw := src.Bytes()
	// 确认测试素材本身是合法 PNG（解码器已注册）
	if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatalf("测试图片无效: %v", err)
	}
	out, err := toJPEG(raw)
	if err != nil {
		t.Fatalf("PNG 转 JPEG 失败: %v", err)
	}
	if !bytes.HasPrefix(out, []byte{0xff, 0xd8}) {
		t.Fatal("输出不是 JPEG")
	}
}
