package layasticker

import (
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
)

var categoryIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)

func ValidateCategories(categories []Category) error {
	if len(categories) == 0 {
		return fmt.Errorf("至少配置一个表情类别")
	}
	if len(categories) > 128 {
		return fmt.Errorf("表情类别数量超过 128")
	}
	seen := make(map[string]struct{}, len(categories))
	hasEnabledNoSend := false
	for _, category := range categories {
		id := strings.TrimSpace(category.ID)
		if !categoryIDPattern.MatchString(id) {
			return fmt.Errorf("类别 ID 无效: %q", category.ID)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("类别 ID 重复: %s", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(category.Description) == "" || len([]rune(category.Description)) > 512 {
			return fmt.Errorf("类别 %s 的说明为空或过长", id)
		}
		if len(category.StickerTags) > 32 {
			return fmt.Errorf("类别 %s 的表情标签数量超过 32", id)
		}
		for _, tag := range category.StickerTags {
			if strings.TrimSpace(tag) == "" || len([]rune(tag)) > 128 {
				return fmt.Errorf("类别 %s 的表情标签无效", id)
			}
		}
		if category.Enabled && category.NoSend {
			hasEnabledNoSend = true
		}
	}
	if !hasEnabledNoSend {
		return fmt.Errorf("至少需要一个启用的不发送类别")
	}
	return nil
}

func ValidateConfig(cfg Config) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return fmt.Errorf("启用 Laya 时必须配置 endpoint")
	}
	if err := validateURLs(cfg); err != nil {
		return err
	}
	if cfg.Timeout <= 0 || cfg.Timeout > 120_000_000_000 {
		return fmt.Errorf("Laya timeout 必须在 1ns-120s 范围内")
	}
	if math.IsNaN(cfg.MinConfidence) || math.IsInf(cfg.MinConfidence, 0) || cfg.MinConfidence < 0 || cfg.MinConfidence > 1 {
		return fmt.Errorf("Laya 置信度下限必须在 0~1 之间")
	}
	if cfg.TaskTTL < 0 {
		return fmt.Errorf("Laya 自动表情有效期不能为负数")
	}
	mode := strings.TrimSpace(cfg.ProtocolMode)
	if mode == "" {
		mode = ProtocolJSON
	}
	method := strings.ToUpper(strings.TrimSpace(cfg.HTTPMethod))
	if method == "" {
		method = http.MethodPost
	}
	// 两种协议目前都只支持 POST；原生 systemone.v1 尤其不接受
	// 未经验证的自定义 Method。
	if method != http.MethodPost {
		return fmt.Errorf("Laya 决策请求目前只支持 POST")
	}
	if len(cfg.APIKey) > 4096 {
		return fmt.Errorf("Laya API key 过长")
	}
	if err := ValidateCategories(cfg.Categories); err != nil {
		return err
	}
	// 响应路径两种协议共用（留空走默认值）。
	if _, err := ParseJSONPointer(cfg.CategoryPointer); err != nil {
		return fmt.Errorf("类别响应路径无效: %w", err)
	}
	if strings.TrimSpace(cfg.ConfidencePointer) != "" {
		if _, err := ParseJSONPointer(cfg.ConfidencePointer); err != nil {
			return fmt.Errorf("置信度响应路径无效: %w", err)
		}
	}
	switch mode {
	case ProtocolJSON:
		// 切回 JSON 模式时重新验证模板。
		if err := ValidateJSONTemplate(cfg.RequestTemplate); err != nil {
			return fmt.Errorf("Laya 请求模板无效: %w", err)
		}
		if len(cfg.RequestTemplate) > 1<<20 {
			return fmt.Errorf("Laya 请求模板过大")
		}
	case ProtocolSystemOne:
		// 原生模式请求由客户端自动生成：要求模型，不要求 JSON 模板；
		// 数据库里遗留的无效/过时模板不阻止切换到原生模式。
		if strings.TrimSpace(cfg.Model) == "" {
			return fmt.Errorf("systemone.v1 模式必须配置模型")
		}
	default:
		return fmt.Errorf("不支持的 Laya protocol mode: %s", mode)
	}
	return nil
}

func ValidateCapabilitySnapshot(snapshot CapabilitySnapshot) error {
	if snapshot.SchemaVersion != 1 {
		return fmt.Errorf("不支持的 capabilities schema_version: %d", snapshot.SchemaVersion)
	}
	if strings.TrimSpace(snapshot.Service) != "laya" {
		return fmt.Errorf("capabilities service 必须是 laya")
	}
	if strings.TrimSpace(snapshot.API.Protocol) == "" {
		return fmt.Errorf("capabilities api.protocol 不能为空")
	}
	if strings.TrimSpace(snapshot.API.DecisionPath) == "" || !strings.HasPrefix(snapshot.API.DecisionPath, "/") {
		return fmt.Errorf("capabilities decision_path 无效")
	}
	method := strings.ToUpper(strings.TrimSpace(snapshot.API.Method))
	if method != http.MethodPost {
		return fmt.Errorf("capabilities api.method 必须是 POST")
	}
	if len(snapshot.Models) == 0 {
		return fmt.Errorf("capabilities models 不能为空")
	}
	seenModels := make(map[string]struct{}, len(snapshot.Models))
	for _, model := range snapshot.Models {
		if strings.TrimSpace(model.ID) == "" {
			return fmt.Errorf("capabilities model id 不能为空")
		}
		if _, ok := seenModels[model.ID]; ok {
			return fmt.Errorf("capabilities model id 重复: %s", model.ID)
		}
		seenModels[model.ID] = struct{}{}
		if model.Sticker.CategoryMode != "fixed" && model.Sticker.CategoryMode != "dynamic" {
			return fmt.Errorf("model %s 的 category_mode 无效", model.ID)
		}
		categories := model.Sticker.Categories
		if model.Sticker.CategoryMode == "dynamic" {
			categories = model.Sticker.SuggestedCategories
		}
		if len(categories) > 0 {
			if err := validateCapabilityCategories(categories); err != nil {
				return fmt.Errorf("model %s categories 无效: %w", model.ID, err)
			}
		}
		if model.Sticker.MaxCategories != nil && *model.Sticker.MaxCategories <= 0 {
			return fmt.Errorf("model %s max_categories 无效", model.ID)
		}
	}
	if strings.TrimSpace(snapshot.Response.CategoryPointer) == "" {
		return fmt.Errorf("capabilities category_pointer 不能为空")
	}
	if _, err := ParseJSONPointer(snapshot.Response.CategoryPointer); err != nil {
		return fmt.Errorf("capabilities category_pointer 无效: %w", err)
	}
	if strings.TrimSpace(snapshot.Response.ConfidencePointer) != "" {
		if _, err := ParseJSONPointer(snapshot.Response.ConfidencePointer); err != nil {
			return fmt.Errorf("capabilities confidence_pointer 无效: %w", err)
		}
	}
	return nil
}

func validateCapabilityCategories(categories []Category) error {
	seen := make(map[string]struct{}, len(categories))
	for _, category := range categories {
		id := strings.TrimSpace(category.ID)
		if !categoryIDPattern.MatchString(id) {
			return fmt.Errorf("类别 ID 无效: %q", category.ID)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("类别 ID 重复: %s", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(category.Description) == "" || len([]rune(category.Description)) > 512 {
			return fmt.Errorf("类别 %s 的说明无效", id)
		}
	}
	return nil
}
