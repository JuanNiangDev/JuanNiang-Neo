package pluggin

import (
	"testing"

	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	lua "github.com/yuin/gopher-lua"
)

// TestGoToLuaValueStringSlice 回归：list 类型配置（[]string）经
// config.get/all 返回时必须转为 Lua table，而非 JSON 字符串。
func TestGoToLuaValueStringSlice(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{}, []string{}},
		{[]string{"123456"}, []string{"123456"}},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		v := goToLuaValue(L, tc.in)
		tbl, ok := v.(*lua.LTable)
		if !ok {
			t.Fatalf("[]string(%#v) 应转为 table, got %T (%v)", tc.in, v, v)
		}
		if tbl.Len() != len(tc.want) {
			t.Fatalf("[]string(%#v) table 长度 = %d, want %d", tc.in, tbl.Len(), len(tc.want))
		}
		for i, w := range tc.want {
			if got := tbl.RawGetInt(i + 1); got.String() != w {
				t.Fatalf("[]string(%#v) 第 %d 项 = %v, want %q", tc.in, i+1, got, w)
			}
		}
	}

	// 嵌套在 map 中（config.all() 的形态）也应正确
	m := map[string]any{"group_whitelist": []string{"1", "2"}}
	tbl, ok := goToLuaValue(L, m).(*lua.LTable)
	if !ok {
		t.Fatal("map 未转为 table")
	}
	inner, ok := tbl.RawGetString("group_whitelist").(*lua.LTable)
	if !ok {
		t.Fatalf("map 内的 []string 未转为 table, got %v", tbl.RawGetString("group_whitelist"))
	}
	if inner.Len() != 2 {
		t.Fatalf("嵌套 []string 长度 = %d, want 2", inner.Len())
	}
}

// TestLuaTableToT2IOptions 回归：t2i.generate/generate_url 可选 options 表解析。
func TestLuaTableToT2IOptions(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// 未传（栈上为 nil）→ (nil, nil)，等价于不指定 options
	L.Push(lua.LNil)
	opts, err := luaTableToT2IOptions(L, 1)
	L.Pop(1)
	if err != nil || opts != nil {
		t.Fatalf("nil options 应返回 (nil, nil), got (%v, %v)", opts, err)
	}

	// 完整 options 表
	tbl := L.NewTable()
	tbl.RawSetString("type", lua.LString("png"))
	tbl.RawSetString("quality", lua.LNumber(90))
	tbl.RawSetString("omit_background", lua.LBool(true))
	tbl.RawSetString("full_page", lua.LBool(false))
	tbl.RawSetString("viewport_width", lua.LNumber(261))
	tbl.RawSetString("viewport_height", lua.LNumber(379))
	tbl.RawSetString("scale", lua.LString("device"))
	tbl.RawSetString("animations", lua.LString("disabled"))
	tbl.RawSetString("caret", lua.LString("hide"))
	tbl.RawSetString("device_scale_factor_level", lua.LString("ultra"))
	tbl.RawSetString("timeout", lua.LNumber(30))
	L.Push(tbl)
	opts, err = luaTableToT2IOptions(L, 1)
	L.Pop(1)
	if err != nil {
		t.Fatalf("解析 options 失败: %v", err)
	}
	if opts.Type != t2icaller.ImageTypePNG ||
		opts.Quality != 90 ||
		!opts.OmitBackground ||
		opts.FullPage == nil || *opts.FullPage ||
		opts.ViewportWidth != 261 ||
		opts.ViewportHeight != 379 ||
		opts.Scale != "device" ||
		opts.Animations != t2icaller.AnimationDisabled ||
		opts.Caret != t2icaller.CaretHide ||
		opts.DeviceScaleFactor != t2icaller.ScaleUltra ||
		opts.Timeout != 30 {
		t.Fatalf("options 解析结果不符合预期: %+v", opts)
	}

	// 未知键 → 返回错误，避免拼写问题静默吞掉
	tbl2 := L.NewTable()
	tbl2.RawSetString("bogus", lua.LBool(true))
	L.Push(tbl2)
	_, err = luaTableToT2IOptions(L, 1)
	L.Pop(1)
	if err == nil {
		t.Fatal("未知选项键应报错")
	}
}
