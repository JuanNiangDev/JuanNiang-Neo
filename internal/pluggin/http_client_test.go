package pluggin

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildHTTPTransportProxySchemes(t *testing.T) {
	cases := []struct {
		name      string
		proxyURL  string
		wantErr   bool
		wantProxy bool // http(s) 代理时 Transport.Proxy 非 nil
	}{
		{name: "空=直连", proxyURL: ""},
		{name: "http 代理", proxyURL: "http://127.0.0.1:7890", wantProxy: true},
		{name: "https 代理", proxyURL: "https://127.0.0.1:7890", wantProxy: true},
		{name: "socks5 代理", proxyURL: "socks5://127.0.0.1:1080"},
		{name: "socks5 带认证", proxyURL: "socks5://user:pass@127.0.0.1:1080"},
		{name: "socks4 代理", proxyURL: "socks4://127.0.0.1:1081"},
		{name: "socks4a 代理", proxyURL: "socks4a://127.0.0.1:1081"},
		{name: "非法 scheme", proxyURL: "ftp://127.0.0.1", wantErr: true},
		{name: "非法 URL", proxyURL: "://bad", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr, err := buildHTTPTransport(c.proxyURL)
			if c.wantErr {
				if err == nil {
					t.Fatalf("期望错误，实际 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildHTTPTransport(%q) 意外错误: %v", c.proxyURL, err)
			}
			if c.wantProxy && tr.Proxy == nil {
				t.Fatalf("http 代理应设置 Transport.Proxy，实际 nil")
			}
			if strings.HasPrefix(c.proxyURL, "socks") && !c.wantErr && tr.Proxy != nil {
				t.Fatalf("socks 代理应清空 Transport.Proxy（避免与环境 HTTP 代理冲突），实际非 nil")
			}
			if tr.ForceAttemptHTTP2 {
				t.Fatalf("必须强制 HTTP/1.1（ForceAttemptHTTP2 应为 false）")
			}
		})
	}
}

// TestHTTPClientCacheLRU 代理 client 缓存 LRU：超上限淘汰最久未使用并关闭空闲连接。
// 回归：此前按代理地址无上限缓存，插件循环生成唯一代理串可无限创建 Transport + 连接池。
func TestHTTPClientCacheLRU(t *testing.T) {
	c := newHTTPClientCache()
	// 填充到上限 + 1：第 65 个触发淘汰第一个
	for i := 0; i < httpClientCacheLimit+1; i++ {
		proxy := ""
		if i > 0 {
			proxy = "socks5://127.0.0.1:" + strconv.Itoa(10000+i) // 唯一代理串
		}
		if _, err := c.get(proxy); err != nil {
			t.Fatalf("get(%q) 失败: %v", proxy, err)
		}
	}
	if len(c.clients) != httpClientCacheLimit {
		t.Fatalf("缓存应收敛到上限 %d，实际 %d", httpClientCacheLimit, len(c.clients))
	}
	// 最早的（直连 proxyURL=""）应被淘汰
	if _, ok := c.clients[""]; ok {
		t.Fatal("最久未使用的直连 client 应被淘汰")
	}
	// 命中已存在 key 不新建（回到 LRU 尾部，不淘汰新值）
	last := c.order[len(c.order)-1]
	if _, err := c.get(last); err != nil {
		t.Fatalf("命中缓存失败: %v", err)
	}
	if c.order[len(c.order)-1] != last {
		t.Fatal("命中后应移到 LRU 尾部")
	}
}

// TestSocks4ConnectTimeout 代理 accept 后不响应：握手应在 deadline 内报错而非永久阻塞。
// 回归：此前 conn.Read 无 SetReadDeadline，代理不响应时调用方 goroutine 永久泄漏。
func TestSocks4ConnectTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		conn.Read(buf) // 读请求后不响应
		time.Sleep(2 * time.Second)
	}()

	// 用短 deadline 的 ctx 触发快速超时（不等 socks4HandshakeTimeout 10s）
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := socks4Connect(ctx, ln.Addr().String(), "example.com", 443); err == nil {
		t.Fatal("代理不响应时应返回超时错误，实际 nil")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("握手应在 deadline 内失败，实际耗时 %s", elapsed)
	}
}

// TestSocks4ConnectHandshake 用假 SOCKS4 服务器验证握手请求格式与响应处理。
func TestSocks4ConnectHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	serverReq := make(chan []byte, 2)
	go func() {
		for i := 0; i < 2; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			req := make([]byte, 64)
			n, _ := conn.Read(req)
			serverReq <- req[:n]
			// 响应：VN=0 CD=0x5A(granted) + 2B 端口 + 4B IP
			conn.Write([]byte{0x00, 0x5A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
			time.Sleep(100 * time.Millisecond)
			conn.Close()
		}
	}()

	addr := ln.Addr().String()
	_, portStr, _ := net.SplitHostPort(addr)
	port, err := parsePort(portStr)
	if err != nil {
		t.Fatalf("parsePort(%q): %v", portStr, err)
	}

	// 域名目标 → socks4a 请求（DSTIP=0.0.0.1 + 域名）
	conn1, err := socks4Connect(context.Background(), addr, "example.com", 443)
	if err != nil {
		t.Fatalf("socks4Connect(域名) 失败: %v", err)
	}
	conn1.Close()
	req := <-serverReq
	if len(req) < 9 || req[0] != 0x04 || req[1] != 0x01 {
		t.Fatalf("socks4 请求头非法: %v", req)
	}
	if req[4] != 0 || req[5] != 0 || req[6] != 0 || req[7] != 1 {
		t.Fatalf("socks4a 应使用 DSTIP=0.0.0.1，实际 %v", req[4:8])
	}
	if body := strings.ToLower(string(req[8:])); !strings.Contains(body, "example.com") {
		t.Fatalf("socks4a 请求应包含目标域名，实际 %q", body)
	}

	// IP 目标 → 标准 socks4 请求（DSTIP=真实 IP）
	host := strings.SplitN(addr, ":", 2)[0]
	if ip := net.ParseIP(host).To4(); ip == nil {
		t.Fatalf("测试服务器地址 %s 非 IPv4", host)
	}
	conn2, err := socks4Connect(context.Background(), addr, host, 443)
	if err != nil {
		t.Fatalf("socks4Connect(IP) 失败: %v", err)
	}
	conn2.Close()
	req2 := <-serverReq
	if len(req2) < 9 || req2[1] != 0x01 {
		t.Fatalf("socks4 IP 请求非法: %v", req2)
	}
	if strings.Contains(strings.ToLower(string(req2[8:])), "example.com") {
		t.Fatalf("socks4 IP 请求不应携带域名，实际 %q", req2[8:])
	}

	// 探测目标端口应正确写入请求（大端）
	if req2[2] != 0x01 || req2[3] != 0xBB { // 443 = 0x01BB
		t.Fatalf("socks4 目标端口应为 443(0x01BB)，实际 %02x%02x", req2[2], req2[3])
	}

	// 拒绝响应 → 应返回错误
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen2: %v", err)
	}
	defer ln2.Close()
	go func() {
		conn, err := ln2.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		conn.Read(buf)
		conn.Write([]byte{0x00, 0x5B, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // rejected
	}()
	if _, err := socks4Connect(context.Background(), ln2.Addr().String(), "example.com", 80); err == nil {
		t.Fatalf("代理拒绝时应返回错误，实际 nil")
	}

	_ = port // 端口合法性已验证
}
