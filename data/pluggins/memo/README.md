# memo

JuanNiang-Neo **`jn.file` API 示例插件**：把便签存成插件目录下的文本文件，每个用户一个文件、一行一条。

## 文件结构

```
data/pluggins/memo/
├── pluggin.yaml   # 插件清单（申请了 file + onebot11 权限）
├── main.lua       # 插件逻辑（jn.file 全家桶演示）
├── config.yaml    # 动态配置（string / list 类型示例）
├── avatar.png     # 插件图标
└── README.md
```

数据目录 `data/`（相对插件目录）在首次写入时自动创建，无需手动建。

## 功能

| 命令 | 说明 |
|------|------|
| `/memo add <内容>` | 添加一条便签（`file.append_line` 追加一行） |
| `/memo list` | 列出全部便签（`file.read` 整体读取 + 按行拆分） |
| `/memo get <n>` | 查看第 n 条（`file.read_line`；越界返回 nil） |
| `/memo set <n> <内容>` | 改写第 n 条（`file.write_line`） |
| `/memo del <n>` | 删除第 n 条（`file.read_lines` + `file.write_lines` 重写） |
| `/memo clear` | 清空全部（`file.exists` + `file.write` 整体覆盖） |

## 覆盖的 file API

| API | 在本插件中的用法 |
|-----|------------------|
| `file.read(path)` | `/memo list` 整体读取原文后手动按行拆分 |
| `file.read_lines(path)` | `load_memos()` 按行读取；`set`/`del` 前获取条数 |
| `file.read_line(path, n)` | `/memo get` 读第 n 条；**越界返回 nil**（非错误） |
| `file.write(path, content)` | `/memo clear` 覆盖写入空串 |
| `file.write_lines(path, lines)` | `/memo del` 删除一行后整体重写 |
| `file.write_line(path, n, content)` | `/memo set` 改写第 n 行 |
| `file.append_line(path, content)` | `/memo add` 追加一行 |
| `file.exists(path)` | `/memo clear` 前置检查文件是否存在 |

> **路径安全**：所有 `path` 均相对插件自身目录（`data/pluggins/memo/`），
> 引擎强制禁止绝对路径与 `..` 越界。数据文件实际落在
> `data/pluggins/memo/data/<QQ号>.txt`。

## 配置项（Web 面板可改）

| 配置 | 类型 | 说明 |
|------|------|------|
| `max_notes` | string | 每人最多便签条数（默认 `"50"`） |
| `max_len` | string | 单条便签最大长度，超出截断（默认 `"200"`） |
| `group_whitelist` | list | 允许使用的群号列表（留空 = 所有群；私聊不受限） |

## 权限说明

`pluggin.yaml` 里 `permissions: [onebot11, file]` —— `file` 是新申请的权限，
未申请时 `jn.file` 不会被注入（`nil`），调用会直接报错。

## 试玩

1. 把 `memo` 目录放入 `data/pluggins/`，Web 面板插件页启用
2. 群里发 `/memo add 记得买牛奶` → `已保存第 1 条便签 📝`
3. `/memo list` → 看到编号列表；`/memo get 1` 查看单条
4. `/memo set 1 记得买酸奶` 改写；`/memo del 1` 删除；`/memo clear` 清空
