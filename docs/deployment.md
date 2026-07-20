# JuanNiang-Neo 部署与调试指南

本文档面向运维和开发者,覆盖部署模式、环境变量、构建流程、健康检查、日志排查和常见故障处理。

> 配置以**环境变量**为准,`config/config.yaml` 仅为参考文档(二进制不读它)。

---

## 目录

- [1. 部署模式选型](#1-部署模式选型)
- [2. 环境变量参考](#2-环境变量参考)
- [3. Docker Compose 一键部署](#3-docker-compose-一键部署)
- [4. 裸跑部署 (systemd)](#4-裸跑部署-systemd)
- [5. 构建流程](#5-构建流程)
- [6. 前端 SPA 服务](#6-前端-spa-服务)
- [7. 反向代理 (nginx / caddy)](#7-反向代理-nginx--caddy)
- [8. 健康检查与监控](#8-健康检查与监控)
- [9. 日志与调试](#9-日志与调试)
- [10. 数据库 / Redis 排错](#10-数据库--redis-排错)
- [11. 优雅退出 / Ctrl-C 行为](#11-优雅退出--ctrl-c-行为)
- [12. 升级与回滚](#12-升级与回滚)
- [13. 常见问题排查 (FAQ)](#13-常见问题排查-faq)

---

## 1. 部署模式选型

| 模式 | 适合场景 | 优点 | 缺点 |
|------|----------|------|------|
| **Docker Compose** | 单机生产 / 试用 | 一键起 Postgres + Redis + app | 单机,扩容需额外工具 |
| **裸跑 + systemd** | 已有 PG/Redis 集群 | 复用现有基础设施 | 需手工管理 unit / 改 env |
| **构建后单 binary** | 离线部署 / 边缘机器 | 一个二进制 + 一份 dist | 仍要外接 PG/Redis |

> 单一二进制本身**不嵌入**前端,生产需提供 `WEB_DIR` 指向 `web/dist`,否则后端会退化为"引导提示页"。

---

## 2. 环境变量参考

由 `cmd/server/main.go` 实际读取,所有变量均有默认值便于本地试用,但**生产环境必须改 `JWT_SECRET` / `DB_PASSWORD` / `REDIS_PASSWORD`**。

### 2.1 Web 面板

| 变量 | 默认 | 说明 |
|------|------|------|
| `API_ADDR` | `:8090` | Hertz web 监听地址 |
| `WEB_DIR` | `web/dist` | 前端构建产物目录;留空禁用 SPA 服务(开发模式) |
| `JWT_SECRET` | `change-me-in-production` | JWT HS256 签名密钥,生产必须改 |
| `REDIS_DB` | `0` | Redis 数据库编号 |

### 2.2 OneBot11 适配器

| 变量 | 默认 | 说明 |
|------|------|------|
| `OB_PORT` | `8081` | 反向 WS 监听端口 |
| `OB_TOKEN` | (空) | 接入校验 token;客户端须带同样 token;留空不校验 |
| `OB_ADMINS` | (空) | 管理员 QQ 号,逗号分隔,例 `"10001,10002"` |

### 2.3 Postgres

| 变量 | 默认 |
|------|------|
| `DB_HOST` | `localhost` |
| `DB_PORT` | `5432` |
| `DB_USER` | `postgres` |
| `DB_PASSWORD` | `postgres` |
| `DB_NAME` | `juan` |

表结构由 GORM `AutoMigrate` 启动时自动建,无需执行 `sql/init.sql`(该文件仅参考)。

### 2.4 Redis

| 变量 | 默认 |
|------|------|
| `REDIS_ADDR` | `localhost:6379` |
| `REDIS_PASSWORD` | `root` |
| `REDIS_DB` | `0` |

### 2.5 可选基础设施(T2I / Sandbox)

这两项**通过 Web 面板热配**,持久化在 Postgres;以下环境变量只是参考,DB 配置后端启动时优先加载。

| 变量 | 默认 | 说明 |
|------|------|------|
| `T2I_BASE_URL` | (空) | 文生图服务 BaseURL(容器版可不设) |
| `SANDBOX_BASE_URL` | (空) | 代码沙箱 BaseURL |
| `SANDBOX_API_KEY` | (空) | 代码沙箱 API Key |

---

## 3. Docker Compose 一键部署

### 3.1 安装依赖

- Docker Engine ≥ 20.10
- Docker Compose v2(`docker compose` 子命令)

### 3.2 步骤

```bash
# 拉代码
git clone <repo-url> JuanNiang-Neo
cd JuanNiang-Neo

# 准备 .env
cp .env.example .env
$EDITOR .env     # 至少改 JWT_SECRET / DB_PASSWORD / REDIS_PASSWORD

# 启动整套
make docker-up
# 或直接:
# docker compose -f deployments/docker-compose.yaml up -d --build

# 看日志
make docker-logs

# 验证
curl http://localhost:8090/health
# -> {"status":"ok"}

# 浏览器访问 http://localhost:8090
# 首次登录 admin / Admin123 -> 立刻在「设置」页改密码
```

### 3.3 端口

| 端口 | 用途 |
|------|------|
| 8081 | OneBot11 反向 WS(QQ 机器人连入此端口) |
| 8090 | Web 管理面板 + API + 前端 SPA |
| 5432 | Postgres(可选对外,生产建议不暴露) |
| 6379 | Redis(同上) |

### 3.4 卷与挂载

- `pgdata` named volume:数据库持久化
- `../data/pluggins → /app/data/pluggins`:Lua 插件目录(bind-mount,宿机改完容器内即生效)

> ⚠️ Compose 现已挂到 `/app/data/pluggins`(匹配 `cmd/server/main.go` 硬编码加载 `"data/pluggins"` 且 CWD 为 `/app`)。**不要**改为 `/var/lib/juan-niang-neo/...`,插件加载不到。

### 3.5 停止 / 清理

```bash
make docker-down              # docker compose down
docker compose -f deployments/docker-compose.yaml down -v  # 同时删 pgdata
```

---

## 4. 裸跑部署 (systemd)

### 4.1 准备外接基础服务

确保 PG 与 Redis 已就绪,能给本机连通,且数据库 `juan` 已创建(`CREATE DATABASE juan;`)。表由 GORM 自动建。

### 4.2 构建产物

```bash
make build
# 输出: bin/juan-niang-neo, web/dist/
```

### 4.3 systemd unit 示例

`/etc/systemd/system/juan-niang-neo.service`:

```ini
[Unit]
Description=JuanNiang-Neo
After=network-online.target postgresql.service redis-server.service
Wants=network-online.target

[Service]
Type=simple
User=juan
Group=juan
WorkingDirectory=/opt/juan-niang-neo
EnvironmentFile=/etc/juan-niang-neo.env
ExecStart=/opt/juan-niang-neo/bin/juan-niang-neo
Restart=on-failure
RestartSec=3
# 优雅退出给 15s
TimeoutStopSec=20
KillSignal=SIGINT
SendSIGKILL=yes

# 限制权限
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/opt/juan-niang-neo/data
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

`/etc/juan-niang-neo.env`(每行 `KEY=VALUE`):

```bash
API_ADDR=:8090
WEB_DIR=/opt/juan-niang-neo/web/dist
OB_PORT=8081
OB_TOKEN=
OB_ADMINS=10001,10002
JWT_SECRET=<openssl rand -hex 32>
DB_HOST=127.0.0.1
DB_PORT=5432
DB_USER=juan
DB_PASSWORD=<strong-password>
DB_NAME=juan
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=<strong-password>
REDIS_DB=0
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now juan-niang-neo
sudo systemctl status juan-niang-neo
sudo journalctl -u juan-niang-neo -f
```

### 4.4 部署目录建议

```
/opt/juan-niang-neo/
  bin/juan-niang-neo
  web/dist/
  data/pluggins/<name>/
    main.lua
    pluggin.yaml
```

---

## 5. 构建流程

### 5.1 依赖

- Go 1.25+
- Node.js 18–24(20 LTS 推荐)
- npm
- make(可选)

### 5.2 make 一键

```bash
make           # 等价 make build
make build     # web-build -> build-go, 产出 web/dist + bin/juan-niang-neo
```

### 5.3 手工分步

```bash
# 前端
cd web
npm ci
npm run build      # 产出 web/dist/
cd ..

# 后端
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/juan-niang-neo ./cmd/server
```

### 5.4 交叉编译

```bash
# Linux amd64 例
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -trimpath -ldflags "-s -w" \
  -o bin/juan-niang-neo-linux-amd64 ./cmd/server
```

> CGO 默认禁用,无 cgo 依赖,纯静态可执行,可直接扔进 `scratch`/`alpine` 镜像。

---

## 6. 前端 SPA 服务

后端复用 Hertz `NoRoute` 兜底同端口服务 Vue SPA。

| 情景 | `WEB_DIR` 应设为 |
|------|-------------------|
| 开发(热更新) | 留空:前端走 Vite `:3000`,后端 `:8090` 仅 API |
| 裸跑生产 | `web/dist` |
| Docker | `/app/web/dist`(默认即如此) |
| 未构建 | 后端返回 200 引导提示页,不影响 API 与 `/health` |

具体见 `docs/architecture.md` 的「前端 SPA 静态服务」段。

---

## 7. 反向代理 (nginx / caddy)

如需 HTTPS 或前后端不同源,前面挂 nginx/caddy。

### 7.1 nginx

```nginx
server {
    listen 443 ssl http2;
    server_name juan.example.com;

    ssl_certificate     /etc/ssl/juan.crt;
    ssl_certificate_key /etc/ssl/juan.key;

    client_max_body_size 50m;       # 插件上传

    location /api/ {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        # SSE: /api/v1/logs/stream 需关掉缓冲
        proxy_buffering off;
        proxy_read_timeout 1h;
    }

    location /health {
        proxy_pass http://127.0.0.1:8090;
        access_log off;
    }

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 7.2 caddy(自动 HTTPS)

```caddy
juan.example.com {
    encode zstd gzip

    @api path /api/*
    reverse_proxy @api 127.0.0.1:8090 {
        flush_interval -1   # SSE
    }

    reverse_proxy 127.0.0.1:8090
}
```

> 反代后端无需知道外部域名。OneBot11 反向 WS 端口(8081)通常**不**通过 nginx 暴露,直接让机器人客户端连本机 IP:8081 即可,反代只服务 Web 面板。

---

## 8. 健康检查与监控

### 8.1 健康端点

```
GET /health            (无鉴权,200 {"status":"ok"})
GET /api/v1/overview   (需 JWT;系统总览)
```

Dockerfile 和 compose 已内置 `HEALTHCHECK`:
```
wget -qO- http://127.0.0.1:8090/health >/dev/null 2>&1 || exit 1
```

### 8.2 监控要点

- `/api/v1/overview` 字段:`goroutine_num` `mem_alloc_bytes` `cpu_count` `go_version` 等
- 日志:`docker logs -f` 或 `journalctl -u juan-niang-neo -f`
- 前端的「仪表盘」页基于 `/overview` 自动渲染

---

## 9. 日志与调试

### 9.1 日志机制

- 后端用 Go `log/slog` 结构化输出
- `logging.NewHandler` 同时推送最近 250 条到 in-memory `LogHub`
- API 暴露:
  - `GET /api/v1/logs` 拉取最近一批
  - `GET /api/v1/logs/stream` SSE 实时流
- 前端 Logs 页轮询 `/logs`(SSE UI 暂未实现,见 README 已知限制)

### 9.2 启用更详细级别

`cmd/server/main.go` 当前固定 `LevelInfo`。如需 debug,改:

```go
slog.SetDefault(slog.New(logging.NewHandler(os.Stdout, logging.Default, &slog.HandlerOptions{
    Level: slog.LevelDebug,
})))
```

### 9.3 数据库验证

```sql
-- 检查 AutoMigrate 结果
\dt

-- 检查 adapter 配置(ID=1)
SELECT * FROM onebot11_adapters LIMIT 1;

-- 检查 admin
SELECT id, username, role FROM admin_users;
```

### 9.4 Redis 验证

```bash
redis-cli -a <password> -h <host> -p <port> -n 0
> KEYS *
> KEYS session:*
```

### 9.5 性能分析(pprof)

当前未内置 pprof 端点。如需调优,临时在 `engine.go` 加:

```go
import _ "net/http/pprof"
// 在某个未受保护端口单独起 http.ListenAndServe("127.0.0.1:6060", nil)
```

---

## 10. 数据库 / Redis 排错

### 10.1 `NOAUTH Authentication required`

Redis 客户端没传 password 可能被丢。**已验证**:本仓库 `infrastructure/redis/client.go:48` 在构造 `redis.Options` 时已带上 `Password` + `DB`。若仍出现该错误,先检查:

- `REDIS_ADDR` 是否指向正确的实例
- `REDIS_PASSWORD` 是否正确(env 里没有引号、空格或转义错)
- `REDIS_DB` 是否对(连错了 DB 编号会显示空但不会 NOAUTH)
- 远程 Redis 是否启了 ACL(`AUTH user pass` 而非 `AUTH pass`),go-redis v9 默认用 `AUTH pass`,需用 `NewFailoverClient` + `Username` 字段或切 ACL 账号

### 10.2 `type "tinyint" does not exist` (Postgres)

历史 bug:某些 model 用了 MySQL 的 `tinyint(1)`。**已修**:全部改为 PG 原生 `type:boolean;default:true/false`。若 PG 里残留了之前半成功建表,清理后重启:

```sql
DROP TABLE IF EXISTS onebot11_adapters, sandbox_configs, t2i_configs, webhook_configs CASCADE;
```

然后重启让 GORM 干净重建。

### 10.3 `relation "xxx" already exists`

通常是 AutoMigrate 与手工 SQL 混用造成。统一交给 AutoMigrate,drop 残留表后重启。

### 10.4 `pq: column "is_active" type "boolean" but value is integer`

历史脏数据。`UPDATE <tbl> SET is_active = (is_active::int)::boolean;` 临时修。

### 10.5 Redis Sentinel

本仓库函数名 `NewRedisSentinelClient` 是历史命名,**当前并没做 Sentinel**。如需 Sentinel,需重写为 `redis.NewFailoverClient`,本仓库之外的事,不在本文档范围。

---

## 11. 优雅退出 / Ctrl-C 行为

### 11.1 现状(已修复)

`cmd/server/main.go` 现在:

1. `signal.NotifyContext` 拦 SIGINT/SIGTERM → 触发 `ctx.Done()`
2. `<-ctx.Done()` 返回后,起 goroutine 跑 `shutdown(...)`,按反向顺序停:
   - `webEngine.Shutdown(ctx 5s)` —— Hertz 不再接新请求
   - `webhookAdapter.Stop(ctx 5s)`
   - `adapterProv.Stop(ctx 5s)` —— 关 OneBot11 反向 WS
   - `hago.Stop()` —— 关 `OutputChan`
3. watchdog 并行 `time.After(15s)`,任一组件超时不拖累整体,15s 总预算超时则 `os.Exit(1)` 强制退出
4. 关键点:**不再用 `webEngine.Spin()`**,因为它自带 SIGINT mask 会和主 ctx 抢信号,且其内部 `Shutdown(context.Background())` 没超时,会让 SSE 长连接(`/api/v1/logs/stream`)卡死

### 11.2 行为

- `Ctrl-C` 通常 1–3 ms 内停掉 Web 引擎并进入优雅流程
- 优雅退出在 5s 内完成时打 `已优雅退出`
- 超过 15s 卡死则强制 `os.Exit(1)`,系统层 `Restart=on-failure` 会自动拉起

### 11.3 systemd 中

unit 设 `TimeoutStopSec=20` `KillSignal=SIGINT` `SendSIGKILL=yes`,先给 systemd 看到的 SIGINT(系统层会再等 20s)。这给 systemd 的预算(20s)略大于后端内部 15s,让内部先优雅完成再退出。

---

## 12. 升级与回滚

### 12.1 Docker

```bash
git pull
make docker-up             # --build 会触发重建
```

`pgdata` 是 named volume,升级保留数据;`web/dist` 每次重建,无状态。

### 12.2 裸跑

```bash
sudo systemctl stop juan-niang-neo
# 备份
cp -a bin/juan-niang-neo bin/juan-niang-neo.prev
cp -a web/dist web/dist.prev
# 拉新代码 + 构建
git pull && make build
# 回滚
# cp bin/juan-niang-neo.prev bin/juan-niang-neo && cp -r web/dist.prev web/dist
sudo systemctl start juan-niang-neo
```

### 12.3 数据库迁移

GORM `AutoMigrate` **只加列不加删**,不会破坏旧数据。出现字段移除时需手动:

```sql
ALTER TABLE <tbl> DROP COLUMN <col>;
```

大版本升级建议先 `pg_dump` 备份。

---

## 13. 常见问题排查 (FAQ)

### Q1. `make build` 失败,提示 `Search string not found: "/supportedTSExtensions ..."`

**已修**:`vue-tsc` 已升级到 ≥ 2.x,支持 Node 18–24。若仍报错,核对 `web/node_modules/vue-tsc/package.json` 版本是否 ≥ 2.x,必要时 `cd web && npm install vue-tsc@^2.1.10 --save-dev`。

### Q2. 浏览器打开 `:8090` 显示「前端尚未构建」

页面提示来自后端 SPA fallback。说明:

- `WEB_DIR` 指向的目录里没有 `index.html`
- 跑 `make web-build` 后重启;或留空 `WEB_DIR` 改用 Vite `:3000` 开发

### Q3. `make web-dev` 起来但 `:3000` 不通

检查 Vite 是否监听 `0.0.0.0`(`vite.config.ts` 里 `server.host: '0.0.0.0'`),linux 上防火墙是否放 3000。本机可改 `localhost:3000`。

### Q4. Ctrl-C 不退出 / 卡死

**已修复**,见第 11 章。若仍卡死,通常是后端某个 Stop 没在 5s 内退出,可调小 `shutdownBudget` 或排查到底是哪步卡。

调试技巧:在 `shutdown()` 里每步前后加 `slog.Info("shutdown step X start/ok")`,再 Ctrl-C 看日志。

### Q5. 插件加载不到

- 容器内路径必须 `/app/data/pluggins/<name>/{main.lua, pluggin.yaml}` (CWD `/app` + 硬编码 `"data/pluggins"`)
- 裸跑:相对工作目录的 `data/pluggins/`
- 启动日志看「插件加载失败」行,找具体原因

### Q6. JWT 401 一直返回

- 确认 `JWT_SECRET` 在签发和校验两端一致(同进程内一致即可,但多副本部署必须相同)
- token 默认 72h 过期
- 前端清 `localStorage.token` 重新登录

### Q7. Webhook adapter 不起来

`WebhookConfig.Enabled` 在 DB 里默认 `false`。把 `/api/v1/webhook/config` 的 `enabled` 改 true 后端会自动重启 webhook。也可直接在 Web 面板「Webhook」页操作。

### Q8. T2I / Sandbox 配了不生效

- 检查 `is_active=true`
- `GET /api/v1/t2i/health` / `/sandbox/health` 看连通性
- 客户端 connguard:`OnUpdateT2I` / `OnUpdateSandbox` 回调在 main.go 注入,DB 更新会自动重建客户端

### Q9. 日志走哪了?

- stdout(初次日志 systemd / docker 都收得到)
- in-memory Hub(最近 250 条 + SSE)
- 无文件输出;如需文件,systemd `StandardOutput=file:...` 或写 unit 自定义 `ExecStartPost` 配 `json-file` log driver

---

## 附录:生产部署 Checklist

- [ ] `JWT_SECRET` 改成 `openssl rand -hex 32` 产生的随机串
- [ ] `DB_PASSWORD` / `REDIS_PASSWORD` 改强密码,Postgres `pg_hba.conf` 不开公网
- [ ] 默认 admin 密码 `Admin123` 第一次登录后立即改
- [ ] Postgres 5432、Redis 6379 不对外暴露(web 反代只暴露 8090)
- [ ] 反代开了 SSE 不缓冲(`proxy_buffering off` / `flush_interval -1`)
- [ ] 部署目录 `data/pluggins/` 备份策略
- [ ] 监控 `/health`,失败自动重启
- [ ] 数据库定期 `pg_dump` 备份
- [ ] 部署前 `make build` 通过;`make vet` / `make lint` 通过
- [ ] 镜像做了 nonroot user(Dockerfile 已设 `jn`);端口未开特权
- [ ] `OB_TOKEN` 改非空 token,避免反向 WS 被乱连
- [ ] `OB_ADMINS` 列出实际管理员 QQ 号,避免空列表无法接搜指令