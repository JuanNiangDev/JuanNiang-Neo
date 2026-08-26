package caller

import "github.com/google/uuid"

// ......... 写入 .........

// BatchItem 批量入库项。
type BatchItem struct {
	Tag  uuid.UUID `json:"tag"`
	Text string    `json:"text"`
}

type UpsertResponse struct {
	Tag        uuid.UUID `json:"tag"`
	ChunkCount int       `json:"chunk_count"`
	Truncated  bool      `json:"truncated"`
}

type BatchItemResponse struct {
	Tag        uuid.UUID `json:"tag"`
	ChunkCount int       `json:"chunk_count"`
	Truncated  bool      `json:"truncated"`
	Error      *string   `json:"error,omitempty"`
}

type BatchResponse struct {
	Results []BatchItemResponse `json:"results"`
}

// ......... 检索 .........

type SearchHit struct {
	Tag   uuid.UUID `json:"tag"`
	Score float64   `json:"score"`
}

type SearchResponse struct {
	Results []SearchHit `json:"results"`
}

// ......... 状态 .........

// ScoopStats 单个分库的规模（/health 与 /info 返回 per-scoop 统计）。
type ScoopStats struct {
	Tags   int `json:"tags"`
	Chunks int `json:"chunks"`
}

type InfoResponse struct {
	Status string `json:"status"`
	Model  struct {
		Ready     bool    `json:"ready"`
		ModelName *string `json:"model_name,omitempty"`
		Dim       *int    `json:"dim,omitempty"`
		NParams   *uint64 `json:"n_params,omitempty"`
		NThreads  *int    `json:"n_threads,omitempty"`
		NContext  *uint32 `json:"n_ctx,omitempty"`
		Error     *string `json:"error,omitempty"`
	} `json:"model"`
	Memory *struct {
		RSSKB   uint64 `json:"rss_kb"`
		VSizeKB uint64 `json:"vsize_kb"`
	} `json:"memory"`
	// 各分库的 tag/块数量（scoop 名 → 统计）
	Scoops map[string]ScoopStats `json:"scoops"`
}
