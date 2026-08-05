package provider

import (
	"JuanNiang-Neo/internal/logging"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var log = logging.NewModule("provider")

// openAIProvider 实现 OpenAI-compatible API 客户端。
type openAIProvider struct {
	cfg ProviderConfig
	cli *http.Client
}

func NewProvider(cfg ProviderConfig) Provider {
	return &openAIProvider{
		cfg: cfg,
		cli: &http.Client{Timeout: 120 * time.Second},
	}
}

func NewProviderFromDB(cfg ProviderConfig) Provider {
	return NewProvider(cfg)
}

func (p *openAIProvider) ID() string      { return p.cfg.ID }
func (p *openAIProvider) Name() string    { return p.cfg.Name }
func (p *openAIProvider) Type() ModelType { return p.cfg.Type }
func (p *openAIProvider) Model() string   { return p.cfg.Model }

// ---------- Chat ----------

func (p *openAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := p.buildChatBody(req, false)
	respBody, err := p.doRequest(ctx, "/v1/chat/completions", body)
	if err != nil {
		return nil, err
	}
	return p.parseChatResponse(respBody)
}

func (p *openAIProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatStreamChunk, error) {
	body := p.buildChatBody(req, true)
	ch := make(chan ChatStreamChunk, 32)

	reqURL := strings.TrimRight(p.cfg.Endpoint, "/") + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	resp, err := p.cli.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("provider chat stream: %w", err)
	}

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			log.Error("provider stream 非200", "status", resp.StatusCode, "body", string(body))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			var chunk rawStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				log.Error("provider stream 解析失败", "err", err)
				continue
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]
			out := ChatStreamChunk{
				Content:      choice.Delta.Content,
				FinishReason: choice.FinishReason,
			}
			for _, tc := range choice.Delta.ToolCalls {
				out.ToolCalls = append(out.ToolCalls, ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: ToolCallFunc{
						Name:      tc.Function.Name,
						Arguments: json.RawMessage(tc.Function.Arguments),
					},
				})
			}
			select {
			case ch <- out:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// ---------- Vision ----------

func (p *openAIProvider) Vision(ctx context.Context, imageData []byte, prompt string) (string, error) {
	imgB64 := base64.StdEncoding.EncodeToString(imageData)
	imgURL := "data:image/jpeg;base64," + imgB64

	reqBody := map[string]any{
		"model": p.cfg.Model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": prompt},
					{"type": "image_url", "image_url": map[string]string{"url": imgURL}},
				},
			},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	respBody, err := p.doRequest(ctx, "/v1/chat/completions", data)
	if err != nil {
		return "", err
	}

	chatResp, err := p.parseChatResponse(respBody)
	if err != nil {
		return "", err
	}
	return chatResp.Message.Content, nil
}

// ---------- 内部方法 ----------

func (p *openAIProvider) buildChatBody(req ChatRequest, stream bool) []byte {
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		msg := map[string]any{"role": m.Role}
		if m.Content != "" {
			msg["content"] = m.Content
		}
		if len(m.ToolCalls) > 0 {
			toolCalls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				toolCalls = append(toolCalls, map[string]any{
					"id":   tc.ID,
					"type": tc.Type,
					"function": map[string]any{
						"name":      tc.Function.Name,
						"arguments": string(tc.Function.Arguments),
					},
				})
			}
			msg["tool_calls"] = toolCalls
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if m.Name != "" {
			msg["name"] = m.Name
		}
		messages = append(messages, msg)
	}

	body := map[string]any{
		"model":    p.cfg.Model,
		"messages": messages,
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	} else {
		body["temperature"] = p.cfg.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Function.Name,
					"description": t.Function.Description,
					"parameters":  json.RawMessage(t.Function.Parameters),
				},
			})
		}
		body["tools"] = tools
	}
	if stream {
		body["stream"] = true
	}
	if p.cfg.EnableThinking {
		// 模型思考开关：兼容主流 OpenAI 兼容实现的扩展字段。
		//   DeepSeek 系: thinking={"type":"enabled"}; 通义千问系: enable_thinking=true。
		// 未知字段通常被忽略，二者同时携带覆盖面最大。
		body["thinking"] = map[string]any{"type": "enabled"}
		body["enable_thinking"] = true
	}

	data, _ := json.Marshal(body)
	return data
}

func (p *openAIProvider) doRequest(ctx context.Context, path string, body []byte) ([]byte, error) {
	url := strings.TrimRight(p.cfg.Endpoint, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("provider request: %w", err)
	}
	p.setHeaders(req)

	resp, err := p.cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("provider read: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("provider %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (p *openAIProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.Token)
}

func (p *openAIProvider) parseChatResponse(body []byte) (*ChatResponse, error) {
	var raw rawChatResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("provider parse: %w", err)
	}
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("provider: 空响应")
	}

	choice := raw.Choices[0]
	msg := ChatMessage{
		Role:       choice.Message.Role,
		Content:    choice.Message.Content,
		ToolCallID: choice.Message.ToolCallID,
	}

	for _, tc := range choice.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: ToolCallFunc{
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}

	return &ChatResponse{
		Message:      msg,
		TokenUsage:   raw.Usage.TotalTokens,
		FinishReason: choice.FinishReason,
	}, nil
}

// ---------- 底层 JSON 类型 ----------

type rawChatResponse struct {
	Choices []struct {
		Message struct {
			Role       string        `json:"role"`
			Content    string        `json:"content"`
			ToolCallID string        `json:"tool_call_id"`
			ToolCalls  []rawToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

type rawToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type rawStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string        `json:"content"`
			ToolCalls []rawToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}
