package pluggin

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// ---------------------------------------------------------------------------
// 插件 HTTP 客户端代理支持（可选参数 proxy 指定代理地址）：
//   - 空 / http(s)://host:port  → 标准 HTTP 代理（http.ProxyURL）
//   - socks5://[user:pass@]host:port → SOCKS5（golang.org/x/net/proxy）
//   - socks4://host:port（或 socks4a://） → SOCKS4(a)（自实现握手，支持域名目标）
//
// 所有 client 保留 HTTP/1.1 强制逻辑（与默认 client 一致，规避 HTTP/2 偶发挂起）。
// ---------------------------------------------------------------------------

// httpClientCache 按代理地址缓存 http.Client（连接池复用，避免每次请求重建）。
type httpClientCache struct {
	mu      sync.Mutex
	clients map[string]*http.Client
}

func newHTTPClientCache() *httpClientCache {
	return &httpClientCache{clients: make(map[string]*http.Client)}
}

// get 返回 (可能新建的) client；proxyURL 为空表示直连。scheme 非法时返回错误。
func (c *httpClientCache) get(proxyURL string) (*http.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cl, ok := c.clients[proxyURL]; ok {
		return cl, nil
	}
	tr, err := buildHTTPTransport(proxyURL)
	if err != nil {
		return nil, err
	}
	cl := &http.Client{Timeout: 30 * time.Second, Transport: tr}
	c.clients[proxyURL] = cl
	return cl, nil
}

// buildHTTPTransport 构造带代理的 Transport，保留 ForceAttemptHTTP2=false 与
// ALPN 清 h2 的逻辑（基于 DefaultTransport.Clone()）。
func buildHTTPTransport(proxyURL string) (*http.Transport, error) {
	// 强制 HTTP/1.1：Go 默认 HTTP/2 与部分站点（api.github.com、mp.weixin.qq.com）
	// 的连接偶发挂起（30s 等不到响应头，wget/HTTP/1.1 秒回），降级为 1.1 稳定。
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	// Clone() 会先触发 DefaultTransport 的 h2 初始化（http2configureTransports 把
	// "h2" 写进其 TLSClientConfig.NextProtos）。TLSNextProto 置空后必须同步从 ALPN
	// 清掉 h2，否则 TLS 协商出 h2 却无处理器，请求直接 EOF。
	if tcc := transport.TLSClientConfig; tcc != nil {
		protos := tcc.NextProtos[:0]
		for _, p := range tcc.NextProtos {
			if p != "h2" {
				protos = append(protos, p)
			}
		}
		tcc.NextProtos = protos
	}

	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return transport, nil
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("无效的代理地址 %q: %w", proxyURL, err)
	}
	switch u.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(u)
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if u.User != nil {
			auth = &proxy.Auth{User: u.User.Username()}
			if p, ok := u.User.Password(); ok {
				auth.Password = p
			}
		}
		d, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 代理初始化失败: %w", err)
		}
		if cd, ok := d.(proxy.ContextDialer); ok {
			transport.DialContext = cd.DialContext
		} else {
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return d.Dial(network, addr)
			}
		}
	case "socks4", "socks4a":
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("socks4 目标地址非法 %q: %w", addr, err)
			}
			port, err := parsePort(portStr)
			if err != nil {
				return nil, err
			}
			return socks4Connect(ctx, u.Host, host, port)
		}
	default:
		return nil, fmt.Errorf("不支持的代理协议 %q（支持 http/https/socks4/socks4a/socks5）", u.Scheme)
	}
	if strings.HasPrefix(u.Scheme, "socks") {
		// 走 socks 拨号器时清掉环境变量代理：请求已被 DialContext 路由到 socks 代理，
		// 保留 Proxy 会让 Transport 先尝试把请求发往环境 HTTP 代理，二者冲突。
		transport.Proxy = nil
	}
	return transport, nil
}

// ---------------------------------------------------------------------------
// SOCKS4 / SOCKS4a 客户端（自实现握手；socks4a 变体支持域名目标）
// ---------------------------------------------------------------------------

const (
	socks4Version    = 0x04
	socks4CmdConnect = 0x01
	socks4Granted    = 0x5A
)

func parsePort(s string) (int, error) {
	var p int
	if _, err := fmt.Sscanf(s, "%d", &p); err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("端口非法 %q", s)
	}
	return p, nil
}

// socks4Connect 通过 socks4(a) 代理建立 CONNECT 隧道。
// 目标为 IP 时走标准 socks4（DSTIP=真实 IP）；目标为主机名时走 socks4a
// （DSTIP=0.0.0.1，USERID 后附加域名）。
func socks4Connect(ctx context.Context, proxyAddr, host string, port int) (net.Conn, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("连接 socks4 代理 %s 失败: %w", proxyAddr, err)
	}
	closeOnErr := func(e error) (net.Conn, error) {
		conn.Close()
		return nil, e
	}

	// 请求头：VN CD DSTPORT(2B 大端) DSTIP(4B) USERID(空) [socks4a: HOSTNAME NUL]
	buf := []byte{socks4Version, socks4CmdConnect, 0, 0, 0, 0, 0, 1, 0}
	buf[2], buf[3] = byte(port>>8), byte(port&0xFF)
	var req []byte
	if ip := net.ParseIP(host); ip != nil {
		ip4 := ip.To4()
		if ip4 == nil {
			return closeOnErr(fmt.Errorf("socks4 仅支持 IPv4 目标地址: %s", host))
		}
		copy(buf[4:8], ip4)
		req = buf
	} else {
		// socks4a：DSTIP=0.0.0.1，USERID 后接主机名 + NUL
		req = append(append(buf, []byte(host)...), 0)
	}

	if _, err := conn.Write(req); err != nil {
		return closeOnErr(fmt.Errorf("socks4 握手写入失败: %w", err))
	}
	resp := make([]byte, 8)
	if _, err := readFull(conn, resp); err != nil {
		return closeOnErr(fmt.Errorf("socks4 握手读取失败: %w", err))
	}
	if resp[0] != 0 {
		return closeOnErr(fmt.Errorf("socks4 代理返回异常版本 %d", resp[0]))
	}
	if resp[1] != socks4Granted {
		return closeOnErr(fmt.Errorf("socks4 代理拒绝连接（code=0x%02X，目标 %s:%d）", resp[1], host, port))
	}
	return conn, nil
}

// readFull 完整读取 n 字节（兼容 conn 一次读不满的场景）。
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
