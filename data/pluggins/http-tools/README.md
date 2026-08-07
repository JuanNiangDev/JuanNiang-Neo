# http-tools

HTTP 请求示例（**同步版 + 异步版双示范**）：`http.get/post`（同步）与 `http.get_async/post_async`（异步），对接一言 API、wttr.in 天气与 httpbin 回显。

## 文件结构

```
data/pluggins/http-tools/
├── pluggin.yaml   # 插件清单（名称/入口/权限）
├── main.lua       # 插件逻辑
├── config.yaml    # 动态配置声明
├── avatar.png     # 插件图标
└── README.md
```

## 功能（每个功能都有同步版 + 异步版）

| 命令 | 模式 | API |
|------|------|-----|
| `/hitokoto [类型]` | 同步 | `http.get` |
| `/hitokoto async [类型]` | 异步 | `http.get_async` + ctx |
| `/weather <城市>` | 同步 | `http.get` |
| `/weather async <城市>` | 异步 | `http.get_async` + ctx |
| `/http post <文本>` | 同步 | `http.post` |
| `/http post async <文本>` | 异步 | `http.post_async` + ctx |

> `[类型]`：`a`动画 `b`漫画 `c`游戏 `d`小说 `e`原创 `f`网络 `g`其他

## 同步 vs 异步

### 同步版（低频快路径，会阻塞事件循环）

```lua
local res, err = jn.http.get("https://v1.hitokoto.cn/?encode=json")
if err then log.warn(err) return end
-- res = {status=number, body=string}
local data = json.decode(res.body)
```

### 异步版（耗时请求推荐，不阻塞事件循环）

```lua
-- 提交：立即返回 req_id（失败返回 0），请求在后台 goroutine 完成
local ctx = { action = "hitokoto", target = { kind = "group", id = 123456 } }
local rid = jn.http.get_async("https://v1.hitokoto.cn/?encode=json", ctx)

-- 完成回调：引擎串行调用（与事件派发互斥）
function on_http_response(req_id, ctx, result, err)
    -- result = {status, body}；ctx 为调用时保存的现场表（原样带回）
end
```

## 调用现场保存（ctx）

异步版最后一个参数是 ctx 表：把回复目标等变量打包传入，引擎按 `req_id` 关联保存，回调时**原样带回**（不序列化）。示例用 `ctx.action` 区分回调来源、`ctx.target` 携带回复目标。业务处理函数（`handle_hitokoto` 等）由同步/异步两条路径共用，避免逻辑重复。

## 权限

`permissions: [http, onebot11]` —— `http` 用于请求外部 API，`onebot11` 用于把结果发回群里。

## 试用

- `/hitokoto` → 随机一言金句（同步）；`/hitokoto async` → 同一功能异步版
- `/weather 重庆` → 重庆天气；`/weather async 重庆` → 异步版
- `/http post 卷娘赛高` → httpbin 回显；`/http post async 卷娘赛高` → 异步版
