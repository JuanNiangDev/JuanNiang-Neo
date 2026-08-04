# data-store

数据持久化示例：**Redis 缓存**（`cache`）+ **Postgres 数据库**（`database`），演示插件的状态存储。

## 文件结构

```
data/pluggins/data-store/
├── pluggin.yaml
├── main.lua
└── README.md
```

## 功能

| 命令 | 说明 |
|------|------|
| `/counter` | Redis 计数器（按 QQ 号，24h 有效，演示 `cache.set/get`） |
| `/note add <内容>` | 保存笔记到 Postgres（演示 `database.exec` + 建表） |
| `/note list` | 列出最近 10 条笔记（`database.query`） |
| `/note del <id>` | 删除笔记 |
| `/cache demo` | 演示 `cache.set/get/exists/del` 全家桶 |

## 覆盖的 API

### cache（Redis，键自动加 `pluggin:<name>:` 前缀）

```lua
jn.cache.set(key, value, ttl_seconds)   -- ttl=0 永不过期
local v = jn.cache.get(key)             -- 自动 JSON 反序列化
jn.cache.exists(key)                    -- 0 或 1
jn.cache.del(key)
```

> 缓存值是 JSON 序列化的：存数字请 `json.encode(n)`，读回来 `tonumber(v)`。

### database（Postgres，SQL 参数化用 `?` 占位）

```lua
-- 加载时建表（自定义表必须加插件前缀，防止与其他插件冲突）
database.exec([[CREATE TABLE IF NOT EXISTS data_store_notes (...)]])
local rows, err = database.query("SELECT ... WHERE id = ?", 1)
local n, err = database.exec("INSERT ... VALUES (?, ?)", a, b)
```

> ⚠ **无命名空间隔离**：表名务必带自己的前缀（示例用 `data_store_`）。

## 权限

`permissions: [database, cache, onebot11]`

## 试用

- `/counter` 连点几次看计数累加
- `/note add 记得买奶茶` → `/note list` → `/note del 1`
- 重启服务后 `/note list` 数据仍在（Postgres 持久化）；`/counter` 24h 内也保留（Redis）
