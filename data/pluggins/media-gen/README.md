# media-gen

媒体生成示例（**同步版 + 异步版双示范**）：**T2I 文生图**（HTML 模板渲染）与 **Sandbox 代码沙箱**（Python / Shell 执行）。

## 文件结构

```
data/pluggins/media-gen/
├── pluggin.yaml   # 插件清单（名称/入口/权限）
├── main.lua       # 插件逻辑
├── config.yaml    # 动态配置声明
├── avatar.png     # 插件图标
└── README.md
```

## 功能（每个功能都有同步版 + 异步版）

| 命令 | 模式 | API |
|------|------|-----|
| `/img sync <文字>` | 同步 | `t2i.generate_url` |
| `/img <文字>` | 异步 | `t2i.generate_url_async` + ctx |
| `/run sync <python代码>` | 同步 | `sandbox.exec_python` |
| `/run <python代码>` | 异步 | `sandbox.exec_python_async` + ctx |
| `/sb shell sync <命令>` | 同步 | `sandbox.exec_shell` |
| `/sb shell <命令>` | 异步 | `sandbox.exec_shell_async` + ctx |
| `/sb del` | 同步 | `sandbox.delete`（管理类快操作） |
| `/sb status` | 同步 | `is_active`/`get_config`（快查询） |

## 同步 vs 异步

### 同步版（低频快路径，会阻塞事件循环）

```lua
local url, err = jn.t2i.generate_url(html)                -- 阻塞直到渲染完成
local out, exit = jn.sandbox.exec_shell(sid, "ls -la")    -- 沙箱执行可能数十秒
```

### 异步版（渲染/执行耗时，推荐，不阻塞事件循环）

```lua
-- 提交：立即返回 req_id（失败返回 0），阻塞操作在后台 goroutine 完成
local ctx = { action = "img", target = { kind = "group", id = 123456 } }
local rid = jn.t2i.generate_url_async(html, nil, ctx)

local ctx2 = { action = "run", sid = "sb-xxx", target = { kind = "group", id = 123456 } }
local rid2 = jn.sandbox.exec_python_async("sb-xxx", "print(1+1)", ctx2)

-- 完成回调：引擎串行调用（与事件派发互斥）
function on_t2i_response(req_id, ctx, result, err)        -- result = 图片 URL
function on_sandbox_response(req_id, ctx, result, err)    -- result = {output, exit_code|error}
```

## 调用现场保存（ctx）

异步版最后一个参数是 ctx 表：把回复目标、沙箱 ID 等变量打包传入，回调时**原样带回**（不序列化）。示例用 `ctx.action` 区分回调来源（`img`/`run`/`shell`）、`ctx.sid` 携带沙箱 ID、`ctx.target` 携带回复目标——一次异步调用后的业务状态全部延续到回调。业务处理函数（`send_image`/`handle_python`/`handle_shell`）由同步/异步两条路径共用。

## 权限

`permissions: [t2i, sandbox, onebot11]`

> 服务未启用时 API 返回 `(nil, "T2I 服务未启用")` 等错误，插件做了友好提示。

## 试用

1. 先到 Web「T2I」「Sandbox」页启用服务
2. `/img 今日宜摸鱼` → 群里收到渐变海报；`/img sync 今日宜摸鱼` → 同步版
3. `/run print(2**10)` → 输出 1024；`/run sync print(2**10)` → 同步版
4. `/sb shell echo hello` → 输出 hello；`/sb shell sync echo hello` → 同步版
5. `/sb status` → 查看服务状态
