# http-tools

HTTP 请求示例：`http.get` / `http.post`，对接一言 API、wttr.in 天气与 httpbin 回显。

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
| `/http post <文本>` | 演示 `http.post` + JSON 编解码（httpbin 回显） |

## 覆盖的 API

```lua
-- GET：返回 {status=number, body=string}
local res, err = jn.http.get("https://v1.hitokoto.cn/?encode=json")
if res and res.status == 200 then
    local data = json.decode(res.body)
    log.info(data.hitokoto)
end

-- POST：http.post(url, content_type, body)
local res, err = jn.http.post("https://httpbin.org/post", "application/json", '{"k":"v"}')
```

- 超时 30 秒
- `http` 权限才会注入该全局表

## 权限

`permissions: [http, onebot11]` —— `http` 用于请求外部 API，`onebot11` 用于把结果发回群里。

## 试用

- `/hitokoto` → 随机一言金句
- `/weather 重庆` → 重庆当前天气
- `/http post 卷娘赛高` → httpbin 回显
