package adapter

import "testing"

func TestParseForwardMsgDataNapCatShape(t *testing.T) {
	data := []byte(`{"messages":[
		{"user_id":123,"sender":{"nickname":"张三","card":""},
		 "message":[{"type":"text","data":{"text":"第一条"}}]},
		{"user_id":456,"sender":{"nickname":"李四","card":"名片"},"message":"纯文本"}
	]}`)
	nodes, err := parseForwardMsgData(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(nodes) != 2 || nodes[0].Name != "张三" || nodes[0].Uin != 123 {
		t.Fatalf("节点字段异常: %+v", nodes)
	}
	if nodes[1].Name != "名片" {
		t.Fatalf("card 应优先于 nickname: %+v", nodes[1])
	}
	content, ok := nodes[0].Content.([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content 应为消息段数组: %#v", nodes[0].Content)
	}
}

func TestParseForwardMsgDataArrayShape(t *testing.T) {
	nodes, err := parseForwardMsgData([]byte(`[{"name":"群友","uin":1,"content":[{"type":"text","data":{"text":"x"}}]}]`))
	if err != nil || len(nodes) != 1 || nodes[0].Name != "群友" {
		t.Fatalf("数组形状解析异常: %+v err=%v", nodes, err)
	}
}
