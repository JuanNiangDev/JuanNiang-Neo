// Package web 提供前端 SPA 静态资源服务。
//
// 运行时通过环境变量 WEB_DIR 指定前端构建产物目录 (默认 web/dist)。
// 后端 Hertz 引擎在所有 /api/v1 业务路由注册之后, 通过 NoRoute 兜底:
//   - /api/* 命中未注册路由 -> 返回统一 JSON 404 信封
//   - /health 之外的其它路径 -> 尝试 serve 文件, 缺失则回退 index.html
//     (支持 Vue Router history 模式的客户端路由)
//   - 若前端未构建 (index.html 缺失) -> 返回引导提示页, 避免裸 404
package web

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"JuanNiang-Neo/internal/api/dto"
)

// DefaultWebDir 是 WEB_DIR 未设置时的默认前端目录。
const DefaultWebDir = "web/dist"

// NotFoundEnvelope 是 /api/* 未命中时返回的标准信封。
var NotFoundEnvelope = dto.Response{Status: 40400, Info: "资源不存在"}

// SPAHandler 返回一个可用于 h.NoRoute 的 handler, 按 webDir 服务前端 SPA。
//
// 行为:
//   - 若请求路径以 /api/ 开头 -> 返回 JSON 404 信封 (不污染前端命名空间)
//   - 否则计算 webDir + path 的安全文件路径:
//       * 文件存在且不是目录 -> 直接 serve
//       * 文件不存在或为目录 -> 回退到 index.html
//   - 若 index.html 缺失 -> 返回 200 文本引导页 (前端未构建)
func SPAHandler(webDir string) app.HandlerFunc {
	indexHTML := filepath.Join(webDir, "index.html")
	return func(ctx context.Context, c *app.RequestContext) {
		path := string(c.Request.URI().Path())

		// /api/* 命中未注册路由: 标准信封 404, 与业务错误格式一致。
		if strings.HasPrefix(path, "/api/") {
			c.AbortWithStatusJSON(consts.StatusNotFound, dto.GenFinalResponse(NotFoundEnvelope, nil))
			return
		}

		// 安全路径: 清理并拼接, 阻止目录穿越。
		rel := filepath.Clean(strings.TrimPrefix(path, "/"))
		if rel == "" || rel == "." || rel == "/" {
			rel = "index.html"
		}
		full := filepath.Join(webDir, rel)
		if !isWithin(webDir, full) {
			c.AbortWithStatus(consts.StatusForbidden)
			return
		}

		// 文件存在且非目录 -> 直接 serve。
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			c.File(full)
			return
		}

		// SPA 兜底: 回退到 index.html, 由前端路由处理。
		if _, err := os.Stat(indexHTML); err == nil {
			c.File(indexHTML)
			return
		}

		// 前端未构建。
		slog.Warn("前端未构建, 请先执行 make web-build 或 npm run build",
			"web_dir", webDir, "missing", indexHTML)
		serveFrontendNotBuiltHint(c)
	}
}

// isWithin 判断 target 是否落在 base 目录内 (含 base 本身)。
func isWithin(base, target string) bool {
	absBase, err1 := filepath.Abs(base)
	absTarget, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// serveFrontendNotBuiltHint 返回引导提示页, 200 让运维直接看到。
func serveFrontendNotBuiltHint(c *app.RequestContext) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	const hint = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<title>JuanNiang-Neo - 前端未构建</title>
<style>body{font-family:system-ui,Segoe UI,sans-serif;margin:8vh auto;max-width:680px;padding:0 1rem;color:#222}code{background:#f4f4f5;padding:.1em .4em;border-radius:4px;font-size:.95em}pre{background:#0d1117;color:#e6edf3;padding:1rem;border-radius:8px;overflow:auto}</style>
</head><body>
<h1>前端尚未构建</h1>
<p>JuanNiang-Neo 后端已就绪, 但 <code>WEB_DIR</code> 目录下未找到 <code>index.html</code>。</p>
<h3>开发模式 (热更新)</h3>
<pre>cd web && npm install && npm run dev</pre>
<p>然后在浏览器访问 Vite 开发服务器 (默认 <code>http://localhost:3000</code>), 它会自动代理 <code>/api</code> 到后端。</p>
<h3>生产模式 (单容器)</h3>
<pre>make web-build   # 产出 web/dist
make run          # 或 ./bin/juan-niang-neo</pre>
<p>或在容器内构建: <code>docker compose up --build</code>。</p>
<p>当前 WEB_DIR = <code>` + DefaultWebDir + `</code> (可通过环境变量覆盖)。</p>
</body></html>`
	c.String(consts.StatusOK, hint)
}

// EnsureDir 校验 webDir 是否可用 (存在且至少含 index.html 或为空目录)。
// 在启动阶段调用, 用于把"配置错误"前置暴露, 而不是等到首个请求才发现。
func EnsureDir(webDir string) error {
	if webDir == "" {
		return nil
	}
	info, err := os.Stat(webDir)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("WEB_DIR 目录不存在, 前端将走引导提示页", "dir", webDir)
			return nil
		}
		return err
	}
	if !info.IsDir() {
		slog.Warn("WEB_DIR 不是目录", "dir", webDir)
	}
	return nil
}
