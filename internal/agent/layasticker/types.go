package layasticker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ProtocolJSON      = "json"
	ProtocolSystemOne = "systemone.v1"
	DefaultTimeout    = 10 * time.Second
	DefaultMaxBody    = 256 << 10
	DefaultMethod     = http.MethodPost

	defaultCategoryPointer   = "/answers/sticker/choice"
	defaultConfidencePointer = "/answers/sticker/answer_confidence"
)

// Category is the administrator-owned mapping between a Laya decision and the
// existing Sticker DAO tags.
type Category struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	StickerTags []string `json:"sticker_tags,omitempty"`
	NoSend      bool     `json:"no_send"`
	Enabled     bool     `json:"enabled"`
}

// Config is a complete per-turn Laya configuration snapshot.
type Config struct {
	Enabled              bool          `json:"enabled"`
	Endpoint             string        `json:"endpoint"`
	CapabilitiesEndpoint string        `json:"capabilities_endpoint,omitempty"`
	APIKey               string        `json:"-"`
	Model                string        `json:"model"`
	Timeout              time.Duration `json:"timeout"`
	ProtocolMode         string        `json:"protocol_mode"`
	HTTPMethod           string        `json:"http_method"`
	RequestTemplate      string        `json:"request_template"`
	CategoryPointer      string        `json:"response_category_path"`
	ConfidencePointer    string        `json:"response_confidence_path"`
	Categories           []Category    `json:"categories"`
	// MinConfidence 置信度下限（0 = 不过滤）：设置后响应必须携带有效置信度，
	// 缺失或低于下限都不发送自动表情。
	// confidence 的语义由 Laya 服务定义，这里只做下限拦截，不当作校准概率。
	MinConfidence float64 `json:"min_confidence"`
	// TaskTTL 自动表情有效期（0 = 不限）：超过该时长的旧表情不再追加，
	// 避免上一个回合的表情挂到已经变化的对话上下文上。
	TaskTTL          time.Duration `json:"task_ttl"`
	MaxResponseBytes int64         `json:"-"`
}

type DecisionRequest struct {
	UserMessage       string            `json:"user_message"`
	AssistantReply    string            `json:"assistant_reply"`
	MessageType       string            `json:"message_type"`
	Model             string            `json:"model"`
	Categories        []Category        `json:"categories"`
	EnabledCategories []Category        `json:"enabled_categories"`
	Criteria          []string          `json:"criteria"`
	CriteriaMap       map[string]string `json:"criteria_map"`
}

type DecisionResult struct {
	Category   string
	Confidence *float64
}

type CapabilitySnapshot struct {
	SourceEndpoint             string             `json:"source_endpoint,omitempty"`
	SourceCapabilitiesEndpoint string             `json:"source_capabilities_endpoint,omitempty"`
	SchemaVersion              int                `json:"schema_version"`
	Service                    string             `json:"service"`
	API                        CapabilityAPI      `json:"api"`
	Models                     []CapabilityModel  `json:"models"`
	Response                   CapabilityResponse `json:"response"`
}

type CapabilityAPI struct {
	Protocol     string `json:"protocol"`
	DecisionPath string `json:"decision_path"`
	Method       string `json:"method"`
}

type CapabilityModel struct {
	Loaded  *bool             `json:"loaded,omitempty"`
	ID      string            `json:"id"`
	Sticker CapabilitySticker `json:"sticker"`
}

type CapabilitySticker struct {
	CategoryMode        string     `json:"category_mode"`
	Categories          []Category `json:"categories,omitempty"`
	SuggestedCategories []Category `json:"suggested_categories,omitempty"`
	MaxCategories       *int       `json:"max_categories,omitempty"`
}

type CapabilityResponse struct {
	CategoryPointer   string `json:"category_pointer"`
	ConfidencePointer string `json:"confidence_pointer,omitempty"`
}

// Client is the protocol boundary used by the Agent and the administrator's
// capability refresh operation. It never sends OneBot messages.
type Client struct {
	config           Config
	httpClient       *http.Client
	maxResponseBytes int64
}

func NewClient(cfg Config) (*Client, error) {
	return NewClientWithHTTP(cfg, nil)
}

func NewClientWithHTTP(cfg Config, httpClient *http.Client) (*Client, error) {
	cfg.ProtocolMode = strings.TrimSpace(cfg.ProtocolMode)
	cfg.HTTPMethod = strings.ToUpper(strings.TrimSpace(cfg.HTTPMethod))
	cfg.CategoryPointer = strings.TrimSpace(cfg.CategoryPointer)
	cfg.ConfidencePointer = strings.TrimSpace(cfg.ConfidencePointer)
	if cfg.ProtocolMode == "" {
		cfg.ProtocolMode = ProtocolJSON
	}
	if cfg.HTTPMethod == "" {
		cfg.HTTPMethod = DefaultMethod
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.CategoryPointer == "" {
		cfg.CategoryPointer = defaultCategoryPointer
	}
	if cfg.ConfidencePointer == "" {
		cfg.ConfidencePointer = defaultConfidencePointer
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = DefaultMaxBody
	}
	if cfg.Enabled {
		if err := ValidateConfig(cfg); err != nil {
			return nil, err
		}
	} else if err := validateURLs(cfg); err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	httpClient = cloneHTTPClient(httpClient)
	previousRedirect := httpClient.CheckRedirect
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && !sameOrigin(via[len(via)-1].URL, req.URL) {
			req.Header.Del("Authorization")
		}
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		return nil
	}
	return &Client{config: cfg, httpClient: httpClient, maxResponseBytes: cfg.MaxResponseBytes}, nil
}

func (c *Client) Config() Config {
	if c == nil {
		return Config{}
	}
	return cloneConfig(c.config)
}

func cloneConfig(cfg Config) Config {
	copied := cfg
	copied.Categories = append([]Category(nil), cfg.Categories...)
	for i := range copied.Categories {
		copied.Categories[i].StickerTags = append([]string(nil), copied.Categories[i].StickerTags...)
	}
	return copied
}

func cloneHTTPClient(client *http.Client) *http.Client {
	copy := *client
	return &copy
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func validateURLs(cfg Config) error {
	if cfg.Endpoint != "" {
		if err := validateHTTPURL(cfg.Endpoint); err != nil {
			return fmt.Errorf("Laya endpoint 无效: %w", err)
		}
	}
	if cfg.CapabilitiesEndpoint != "" {
		if err := validateHTTPURL(cfg.CapabilitiesEndpoint); err != nil {
			return fmt.Errorf("Laya capabilities endpoint 无效: %w", err)
		}
	}
	return nil
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("必须是 http/https URL")
	}
	if u.User != nil || u.Fragment != "" {
		return errors.New("不得包含用户信息或 fragment")
	}
	return nil
}

func (c *Client) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := c.config.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return context.WithTimeout(ctx, timeout)
}
