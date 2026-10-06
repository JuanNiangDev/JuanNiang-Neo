package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"JuanNiang-Neo/internal/agent/layasticker"
	"JuanNiang-Neo/internal/api/dto"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

var layaSecretRe = regexp.MustCompile(`(?i)(bearer\s+|api[_-]?key\s*[:=]\s*)[^\s,;]+`)

func dtoCategoriesToLaya(categories []dto.LayaCategoryReq) []layasticker.Category {
	result := make([]layasticker.Category, len(categories))
	for i, category := range categories {
		result[i] = layasticker.Category{
			ID:          category.ID,
			Description: category.Description,
			StickerTags: append([]string(nil), category.StickerTags...),
			NoSend:      category.NoSend,
			Enabled:     category.Enabled,
		}
	}
	return result
}

func categoriesToResponse(categories []layasticker.Category) []dto.LayaCategoryResp {
	result := make([]dto.LayaCategoryResp, len(categories))
	for i, category := range categories {
		result[i] = dto.LayaCategoryResp{
			ID:          category.ID,
			Description: category.Description,
			StickerTags: append([]string(nil), category.StickerTags...),
			NoSend:      category.NoSend,
			Enabled:     category.Enabled,
		}
	}
	return result
}

func modelCategories(cfg *models.ReplyStrategyConfig) ([]layasticker.Category, error) {
	if strings.TrimSpace(cfg.LayaStickerCategories) == "" {
		return nil, nil
	}
	var categories []layasticker.Category
	if err := json.Unmarshal([]byte(cfg.LayaStickerCategories), &categories); err != nil {
		return nil, fmt.Errorf("Laya categories JSON 无效: %w", err)
	}
	return categories, nil
}

func applyLayaConfig(cfg *models.ReplyStrategyConfig, req dto.UpdateReplyStrategyReq) error {
	categories := dtoCategoriesToLaya(req.LayaStickerCategories)
	timeout := req.LayaStickerTimeout
	if timeout <= 0 {
		timeout = 10
	}
	taskTTL := req.LayaStickerTaskTTL
	if taskTTL <= 0 {
		taskTTL = 30
	}
	minConfidence := req.LayaStickerMinConfidence
	if minConfidence < 0 {
		minConfidence = 0
	}
	if minConfidence > 1 {
		return fmt.Errorf("Laya 置信度下限必须在 0~1 之间")
	}
	protocolMode := strings.TrimSpace(req.LayaStickerProtocolMode)
	if protocolMode == "" {
		protocolMode = layasticker.ProtocolJSON
	}
	httpMethod := strings.ToUpper(strings.TrimSpace(req.LayaStickerHTTPMethod))
	if httpMethod == "" {
		httpMethod = http.MethodPost
	}
	categoryPath := strings.TrimSpace(req.LayaStickerResponseCategoryPath)
	if categoryPath == "" {
		categoryPath = "/answers/sticker/choice"
	}
	confidencePath := strings.TrimSpace(req.LayaStickerResponseConfidencePath)
	if confidencePath == "" {
		confidencePath = "/answers/sticker/answer_confidence"
	}
	apiKey := cfg.LayaStickerAPIKey
	if req.LayaStickerClearAPIKey {
		apiKey = ""
	} else if strings.TrimSpace(req.LayaStickerAPIKey) != "" {
		apiKey = req.LayaStickerAPIKey
	}

	newConfig := layasticker.Config{
		Enabled:              req.LayaStickerEnabled,
		Endpoint:             strings.TrimSpace(req.LayaStickerEndpoint),
		CapabilitiesEndpoint: strings.TrimSpace(req.LayaCapabilitiesEndpoint),
		APIKey:               apiKey,
		Model:                strings.TrimSpace(req.LayaStickerModel),
		Timeout:              time.Duration(timeout) * time.Second,
		ProtocolMode:         protocolMode,
		HTTPMethod:           httpMethod,
		RequestTemplate:      req.LayaStickerRequestTemplate,
		CategoryPointer:      categoryPath,
		ConfidencePointer:    confidencePath,
		Categories:           categories,
		MinConfidence:        minConfidence,
		TaskTTL:              time.Duration(taskTTL) * time.Second,
	}
	// Preview disables decision validation, but must still enforce credential
	// origins before any capability request can be sent.
	if cfg.LayaStickerAPIKey != "" && !req.LayaStickerClearAPIKey && strings.TrimSpace(req.LayaStickerAPIKey) == "" {
		previous := layasticker.Config{Endpoint: cfg.LayaStickerEndpoint, CapabilitiesEndpoint: cfg.LayaCapabilitiesEndpoint}
		if !layasticker.SameServiceOrigins(previous, newConfig) {
			return errors.New("Laya 服务来源已变化，请重新输入 API Key 或明确清除已保存的 API Key")
		}
	}
	if err := layasticker.ValidateConfig(newConfig); err != nil {
		return err
	}
	categoryJSON, err := json.Marshal(categories)
	if err != nil {
		return fmt.Errorf("序列化 Laya categories 失败: %w", err)
	}
	if cfg.LayaStickerEndpoint != newConfig.Endpoint || cfg.LayaCapabilitiesEndpoint != newConfig.CapabilitiesEndpoint || cfg.LayaStickerAPIKey != apiKey {
		// 新来源不能沿用旧服务或旧凭据的能力状态。
		cfg.LayaCapabilitySnapshot = ""
		cfg.LayaCapabilityFetchedAt = nil
		cfg.LayaCapabilityError = ""
	}
	cfg.LayaStickerEnabled = req.LayaStickerEnabled
	cfg.LayaStickerEndpoint = strings.TrimSpace(req.LayaStickerEndpoint)
	cfg.LayaCapabilitiesEndpoint = strings.TrimSpace(req.LayaCapabilitiesEndpoint)
	cfg.LayaStickerAPIKey = apiKey
	cfg.LayaStickerModel = strings.TrimSpace(req.LayaStickerModel)
	cfg.LayaStickerTimeout = timeout
	cfg.LayaStickerProtocolMode = protocolMode
	cfg.LayaStickerHTTPMethod = httpMethod
	cfg.LayaStickerRequestTemplate = req.LayaStickerRequestTemplate
	cfg.LayaStickerCategoryPath = categoryPath
	cfg.LayaStickerConfidencePath = confidencePath
	cfg.LayaStickerCategories = string(categoryJSON)
	cfg.LayaStickerMinConfidence = minConfidence
	cfg.LayaStickerTaskTTL = taskTTL
	return nil
}

func replyStrategyResp(cfg *models.ReplyStrategyConfig) (dto.ReplyStrategyResp, error) {
	categories, err := modelCategories(cfg)
	if err != nil {
		return dto.ReplyStrategyResp{}, err
	}
	var snapshot any
	if strings.TrimSpace(cfg.LayaCapabilitySnapshot) != "" {
		if err := json.Unmarshal([]byte(cfg.LayaCapabilitySnapshot), &snapshot); err != nil {
			return dto.ReplyStrategyResp{}, fmt.Errorf("Laya capability snapshot JSON 无效: %w", err)
		}
	}
	return dto.ReplyStrategyResp{
		Strategy:                          string(cfg.Strategy),
		RelevanceThreshold:                cfg.RelevanceThreshold,
		BotName:                           cfg.BotName,
		StripMarkdown:                     cfg.StripMarkdown,
		AgentLite:                         cfg.AgentLite,
		RelevancePrompt:                   cfg.RelevancePrompt,
		RelevanceModel:                    cfg.RelevanceModel,
		RelevanceTimeout:                  cfg.RelevanceTimeout,
		JudgeFailPolicy:                   cfg.JudgeFailPolicy,
		LayaStickerEnabled:                cfg.LayaStickerEnabled,
		LayaStickerEndpoint:               cfg.LayaStickerEndpoint,
		LayaCapabilitiesEndpoint:          cfg.LayaCapabilitiesEndpoint,
		LayaStickerAPIKeySet:              strings.TrimSpace(cfg.LayaStickerAPIKey) != "",
		LayaStickerModel:                  cfg.LayaStickerModel,
		LayaStickerTimeout:                cfg.LayaStickerTimeout,
		LayaStickerProtocolMode:           cfg.LayaStickerProtocolMode,
		LayaStickerHTTPMethod:             cfg.LayaStickerHTTPMethod,
		LayaStickerRequestTemplate:        cfg.LayaStickerRequestTemplate,
		LayaStickerResponseCategoryPath:   cfg.LayaStickerCategoryPath,
		LayaStickerResponseConfidencePath: cfg.LayaStickerConfidencePath,
		LayaStickerCategories:             categoriesToResponse(categories),
		LayaStickerMinConfidence:          cfg.LayaStickerMinConfidence,
		LayaStickerTaskTTL:                cfg.LayaStickerTaskTTL,
		LayaCapabilitySnapshot:            snapshot,
		LayaCapabilityFetchedAt:           cfg.LayaCapabilityFetchedAt,
		LayaCapabilityError:               cfg.LayaCapabilityError,
	}, nil
}

func discoverLayaCapabilities(ctx context.Context, cfg *models.ReplyStrategyConfig) (layasticker.CapabilitySnapshot, error) {
	if cfg == nil {
		return layasticker.CapabilitySnapshot{}, fmt.Errorf("Laya capabilities 配置为空")
	}
	categories, err := modelCategories(cfg)
	if err != nil {
		return layasticker.CapabilitySnapshot{}, err
	}
	timeout := cfg.LayaStickerTimeout
	if timeout <= 0 {
		timeout = 10
	}
	client, err := layasticker.NewClient(layasticker.Config{
		// Capability discovery is a configuration helper and does not need a
		// complete decision configuration (categories/template may still be
		// empty). Decision validation remains enforced when the feature runs.
		Enabled:              false,
		Endpoint:             cfg.LayaStickerEndpoint,
		CapabilitiesEndpoint: cfg.LayaCapabilitiesEndpoint,
		APIKey:               cfg.LayaStickerAPIKey,
		Model:                cfg.LayaStickerModel,
		Timeout:              time.Duration(timeout) * time.Second,
		ProtocolMode:         cfg.LayaStickerProtocolMode,
		HTTPMethod:           cfg.LayaStickerHTTPMethod,
		RequestTemplate:      cfg.LayaStickerRequestTemplate,
		CategoryPointer:      cfg.LayaStickerCategoryPath,
		ConfidencePointer:    cfg.LayaStickerConfidencePath,
		Categories:           categories,
	})
	if err != nil {
		return layasticker.CapabilitySnapshot{}, err
	}
	return client.DiscoverCapabilities(ctx)
}

func (s *Service) refreshLayaCapabilities(ctx context.Context, cfg *models.ReplyStrategyConfig) error {
	snapshot, err := discoverLayaCapabilities(ctx, cfg)
	if err != nil {
		return s.persistLayaCapabilityError(ctx, cfg, err)
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return s.persistLayaCapabilityError(ctx, cfg, err)
	}
	return s.DAO.ReplyStrategy.UpdateLayaCapabilitySnapshot(ctx, cfg, string(raw), time.Now())
}

func (s *Service) persistLayaCapabilityError(ctx context.Context, cfg *models.ReplyStrategyConfig, err error) error {
	if err == nil {
		return nil
	}
	secret := ""
	if cfg != nil {
		secret = cfg.LayaStickerAPIKey
	}
	message := sanitizeLayaError(err.Error(), secret)
	if cfg == nil {
		return fmt.Errorf("Laya capabilities 刷新失败: %s", message)
	}
	if saveErr := s.DAO.ReplyStrategy.UpdateLayaCapabilityError(ctx, cfg, message); saveErr != nil {
		return saveErr
	}
	return fmt.Errorf("Laya capabilities 刷新失败: %s", message)
}

func sanitizeLayaError(message, secret string) string {
	message = strings.TrimSpace(layaSecretRe.ReplaceAllString(message, "$1[redacted]"))
	if secret != "" {
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	if len([]rune(message)) > 512 {
		message = string([]rune(message)[:512])
	}
	return message
}

// RefreshLayaCapabilities previews request-body configuration without persistence.
// An empty body refreshes the saved configuration snapshot.
func (s *Service) RefreshLayaCapabilities(ctx context.Context, c *app.RequestContext) {
	cfg, err := s.DAO.ReplyStrategy.GetOrCreate(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	// 能力检测允许携带页面当前尚未保存的配置。这样管理员修改 endpoint/API Key
	// 后可以直接检测，不会误测数据库中的旧值；检测只使用临时副本，不会把配置
	// 变更、快照或错误状态写回数据库。无请求体时才刷新正式快照。
	raw := c.GetRawData()
	if len(bytes.TrimSpace(raw)) > 0 {
		var req dto.UpdateReplyStrategyReq
		if err := json.Unmarshal(raw, &req); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
		candidate := *cfg
		// 能力探测不要求页面当前已经满足“正式启用 Laya”的完整条件（例如
		// 类别映射还没填完）。先用关闭状态做字段归一化，再保留探测所需的
		// endpoint/API Key 等临时值；正式保存仍由 applyLayaConfig 完整校验。
		probeReq := req
		probeReq.LayaStickerEnabled = false
		if err := applyLayaConfig(&candidate, probeReq); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.Response{Status: 40032, Info: err.Error()}, nil))
			return
		}
		snapshot, err := discoverLayaCapabilities(ctx, &candidate)
		if err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.Response{Status: 50200, Info: "Laya capabilities 预览失败"}, dto.ErrorDetail{ErrorDetail: sanitizeLayaError(err.Error(), candidate.LayaStickerAPIKey)}))
			return
		}
		resp, err := replyStrategyResp(cfg)
		if err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
		now := time.Now()
		resp.LayaCapabilitySnapshot = snapshot
		resp.LayaCapabilityFetchedAt = &now
		resp.LayaCapabilityError = ""
		resp.LayaCapabilityPreview = true
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, resp))
		return
	}
	if err := s.refreshLayaCapabilities(ctx, cfg); err != nil {
		if errors.Is(err, dao.ErrLayaCapabilityConfigChanged) {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.Response{Status: 40901, Info: err.Error()}, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.Response{Status: 50200, Info: "Laya capabilities 刷新失败"}, dto.ErrorDetail{ErrorDetail: sanitizeLayaError(err.Error(), "")}))
		return
	}
	fresh, err := s.DAO.ReplyStrategy.GetOrCreate(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	resp, err := replyStrategyResp(fresh)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, resp))
}
