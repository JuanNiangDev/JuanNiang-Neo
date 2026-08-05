# 插件商店（Plugin Store）与动态配置

本文档介绍插件商店浏览/安装、插件动态配置（`config.yaml` + `jn.config`）、镜像源管理，以及插件仓库的元数据与审核流程。

## 1. 统一插件格式（新格式 5 件套）

每个插件目录包含：

```
plugins/<name>/
├── *.lua          # 插件入口（默认 main.lua）
├── pluggin.yaml   # 元数据（name/version/author/description/entry/permissions/system/enabled）
├── config.yaml    # 动态配置声明（type: bool/string/list）
├── README.md      # 说明文档（商店详情页渲染）
└── avatar.png     # 图标（网格卡片展示）
```

`pluggin.yaml` + `main.lua` 为必需；`config.yaml` / `README.md` / `avatar.png` 缺失时插件仍可运行，但商店会标记缺项。

## 2. 动态配置（config.yaml + jn.config）

### config.yaml 声明

```yaml
configs:
  - key: admin_qq
    type: string
    label: 管理员QQ
    description: 可操作本插件的管理员
    default: ""
  - key: auto_reply
    type: bool
    label: 自动回复
    default: true
  - key: trigger_words
    type: list
    label: 触发关键词
    default: ["你好", "在吗"]
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `key` | string | 配置键（唯一） |
| `type` | string | `bool`\|`string`\|`list` |
| `label` | string | Web 展示名 |
| `description` | string | 说明（可选） |
| `default` | any | 默认值 |
| `value` | any | 用户当前值（缺省回退 default） |

### type → Web 控件映射

| type | Web 表现 | 值类型 |
|------|----------|--------|
| `bool` | 开关（v-switch） | bool |
| `string` | 单行输入框 | string |
| `list` | 可增删的多项输入框 | `string[]` |

### Lua API（`jn.config`，无需权限默认注入）

```lua
local jn = require("jn")
jn.config.get("key")     -- 单值：value 优先，回退 default
jn.config.all()          -- 全部配置 {key=value}
jn.config.schema()       -- 完整 schema
```

保存配置（Web 面板）后插件自动重载使新值生效。

## 3. Web 界面

### Plugin 管理页（`/plugins`）

- **网格卡片**：展示 avatar + 名称 + 简介 + 右上角启用/停用开关
- 点击卡片弹出**三页签**对话框：
  1. **说明**：渲染 `README.md`
  2. **元数据 / 命令**：插件元信息与注册命令
  3. **配置**：按 `config.yaml` 动态渲染表单（无配置项时显示「暂无可配置项」）
- 右下角按钮：启用/停用、重载、删除

### Plugin 商店页（`/plugin-store`）

- 网格卡片浏览商店插件（avatar + 名称 + 简介 + 作者 + 版本）
- 点击卡片查看仓库 `README.md` 预览，可一键安装
- 右上角「镜像源设置」：管理仓库信息与自定义镜像源

## 4. 后端 API

### 已安装插件管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/plugins` | 插件列表 |
| POST | `/api/v1/plugins/upload` | ZIP 上传 |
| POST | `/api/v1/plugins/reload` | 全量重载 |
| POST | `/api/v1/plugins/:id/reload` | 单插件重载 |
| PUT | `/api/v1/plugins/:id/toggle` | 启用/停用 |
| DELETE | `/api/v1/plugins/:id` | 删除 |
| GET | `/api/v1/plugins/:id/config` | 配置 schema + 值 |
| PUT | `/api/v1/plugins/:id/config` | 保存配置 |
| GET | `/api/v1/plugins/:id/readme` | README.md 内容 |
| GET | `/api/v1/plugins/:id/avatar` | avatar.png |

### 插件商店

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/plugin-store` | 商店插件列表 |
| GET | `/api/v1/plugin-store/readme?path=` | 仓库 README |
| GET | `/api/v1/plugin-store/avatar?path=` | 仓库 avatar |
| POST | `/api/v1/plugin-store/install?path=` | 安装插件 |
| GET | `/api/v1/plugin-store/config` | 商店配置 + 镜像列表 |
| PUT | `/api/v1/plugin-store/config` | 更新商店配置 |
| POST | `/api/v1/plugin-store/mirror` | 添加自定义镜像 |
| POST | `/api/v1/plugin-store/mirror/test` | 测试镜像源连通性（拉取 `plugins.json` 验证并返回延迟） |
| POST | `/api/v1/plugin-store/mirror/select` | 手动指定生效镜像源（`mirror` 为空恢复默认自动选择） |
| DELETE | `/api/v1/plugin-store/mirror` | 删除自定义镜像 |

## 5. 镜像源与国内加速

商店客户端（`internal/pluggin/store.go`）按顺序尝试镜像源，第一个成功即命中。内置默认镜像：

```text
https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}
https://ghproxy.net/https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}
https://gh-proxy.com/https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}
https://raw.gitmirror.com/{owner}/{repo}/{branch}/{path}
https://cdn.jsdelivr.net/gh/{owner}/{repo}@{branch}/{path}
```

镜像模板支持占位符：`{owner}` `{repo}` `{branch}` `{path}`。Web 「镜像源设置」中可在**下拉框中手动选择生效镜像源**（不做自动切换：选择后只使用该镜像，恢复「自动选择」后才按列表顺序尝试），并可对所选镜像点击「测试」验证连通性（拉取 `plugins.json` 并返回延迟）；自定义镜像通过下拉输入框手动添加。配置持久化到 `data/plugin_store.json`。

## 6. 插件仓库（JuanNiang-Plugins）

### 元数据文档

- `plugins.json` → 分片索引 `{ total, chunks, updated_at }`
- `metadata/chunk_N.json` → 插件条目数组（含 `name/version/author/description/path/image/has_config/has_readme`）
- 商店客户端按以下路径拉取文件：元数据在仓库根（`plugins.json` / `metadata/`），插件的 README/头像在 `plugins/<name>/` 下，安装包在 `dist/<name>.zip`（`dist/` 由每晚 workflow 强制提交）

### GitHub Workflow

- **`metadata-update.yml`**：每晚 UTC 16:00（北京时间次日 0:00）自动 `hago scan` 更新元数据并打包 `dist/*.zip` 后一并提交。
- **`plugin-review.yml`**：PR 涉及 `plugins/**` 时自动校验格式（必需文件 / config.yaml schema / 版本递增），失败在 PR 留言。

> **dist 说明**：商店安装依赖仓库根目录 `dist/<name>.zip`（`internal/pluggin/store.go` 的 `DownloadPlugin` 拉取）。`dist/` 在 `.gitignore` 中，由每晚 workflow `git add -f` 强制提交。若需立即安装未生成 zip 的新插件，请先在插件仓库运行 `make pack-all` 并手动提交 `dist/`。

### 审核流程（PR 即审核）

1. 作者 Fork 后在 `plugins/<name>/` 提交 5 件套并发 PR。
2. CI 校验格式与版本；维护者（`.github/CODEOWNERS`）Review + Merge。
3. Merge 后每晚自动 `scan` 更新元数据，商店可见。

详见插件仓库的 [CONTRIBUTING.md](../JuanNiang-Plugins/CONTRIBUTING.md)（仓库相对路径：`../JuanNiang-Plugins/CONTRIBUTING.md`）。

## 7. 开发工具（hago CLI）

```bash
make build                    # 编译 hago
make init NAME=my-plugin      # 交互式创建 5 件套
make validate NAME=my-plugin  # 校验格式（--strict）
make scan                     # 扫描并更新元数据
make pack NAME=my-plugin      # 打包 zip
```