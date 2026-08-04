# media-gen

媒体生成示例：**T2I 文生图**（HTML 模板渲染）与 **Sandbox 代码沙箱**（Python / Shell 执行）。

## 文件结构

```
data/pluggins/media-gen/
├── pluggin.yaml
├── main.lua
└── README.md
```

## 功能

| 命令 | 说明 |
|------|------|
| `/img <文字>` | T2I 生成渐变海报并发送（`t2i.generate_url` → 富文本带图片） |
| `/run <python代码>` | 沙箱执行 Python（如 `/run print(1+1)`） |
| `/sb shell <命令>` | 沙箱执行 Shell（如 `/sb shell uname -a`） |
| `/sb del` | 删除当前沙箱 |
| `/sb status` | 查看 T2I / Sandbox 启用状态与配置 |

## 覆盖的 API

### t2i（需先在 Web「T2I」页启用服务）

```lua
local id, err = jn.t2i.generate(html)           -- 返回图片 ID
local url, err = jn.t2i.generate_url(html)      -- 返回公开 URL（适合发消息）
local active = jn.t2i.is_active()
local cfg = jn.t2i.get_config()
-- jn.t2i.toggle(true/false) 也可以启停服务
```

### sandbox（需先在 Web「Sandbox」页启用服务）

```lua
local sb, err = jn.sandbox.create()              -- {sandbox_id, status}
local out, exit = jn.sandbox.exec_shell(sid, "ls -la")
local out, e = jn.sandbox.exec_python(sid, "print(1+1)")
local ok, err = jn.sandbox.delete(sid)
local active = jn.sandbox.is_active()
local cfg = jn.sandbox.get_config()
local list, err = jn.sandbox.list()
```

## 权限

`permissions: [t2i, sandbox, onebot11]`

> 服务未启用时 API 返回 `(nil, "T2I 服务未启用")` 等错误，插件做了友好提示。

## 试用

1. 先到 Web「T2I」「Sandbox」页启用服务
2. `/img 今日宜摸鱼` → 群里收到生成的渐变海报
3. `/run print(2**10)` → 输出 1024
4. `/sb shell echo hello` → 输出 hello
5. `/sb status` → 查看服务状态
