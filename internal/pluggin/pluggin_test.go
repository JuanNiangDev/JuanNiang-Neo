package pluggin

import (
	"testing"

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
