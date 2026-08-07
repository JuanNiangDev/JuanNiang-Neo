# http-tools

HTTP 请求示例（**异步版**）：`http.get_async` / `http.post_async`，对接一言 API、wttr.in 天气与 httpbin 回显。

## 文件结构

```
data/pluggins/http-tools/
├── pluggin.yaml   # 插件清单（名称/入口/权限）
├── main.lua       # 插件逻辑
├── config.yaml    # 动态配置声明
├── avatar.png     # 插件图标
└── README.md
```

## 功能

| 命令 | 说明 |
|------|------|
| `/hitokoto [类型]` | 一言金句（`a`动画 `b`漫画 `c`游戏 `d`小说 `e`原创 `f`网络 `g`其他） |
| `/weather <城市>` | wttr.in 简版天气（默认北京） |
| `/http post <文本>` | 演示 `http.post_async` + JSON 编解码（httpbin 回显） |

## 覆盖的 API（异步）

外部 HTTP 请求可能耗时（秒级），示例全部使用**异步版**，不阻塞事件循环：

```lua
-- 提交：立即返回 req_id（失败返回 0），阻塞请求在后台 goroutine 完成
local ctx = { action = "weather", target = { kind = "group", id = 123456 } }
local rid = jn.http.get_async("https://wttr.in/北京?format=3", ctx)

-- 完成回调：引擎串行调用 on_http_response(req_id, ctx, result, err)
function on_http_response(req_id, ctx, result, err)
    if err then return end
    -- result = {status=number, body=string}；ctx 为调用时保存的现场表（原样带回）
end
```

同步版 `http.get(url)` / `http.post(url, ct, body)` 仍可用（适合命令等低频快路径），但会阻塞事件循环；耗时请求一律用 `xxx_async`。

## 调用现场保存（ctx）

调用 `xxx_async` 时把要保留的变量打包成一张表作为最后一个参数传入（`get_async(url, ctx)`），引擎按 `req_id` 关联保存，回调时**原样带回**（不序列化，可含函数）。示例中用 `ctx.action` 区分不同命令的响应，`ctx.target` 携带回复目标：

```lua
local ctx = { action = "hitokoto", target = { kind = "group", id = event.group_id } }
local rid = jn.http.get_async(url, ctx)
```

## 权限

`permissions: [http, onebot11]` —— `http` 用于请求外部 API，`onebot11` 用于把结果发回群里。

## 试用

- `/hitokoto` → 随机一言金句
- `/weather 重庆` → 重庆当前天气
- `/http post 卷娘赛高` → httpbin 回显
