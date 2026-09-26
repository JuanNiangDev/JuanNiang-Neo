package layasticker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) Decide(ctx context.Context, request DecisionRequest) (DecisionResult, error) {
	if c == nil {
		return DecisionResult{}, errors.New("Laya client 未初始化")
	}
	if !c.config.Enabled {
		return DecisionResult{}, errors.New("Laya 表情决策未启用")
	}
	if request.Model == "" {
		request.Model = c.config.Model
	}
	if len(request.Categories) == 0 {
		request.Categories = cloneCategories(c.config.Categories)
	}
	if len(request.EnabledCategories) == 0 {
		for _, category := range request.Categories {
			if category.Enabled {
				request.EnabledCategories = append(request.EnabledCategories, category)
			}
		}
	}
	if len(request.Criteria) == 0 {
		for _, category := range request.EnabledCategories {
			request.Criteria = append(request.Criteria, category.ID)
		}
	}
	if len(request.CriteriaMap) == 0 {
		request.CriteriaMap = make(map[string]string)
		for _, category := range request.EnabledCategories {
			if category.Enabled {
				request.CriteriaMap[category.ID] = category.Description
			}
		}
	}
	// 协议分发：json 使用管理员配置的可配置模板；systemone.v1 使用
	// 原生请求构造函数。两种模式共用同一套 doJSON（超时、Bearer、
	// 响应大小限制、跨域重定向去鉴权）与 JSON Pointer 响应解析。
	var body []byte
	var err error
	method := c.config.HTTPMethod
	switch c.config.ProtocolMode {
	case ProtocolJSON:
		body, err = BuildJSONTemplate(c.config.RequestTemplate, request)
	case ProtocolSystemOne:
		// 原生协议只支持规范定义的 POST，不信任配置里的自定义 Method。
		method = http.MethodPost
		body, err = BuildSystemOneRequest(request)
	default:
		return DecisionResult{}, fmt.Errorf("不支持的 Laya protocol mode: %s", c.config.ProtocolMode)
	}
	if err != nil {
		return DecisionResult{}, err
	}
	resultBody, err := c.doJSON(ctx, method, c.config.Endpoint, body, "application/json")
	if err != nil {
		return DecisionResult{}, err
	}
	var document any
	if err := json.Unmarshal(resultBody, &document); err != nil {
		return DecisionResult{}, fmt.Errorf("Laya 响应 JSON 无效: %w", err)
	}
	categoryValue, err := ExtractJSONPointer(document, c.config.CategoryPointer)
	if err != nil {
		return DecisionResult{}, fmt.Errorf("Laya 响应缺少类别: %w", err)
	}
	category, ok := categoryValue.(string)
	if !ok || strings.TrimSpace(category) == "" || len([]rune(category)) > 128 {
		return DecisionResult{}, errors.New("Laya 响应类别类型无效")
	}
	result := DecisionResult{Category: strings.TrimSpace(category)}
	if strings.TrimSpace(c.config.ConfidencePointer) != "" {
		if confidenceValue, confidenceErr := ExtractJSONPointer(document, c.config.ConfidencePointer); confidenceErr == nil {
			confidence, ok := numberValue(confidenceValue)
			if !ok || math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
				return DecisionResult{}, errors.New("Laya 响应置信度类型无效")
			}
			result.Confidence = &confidence
		}
	}
	return result, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, body []byte, contentType string) ([]byte, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, errors.New("Laya endpoint 为空")
	}
	// Reject an insecure first hop before attaching a Bearer credential.
	// Local Laya instances may use HTTP; all other credential destinations need TLS.
	if strings.TrimSpace(c.config.APIKey) != "" {
		target, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("Laya endpoint URL 无效: %w", err)
		}
		hostIP := net.ParseIP(target.Hostname())
		if target.Scheme == "http" && !strings.EqualFold(target.Hostname(), "localhost") &&
			(hostIP == nil || !hostIP.IsLoopback()) {
			return nil, errors.New("配置 API Key 时，Laya HTTP endpoint 仅允许 localhost 或回环 IP；其他地址必须使用 HTTPS")
		}
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造 Laya 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(c.config.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Laya 请求返回 HTTP status %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, c.maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("读取 Laya 响应失败: %w", err)
	}
	if int64(len(data)) > c.maxResponseBytes {
		return nil, fmt.Errorf("Laya 响应过大，超过 %d bytes", c.maxResponseBytes)
	}
	return data, nil
}

func cloneCategories(categories []Category) []Category {
	cloned := append([]Category(nil), categories...)
	for i := range cloned {
		cloned[i].StickerTags = append([]string(nil), cloned[i].StickerTags...)
	}
	return cloned
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (c *Client) capabilitiesEndpoint() (string, error) {
	if strings.TrimSpace(c.config.CapabilitiesEndpoint) != "" {
		return c.config.CapabilitiesEndpoint, nil
	}
	parsed, err := url.Parse(c.config.Endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("无法从 Laya endpoint 推导 capabilities endpoint")
	}
	// Laya 的 capabilities 固定在服务根路径，与决策路径（如 /v1/systemone）无关。
	parsed.Path = "/capabilities"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (c *Client) DiscoverCapabilities(ctx context.Context) (CapabilitySnapshot, error) {
	if c == nil {
		return CapabilitySnapshot{}, errors.New("Laya client 未初始化")
	}
	endpoint, err := c.capabilitiesEndpoint()
	if err != nil {
		return CapabilitySnapshot{}, err
	}
	body, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, "")
	if err != nil {
		return CapabilitySnapshot{}, err
	}
	var snapshot CapabilitySnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return CapabilitySnapshot{}, fmt.Errorf("capabilities JSON 无效: %w", err)
	}
	if err := ValidateCapabilitySnapshot(snapshot); err != nil {
		return CapabilitySnapshot{}, err
	}
	// 来源由本地请求确定，不信任服务端自报的地址。
	snapshot.SourceEndpoint = c.config.Endpoint
	snapshot.SourceCapabilitiesEndpoint = endpoint
	return snapshot, nil
}
