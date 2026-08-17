package tool

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// TestImageURLCandidates 候选链覆盖：原始 → HTML 实体解码 → 百分号解码变体。
// 群聊相关性识图与 vision 工具共用该候选逻辑，任一成功即下载成功。
func TestImageURLCandidates(t *testing.T) {
	raw := "https://multimedia.nt.qq.com.cn/download?appid=1407&amp;rkey=CAESM"
	got := ImageURLCandidates(raw)
	want := []string{
		raw,
		"https://multimedia.nt.qq.com.cn/download?appid=1407&rkey=CAESM",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("实体解码候选错误\n got: %v\nwant: %v", got, want)
	}

	// 百分号编码（LLM 复刻变形）：%26 应产生 QueryUnescape 变体
	// （去重后：原始 + 解码，共 2 个候选）
	encoded := "https://multimedia.nt.qq.com.cn/download?appid=1407%26rkey=x"
	got = ImageURLCandidates(encoded)
	if len(got) != 2 {
		t.Fatalf("百分号编码候选数量错误: %d (%v)", len(got), got)
	}
	if got[1] != "https://multimedia.nt.qq.com.cn/download?appid=1407&rkey=x" {
		t.Fatalf("QueryUnescape 变体缺失: %v", got)
	}
	// 原始 URL 必须始终是第一个候选（最优路径优先）
	if got[0] != encoded {
		t.Fatalf("原始 URL 应为首个候选: %v", got)
	}
}

// TestImageURLCandidatesNoChange 无实体/无编码时不应产生重复候选。
func TestImageURLCandidatesNoChange(t *testing.T) {
	raw := "https://example.com/a.png"
	got := ImageURLCandidates(raw)
	if len(got) != 1 || got[0] != raw {
		t.Fatalf("无变形 URL 应只有原始候选: %v", got)
	}
}

// TestBlockedIP SSRF 防护范围：loopback/私网/链路本地/组播/未指定/CGNAT 拒绝，
// 公网地址放行。
func TestBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.1.1", "0.0.0.0", "100.64.1.1", "224.0.0.1",
		"::1", "fe80::1", "fc00::1", "ff02::1",
	}
	for _, s := range blocked {
		if !blockedIP(net.ParseIP(s)) {
			t.Errorf("%s 应被 SSRF 防护拒绝", s)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "2400:3200::1"}
	for _, s := range allowed {
		if blockedIP(net.ParseIP(s)) {
			t.Errorf("%s 不应被 SSRF 防护拒绝", s)
		}
	}
}

// TestDialRestrictedRejectsLoopback 受限拨号必须拒绝回环地址（SSRF 关键路径）。
func TestDialRestrictedRejectsLoopback(t *testing.T) {
	ctx := context.Background()
	if _, err := dialRestricted(ctx, "tcp", "127.0.0.1:8080"); err == nil {
		t.Fatal("回环地址应被拒绝")
	}
	if _, err := dialRestricted(ctx, "tcp", "[::1]:8080"); err == nil {
		t.Fatal("IPv6 回环地址应被拒绝")
	}
}

// TestDownloadImageBytesRejectsLocalServer 完整路径：httpDownload 经受限客户端
// 下载本地服务必须失败（SSRF 拒绝），防止攻击者借 vision 工具探测内部网络。
func TestDownloadImageBytesRejectsLocalServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("internal data"))
	}))
	defer srv.Close()

	_, err := DownloadImageBytes(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("下载本地服务应被 SSRF 防护拒绝")
	}
	if !strings.Contains(err.Error(), "拒绝访问非公网地址") {
		t.Fatalf("应返回 SSRF 拒绝错误, got: %v", err)
	}
}
