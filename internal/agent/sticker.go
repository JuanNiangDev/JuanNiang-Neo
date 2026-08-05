package agent

import (
	"context"
	"fmt"
	"strings"
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
