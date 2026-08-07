# media-gen

媒体生成示例（**异步版**）：**T2I 文生图**（HTML 模板渲染）与 **Sandbox 代码沙箱**（Python / Shell 执行）。

## 文件结构

```
data/pluggins/media-gen/
├── pluggin.yaml   # 插件清单（名称/入口/权限）
├── main.lua       # 插件逻辑
├── config.yaml    # 动态配置声明
├── avatar.png     # 插件图标
└── README.md
```

## 功能

| 命令 | 说明 |
|------|------|
| `/img <文字>` | T2I 生成渐变海报并发送（`t2i.generate_url_async` → 富文本带图片） |
| `/run <python代码>` | 沙箱异步执行 Python（如 `/run print(1+1)`） |
| `/sb shell <命令>` | 沙箱异步执行 Shell（如 `/sb shell uname -a`） |
| `/sb del` | 删除当前沙箱（同步，快操作） |
| `/sb status` | 查看 T2I / Sandbox 启用状态与配置（同步，快查询） |

## 覆盖的 API

### t2i（需先在 Web「T2I」页启用服务）

渲染图片可能耗时，示例使用**异步版**：

```lua
local ctx = { action = "img", target = { kind = "group", id = 123456 } }
local rid = jn.t2i.generate_url_async(html, nil, ctx)  -- 立即返回 req_id

function on_t2i_response(req_id, ctx, result, err)
    -- result = 图片 URL；ctx 为调用时保存的现场表（原样带回）
end
```

同步版 `t2i.generate` / `t2i.generate_url` 仍可用（低频路径），但会阻塞事件循环。

### sandbox（需先在 Web「Sandbox」页启用服务）

`create` 只返回 ID（快，保留同步）；**代码执行用异步版**：

```lua
local sb, err = jn.sandbox.create()                    -- 同步，{sandbox_id, status}
local ctx = { action = "run", sid = sb.sandbox_id, target = { kind = "group", id = 123456 } }
local rid = jn.sandbox.exec_python_async(sb.sandbox_id, "print(1+1)", ctx)

function on_sandbox_response(req_id, ctx, result, err)
    -- result：exec_python → {output, error}；exec_shell → {output, exit_code}
end
```

同步版 `exec_shell` / `exec_python` 仍可用，但沙箱执行可能耗时数十秒，务必用异步版。

## 调用现场保存（ctx）

`xxx_async` 最后一个 table 参数为调用现场，回调时**原样带回**（不序列化）。示例中 `ctx.action` 区分回调来源（`img`/`run`/`shell`），`ctx.sid` 携带沙箱 ID，`ctx.target` 携带回复目标——一次异步调用后的所有业务状态都延续到回调。

## 权限

`permissions: [t2i, sandbox, onebot11]`

> 服务未启用时 API 返回 `(nil, "T2I 服务未启用")` 等错误，插件做了友好提示。

## 试用

1. 先到 Web「T2I」「Sandbox」页启用服务
2. `/img 今日宜摸鱼` → 群里收到生成的渐变海报
3. `/run print(2**10)` → 输出 1024
4. `/sb shell echo hello` → 输出 hello
5. `/sb status` → 查看服务状态
