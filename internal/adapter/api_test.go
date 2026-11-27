package adapter

import "testing"

// TestParseForwardMsgID 合并转发响应的 message_id 解析：
// 兼容 JSON 数字（LLBot/NapCat）与十进制字符串（go-cqhttp），非法/缺失值必须报错，
// 不能像旧实现那样吞掉反序列化错误返回 0。
func TestParseForwardMsgID(t *testing.T) {
	cases := []struct {
		name    string
		rsp     *APIResponse
		want    int64
		wantErr bool
	}{
		{name: "数字", rsp: &APIResponse{Data: `{"message_id":12345}`}, want: 12345},
		{name: "字符串", rsp: &APIResponse{Data: `{"message_id":"67890"}`}, want: 67890},
		{name: "null", rsp: &APIResponse{Data: `{"message_id":null}`}, wantErr: true},
		{name: "缺失字段", rsp: &APIResponse{Data: `{}`}, wantErr: true},
		{name: "非十进制字符串", rsp: &APIResponse{Data: `{"message_id":"abc"}`}, wantErr: true},
		{name: "非法JSON", rsp: &APIResponse{Data: `oops`}, wantErr: true},
		{name: "data非字符串", rsp: &APIResponse{Data: map[string]any{"message_id": 1}}, wantErr: true},
		{name: "nil响应", rsp: nil, wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseForwardMsgID("test", tc.rsp)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: 应返回错误，实际 got=%d", tc.name, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: 不应报错: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
