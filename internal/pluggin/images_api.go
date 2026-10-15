package pluggin

import (
	"context"
	"strings"

	lua "github.com/yuin/gopher-lua"

	"JuanNiang-Neo/internal/core/models"
)

// normalizeImageFolderArg 归一化插件传入的图床目录参数：
// 空串 → "/"（根）；不带前导斜杠的名称（如 "表情包"）→ "/表情包"，
// 与图床存储的 folder 值（"/<name>"）对齐，否则 DAO 查询恒为空。
func normalizeImageFolderArg(folder string) string {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return "/"
	}
	if !strings.HasPrefix(folder, "/") {
		folder = "/" + folder
	}
	return folder
}

// injectImagesAPI 注入 jn.images：图床枚举/搜索（需要 image 权限）。
//
// images.list(folder)        → {{id,name,folder,mime_type,size_bytes},...}
//                              folder 空串或 "/" 表示根目录；最多返回 500 条。
// images.search(keyword, n?) → 同结构，按展示名称模糊匹配（前缀优先）。
//
// 返回的 id 可直接拼 [CQ:image,file=imgs://<id>]：adapter 发送层统一解析为
// base64（OneBot11 与图床网络可能不互通，见 cmd/server/main.go SetImageResolver）。
func (pe *PluginEngine) injectImagesAPI(L *lua.LState) {
	pushAssets := func(L *lua.LState, list []models.ImageAsset) {
		result := L.NewTable()
		for i, it := range list {
			row := L.NewTable()
			row.RawSetString("id", lua.LString(it.ID))
			row.RawSetString("name", lua.LString(it.Name))
			row.RawSetString("folder", lua.LString(it.Folder))
			row.RawSetString("mime_type", lua.LString(it.MimeType))
			row.RawSetString("size_bytes", lua.LNumber(it.SizeBytes))
			result.RawSetInt(i+1, row)
		}
		L.Push(result)
	}

	tbl := L.NewTable()
	L.SetFuncs(tbl, map[string]lua.LGFunction{
		"list": func(L *lua.LState) int {
			if pe.dao == nil {
				log.Warn("[plugin:images] 引擎未接入 DAO，images.list 返回空")
				L.Push(L.NewTable())
				return 1
			}
			folder := ""
			if L.GetTop() >= 1 && L.Get(1).Type() != lua.LTNil {
				folder = L.CheckString(1)
			}
			folder = normalizeImageFolderArg(folder)
			ctx := context.Background()
			// 单次查询（上限 500）：分页 OFFSET 循环在查询间隙有增删时会重复/遗漏；
			// DAO 方法带 created_at DESC, id DESC 稳定排序
			list, err := pe.dao.Image.ListFolder(ctx, folder, 500)
			if err != nil {
				log.Warn("[plugin:images] 图床枚举失败", "folder", folder, "err", err)
			}
			pushAssets(L, list)
			return 1
		},
		"search": func(L *lua.LState) int {
			if pe.dao == nil {
				log.Warn("[plugin:images] 引擎未接入 DAO，images.search 返回空")
				L.Push(L.NewTable())
				return 1
			}
			keyword := strings.TrimSpace(L.CheckString(1))
			limit := 20
			if L.GetTop() >= 2 && L.Get(2).Type() == lua.LTNumber {
				limit = int(L.Get(2).(lua.LNumber))
				if limit <= 0 || limit > 100 {
					limit = 20
				}
			}
			if keyword == "" {
				L.Push(L.NewTable())
				return 1
			}
			list, err := pe.dao.Image.SearchByName(context.Background(), keyword, limit)
			if err != nil {
				log.Warn("[plugin:images] 图床搜索失败", "keyword", keyword, "err", err)
				L.Push(L.NewTable())
				return 1
			}
			pushAssets(L, list)
			return 1
		},
	})
	L.SetGlobal("images", tbl)
}
