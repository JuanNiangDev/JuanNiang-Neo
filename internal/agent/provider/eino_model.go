package provider

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// EinoModelAdapter wraps a Provider as Eino's model.ToolCallingChatModel.
type EinoModelAdapter struct {
	provider Provider
	tools    []*schema.ToolInfo
}

// Compile-time interface check.
var _ model.ToolCallingChatModel = (*EinoModelAdapter)(nil)

// NewEinoModelAdapter creates a new Eino-compatible model adapter from a Provider.
func NewEinoModelAdapter(p Provider) *EinoModelAdapter {
	return &EinoModelAdapter{provider: p}
}

// Generate calls the underlying provider's Chat method and returns an Eino message.
func (a *EinoModelAdapter) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	req := a.buildRequest(input)

	resp, err := a.provider.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	return providerRespToEino(resp), nil
}

// Stream calls the underlying provider's ChatStream method and bridges the
// provider's channel-based stream to an Eino StreamReader via schema.Pipe.
func (a *EinoModelAdapter) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	req := a.buildRequest(input)

	providerCh, err := a.provider.ChatStream(ctx, req)
	if err != nil {
		return nil, err
	}

	// Bridge provider channel → Eino StreamReader.
	pipeReader, pipeWriter := schema.Pipe[*schema.Message](32)
	go func() {
		defer pipeWriter.Close()
		for chunk := range providerCh {
			einoMsg := streamChunkToEino(&chunk)
			if pipeWriter.Send(einoMsg, nil) {
				return // stream closed by receiver
			}
		}
	}()

	return pipeReader, nil
}

// WithTools returns a new EinoModelAdapter with the given tools attached.
// The original adapter is not modified, making this safe for concurrent use.
func (a *EinoModelAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return &EinoModelAdapter{
		provider: a.provider,
		tools:    tools,
	}, nil
}

// buildRequest constructs a ChatRequest from Eino messages and attached tools.
func (a *EinoModelAdapter) buildRequest(input []*schema.Message) ChatRequest {
	providerMsgs := make([]ChatMessage, 0, len(input))
	for _, m := range input {
		providerMsgs = append(providerMsgs, einoMsgToProvider(m))
	}

	req := ChatRequest{Messages: providerMsgs}

	if len(a.tools) > 0 {
		req.Tools = make([]ToolDef, 0, len(a.tools))
		for _, t := range a.tools {
			if td := einoToolInfoToProviderToolDef(t); td != nil {
				req.Tools = append(req.Tools, *td)
			}
		}
	}

	return req
}

// --- Conversion helpers ---

func einoMsgToProvider(m *schema.Message) ChatMessage {
	cm := ChatMessage{
		Role:       string(m.Role),
		Content:    m.Content,
		Name:       m.Name,
		ToolCallID: m.ToolCallID,
	}
	for _, tc := range m.ToolCalls {
		cm.ToolCalls = append(cm.ToolCalls, ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: ToolCallFunc{
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}
	return cm
}

func providerRespToEino(resp *ChatResponse) *schema.Message {
	msg := &schema.Message{
		Role:    schema.RoleType(resp.Message.Role),
		Content: resp.Message.Content,
	}
	for _, tc := range resp.Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: schema.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: string(tc.Function.Arguments),
			},
		})
	}
	msg.ResponseMeta = &schema.ResponseMeta{
		FinishReason: resp.FinishReason,
		Usage: &schema.TokenUsage{
			TotalTokens: resp.TokenUsage,
		},
	}
	return msg
}

func streamChunkToEino(chunk *ChatStreamChunk) *schema.Message {
	msg := &schema.Message{
		Role:    schema.Assistant,
		Content: chunk.Content,
	}
	for _, tc := range chunk.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: schema.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: string(tc.Function.Arguments),
			},
		})
	}
	if chunk.FinishReason != "" {
		msg.ResponseMeta = &schema.ResponseMeta{
			FinishReason: chunk.FinishReason,
		}
	}
	return msg
}

// einoToolInfoToProviderToolDef converts Eino's ToolInfo to the provider's ToolDef.
// Returns nil if the input is nil.
func einoToolInfoToProviderToolDef(t *schema.ToolInfo) *ToolDef {
	if t == nil {
		return nil
	}
	td := &ToolDef{
		Type: "function",
		Function: ToolDefFunc{
			Name:        t.Name,
			Description: t.Desc,
		},
	}
	// Convert ParamsOneOf to JSON schema bytes.
	if t.ParamsOneOf != nil {
		js, err := t.ParamsOneOf.ToJSONSchema()
		if err == nil && js != nil {
			paramsJSON, marshalErr := json.Marshal(js)
			if marshalErr == nil {
				td.Function.Parameters = paramsJSON
			}
		}
	}
	return td
}
