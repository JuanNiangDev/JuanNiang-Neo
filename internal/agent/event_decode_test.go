package agent

import "testing"

// TestDecodeQQImageEntitiesCQImageCode CQ 图片码内的 &amp; 必须解码为 &（QQ 图床 URL 查询参数），
// 否则 LLM 复刻 URL 时得到无法下载的地址（400 invalid rkey 根因之一）。
func TestDecodeQQImageEntitiesCQImageCode(t *testing.T) {
	in := `[CQ:image,file=abc.png,url=https://multimedia.nt.qq.com.cn/download?appid=1407&amp;fileid=xyz&amp;rkey=CAESM]
`
	want := `[CQ:image,file=abc.png,url=https://multimedia.nt.qq.com.cn/download?appid=1407&fileid=xyz&rkey=CAESM]
`
	if got := decodeQQImageEntities(in); got != want {
		t.Fatalf("CQ 图片码解码错误\n got: %q\nwant: %q", got, want)
	}
}

// TestDecodeQQImageEntitiesKeepsLiteralAmp 普通用户文本中的字面 &amp; 必须原样保留，
// 防止 LLM 收到的内容与原始消息不一致（CodeRabbit 审查点）。
func TestDecodeQQImageEntitiesKeepsLiteralAmp(t *testing.T) {
	in := "今天的折扣码是 A&amp;B，记得查收"
	want := in
	if got := decodeQQImageEntities(in); got != want {
		t.Fatalf("普通文本被改写\n got: %q\nwant: %q", got, want)
	}
}

// TestDecodeQQImageEntitiesPlainCDNURL 纯文本（非 CQ 码）形式的 QQ 图床 URL 也要解码。
func TestDecodeQQImageEntitiesPlainCDNURL(t *testing.T) {
	in := `看图 https://multimedia.nt.qq.com.cn/download?appid=1407&amp;rkey=x`
	want := `看图 https://multimedia.nt.qq.com.cn/download?appid=1407&rkey=x`
	if got := decodeQQImageEntities(in); got != want {
		t.Fatalf("纯文本 CDN URL 解码错误\n got: %q\nwant: %q", got, want)
	}
}

// TestDecodeQQImageEntitiesMixed 图片码 + 普通文本混合：只解码图片相关部分。
func TestDecodeQQImageEntitiesMixed(t *testing.T) {
	in := `图片来了 [CQ:image,url=http://a.cn/x?p=1&amp;q=2] 请把 A&amp;B 发我`
	want := `图片来了 [CQ:image,url=http://a.cn/x?p=1&q=2] 请把 A&amp;B 发我`
	if got := decodeQQImageEntities(in); got != want {
		t.Fatalf("混合消息解码错误\n got: %q\nwant: %q", got, want)
	}
}
