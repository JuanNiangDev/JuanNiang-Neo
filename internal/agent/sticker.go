package agent

import (
	"context"
	"fmt"
	"strings"

	"JuanNiang-Neo/internal/agent/tool"
)

// listStickerTagsForTool 供 Agent 内置工具 list_sticker_tags 使用。
func (h *HagoCenter) listStickerTagsForTool(ctx context.Context) (string, error) {
	if h.DAO == nil || h.DAO.Sticker == nil {
		return "表情包库未初始化", nil
	}
	tags, err := h.DAO.Sticker.TagList(ctx)
	if err != nil {
		return "", err
	}
	if len(tags) == 0 {
		return "表情包库暂无标签（可在 Web 管理面板先创建标签）", nil
	}
	var sb strings.Builder
	sb.WriteString("表情包库全部标签：\n")
	for _, t := range tags {
		fmt.Fprintf(&sb, "- %s\n", t.Name)
	}
	return sb.String(), nil
}

// listStickersForTool 供 Agent 内置工具 list_stickers 使用：按标签分页列出表情。
func (h *HagoCenter) listStickersForTool(ctx context.Context, tag string, page, pageSize int) (string, error) {
	if h.DAO == nil || h.DAO.Sticker == nil {
		return "表情包库未初始化", nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}
	tag = strings.TrimSpace(tag)
	list, err := h.DAO.Sticker.List(ctx, tag, "", pageSize, (page-1)*pageSize)
	if err != nil {
		return "", err
	}
	total, _ := h.DAO.Sticker.Count(ctx, tag, "")
	if len(list) == 0 {
		return fmt.Sprintf("表情包库第 %d 页没有表情（共 %d 个，标签=%s）", page, total, tagOrAll(tag)), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "表情列表（第 %d 页，共 %d 个，标签=%s，发送用 send_sticker + ID）：\n", page, total, tagOrAll(tag))
	for i := range list {
		s := &list[i]
		desc := strings.TrimSpace(s.Desc)
		if desc == "" {
			desc = "(无简介)"
		}
		fmt.Fprintf(&sb, "- ID=%s 名称=%s 简介=%s 标签=%s\n", s.ID, s.Name, desc, strings.Join(s.Tags, "/"))
	}
	return sb.String(), nil
}

// searchStickersForTool 供 Agent 内置工具 search_stickers 使用：模糊匹配名称与简介。
func (h *HagoCenter) searchStickersForTool(ctx context.Context, keyword string, limit int) (string, error) {
	if h.DAO == nil || h.DAO.Sticker == nil {
		return "表情包库未初始化", nil
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "搜索关键词不能为空", nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	list, err := h.DAO.Sticker.List(ctx, "", keyword, limit, 0)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return fmt.Sprintf("没有找到与「%s」相关的表情", keyword), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "与「%s」相关的表情（共 %d 个，发送用 send_sticker + ID）：\n", keyword, len(list))
	for i := range list {
		s := &list[i]
		desc := strings.TrimSpace(s.Desc)
		if desc == "" {
			desc = "(无简介)"
		}
		fmt.Fprintf(&sb, "- ID=%s 名称=%s 简介=%s 标签=%s\n", s.ID, s.Name, desc, strings.Join(s.Tags, "/"))
	}
	return sb.String(), nil
}

// tagOrAll 标签为空时显示"全部"。
func tagOrAll(tag string) string {
	if tag == "" {
		return "全部"
	}
	return tag
}

// commonStickerTag 表情包库「常用」标签名：每轮对话会把该标签下的表情 ID/描述注入提示词，
// Agent 可直接用 send_sticker + ID 发送；用户可在 Web 表情包管理页把常用表情加入此标签。
const commonStickerTag = "常用"

// stickerContextLimit 每轮注入「常用」标签下的表情条数上限（控制 token 占用）。
const stickerContextLimit = 20

// buildStickerContext 构建表情包上下文，每轮对话注入系统指令：
//  1. 表情包库全部标签 → Agent 优先用 list_stickers 按标签获取，或 send_sticker_by_keyword 按意图发送；
//  2. 「常用」标签下的表情（ID/名称/简介），按表情自身标签分组 → 命中场景时可直接用 send_sticker + ID。
//
// 空库/无标签时返回空串（不注入）。
func (h *HagoCenter) buildStickerContext(ctx context.Context) string {
	if h.DAO == nil || h.DAO.Sticker == nil {
		return ""
	}
	var sb strings.Builder

	// 1. 全部标签
	tags, err := h.DAO.Sticker.TagList(ctx)
	if err == nil && len(tags) > 0 {
		sb.WriteString("表情包库全部标签：")
		for _, t := range tags {
			sb.WriteString("「" + t.Name + "」")
		}
		sb.WriteString("\n需要表情时优先用 send_sticker_by_keyword 按意图搜索发送，或 list_stickers 按标签浏览后再 send_sticker。\n")
	}

	// 2. 「常用」标签下的表情，按表情自身标签分组展示
	list, err := h.DAO.Sticker.List(ctx, commonStickerTag, "", stickerContextLimit, 0)
	if err == nil && len(list) > 0 {
		sb.WriteString("表情包库「常用」标签下的表情（可直接用 send_sticker + ID 发送）：\n")
		grouped := map[string][]string{}
		var order []string
		for i := range list {
			s := &list[i]
			g := "其他"
			if len(s.Tags) > 0 && strings.TrimSpace(s.Tags[0]) != "" {
				g = strings.TrimSpace(s.Tags[0])
			}
			desc := strings.TrimSpace(s.Desc)
			if desc == "" {
				desc = "(无简介)"
			}
			if _, ok := grouped[g]; !ok {
				order = append(order, g)
			}
			grouped[g] = append(grouped[g], fmt.Sprintf("ID=%s 名称=%s 简介=%s", s.ID, s.Name, desc))
		}
		for _, g := range order {
			sb.WriteString("- [")
			sb.WriteString(g)
			sb.WriteString("] ")
			sb.WriteString(strings.Join(grouped[g], "；"))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// sendStickerByKeywordForTool 供内置工具 send_sticker_by_keyword 使用：
// 按关键词搜索表情包库并直接发送最匹配的一个（一步完成，Agent 无需先查 ID 再发）。
// 取搜索结果第一条（名称/简介/标签命中），通过延迟发送队列或直发投递。
func (h *HagoCenter) sendStickerByKeywordForTool(ctx context.Context, keyword, msgType string, targetID int64) (string, error) {
	if h.DAO == nil || h.DAO.Sticker == nil {
		return "表情包库未初始化", nil
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "搜索关键词不能为空", nil
	}
	list, err := h.DAO.Sticker.List(ctx, "", keyword, 1, 0)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return fmt.Sprintf("表情包库中没有找到与「%s」相关的表情，可换个关键词或改用 list_stickers 浏览标签", keyword), nil
	}
	s := &list[0]
	msg := fmt.Sprintf("[CQ:image,file=stk://%s,subType=1]", s.ID)

	// 目标推断：显式传入 > 当前会话
	if targetID == 0 {
		if cur := GetMsgSessionCtx(ctx); cur != nil && cur.Msg != nil {
			msgType = cur.Msg.MessageType
			if cur.Msg.MessageType == "private" {
				targetID = cur.Msg.UserID
			} else {
				targetID = cur.Msg.GroupID
			}
		}
	}
	if targetID == 0 {
		return "缺少发送目标，且无法从当前会话推断", nil
	}

	// 任务执行期间的发送统一入队，执行完成后统一投递（与 send_sticker 一致）
	if q := tool.GetDeferredSendQueue(ctx); q != nil {
		q.Add(tool.DeferredSend{MessageType: msgType, TargetID: targetID, Message: msg, Delivery: true})
		return fmt.Sprintf("已按「%s」找到表情「%s」，加入发送队列待任务完成后发送", keyword, s.Name), nil
	}

	var id int64
	if msgType == "private" {
		id, err = h.Adapter.SendPrivateMsg(targetID, msg)
	} else {
		id, err = h.Adapter.SendGroupMsg(targetID, msg)
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已按「%s」发送表情「%s」，message_id: %d", keyword, s.Name, id), nil
}
