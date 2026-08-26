package caller

import (
	"JuanNiang-Neo/internal/otelx"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Config RAG 客户端配置
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// Client JuanNiang-RAG-Service API 客户端。
// API 契约（tag ↔ 全文，长文服务端透明分块）：见 RAG-Service docs/API.md。
type Client struct {
	Config     Config
	HttpClient *http.Client
}

// ────────────────────── 内部辅助 ──────────────────────

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.Config.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	return resp, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any) (*http.Response, error) {
	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	return c.do(ctx, method, path, body)
}

func (c *Client) decodeJSON(resp *http.Response, dest any) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// ────────────────────── 健康检查 / 状态 ──────────────────────

// HealthCheck 探测 RAG-Service 存活（GET /health，200 即健康）。
// 与 t2i 不同：RAG-Service 有独立 /health 端点。
func (c *Client) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Config.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// Info 查询模型状态 / 内存 / 规模（Web 面板展示用）。
func (c *Client) Info(ctx context.Context) (*InfoResponse, error) {
	resp, err := c.do(ctx, http.MethodGet, "/info", nil)
	if err != nil {
		return nil, err
	}
	var result InfoResponse
	if err := c.decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ────────────────────── 写入 ──────────────────────

// Upsert 入库（幂等：已存在则覆写）。长文服务端自动分块。scoop 为分库名
// （ragtag.ScoopKnowledge 等）；同一 tag 只允许归属一个 scoop（跨库返回 409）。
func (c *Client) Upsert(ctx context.Context, scoop string, tag uuid.UUID, text string) (resp *UpsertResponse, err error) {
	// 链路追踪：RAG 写入 span（scoop 分库维度）
	_, span := otelx.Span(ctx, "rag.upsert",
		attribute.String("scoop", scoop),
		attribute.String("tag", tag.String()),
	)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	httpResp, err := c.doJSON(ctx, http.MethodPut, "/scoops/"+scoop+"/tags/"+tag.String(), map[string]any{"text": text})
	if err != nil {
		return nil, err
	}
	var result UpsertResponse
	if err := c.decodeJSON(httpResp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BatchUpsert 批量入库：一次嵌入 + 一次发布（RAG-Service 按批摊销 COW 成本）。
// 返回逐条结果（含单条失败）。全部条目必须隶属同一 scoop。
func (c *Client) BatchUpsert(ctx context.Context, scoop string, items []BatchItem) (result *BatchResponse, err error) {
	// 链路追踪：RAG 批量写入 span
	_, span := otelx.Span(ctx, "rag.batch",
		attribute.String("scoop", scoop),
		attribute.Int("items", len(items)),
	)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	if len(items) == 0 {
		return &BatchResponse{Results: []BatchItemResponse{}}, nil
	}
	resp, err := c.doJSON(ctx, http.MethodPost, "/scoops/"+scoop+"/tags/batch", map[string]any{"items": items})
	if err != nil {
		return nil, err
	}
	if err := c.decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Delete 删除 tag 及其全部块（只能删除属于该 scoop 的 tag）。
func (c *Client) Delete(ctx context.Context, scoop string, tag uuid.UUID) (err error) {
	// 链路追踪：RAG 删除 span
	_, span := otelx.Span(ctx, "rag.delete",
		attribute.String("scoop", scoop),
		attribute.String("tag", tag.String()),
	)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	resp, err := c.do(ctx, http.MethodDelete, "/scoops/"+scoop+"/tags/"+tag.String(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

// ────────────────────── 检索 ──────────────────────

// Search 语义检索（限定在单个 scoop 内）：返回命中 tag + 分数（0~1），按分数降序。
// k 默认 10（上限 100）；minScore 可选（缺省不过滤）。
func (c *Client) Search(ctx context.Context, scoop, query string, k int, minScore *float64) (hits []SearchHit, err error) {
	// 链路追踪：RAG 检索 span（scoop 分库维度；query 截断防敏感/超长）
	_, span := otelx.Span(ctx, "rag.search",
		attribute.String("scoop", scoop),
		attribute.Int("k", k),
		attribute.String("query_head", headText(query, 30)),
	)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetAttributes(attribute.Int("hits", len(hits)))
		}
		span.End()
	}()
	if k <= 0 {
		k = 10
	}
	if k > 100 {
		k = 100
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("k", fmt.Sprintf("%d", k))
	if minScore != nil {
		q.Set("min_score", fmt.Sprintf("%f", *minScore))
	}
	resp, err := c.do(ctx, http.MethodGet, "/scoops/"+scoop+"/tags/search?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var result SearchResponse
	if err := c.decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	hits = result.Results
	return hits, nil
}

// headText 截取前 n 个 rune（中文不切坏），超长加省略号。span 属性用，防敏感/超长。
func headText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
