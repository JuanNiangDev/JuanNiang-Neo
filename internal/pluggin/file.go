package pluggin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// ====================================================================
// 插件文件读写 API（需要 file 权限）
// ====================================================================
// 所有路径均相对于插件自身目录 (data/pluggins/<name>/)，仅可读写文本文件。
// 禁止绝对路径与 .. 越权访问，防止插件读写其他插件或系统文件。

// errInvalidPluginPath 目录越界错误文案。
var errInvalidPluginPath = errors.New("非法路径：仅允许插件目录内的相对路径（禁止绝对路径与 .. 越界）")

// pluginFilePath 校验并拼接插件目录内的文件路径，防止目录穿越。
func pluginFilePath(basePath, pluginName, path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", errInvalidPluginPath
	}
	pluginDir := filepath.Join(basePath, pluginName)
	full := filepath.Join(pluginDir, path)
	rel, err := filepath.Rel(pluginDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errInvalidPluginPath
	}
	return full, nil
}

// readTextLines 读取文本文件并按行拆分（自动去除行尾 \n / \r）。
func readTextLines(full string) ([]string, error) {
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSuffix(string(data), "\n")
	if content == "" {
		return []string{}, nil
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	return lines, nil
}

// writeTextLines 将行数组写回文件（每行以 \n 结尾，自动创建父目录）。
func writeTextLines(full string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	return os.WriteFile(full, []byte(sb.String()), 0o644)
}

// injectFileAPI 注入 file 全局表（需要 file 权限）。
// 函数均返回 (result, err)；path 参数相对插件目录。
func (pe *PluginEngine) injectFileAPI(L *lua.LState, pluginName string) {
	pushFileValueErr := func(err error) int {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}

	fileTable := L.NewTable()
	L.SetFuncs(fileTable, map[string]lua.LGFunction{
		// read(path) → string, err  读取整个文件内容
		"read": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				return pushFileValueErr(err)
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return pushFileValueErr(err)
			}
			L.Push(lua.LString(string(data)))
			return 1
		},
		// read_lines(path) → string[], err  读取全部行
		"read_lines": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				return pushFileValueErr(err)
			}
			lines, err := readTextLines(full)
			if err != nil {
				return pushFileValueErr(err)
			}
			tbl := L.NewTable()
			for i, l := range lines {
				L.SetTable(tbl, lua.LNumber(i+1), lua.LString(l))
			}
			L.Push(tbl)
			return 1
		},
		// read_line(path, n) → string, err  读取第 n 行（1 起）；越界返回 nil（非错误，便于循环读取）
		"read_line": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				return pushFileValueErr(err)
			}
			n := L.CheckInt(2)
			if n < 1 {
				L.Push(lua.LNil)
				L.Push(lua.LString("行号必须 >= 1"))
				return 2
			}
			lines, err := readTextLines(full)
			if err != nil {
				return pushFileValueErr(err)
			}
			if n > len(lines) {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(lua.LString(lines[n-1]))
			return 1
		},
		// write(path, content) → bool, err  覆盖写入整个文件
		"write": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				return pushFileValueErr(err)
			}
			content := L.CheckString(2)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return pushFileValueErr(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				return pushFileValueErr(err)
			}
			L.Push(lua.LBool(true))
			return 1
		},
		// write_lines(path, lines) → bool, err  覆盖写入多行（每行自动补 \n）
		"write_lines": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				return pushFileValueErr(err)
			}
			linesArg := L.CheckTable(2)
			lines := make([]string, 0, linesArg.Len())
			linesArg.ForEach(func(_, v lua.LValue) {
				lines = append(lines, v.String())
			})
			if err := writeTextLines(full, lines); err != nil {
				return pushFileValueErr(err)
			}
			L.Push(lua.LBool(true))
			return 1
		},
		// write_line(path, n, content) → bool, err  改写第 n 行（文件不足自动补空行）
		"write_line": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				return pushFileValueErr(err)
			}
			n := L.CheckInt(2)
			content := L.CheckString(3)
			if n < 1 {
				L.Push(lua.LBool(false))
				L.Push(lua.LString("行号必须 >= 1"))
				return 2
			}
			lines, err := readTextLines(full)
			if err != nil {
				if !os.IsNotExist(err) {
					return pushFileValueErr(err)
				}
				lines = nil
			}
			for len(lines) < n {
				lines = append(lines, "")
			}
			lines[n-1] = content
			if err := writeTextLines(full, lines); err != nil {
				return pushFileValueErr(err)
			}
			L.Push(lua.LBool(true))
			return 1
		},
		// append(path, content) → bool, err  追加内容到文件末尾（不自动补换行）
		"append": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				return pushFileValueErr(err)
			}
			content := L.CheckString(2)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return pushFileValueErr(err)
			}
			f, err := os.OpenFile(full, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return pushFileValueErr(err)
			}
			defer f.Close()
			if _, err := f.WriteString(content); err != nil {
				return pushFileValueErr(err)
			}
			L.Push(lua.LBool(true))
			return 1
		},
		// append_line(path, content) → bool, err  追加一行（末尾无换行时自动补）
		"append_line": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				return pushFileValueErr(err)
			}
			content := L.CheckString(2)
			lines, err := readTextLines(full)
			if err != nil {
				if !os.IsNotExist(err) {
					return pushFileValueErr(err)
				}
				lines = nil
			}
			lines = append(lines, content)
			if err := writeTextLines(full, lines); err != nil {
				return pushFileValueErr(err)
			}
			L.Push(lua.LBool(true))
			return 1
		},
		// exists(path) → bool  判断文件是否存在
		"exists": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				L.Push(lua.LBool(false))
				return 1
			}
			_, err = os.Stat(full)
			L.Push(lua.LBool(err == nil))
			return 1
		},
		// remove(path) → bool, err  删除文件
		"remove": func(L *lua.LState) int {
			full, err := pluginFilePath(pe.basePath, pluginName, L.CheckString(1))
			if err != nil {
				return pushFileValueErr(err)
			}
			if err := os.Remove(full); err != nil {
				return pushFileValueErr(err)
			}
			L.Push(lua.LBool(true))
			return 1
		},
	})
	L.SetGlobal("file", fileTable)
}
