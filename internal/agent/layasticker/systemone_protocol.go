package layasticker

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// systemOneInstructions 原生协议中 sticker 问题的固定指令。
const systemOneInstructions = "根据当前对话和助手回复，判断是否需要追加表情，并从候选类别中选择最合适的一项。"

// systemOneRequest 是 Laya 原生 systemone.v1 决策请求（POST /v1/systemone）。
// 使用结构体 + json.Marshal 构造，杜绝字符串拼接破坏 JSON 的可能。
type systemOneRequest struct {
	Model     string             `json:"model"`
	State     systemOneState     `json:"state"`
	Questions systemOneQuestions `json:"questions"`
}

type systemOneState struct {
	UserMessage    string `json:"user_message"`
	AssistantReply string `json:"assistant_reply"`
}

type systemOneQuestions struct {
	Sticker systemOneStickerQuestion `json:"sticker"`
}

type systemOneStickerQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	// Criteria 是「类别 ID → 类别描述」的 JSON 对象：只包含启用类别
	// （含 no_send 类别，不发送与否由卷娘侧类别配置决定），
	// 不含禁用类别，也不含表情标签、Sticker ID 等内部映射信息。
	Criteria map[string]string `json:"criteria"`
}

// BuildSystemOneRequest 把内部 DecisionRequest 转换为原生 systemone.v1 请求体。
// 模型为空或没有启用类别时在发起 HTTP 前返回明确错误。
func BuildSystemOneRequest(request DecisionRequest) ([]byte, error) {
	if strings.TrimSpace(request.Model) == "" {
		return nil, errors.New("systemone.v1 请求缺少模型配置")
	}
	criteria := make(map[string]string, len(request.EnabledCategories))
	for _, category := range request.EnabledCategories {
		id := strings.TrimSpace(category.ID)
		if id == "" || !category.Enabled {
			continue
		}
		criteria[id] = category.Description
	}
	if len(criteria) == 0 {
		return nil, errors.New("systemone.v1 请求没有可用的启用类别")
	}
	body, err := json.Marshal(systemOneRequest{
		Model: strings.TrimSpace(request.Model),
		State: systemOneState{
			UserMessage:    request.UserMessage,
			AssistantReply: request.AssistantReply,
		},
		Questions: systemOneQuestions{
			Sticker: systemOneStickerQuestion{
				Type:         "choice",
				Instructions: systemOneInstructions,
				Criteria:     criteria,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("序列化 systemone.v1 请求失败: %w", err)
	}
	return body, nil
}
