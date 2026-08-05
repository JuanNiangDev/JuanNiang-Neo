# rich-message

富文本消息与 OneBot11 查询示例：演示**消息段数组**（文字/@/图片/表情）、**群信息查询**和**图床表情发送**。

## 文件结构

```
data/pluggins/rich-message/
├── pluggin.yaml
├── main.lua
├── img/dot.png     # 示例本地图片（演示"相对路径自动转 base64"）
└── README.md
```

## 功能

| 命令 | 说明 |
|------|------|
| `/send card` | 发送一条富文本消息，包含 @ 某人、CQ 表情、本地图片、网络图片 URL、图床图片 |
| `/group info` | 查询当前群信息（群名/群号/成员数） |
| `/group members` | 查询当前群成员列表（前 10 个） |
| `/sticker <表情ID>` | 发送表情包库中的表情（短 UUID，底层自动映射图床图片） |

## 覆盖的 API

### 消息段数组（富文本）
`onebot11.send_group_msg / send_private_msg` 的 `message` 参数支持 Lua 数组：

```lua
{
    { type = "text", data = { text = "文字" } },
    { type = "at",   data = { qq = "123" } },
    { type = "face", data = { id = "66" } },          -- CQ 表情
    { type = "image", data = { file = "img/dot.png" } }, -- ① 插件目录相对路径 → 自动转 base64
    { type = "image", data = { file = "https://..." } }, -- ② URL 直链
    { type = "image", data = { file = "imgs://<ID>" } }, -- ③ 图床引用 → 发送层转 base64
}
```

### 查询
- `onebot11.get_group_info(group_id)` → `{group_id, group_name, member_count, max_member_count}`
- `onebot11.get_group_member_list(group_id)` → `[{user_id, nickname, card, role, ...}]`

### 图床表情
- `onebot11.send_group_sticker(group_id, sticker_id)`
- `onebot11.send_private_sticker(user_id, sticker_id)`

## 使用前提

- `/send card` 中的图床图片段请先在图床上传一张图，然后把 `imgs://REPLACE_WITH_IMAGE_ID` 替换成真实图片 ID
- `/sticker` 需要先到「表情包库」页面创建表情，拿到短 UUID

## 权限

`permissions: [onebot11, http]` —— 只申请了用到的权限；`http` 用于插件内部示例（本插件未实际使用，仅演示可扩展）。
