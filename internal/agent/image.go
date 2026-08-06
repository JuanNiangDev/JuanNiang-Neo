package agent

import (
	"context"
	"fmt"
	"strings"
)

// listImagesForTool 供 Agent 内置工具 list_images 使用：列出图床图片（ID/名称/路径），
// 返回格式化文本，LLM 拿到 ID 后可拼 [CQ:image,file=imgs://<ID>] 引用。
func (h *HagoCenter) listImagesForTool(ctx context.Context, folder string, limit int) (string, error) {
	if h.DAO == nil || h.DAO.Image == nil {
		return "图床未初始化", nil
	}
	if strings.TrimSpace(folder) == "" {
		folder = "/"
	} else {
		folder = "/" + strings.Trim(strings.TrimSpace(folder), "/")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	list, err := h.DAO.Image.List(ctx, folder, limit, 0)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return fmt.Sprintf("图床文件夹 %s 下没有图片", folder), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "图床图片列表（文件夹 %s，共 %d 张，引用格式 [CQ:image,file=imgs://图片ID]）：\n", folder, len(list))
	for i := range list {
		img := &list[i]
		name := strings.TrimSpace(img.Name)
		if name == "" {
			name = "(无名称)"
		}
		fmt.Fprintf(&sb, "- ID=%s 名称=%s 路径=%s 大小=%d字节\n",
			img.ID, name, img.Folder, img.SizeBytes)
	}
	sb.WriteString("提示：发送时把 imgs:// 后面的 ID 换成上面列出的 ID 即可引用对应图片。")
	return sb.String(), nil
}

// searchImagesForTool 供 Agent 内置工具 search_images 使用：按图片展示名称模糊搜索图床，
// 返回 ID/名称/文件夹，LLM 拿到 ID 后可拼 [CQ:image,file=imgs://<ID>] 引用。
func (h *HagoCenter) searchImagesForTool(ctx context.Context, keyword string, limit int) (string, error) {
	if h.DAO == nil || h.DAO.Image == nil {
		return "图床未初始化", nil
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "请提供搜索关键词", nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	list, err := h.DAO.Image.SearchByName(ctx, keyword, limit)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return fmt.Sprintf("未找到名称包含 %q 的图床图片", keyword), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "按名称 %q 搜索到 %d 张图床图片（引用格式 [CQ:image,file=imgs://图片ID]）：\n", keyword, len(list))
	for i := range list {
		img := &list[i]
		name := strings.TrimSpace(img.Name)
		if name == "" {
			name = "(无名称)"
		}
		fmt.Fprintf(&sb, "- ID=%s 名称=%s 路径=%s 大小=%d字节\n",
			img.ID, name, img.Folder, img.SizeBytes)
	}
	sb.WriteString("提示：发送时把 imgs:// 后面的 ID 换成上面列出的 ID 即可引用对应图片。")
	return sb.String(), nil
}
