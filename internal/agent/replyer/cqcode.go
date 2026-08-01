package replyer

import (
	"fmt"
	"strings"
)

// CQImage 生成图片 CQ 码。
// url: HTTP(S) URL 或 base64:// 格式
func CQImage(url string) string {
	return fmt.Sprintf("[CQ:image,file=%s]", url)
}

// CQImageBase64 从 base64 数据生成图片 CQ 码。
func CQImageBase64(b64 string) string {
	// 去掉可能的 data:image/...;base64, 前缀
	if idx := strings.Index(b64, ","); idx != -1 {
		b64 = b64[idx+1:]
	}
	return fmt.Sprintf("[CQ:image,file=base64://%s]", b64)
}

// CQImageFromSource 从 ImageSource 生成图片 CQ 码。
func CQImageFromSource(src ImageSource) string {
	switch src.Type {
	case "base64":
		return CQImageBase64(src.URL)
	default:
		return CQImage(src.URL)
	}
}

// CQAt 生成 @某人 CQ 码。
func CQAt(qq int64) string {
	return fmt.Sprintf("[CQ:at,qq=%d]", qq)
}

// CQAtAll 生成 @全体成员 CQ 码。
func CQAtAll() string {
	return "[CQ:at,qq=all]"
}

// CQFace 生成 QQ 表情 CQ 码。
func CQFace(id int) string {
	return fmt.Sprintf("[CQ:face,id=%d]", id)
}

// CQReply 生成引用回复 CQ 码。
func CQReply(msgID int64) string {
	return fmt.Sprintf("[CQ:reply,id=%d]", msgID)
}

// CQRecord 生成语音 CQ 码。
func CQRecord(url string) string {
	return fmt.Sprintf("[CQ:record,file=%s]", url)
}

// CQVideo 生成视频 CQ 码。
func CQVideo(url string) string {
	return fmt.Sprintf("[CQ:video,file=%s]", url)
}

// BuildCQRaw 将多个 MessageSegment 拼接为 CQ 码原始文本。
func BuildCQRaw(segments []MessageSegment) string {
	var sb strings.Builder
	for _, seg := range segments {
		switch seg.Type {
		case "text":
			if text, ok := seg.Data["text"].(string); ok {
				sb.WriteString(text)
			}
		case "image":
			if url, ok := seg.Data["file"].(string); ok {
				sb.WriteString(CQImage(url))
			}
		case "at":
			if qq, ok := seg.Data["qq"]; ok {
				switch v := qq.(type) {
				case int64:
					sb.WriteString(CQAt(v))
				case string:
					if v == "all" {
						sb.WriteString(CQAtAll())
					}
				}
			}
		case "face":
			if id, ok := seg.Data["id"].(int); ok {
				sb.WriteString(CQFace(id))
			}
		case "reply":
			if id, ok := seg.Data["id"].(int64); ok {
				sb.WriteString(CQReply(id))
			}
		}
	}
	return sb.String()
}
