package provider

import "sync"

type ModelType string

const (
	ModelTypeText      ModelType = "text_model"
	ModelTypeImage     ModelType = "image_model"
	ModelTypeEmbedding ModelType = "embedding_model"
)

type ProviderConfig struct {
	ID          string
	Type        ModelType
	Name        string
	Endpoint    string
	Token       string
	Model       string
	Temperature float32
}

type Provider interface { // TODO
}

type ProviderGroup struct {
	mu        sync.Mutex
	TextLLM   []Provider // 聊天模型
	ImageLLM  []Provider // 识图模型
	Embedding []Provider // 向量模型
}

func NewProviderGroup() *ProviderGroup {
	return &ProviderGroup{
		mu:        sync.Mutex{},
		TextLLM:   make([]Provider, 0),
		ImageLLM:  make([]Provider, 0),
		Embedding: make([]Provider, 0),
	}
}
