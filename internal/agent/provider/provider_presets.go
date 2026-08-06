package provider

// ---------- 国产厂商协议能力预设表（§5.2） ----------
// 官方文档实证（2026-08-06）。前端据此渲染"厂商 → 协议下拉"，选中后自动预填
// base URL / 认证头 / api_mode，用户可再手动覆盖（url_editable=true）。

// ProviderProtocol 国产厂商预设中的单个协议能力。
type ProviderProtocol struct {
	APIMode    APIMode // chat_completions | anthropic_messages | openai_responses
	BaseURL    string  // 该协议下的 base URL（URLMode=auto 时追加协议后缀）
	AuthHeader string  // ""=协议默认；国产 anthropic 端点认证头差异见下表
	Note       string  // 覆盖范围限制（订阅 / 版本 / 模型），前端展示警告
}

// ProviderPreset 国产厂商硬编码预设。
type ProviderPreset struct {
	Key       string
	Name      string
	Protocols []ProviderProtocol // 用户可选的协议列表（硬编码，前端据此渲染下拉）
}

// providerPresets 国产厂商协议能力预设表。
var providerPresets = map[string]ProviderPreset{
	"deepseek": {
		Key: "deepseek", Name: "DeepSeek",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://api.deepseek.com", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://api.deepseek.com/anthropic", AuthHeader: "x-api-key"},
			{APIMode: APIModeOpenAIResponses, BaseURL: "https://api.deepseek.com", AuthHeader: "bearer", Note: "仅 deepseek-v4-flash 支持；无后续状态（previous_response_id/store 不支持）"},
		},
	},
	"zhipu": {
		Key: "zhipu", Name: "智谱 Z.AI",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://open.bigmodel.cn/api/paas/v4", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://open.bigmodel.cn/api/anthropic", AuthHeader: "x-api-key"},
		},
	},
	"kimi": {
		Key: "kimi", Name: "Moonshot Kimi",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://api.moonshot.cn/v1", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://api.moonshot.cn/anthropic", AuthHeader: "bearer"},
		},
	},
	"alibaba": {
		Key: "alibaba", Name: "阿里百炼",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://dashscope.aliyuncs.com/apps/anthropic", AuthHeader: "x-api-key"},
		},
	},
	"volcengine": {
		Key: "volcengine", Name: "火山方舟",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://ark.cn-beijing.volces.com/api/v3", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://ark.cn-beijing.volces.com/api/v3/anthropic", AuthHeader: "bearer", Note: "需 Coding Plan 订阅"},
			{APIMode: APIModeOpenAIResponses, BaseURL: "https://ark.cn-beijing.volces.com/api/v3", AuthHeader: "bearer", Note: "250615+ 新版模型默认支持（doubao-1-5-pro-32k-character-250715 例外）"},
		},
	},
	"minimax": {
		Key: "minimax", Name: "MiniMax",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://api.minimaxi.com/v1", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://api.minimaxi.com/anthropic", AuthHeader: "bearer", Note: "仅 M 系列支持"},
		},
	},
	"xiaomi": {
		Key: "xiaomi", Name: "小米 MiMo",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://api.xiaomimimo.com/v1", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://api.xiaomimimo.com/anthropic", AuthHeader: "api-key"},
		},
	},
	"stepfun": {
		Key: "stepfun", Name: "阶跃星辰",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://api.stepfun.com/v1", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://api.stepfun.com/step_plan", AuthHeader: "bearer", Note: "需 Step Plan 订阅"},
		},
	},
	"tencent": {
		Key: "tencent", Name: "腾讯混元",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://api.hunyuan.cloud.tencent.com/v1", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://api.hunyuan.cloud.tencent.com/anthropic", AuthHeader: "x-api-key"},
		},
	},
	"baidu": {
		Key: "baidu", Name: "百度千帆",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://qianfan.baidubce.com/v2", AuthHeader: "bearer"},
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://qianfan.baidubce.com/anthropic", AuthHeader: "x-api-key"},
		},
	},
	"siliconflow": {
		Key: "siliconflow", Name: "硅基流动",
		Protocols: []ProviderProtocol{
			{APIMode: APIModeChatCompletions, BaseURL: "https://api.siliconflow.cn/v1", AuthHeader: "bearer"},
			// base 已含 /v1，URLMode=auto 会追加 /messages
			{APIMode: APIModeAnthropicMessages, BaseURL: "https://api.siliconflow.cn/v1", AuthHeader: "bearer"},
		},
	},
}

// GetProviderPreset 返回指定厂商的协议预设。
func GetProviderPreset(key string) (ProviderPreset, bool) {
	p, ok := providerPresets[normalizeProviderKey(key)]
	return p, ok
}

// ListProviderPresets 返回全部厂商预设（供前端渲染）。
func ListProviderPresets() []ProviderPreset {
	list := make([]ProviderPreset, 0, len(providerPresets))
	for _, p := range providerPresets {
		list = append(list, p)
	}
	return list
}
