# group-manager

群管理示例：**高危操作（禁言/踢人/设名片）带 admin 权限校验** + 入群欢迎（on_notice）+ 好友申请处理（on_request）。

## 文件结构

```
data/pluggins/group-manager/
├── pluggin.yaml   # 插件清单（名称/入口/权限）
├── main.lua       # 插件逻辑
├── config.yaml    # 动态配置声明（welcome_message / friend_code / poke_reply）
├── avatar.png     # 插件图标
└── README.md
```

## 功能

| 命令 / 事件 | 说明 |
|------------|------|
| `/ban <QQ号> <秒数>` | 禁言群成员（**仅管理员**，默认 60 秒） |
| `/kick <QQ号>` | 踢出群成员（**仅管理员**） |
| `/card <QQ号> <新名片>` | 设置群名片（**仅管理员**） |
| 群成员入群 | 自动发送欢迎消息（`on_notice`） |
| 戳一戳机器人 | 自动回应（`on_notice` + `sub_type=poke`） |
| 好友申请 | 验证信息含"卷娘"自动同意，否则拒绝（`on_request`） |

## ⚠ 安全要点（重要）

高危操作（禁言/踢人/名片）**必须**先校验 `is_admin(event.user_id, event)`：

```lua
local function is_admin(user_id, event)
    if not event.admins then return false end
    local uid = tostring(user_id)
    for _, a in ipairs(event.admins) do
        if a == uid then return true end
    end
    return false
end
```

`event.admins` 透传 OneBot11 适配器配置的 `OB_ADMINS` 管理员 QQ 列表。未通过校验直接拒绝并提示，**坚决不执行**。

## 覆盖的 API

- **群管理**：`onebot11.ban_group_member(group_id, user_id, duration)` / `kick_group_member(group_id, user_id, reject_add)` / `set_group_card(group_id, user_id, card)`
- **请求处理**：`onebot11.handle_friend_request(flag, approve, remark)`（`handle_group_request(flag, sub_type, approve, reason)` 同理）
- **回调**：`on_notice(event)`（`notice_type` / `sub_type` / `user_id` / `group_id` / `operator_id` / `target_id` / `duration` 等字段）、`on_request(event)`（`request_type` / `comment` / `flag`）

## 试用

1. 在 `OB_ADMINS` / Web「Adapter」页配置你的 QQ 为管理员
2. 群里 `/ban <某人QQ> 60` 验证权限校验（非管理员会被拒绝）
3. 拉个新人入群看欢迎消息；戳一戳机器人看回应
