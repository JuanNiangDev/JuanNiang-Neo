# 多协议 LLM Provider 适配实施方案

> 状态：待评审（未实施）
> 关联 Issue：[JuanNiang-Neo#22](https://github.com/JuanNiangDev/JuanNiang-Neo/issues/22)（URL 后缀硬编码）
> 参考实现：MintWord（`src-tauri/src/ai.rs` + `src/lib/aiProviders.ts`）、cloudwego/eino-ext（context7 实测组件文档）

---

## 1. 背景与目标

### 1.1 现状问题

| 问题 | 位置 | 影响 |
|---|---|---|
| 只支持 OpenAI Chat Completions 一种协议 | `internal/agent/provider/provider.go` | 无法直连 Anthropic / Gemini / OpenAI Responses 原生端点 |
| `/v1/chat/completions` 硬编码 3 处 | provider.go :45 `Chat`、:56 `ChatStream`、:146 `Vision` | Issue #22：用户无法指定完整 URL，网关（Open WebUI 等非标准路径）无法接入 |
| ChatStream 绕过 `doRequest` 重复拼接 URL | provider.go :56 vs :232 | 拼接逻辑漂移风险 |
| thinking 适配为"双字段全开"（`thinking` + `enable_thinking` 同时携带） | provider.go `buildChatBody` | 只对宽松实现有效，严格校验请求体的厂商（Anthropic/Gemini）无法适配 |
| 多模态仅支持图片 → OpenAI `image_url` 格式 | provider.go `Vision` | 无 MIME 信息、无 Anthropic image block / Gemini inlineData |
| 无模型能力元数据 | `ProviderConfig` | 无 contextWindow / maxOutput，`max_tokens` 无模型级默认 |

### 1.2 目标

1. **解决 Issue #22**：URL 后缀可由用户指定（填完整 URL 或 base URL 均可）
2. **多协议**：`chat_completions` / `anthropic_messages` / `openai_responses` / `gemini_native` 四种协议分派
3. **不同模型适配**：thinking/reasoning 按厂商矩阵精确适配、生成参数按协议映射、多模态按协议转换
4. **不破坏存量**：默认 `chat_completions`，存量配置零迁移

### 1.3 非目标

- 不引入 eino-ext 作为协议实现（见 §3.2 论证；其组件配置规格仅作参考）
- 不做流式响应的全协议覆盖（Phase 2 流式范围见 §6）
- 不做上下文压缩/截断策略（属后续任务，方案预留元数据基础）

---

## 2. 现状代码定位

| 层 | 文件 | 说明 |
|---|---|---|
| Provider 接口与配置 | `internal/agent/provider/root.go` | `ProviderConfig{ID,Type,Name,Endpoint,Token,Model,Temperature,EnableThinking}`；`ModelType` = text/image/embedding |
| 协议实现 | `internal/agent/provider/provider.go` | `openAIProvider`：Chat / ChatStream / Vision / doRequest / buildChatBody / parseChatResponse |
| Eino 适配 | `internal/agent/provider/eino_model.go` | `EinoModelAdapter` 包装 `Provider` 为 Eino `ToolCallingChatModel` |
| GORM 模型 | `internal/core/models/provider.go` | `Provider{Type,Endpoint,Token,Model,Temperature,...}` |
| API DTO | `internal/api/dto/request.go` :63-78、`response.go` :85-89、`transfer.go` | Add/Update/Resp 三套结构 |
| 服务层 | `internal/api/service/service.go` :203-395 | model ↔ DTO 转换、创建/更新 |
| 前端 | `web/src/api/index.ts` :12-13、`web/src/views/ProvidersPage.vue` | `AddProviderReq`、表单"API 地址" |

---

## 3. 参考设计

### 3.1 MintWord（已实测，`src-tauri/src/ai.rs`）

- **`api_mode` 四协议枚举**：`chat_completions | anthropic_messages | openai_responses | gemini_native`
- **协议分派**：`match api_mode` 分发到 4 个 build 函数 + 1 个 `extract_response_content`：

| api_mode | URL 拼接 | 认证 | 请求体要点 |
|---|---|---|---|
| chat_completions | `{url}/chat/completions` | `Authorization: Bearer` | messages + tools + 采样参数 |
| anthropic_messages | `{url}/messages` | `x-api-key` + `anthropic-version: 2023-06-01` | `system` 独立字段、`max_tokens` 必填 |
| openai_responses | `{url}/responses` | `Authorization: Bearer` | `input`（非 messages） |
| gemini_native | `{url}/models/{model}:generateContent?key={key}` | 无 header（key 在 query） | `contents[].parts[]` + `generationConfig` |

- **thinking 按厂商矩阵**（`apply_thinking_chat` 按 `provider_id` 分派）：

| 厂商 | 字段 |
|---|---|
| deepseek / kimi | `thinking={type:enabled/disabled}` + `reasoning_effort`（kimi） |
| zhipu / zai | `thinking={type:enabled/disabled}` |
| alibaba / siliconflow | `enable_thinking` |
| stepfun | `reasoning_effort` |
| minimax | `reasoning_split` |
| 默认（openai 兼容） | `reasoning_effort` |

- **Anthropic thinking**：模型含 `4-7`/`4-8` → `thinking={type:"adaptive"}`；否则 `thinking={type:"enabled", budget_tokens:N}`
- **Gemini thinking**：`gemini-3*` → `thinkingConfig={includeThoughts:true, thinkingLevel:effort.UPPER}`；否则 `{includeThoughts:true, thinkingBudget:N}`
- **模型元数据**（`aiProviders.ts`，76KB 预设表）：每模型标注 `contextWindow` / `maxOutput` / `thinkingSupport` / `thinkingPattern` / `thinkingLevels` / `supportedParams`；每厂商标注 `baseUrl` / `protocols` / `urlEditable` / `subOptions` / `urlMap`
- **已知缺陷**（本方案规避）：fully-custom 模式提示用户填完整 URL，但后端 `format!("{}/chat/completions", url)` 仍追加，产生双份路径
- **国产厂商协议结论**（官方文档实证，2026-08-06，详见 §5.2 矩阵）：MintWord 中**全部国产厂商（含编码计划/按量计费）预设 `apiMode` 均为 `chat_completions`**，无一走 anthropic_messages —— `protocols: ['openai','anthropic']`（MiniMax/小米）只是声明端点双协议能力，代码仅读 `apiMode`，唯一切换入口是 fully-custom 手动子选项。**注意**：此限制是 MintWord 自身选择而非厂商能力 —— 全部 11 家国产厂商官方均提供 Anthropic 兼容端点（DeepSeek 另有 `/anthropic`、`/responses`），本方案 §5.2 已将其纳入协议矩阵

### 3.2 eino-ext（context7 实测组件规格）

**为何不直接采用 eino-ext 组件实现**：eino-ext 组件各自返回 Eino `model.ToolCallingChatModel`，与本项目 `provider.Provider` 接口、`ProviderGroup` 容器、自研 `openAIProvider` 体系不同构；且各组件生成/流式/thinking 提取 API 差异大（`claude.GetThinking`、`deepseek.GetReasoningContent`、gemini `ReasoningContent` 字段），接入改造面大于自研协议层扩展。**但其配置规格是本方案自研实现的标准参考**：

| 组件 | 配置要点（作为规格） | 本方案对应 |
|---|---|---|
| openai | `BaseURL`、`ReasoningEffort`（low/medium/high）、`MaxCompletionTokens`、`Temperature`、`ByAzure` | chat_completions + reasoning_effort |
| claude | `APIKey/Model/MaxTokens` **必填**、`BaseURL *string`、`WithThinkingConfig(OfAdaptive/OfEnabled budget_tokens)`、`GetThinking` | anthropic_messages |
| gemini | `genai.ClientConfig{HTTPOptions.BaseURL}`、`ThinkingConfig{IncludeThoughts,ThinkingBudget}`、`ReasoningContent`、图片输入 `UserInputMultiContent{Base64Data,MIMEType}` | gemini_native |
| deepseek | `BaseURL: https://api.deepseek.com/beta`、`GetReasoningContent` | chat_completions + thinking 矩阵 |
| qwen | `BaseURL: .../compatible-mode/v1`、`MaxTokens/Temperature/TopP` | chat_completions |
| openrouter | `Reasoning{Effort}` | chat_completions + reasoning_effort |
| ark | 多模态 generate_with_image（base64 图片） | 多模态参考 |

---

## 4. 总体架构

```
┌─────────────────────────────────────────────────────────┐
│ ProviderGroup (root.go)                                  │
│   接口不变：Chat / ChatStream / Vision / Model()          │
├─────────────────────────────────────────────────────────┤
│ openAIProvider (provider.go) —— 单实现，按 cfg.APIMode 分派 │
│                                                         │
│  endpointURL(apimode)  ──►  base URL + 自动识别完整 URL   │
│  buildRequest(apimode) ──►  4 个协议构造器（MintWord 模式）│
│  parseResponse(apimode) ──►  4 个响应提取器               │
│  buildVisionPart(apimode)─►  多模态格式转换               │
│  applyThinking(apimode, provider) ──►  厂商矩阵           │
├─────────────────────────────────────────────────────────┤
│ ProviderConfig (root.go) / GORM / DTO / 前端              │
│  新增：APIMode, ThinkingEffort, ThinkingBudget,          │
│        MaxTokens, TopP, TopK, FrequencyPenalty,         │
│        PresencePenalty, RepetitionPenalty（可选渐进）     │
└─────────────────────────────────────────────────────────┘
```

设计原则：**接口层零改动**（`Provider` 接口、`EinoModelAdapter`、`agent.go` 等调用方不受影响），协议差异全部收敛在 `provider.go` 内部。

---

## 5. 详细设计

### 5.1 配置模型扩展

**`internal/agent/provider/root.go`**：

```go
// APIMode 协议模式（与 MintWord apiMode 对齐）
type APIMode string

const (
    APIModeChatCompletions   APIMode = "chat_completions"
    APIModeAnthropicMessages APIMode = "anthropic_messages"
    APIModeOpenAIResponses   APIMode = "openai_responses"
    APIModeGeminiNative      APIMode = "gemini_native"
)

type ProviderConfig struct {
    ID          string
    Type        ModelType
    Name        string
    Endpoint    string   // base URL 或完整端点（自动识别，见 §5.3）
    Token       string
    Model       string
    Temperature float32
    EnableThinking bool
    // ---- 新增 ----
    APIMode            APIMode  `json:"api_mode,omitempty"`            // 空 = chat_completions
    ThinkingEffort     string   `json:"thinking_effort,omitempty"`     // off/low/medium/high
    ThinkingBudget     int      `json:"thinking_budget,omitempty"`     // anthropic/gemini budget
    MaxTokens          int      `json:"max_tokens,omitempty"`          // 0 = 协议默认
    TopP               *float32 `json:"top_p,omitempty"`
    TopK               *int     `json:"top_k,omitempty"`
    FrequencyPenalty   *float32 `json:"frequency_penalty,omitempty"`
    PresencePenalty    *float32 `json:"presence_penalty,omitempty"`
    RepetitionPenalty  *float32 `json:"repetition_penalty,omitempty"`
    // ---- 多协议自定义（国产厂商调研结论并入）----
    ProviderKey string `json:"provider_key,omitempty"` // 厂商分组（deepseek/kimi/zhipu/...），驱动 thinking 矩阵与认证头分派；空 = 按 Name 关键词匹配
    AuthHeader  string `json:"auth_header,omitempty"`  // ""|"bearer"|"x-api-key"|"api-key"：anthropic 国产端点认证头不统一（§5.2）
    URLMode     string `json:"url_mode,omitempty"`     // ""|"auto"|"exact"：auto=base+协议后缀自动拼接（默认）；exact=完整端点原样使用（完全自定义，必须自写 v1 后完整后缀，§5.3）
}
```

**GORM 模型**（`internal/core/models/provider.go`）追加同名字段（GORM AutoMigrate 自动加列，无需手写 SQL；`sql/init.sql` 仅注释参考）。默认值语义：

- `api_mode` 空/`""` → 运行时按 `chat_completions` 处理（存量兼容）
- `thinking_effort` 空 → 关闭 thinking（`EnableThinking` 兼容保留：`EnableThinking=true` 且 effort 空 → `medium`）
- `url_mode` 空 → `auto`（存量 `Endpoint` 均为 base URL，行为不变）
- `auth_header` 空 → 按协议默认（chat_completions/responses=Bearer；anthropic=`x-api-key`；gemini=`x-goog-api-key`）

**API DTO**（`internal/api/dto/request.go` / `response.go` / `transfer.go`）与 **service.go**（:203-395）同步透传新字段。

**前端**（`web/src/api/index.ts` `AddProviderReq`、`ProvidersPage.vue` 表单）：

- 类型下拉新增 `api_mode` 选择（默认 chat_completions），**预置 4 类自定义入口**（详见 §5.7 预设表）：
  - OpenAI 兼容自定义 → `不需要包含 /chat/completions 后缀`
  - OpenAI Responses 自定义 → `不需要包含 /responses 后缀`
  - Anthropic 自定义 → `不需要包含 /v1/messages 后缀`（认证头可配：x-api-key / Bearer / api-key）
  - **完全自定义** → `必须自写 v1 之后的完整端点路径（如 /openai/v1/chat/completions），系统不追加任何后缀`
- 通用提示：`也可以直接粘贴完整端点地址（以协议后缀结尾），系统自动识别`
- 国产厂商联动：选择国产厂商 → 按**硬编码协议能力表**（§5.2 预设表）渲染**协议下拉**（openai 兼容 / anthropic / responses，按该厂商支持情况）→ 选中后自动预填 base URL、认证头、api_mode（可再手动覆盖）；协议级限制（订阅 / 版本 / 模型范围）以警示文案展示
- thinking 配置：`thinking_effort` 下拉（off/low/medium/high，按协议显示可用档位）

### 5.2 协议层（provider.go 重构）

`openAIProvider` 增加协议分派，三个方法统一入口：

```go
// Chat / Vision 统一走 doRequest
func (p *openAIProvider) Chat(...) {
    body := p.buildRequest(req, false)
    resp, err := p.doRequest(ctx, p.endpointURL(), body)   // 原硬编码 path 删除
    return p.parseResponse(resp)
}

func (p *openAIProvider) ChatStream(...) {
    // 复用 endpointURL()，删除 :56 独立拼接
}
```

**请求构造器**（MintWord `build_*` 模式）：

```go
func (p *openAIProvider) buildRequest(req ChatRequest, stream bool) ([]byte, error) {
    switch p.cfg.APIMode {
    case APIModeAnthropicMessages: return p.buildAnthropic(req, stream)
    case APIModeOpenAIResponses:   return p.buildResponses(req, stream)
    case APIModeGeminiNative:      return p.buildGemini(req, stream)
    default:                       return p.buildChatCompletions(req, stream)
    }
}
```

各协议请求体要点（规格，参考 MintWord + eino-ext）：

| | chat_completions | anthropic_messages | openai_responses | gemini_native |
|---|---|---|---|---|
| 端点 | `{url}/chat/completions` | `{url}/messages` | `{url}/responses` | `{url}/models/{model}:generateContent` |
| 认证 | `Authorization: Bearer` | `x-api-key` + `anthropic-version: 2023-06-01` | `Authorization: Bearer` | `x-goog-api-key`（推荐，替代 MintWord 的 `?key=` query） |
| 系统提示 | messages[0] role=system | 顶层 `system` 字段 | 无（并入 input 首条 system message） | 拼接进首条 user parts 或 `systemInstruction` |
| 消息体 | `messages[]` | `messages[]`（user/assistant 交替） | `input`（非 messages，支持 `input_image`） | `contents[]`（role user/model 交替） |
| max_tokens | `max_tokens` | `max_tokens` **必填**，默认 4096 | `max_tokens` | `generationConfig.maxOutputTokens` |
| 温度 | `temperature` | `temperature` | — | `generationConfig.temperature` |
| top_p / top_k | `top_p` / — | `top_p` / `top_k` | — | `generationConfig.topP` / `topK` |
| 惩罚 | `frequency_penalty` / `presence_penalty` | — | — | `repetition_penalty`（`generationConfig`） |
| 工具 | `tools[]`(function) | `tools[]`(function) | `tools[]` | `tools[]`(functionDeclaration) |

**国产厂商协议能力预设表**（硬编码，官方文档实证 2026-08-06）—— 每个国产厂商预置其支持的全部协议，**用户配置时按协议下拉自选**（openai 兼容 / anthropic / responses），系统按所选协议自动填 base URL + 认证头 + api_mode：

| 厂商 | openai 兼容 | anthropic | responses | openai 端点 | anthropic 端点（认证头） | responses 端点 / 覆盖 |
|---|---|---|---|---|---|---|
| DeepSeek | ✓ | ✓ | ✓ | `api.deepseek.com` | `api.deepseek.com/anthropic`（x-api-key） | `api.deepseek.com/responses`（**仅 v4-flash**） |
| 智谱 Z.AI | ✓ | ✓ | ✗ | `open.bigmodel.cn/api/paas/v4` | `open.bigmodel.cn/api/anthropic`（x-api-key） | — |
| Moonshot Kimi | ✓ | ✓ | ✗ | `api.moonshot.cn/v1`（国际 `.ai`） | `api.moonshot.cn/anthropic`（Bearer） | — |
| 阿里百炼 | ✓ | ✓ | ✗ | `dashscope.aliyuncs.com/compatible-mode/v1` | `dashscope.aliyuncs.com/apps/anthropic`（x-api-key 或 Bearer） | — |
| 火山方舟 | ✓ | ✓ | ✓ | `ark.cn-beijing.volces.com/api/v3` | `ark.cn-beijing.volces.com/api/v3/anthropic`（Bearer） | `.../api/v3/responses`（**250615+ 新版模型**默认支持；`doubao-1-5-pro-32k-character-250715` 例外） |
| MiniMax | ✓ | ✓ | ✗ | `api.minimaxi.com/v1`（国际 `.io`） | `api.minimaxi.com/anthropic`（Bearer，非 x-api-key） | — |
| 小米 MiMo | ✓ | ✓ | ✗ | `api.xiaomimimo.com/v1` | `api.xiaomimimo.com/anthropic`（api-key 或 Bearer） | — |
| 阶跃星辰 | ✓ | ✓ | ✗ | `api.stepfun.com/v1` | `api.stepfun.com/step_plan`（Bearer） | — |
| 腾讯混元 | ✓ | ✓ | ✗ | `api.hunyuan.cloud.tencent.com/v1` | `api.hunyuan.cloud.tencent.com/anthropic`（x-api-key 必选） | — |
| 百度千帆 | ✓ | ✓ | ✗ | `qianfan.baidubce.com/v2` | `qianfan.baidubce.com/anthropic`（x-api-key） | — |
| 硅基流动 | ✓ | ✓ | ✗ | `api.siliconflow.cn/v1` | `api.siliconflow.cn/v1/messages`（Bearer，base 已含 /v1） | — |

**硬编码预设结构**（`models_catalog.go` 或独立 `provider_presets.go`）：

```go
// ProviderProtocol 国产厂商预设中的单个协议能力
type ProviderProtocol struct {
    APIMode    APIMode  // chat_completions | anthropic_messages | openai_responses
    BaseURL    string   // 该协议下的 base URL（端点层级见 §5.3 auto 模式）
    AuthHeader string   // ""=协议默认；国产 anthropic 差异见下表
    Note       string   // 覆盖范围限制（订阅 / 版本 / 模型），前端展示警告
}

// ProviderPreset 国产厂商硬编码预设
type ProviderPreset struct {
    Key       string
    Name      string
    Protocols []ProviderProtocol // 用户可选的协议列表（硬编码，前端据此渲染下拉）
}

var providerPresets = map[string]ProviderPreset{
    "deepseek": {
        Key: "deepseek", Name: "DeepSeek",
        Protocols: []ProviderProtocol{
            {APIMode: APIModeChatCompletions,   BaseURL: "https://api.deepseek.com",                 AuthHeader: "bearer"},
            {APIMode: APIModeAnthropicMessages, BaseURL: "https://api.deepseek.com/anthropic",       AuthHeader: "x-api-key"},
            {APIMode: APIModeOpenAIResponses,   BaseURL: "https://api.deepseek.com",                 AuthHeader: "bearer", Note: "仅 deepseek-v4-flash"},
        },
    },
    // 其余 10 家同构（表见上）
}
```

**协议选择流程**：
1. 前端选厂商 → 读 `preset.Protocols` 渲染**协议下拉**（如 DeepSeek 3 项、智谱 2 项、火山 3 项）
2. 用户选协议 → 自动填 `baseURL` / `authHeader` / `api_mode`（可再手动覆盖，`urlEditable=true`）
3. 协议级限制（`Note`）前端提示：DeepSeek responses 仅 flash；火山 anthropic 需 **Coding Plan 订阅**、responses 需 250615+ 新版模型；阶跃 anthropic 需 **Step Plan 订阅**；MiniMax anthropic 仅 M 系列
4. 后端无需再猜协议 —— `api_mode` + `auth_header` + `url_mode` 由预设直接落地，硬编码表与厂商官方文档同步维护

**认证头分派**（`setHeaders` 按 `cfg.AuthHeader`，默认按协议）：`bearer`→`Authorization: Bearer {token}`；`x-api-key`→`x-api-key: {token}`（anthropic 附带 `anthropic-version: 2023-06-01`）；`api-key`→`api-key: {token}`（小米）

**端点形态**（全部可被 §5.3 auto 模式覆盖）：`{base}/anthropic`（DeepSeek/智谱/Kimi/MiniMax/小米/混元/千帆）、`{base}/apps/anthropic`（阿里）、`{base}/v1/messages`（硅基）、`{base}/step_plan`（阶跃，SDK 拼 `/v1/messages`）

**OpenAI Responses 国产支持**（覆盖范围差异）：

| 厂商 | Responses 端点 | 覆盖模型 | 差异 |
|---|---|---|---|
| 火山方舟 | `https://ark.cn-beijing.volces.com/api/v3/responses` | **250615 及之后版本的大语言模型默认支持**（`doubao-1-5-pro-32k-character-250715` 例外；TPM 保障包/精调/智能路由不支持） | `previous_response_id` 多轮、`store:true`、caching、内置工具 |
| DeepSeek | `https://api.deepseek.com/responses` | **仅 `deepseek-v4-flash`**（v4-pro 2026-08 初） | **无状态**（previous_response_id/store 不支持）；SSE 语义事件结束（response.completed/incomplete/failed），**无 `data: [DONE]`** |
| 智谱 Z.AI | 未提供 | — | — |

**流式解析差异**（Phase 2 实现要点）：DeepSeek Responses 流式按 `event` 字段分派（`response.output_text.delta` / `response.completed`），与 chat_completions 的 `data:` 前缀解析不同，`parseResponsesStream` 需独立实现。

**响应提取器**：

```go
func (p *openAIProvider) parseResponse(body []byte) (*ChatResponse, error) {
    switch p.cfg.APIMode {
    case APIModeAnthropicMessages: return p.parseAnthropic(body)  // content[0].text + thinking blocks
    case APIModeOpenAIResponses:   return p.parseResponses(body)  // output[0].content[] → text
    case APIModeGeminiNative:      return p.parseGemini(body)     // candidates[0].content.parts[0].text + reasoning
    default:                       return p.parseChatCompletions(body)
    }
}
```

流式：Phase 2 范围 —— chat_completions 保持现有；anthropic（`content_block_delta`）/gemini（`candidates[].content.parts[]`）SSE 格式不同，单独实现；responses（`response.output_text.delta`）同批实现；**其余协议流式不支持时报明确错误**（与 eino-ext 组件行为一致）。

### 5.3 URL 规则（解决 Issue #22）

```go
// 协议已知后缀表：URL 以其结尾 → 视为完整端点，不再追加
var apiSuffixes = map[APIMode][]string{
    APIModeChatCompletions:   {"/chat/completions"},
    APIModeAnthropicMessages: {"/messages"},
    APIModeOpenAIResponses:   {"/responses"},
    APIModeGeminiNative:      {":generateContent"},
}

func (p *openAIProvider) endpointURL() string {
    if p.cfg.URLMode == "exact" {
        return p.cfg.Endpoint // 完全自定义：原样使用，不追加不识别（必须自写 v1 后完整后缀）
    }
    e := strings.TrimRight(p.cfg.Endpoint, "/")
    for _, s := range apiSuffixes[p.cfg.APIMode] {
        if strings.HasSuffix(strings.ToLower(e), s) {
            return e // 用户已提供完整端点
        }
    }
    switch p.cfg.APIMode {
    case APIModeGeminiNative:
        return e + "/models/" + url.PathEscape(p.cfg.Model) + ":generateContent"
    case APIModeAnthropicMessages:
        return e + "/messages"
    case APIModeOpenAIResponses:
        return e + "/responses"
    default:
        return e + "/chat/completions"
    }
}
```

要点：

- 向后兼容：存量 `Endpoint`（如 `https://api.deepseek.com`、`https://api.openai.com/v1`）行为不变
- 完整 URL：用户填 `https://openwebui.local/openai/v1/chat/completions` → 直接使用（修 MintWord 双份 bug）
- **`url_mode=exact`（完全自定义）**：`endpointURL()` 直接返回 `Endpoint`，跳过后缀识别与追加 —— 用户必须自写 v1 之后完整后缀；与 MintWord fully-custom（无条件追加导致双份路径 bug）的本质区别
- 国产 Anthropic 端点形态（§5.2）在 auto 模式下的填法：`https://api.deepseek.com/anthropic`（拼 `/messages`）、`https://dashscope.aliyuncs.com/apps/anthropic`、`https://api.siliconflow.cn/v1`（拼 `/messages`）、`https://api.stepfun.com/step_plan` —— 统一由 suffix 表 + 完整 URL 识别覆盖
- `Chat` / `ChatStream` / `Vision` 统一走 `endpointURL()`，删除 3 处硬编码
- gemini 特殊：`{model}` 内嵌路径，URL 识别以 `:generateContent` 结尾为准

### 5.4 参数映射矩阵

`buildChatCompletions` 现有采样字段保留，新增可选字段（§5.1）仅在配置非零时携带；各协议映射见 §5.2 表。`req.MaxTokens`（请求级）优先于 `cfg.MaxTokens`（配置级），再回落协议默认（anthropic 4096 / gemini 8192 / 其他 0=不携带）。

### 5.5 thinking / reasoning 适配矩阵

**chat_completions**（MintWord `apply_thinking_chat` + eino-ext 规格，按 provider 分组）：

| provider 分组 | thinking_effort != off 时携带 | off 时 |
|---|---|---|
| deepseek | `thinking={type:"enabled"}`；V4 系列另支持顶层 `reasoning_effort` | `thinking={type:"disabled"}` |
| kimi | K2.x：`thinking={type:"enabled"}` + `reasoning_effort`；**K3：顶层 `reasoning_effort`**（low/high/max，默认 max，始终思考） | K2.x：`thinking={type:"disabled"}`；K3：省略 reasoning_effort |
| zhipu / zai | `thinking={type:"enabled"}`；**GLM-5.2 增加顶层 `reasoning_effort`**（如 max） | `thinking={type:"disabled"}` |
| alibaba / siliconflow | `enable_thinking=true` | `enable_thinking=false` |
| stepfun | `reasoning_effort` | — |
| minimax | `reasoning_split=true` | — |
| tencent | 无 | — |
| 默认（openai 兼容） | `reasoning_effort`（GPT-5.6 支持 `none|low|medium|high|xhigh|max`） | — |

provider 分组判定：新增 `ProviderConfig.ProviderKey`（可选，默认取 `Name` 小写匹配 deepseek/kimi/zhipu/alibaba/siliconflow/stepfun/minimax/tencent 关键词）；不配置则回退"默认"分支。**替换**现有 `EnableThinking` 双字段全开逻辑（`EnableThinking` 保留为旧字段兼容，映射为 effort=medium）。

**anthropic_messages**（MintWord + eino-ext `OfAdaptive` + 官方文档）：

- `thinking_effort=off` → 不带 thinking
- **adaptive 判定扩展**（官方文档，2026-08）：模型名含 `opus-5|sonnet-5|fable-5|4-7|4-8|m3|M3`（或配置 `ThinkingPattern=adaptive`）→ `thinking={type:"adaptive"}`；**Opus 5 / Sonnet 5 / Fable 5 不支持 extended thinking**（`type:"enabled"` 会被拒绝，只能用 adaptive）
- 否则 → `thinking={type:"enabled", budget_tokens: cfg.ThinkingBudget|8000}`（Haiku 4.5、Opus 4.6 及更早、国产端点）
- 国产 Anthropic 端点（§5.2 矩阵）同样接收上述 thinking 结构；MiniMax M3 为 adaptive

**gemini_native**（eino-ext `ThinkingConfig`）：

- `gemini-3*`（含 3.5-flash / 3.6-flash / 3.5-flash-lite）→ `thinkingConfig={includeThoughts:true, thinkingLevel:effort.UPPER}`
- 否则 → `thinkingConfig={includeThoughts:true, thinkingBudget: cfg.ThinkingBudget|8192}`
- off → `thinkingConfig={includeThoughts:false}`

**openai_responses**（eino-ext openai `ReasoningEffort` + GPT-5.6）：

- `reasoning: {effort: <none|low|medium|high|xhigh|max>, summary: "auto"}`（GPT-5.6 支持 max，默认 medium）
- 可选扩展：`mode: "pro"`（GPT-5.6 pro mode，独立于 effort）、`context: "all_turns"|"current_turn"`（persisted reasoning，GPT-5.6 默认 all_turns）
- off → 不带 reasoning

**模型级适配要点**（2026-08 官方文档）：

- GPT-5.6（sol/terra/luna）：chat_completions 走 `reasoning_effort`；**官方推荐 Responses API**（reasoning/工具/多轮）
- Kimi K3：`reasoning_effort` 是**请求顶层字段**（非 thinking 对象内），流式返回 `reasoning_content` 增量
- DeepSeek V4：默认思考模式，可切非思考；Responses 仅 flash
- opencode-go：**协议由模型决定**（gpt-5.6-luna→responses、MiniMax/Qwen 全系→anthropic、其余→chat_completions），配置预设时按模型映射 api_mode

### 5.6 多模态适配

**接口扩展**（最小侵入）：`Vision` 增加 MIME 参数（旧签名保留，走默认 jpeg）：

```go
type VisionInput struct {
    Data    []byte
    MIMEType string   // image/jpeg 等
    Prompt  string
}
```

**格式转换**（按 api_mode）：

| 协议 | 图片 part |
|---|---|
| chat_completions | `{type:"image_url", image_url:{url:"data:<mime>;base64,<b64>"}}`（现状，补 MIME） |
| anthropic_messages | `{type:"image", source:{type:"base64", media_type:"<mime>", data:"<b64>"}}` |
| openai_responses | `input: [{role:"user", content:[{type:"input_image", image_url:"data:<mime>;base64,<b64>", detail:"auto"}]}]` |
| gemini_native | `contents[].parts[]: [{text: prompt}, {inlineData:{mimeType:"<mime>", data:"<b64>"}}]` |

（参考 eino-ext gemini/openrouter 的 `UserInputMultiContent{Base64Data, MIMEType}` 中间表示。）

**多模态扩展范围**（2026-08 官方文档）：

- **Gemini 3.5 Flash 系**：输入支持 Text/Image/Video/Audio/PDF 五模态 —— `inlineData` part 按 MIME 扩展（`application/pdf` 等），文本/图片现有实现可直接承载
- **Kimi K3**：OpenAI 兼容 `image_url`（base64 data URL）或 `video_url`（`ms://{file_id}`，需先 `files.create` 上传）—— video_url 为 Kimi 扩展 part 类型，chat_completions 构造器需放行未知 part 类型透传
- **多模态文件引用**（可选阶段）：Anthropic/OpenAI 文件引用需先上传获取 file_id；本方案最小实现保持 base64 内联，后续如需文件引用再按各厂商上传接口单独扩展

### 5.7 模型能力元数据（可选阶段，MintWord `aiProviders.ts` 模式）

Go 侧预设表 `internal/agent/provider/models_catalog.go`：

```go
type ModelCapability struct {
    ContextWindow   int      // token
    MaxOutput       int      // token
    ThinkingSupport bool
    ThinkingPattern string   // budget_tokens | adaptive | builtin | thinking_config
    SupportedParams []string // temperature, top_p, top_k, frequency_penalty, ...
}
var modelCatalog = map[string]ModelCapability{ /* deepseek-v3, qwen3, gemini-3*, claude-opus-4-7, gpt-5.x ... */ }
```

用途：`max_tokens` 配置默认值校验、前端表单能力提示（当前模型支持哪些参数）、后续上下文截断策略的数据基础。阶段 5 可选，不阻塞主链路。

**能力表数据源**：
- 新模型规格（contextWindow / maxOutput / thinking 能力）已**按厂商归并**：GPT-5.6 → §11.3 OpenAI 系、Claude 5 → Anthropic、Gemini 3.6/3.5 → Gemini、Grok 4.5 / DeepSeek V4 / Kimi K3 → Grok/DeepSeek/Kimi、Qwen3.8/3.7 → 阿里、GLM-5.2 → 智谱，连同 §11.3 MintWord 全量模型明细作为 `modelCatalog` 初始数据
- opencode-go（三协议混合）→ **§11.2 厂商预设总表 #14 + §11.3 opencode-go 小节**（官方支持 18 个模型×协议映射，2026-08-06）：端点 `https://opencode.ai/zen/go/v1`、订阅 API Key（Bearer）、模型 ID 格式 `opencode-go/<model-id>`、**列表动态（`/v1/models` 含已下线残留，应以官方列表过滤）**、用量 5h $12 / 周 $30 / 月 $60

**四类自定义预设**（前端模板，与 `api_mode`/`URLMode` 组合）：

| 预设 | apiMode | URLMode | URL 行为 |
|---|---|---|---|
| OpenAI 兼容自定义 | chat_completions | auto | base + `/chat/completions`（或完整 URL 识别） |
| OpenAI Responses 自定义 | openai_responses | auto | base + `/responses` |
| Anthropic 自定义 | anthropic_messages | auto | base + `/messages`（AuthHeader 可配） |
| **完全自定义** | 用户指定（默认 chat_completions） | **exact** | **必须自写 v1 后完整后缀，原样发送不追加** |

**与国产厂商预设的关系**（§5.2 硬编码协议能力表）：国产厂商 = 预设驱动（厂商 → 协议下拉 → 自动填 base URL/认证头/api_mode，`url_editable=true` 可覆盖）；四类自定义 = 完全手动（不依赖预设，适合网关/自建端点）。二者共用同一套 `api_mode` + `auth_header` + `url_mode` 字段，后端无差别处理。

---

## 6. 实施步骤

### Phase 1：URL 收敛 + api_mode 字段（解决 Issue #22，最小闭环）

| 文件 | 改动 |
|---|---|
| `internal/core/models/provider.go` | +`APIMode string` 列（GORM AutoMigrate） |
| `internal/agent/provider/root.go` | +`APIMode` 常量与配置字段；`APIMode()` 归一化（空→chat_completions） |
| `internal/agent/provider/provider.go` | +`endpointURL()`；Chat/ChatStream/Vision/doRequest 改用；删除 3 处硬编码与 ChatStream 重复拼接 |
| `internal/api/dto/request.go` / `response.go` / `transfer.go` | 透传 `api_mode` |
| `internal/api/service/service.go` | 透传 `api_mode`（:203-395 各构造点） |
| `web/src/api/index.ts` / `web/src/views/ProvidersPage.vue` | 表单 + 类型 + 提示文案 |
| `internal/agent/provider/provider_test.go` | URL 规则单测（见 §7） |

验收：存量 provider（无 api_mode）行为不变；新增完整 URL 的 provider 请求打完整端点；`go test ./internal/agent/provider/` 通过。

### Phase 2：协议构造器 + 响应提取器（anthropic / gemini / responses）

| 文件 | 改动 |
|---|---|
| `internal/agent/provider/provider.go` | `buildAnthropic` / `buildResponses` / `buildGemini`；`parseAnthropic` / `parseResponses` / `parseGemini`；认证头分派（`setHeaders` 按模式） |
| 同上 | 流式：anthropic（content_block_delta）、gemini（SSE parts）、responses（output_text.delta）；不支持协议返回明确错误 |
| 同上 | `Vision` 走协议分派（§5.6 先行实现 image 三格式） |

验收：三种协议非流式 Chat 调用通过（可用 httptest mock 端点）；流式行为与 eino-ext 组件一致。

### Phase 3：thinking 矩阵 + 参数映射

| 文件 | 改动 |
|---|---|
| `internal/agent/provider/provider.go` | `applyThinking(apimode, providerKey, effort, budget)` 替换 `EnableThinking` 双字段逻辑 |
| `internal/agent/provider/root.go` | +`ThinkingEffort` / `ThinkingBudget` / `MaxTokens` / `TopP` / `TopK` / 惩罚字段 |
| DTO / service / 前端 | 透传新字段 |

验收：deepseek/zhipu/alibaba 等矩阵用例走对应字段（单测快照）；`EnableThinking=true` 旧配置映射为 effort=medium。

### Phase 4：多模态扩展

| 文件 | 改动 |
|---|---|
| `internal/agent/provider/provider.go` | `VisionInput{MIMEType}`；四协议图片 part 转换 |
| 调用方 | `internal/agent/`（Vision 调用点）传 MIME |

验收：同图在四种协议下的请求体快照正确。

### Phase 5（可选）：模型能力元数据

| 文件 | 改动 |
|---|---|
| `internal/agent/provider/models_catalog.go` | 预设能力表 |
| 前端 | 按能力表显示参数/思考档位 |

---

## 7. 测试计划

`internal/agent/provider/provider_test.go`（新增，覆盖所有新契约）：

1. **URL 规则**：base URL 追加 / 完整 URL 识别 / gemini 模型路径 / 尾斜杠处理 / 大小写后缀
2. **协议请求体快照**：四种协议 ×（纯文本 / 带工具 / 带 thinking / 带多模态）JSON 快照断言
3. **响应解析**：四种协议响应样例 → `ChatResponse`（含 reasoning 提取）
4. **thinking 矩阵**：各厂商分组字段断言（off 与开两级）
5. **参数映射**：`req.MaxTokens` > `cfg.MaxTokens` > 协议默认 的优先级
6. **兼容性**：`api_mode` 空 → chat_completions 行为与旧实现一致
7. **流式**：anthropic / gemini SSE 解析（`bufio.Scanner` 逐行），断流与 `[DONE]` 等价处理

现有 `segment_test.go`、`dao_test.go` 保持通过。

---

## 8. 兼容性与迁移

| 项 | 策略 |
|---|---|
| 存量 provider 记录 | `api_mode` 列默认空 → 运行时按 chat_completions，**零迁移** |
| `EnableThinking` 旧字段 | 保留解析，映射为 `ThinkingEffort=medium`；新配置优先读 `ThinkingEffort` |
| `Endpoint` 存量值 | 均为 base URL，追加逻辑不变 |
| `Provider` 接口 / `EinoModelAdapter` / `agent.go` 调用方 | 零改动（协议差异收敛在 provider.go 内部） |
| 前端旧表单 | `api_mode` 可空提交，后端归一化 |

---

## 9. 风险与权衡

| 风险 | 缓解 |
|---|---|
| gemini/responses 流式格式差异大，工作量大 | Phase 2 拆分交付；非核心协议流式可先返回"暂不支持流式"错误 |
| anthropic `max_tokens` 必填，缺失报错 | 协议默认 4096，配置可覆盖 |
| responses 协议消息映射复杂（无 messages 数组） | 参考 eino-ext openai responses 组件语义；Phase 2 单独验收 |
| thinking 矩阵字段随厂商演进漂移 | 矩阵集中在一处 `applyThinking`，字段表化（驱动数据而非 if-else 堆叠） |
| 与 eino-ext 组件路径并行维护 | 本方案为渐进自研；如未来切换 eino-ext，ProviderConfig 字段（APIMode/ThinkingEffort）可直接映射其组件配置，不浪费 |

---

## 10. 附录：eino-ext 组件配置参考表（context7 实测）

| 组件 | BaseURL 配置 | thinking/reasoning | 多模态 | 备注 |
|---|---|---|---|---|
| `openai` | `ChatModelConfig.BaseURL`（可 Azure `ByAzure`+`APIVersion`） | `ReasoningEffort`（low/medium/high） | 图片 image_url | `MaxCompletionTokens` |
| `claude` | `Config.BaseURL *string`（可 Bedrock/Vertex） | `WithThinkingConfig`：`OfAdaptive` / `OfEnabled{budget_tokens}`；`GetThinking` 提取 | — | `MaxTokens` **必填** |
| `gemini` | `genai.ClientConfig.HTTPOptions.BaseURL` | `ThinkingConfig{IncludeThoughts, ThinkingBudget}`；`ReasoningContent` | `UserInputMultiContent{Base64Data, MIMEType}` | |
| `ark` | — | 有 | generate_with_image（base64） | 火山方舟 |
| `deepseek` | `ChatModelConfig.BaseURL`（`/beta`） | `GetReasoningContent` | — | |
| `qwen` | `ChatModelConfig.BaseURL`（compatible-mode/v1） | — | — | `MaxTokens/Temperature/TopP` |
| `openrouter` | `Config.BaseURL` | `Reasoning{Effort}` | 图片 | |
| `ollama` / `qianfan` / `zhipu` | BaseURL | — | — | 社区/官方组件 |

---

## 11. 附录：MintWord 全部厂商与模型预设清单

> 来源：`~/MintWord/src/lib/aiProviders.ts`（1463 行，`PROVIDERS: AiProvider[]`），逐条提取，与源码一致。
> 简写：`D`=default 参数（maxTokens, temperature, topP）；`F`=full 参数（+topK, frequencyPenalty, presencePenalty, repetitionPenalty）；`T`=thinking 参数（F+thinkingBudget）。

### 11.1 数据结构定义

```ts
type ApiMode = 'chat_completions' | 'anthropic_messages' | 'openai_responses' | 'gemini_native';

type ThinkingPattern =
  | 'reasoning_effort'    // openai 兼容: reasoning_effort 字段
  | 'thinking_object'     // deepseek/kimi/zhipu/glm: thinking={type:enabled} 对象
  | 'enable_thinking'     // 通义/硅基: enable_thinking 布尔
  | 'thinking_config'     // gemini-3/stepfun: thinkingConfig / thinking_level
  | 'budget_tokens'       // anthropic 旧代/gemini 2.5: budget_tokens
  | 'adaptive'            // anthropic 新代(4-7/4-8)/minimax-M3: thinking adaptive
  | 'builtin';            // o系列/grok/mistral-magistral/perplexity: 模型内置思考，无参数

type SupportedParam = 'maxTokens' | 'temperature' | 'topP' | 'topK'
  | 'frequencyPenalty' | 'presencePenalty' | 'repetitionPenalty' | 'thinkingBudget';

interface AiModel {
  id: string; displayName: string;
  contextWindow?: number; maxOutput?: number;
  thinkingSupport: boolean; thinkingPattern?: ThinkingPattern;
  thinkingLevels?: string[]; supportedParams?: SupportedParam[];
  subOptionFilter?: Record<string, string>;   // 子选项过滤（如 planType: 'coding|agent'）
}

interface AiProvider {
  id: string; iconKey: string;
  baseUrl: string;                    // 预设 base URL（含 /v1 等前缀，不含协议后缀）
  apiMode: ApiMode; urlEditable: boolean;
  categories: ('api'|'mainland'|'aggregator'|'coding-plan'|'custom')[];
  protocols?: ('openai'|'anthropic')[];
  models: AiModel[];
  subOptions?: SubOption[];           // 影响 URL / 模型列表的子选项
  urlMap?: Record<string, string>;    // 子选项组合 → 实际 baseUrl
  notesKey?: string;
}
```

### 11.2 厂商预设总表（49 条目）

| # | provider id | baseUrl | apiMode | urlEditable | categories | 子选项/urlMap | 模型 |
|---|---|---|---|---|---|---|---|
| 1 | `openai` | https://api.openai.com/v1 | chat_completions | ✗ | api | — | 6 |
| 2 | `openai-responses` | https://api.openai.com/v1 | openai_responses | ✗ | api | — | 6 |
| 3 | `openai-compatible` | （用户填） | chat_completions | ✓ | api, custom | — | 0（自定义） |
| 4 | `chatgpt-plus` | https://chatgpt.com/backend-api/codex | openai_responses | ✗ | coding-plan | loginMethod: url/device/api | 0 |
| 5 | `anthropic` | https://api.anthropic.com | anthropic_messages | ✗ | api | — | 9 |
| 6 | `anthropic-compatible` | （用户填） | anthropic_messages | ✓ | api, custom | — | 0 |
| 7 | `fully-custom` | （用户填） | chat_completions | ✓ | custom | apiMode: openai/anthropic | 0 |
| 8 | `gemini` | https://generativelanguage.googleapis.com/v1beta | **chat_completions**（非 native） | ✗ | api | — | 6 |
| 9 | `vertexai-google` | https://{region}-aiplatform.googleapis.com/v1/projects/{project}/locations/{region}/endpoints/openapi | chat_completions | ✗ | aggregator | — | 6 |
| 10 | `vertexai-anthropic` | 同上 | anthropic_messages | ✗ | aggregator | — | 9 |
| 11 | `grok` | https://api.x.ai/v1 | chat_completions | ✗ | api | — | 2 |
| 12 | `github-copilot` | https://api.githubcopilot.com | chat_completions | ✗ | aggregator | protocols: [openai] | 15 |
| 13 | `opencode-zen` | https://opencode.ai/zen/v1 | chat_completions | ✗ | aggregator | 动态模型 | 0 |
| 14 | `opencode-go` | https://opencode.ai/zen/go/v1 | **多协议（按模型）** | ✗ | aggregator, coding-plan | 动态模型（官方列表 18，`/v1/models` 轮询） | 18（官方） |
| 15 | `deepseek` | https://api.deepseek.com | chat_completions | ✗ | api | — | 2 |
| 16 | `kimi` | https://api.moonshot.cn/v1 | chat_completions | ✗ | mainland, api | — | 2 |
| 17 | `kimi-intl` | https://api.moonshot.ai/v1 | chat_completions | ✗ | api | — | 2 |
| 18 | `kimi-code` | https://api.kimi.com/coding/v1 | chat_completions | ✗ | mainland, coding-plan | — | 1 |
| 19 | `mistral` | https://api.mistral.ai/v1 | chat_completions | ✗ | api | — | 8 |
| 20 | `perplexity` | https://api.perplexity.ai | chat_completions | ✗ | api | — | 5 |
| 21 | `huggingface` | https://api-inference.huggingface.co/v1 | chat_completions | ✗ | aggregator | 动态模型 | 0 |
| 22 | `azure` | https://{resource}.openai.azure.com | chat_completions | ✓ | aggregator | — | 0 |
| 23 | `openrouter` | https://openrouter.ai/api/v1 | chat_completions | ✗ | aggregator | 动态模型 | 0 |
| 24 | `lmstudio` | http://localhost:1234/v1 | chat_completions | ✓ | aggregator | 本地模型 | 0 |
| 25 | `modelscope` | https://api-inference.modelscope.cn/v1 | chat_completions | ✗ | aggregator | 动态模型 | 0 |
| 26 | `poe` | https://api.poe.com/v1 | chat_completions | ✗ | aggregator | 动态模型 | 0 |
| 27 | `nvidia` | https://integrate.api.nvidia.com/v1 | chat_completions | ✗ | aggregator | 动态模型 | 0 |
| 28 | `ollama` | http://localhost:11434/v1 | chat_completions | ✓ | aggregator | 本地模型 | 0 |
| 29 | `cloudflare` | https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1 | chat_completions | ✓ | aggregator | — | 9 |
| 30 | `bedrock` | https://bedrock-runtime.{region}.amazonaws.com | chat_completions | ✓ | aggregator | — | 7 |
| 31 | `alibaba` | urlMap 3 档 | chat_completions | ✗ | mainland, api, coding-plan | planType: api/token/coding | 11 |
| 32 | `alibaba-intl` | urlMap region×planType | chat_completions | ✗ | api, coding-plan | region: sg/us; planType: api/token/coding | 11 |
| 33 | `minimax-cn` | https://api.minimaxi.com/v1 | chat_completions | ✗ | mainland, coding-plan | planType: api/token | 8 |
| 34 | `minimax` | https://api.minimax.io/v1 | chat_completions | ✗ | api, coding-plan | planType: api/token | 8 |
| 35 | `xiaomi` | https://api.xiaomimimo.com/v1 | chat_completions | ✗ | mainland | — | 2 |
| 36 | `xiaomi-token-plan` | urlMap cn/sgp/ams | chat_completions | ✗ | mainland, coding-plan | region: cn/sgp/ams | 2 |
| 37 | `zhipu` | https://open.bigmodel.cn/api/paas/v4 | chat_completions | ✗ | mainland | — | 6 |
| 38 | `zhipu-coding` | https://open.bigmodel.cn/api/coding/paas/v4 | chat_completions | ✗ | mainland, coding-plan | — | 6 |
| 39 | `zai` | https://api.z.ai/api/paas/v4 | chat_completions | ✗ | api | — | 6 |
| 40 | `zai-coding` | https://api.z.ai/api/coding/paas/v4 | chat_completions | ✗ | coding-plan | — | 6 |
| 41 | `stepfun` | urlMap cn/intl × api/step-plan | chat_completions | ✗ | mainland, api | region; planType | 4 |
| 42 | `tencent-hunyuan` | https://api.hunyuan.cloud.tencent.com/v1 | chat_completions | ✗ | mainland, api | modelType: chat/translate | 5 |
| 43 | `tencent-token-plan` | urlMap cn/intl-sg/intl-gz | chat_completions | ✗ | aggregator, coding-plan | region | 11 |
| 44 | `volcengine` | urlMap api/coding/agent | chat_completions | ✗ | mainland, aggregator, coding-plan | planType | 9 |
| 45 | `volcengine-intl` | urlMap api/coding | chat_completions | ✗ | aggregator, coding-plan | planType | 8 |
| 46 | `siliconflow-cn` | https://api.siliconflow.cn/v1 | chat_completions | ✗ | mainland, aggregator | 动态模型 | 0 |
| 47 | `siliconflow-intl` | https://api.siliconflow.com/v1 | chat_completions | ✗ | aggregator | 动态模型 | 0 |
| 48 | `baidu` | urlMap api/coding | chat_completions | ✗ | mainland, coding-plan | planType | 8 |
| 49 | `baidu-intl` | urlMap api/coding | chat_completions | ✗ | api, coding-plan | planType | 5 |

要点：
- **协议分布**：46 条目单一 chat_completions；2 条目 anthropic_messages（anthropic、vertexai-anthropic）；2 条目 openai_responses（openai-responses、chatgpt-plus）；**1 条目三协议混合（opencode-go，协议由模型决定**：chat_completions 多数 / responses 仅 gpt-5.6-luna / messages MiniMax+Qwen 全系，见 §11.3）；**无 gemini_native 条目**（Gemini 走 OpenAI 兼容端点）
- **URL 策略**：官方直连 → `urlEditable=false` 锁 URL；兼容/本地/网关 → `urlEditable=true`；多环境厂商（阿里/腾讯/火山/小米/阶跃/百度）→ `urlMap` 按子选项组合选 baseUrl
- **认证差异**：chat_completions 全部 `Bearer`；anthropic_messages 用 `x-api-key`（MintWord 实现，见 §3.1；国产厂商 anthropic 端点认证头差异见 §5.2 预设表）
- **opencode-go 接入**：端点 `https://opencode.ai/zen/go/v1`，订阅 API Key（`Authorization: Bearer`），模型 ID 格式 `opencode-go/<model-id>`，动态列表权威源 `https://opencode.ai/zen/go/v1/models`（应轮询而非硬编码），用量 5h $12 / 周 $30 / 月 $60

### 11.3 模型明细（含全部配置）

#### OpenAI 系（openai / openai-responses / github-copilot 部分，contextWindow 均为 1M、无 maxOutput、无 thinking、D 参数）

| 模型 id | ctx | 备注 |
|---|---|---|
| gpt-5.5 / gpt-5.4 / gpt-5.4-mini / gpt-5.4-nano / gpt-5.3-codex / gpt-5.2 | 1M | openai + openai-responses 双条目 |
| **gpt-5.6-sol** | **1.05M** | **128k** | 旗舰（`gpt-5.6` 别名）；reasoning.effort none~max + `reasoning.mode:"pro"`、`reasoning.context`；官方推荐 Responses API（models.dev 2026-07-09） |
| **gpt-5.6-terra** | **1.05M** | **128k** | 性价比档；同上参数面（models.dev 2026-07-09） |
| **gpt-5.6-luna** | **1.05M** | **128k** | 高吞吐档；同上参数面；opencode-go 网关 responses 协议默认模型（models.dev 2026-07-09） |
| github-copilot 追加：claude-opus-4-8（200k, adaptive）、claude-opus-4-7（1M, adaptive）、claude-opus-4-6（200k, budget_tokens）、claude-sonnet-4-6 / 4-5（200k, budget_tokens）、claude-haiku-4-5（200k, 无 thinking）、gemini-2.5-pro（1M, budget_tokens）、o3 / o4-mini（200k, builtin） | — | 聚合 |

#### Anthropic（anthropic / vertexai-anthropic 相同，T 参数，thinkingLevels: low/medium/high/xhigh/max）

| 模型 id | ctx | maxOut | thinkingPattern |
|---|---|---|---|
| claude-opus-4-7 | 1M | 64k | adaptive |
| claude-opus-4-6 / 4-5 | 200k | 64k | budget_tokens |
| claude-sonnet-4-6 / 4-5 | 200k | 64k | budget_tokens |
| claude-haiku-4-5 | 200k | 8k | 无（D） |
| claude-opus-4-1 | 200k | 64k | budget_tokens |
| claude-opus-4 | 200k | 32k | budget_tokens |
| claude-sonnet-4 | 200k | 16k | budget_tokens |
| **claude-opus-5** | **1M** | **128k** | **adaptive**（$5/$25；不支持 extended thinking） |
| **claude-sonnet-5** | **1M** | **128k** | **adaptive**（$3/$15，intro $2/$10 至 2026-08-31；不支持 extended thinking） |
| **claude-fable-5** | **1M** | **128k** | **adaptive**（$10/$50，Mythos 架构类，高于 Opus 档；仅 adaptive thinking，budget_tokens 返回 400，禁 temperature/top_p/top_k 与 assistant 前缀，effort low~xhigh/max；2026-06-09 发布；Mythos 5 仅限 Glasswing 合作伙伴，不走通用 API） |

#### Gemini（gemini / vertexai-google 相同；apiMode=chat_completions，T 参数）

| 模型 id | ctx | thinkingPattern | levels |
|---|---|---|---|
| gemini-3.1-pro-preview / gemini-3.1-flash-preview / gemini-3-flash-preview | 1M | thinking_config | low/medium/high |
| gemini-2.5-pro / gemini-2.5-flash / gemini-2.5-flash-lite | 1M | budget_tokens | — |
| **gemini-3.6-flash** | **1M** | **65k 输出**；$1.50/$7.00；thinking_config（models.dev 2026-07-21） |
| **gemini-3.5-flash-lite** | **1M** | **65k 输出**；$0.30/$2.50（models.dev 2026-07-21） |
| gemini-3.5-flash（官方文档） | 1M | 65k 输出；输入 Text/Image/Video/Audio/PDF 五模态 |

#### Grok / DeepSeek / Kimi

| 厂商 | 模型 id | ctx | thinkingPattern | levels |
|---|---|---|---|---|
| grok | grok-4 | 131k | 无 | — |
| grok | grok-420-reasoning | 131k | builtin | — |
| grok | **grok-4.5** | **500k** | **builtin**（$2/$6，≥200k prompt 翻倍；models.dev 2026-07-08） |
| deepseek | deepseek-v4-pro / deepseek-v4-flash | 1M | thinking_object | low/medium/high/max |
| deepseek | **deepseek-v4-flash-0731** | **1M** | **384k 输出**；thinking_object；开源（models.dev 2026-07-31） |
| kimi / kimi-intl | kimi-k2.6 / kimi-k2.5 | 256k | thinking_object | low/medium/high |
| kimi / kimi-intl | **kimi-k3** | **1M** | **131k 输出**；**顶层 reasoning_effort**（low/high/max，默认 max）；视觉 image_url/video_url（models.dev 2026-07-16） |
| kimi / kimi-intl | **kimi-k2.7-code** | **262k** | **262k 输出**；thinking_object（models.dev 2026-06-12） |
| kimi-code | kimi-for-coding | 256k | thinking_object | low/medium/high |

#### Mistral（F 参数为主）

| 模型 id | ctx | thinking |
|---|---|---|
| mistral-large-latest / mistral-medium-latest / mistral-small-latest | 128k | 无（F） |
| magistral-medium-latest / magistral-small-latest | 128k | builtin（D） |
| devstral-small-latest | 128k | 无（F） |
| codestral-latest | 256k | 无（F） |
| pixtral-large-latest | 128k | 无（F，多模态） |

#### Perplexity（D 参数）

| 模型 id | ctx | thinking |
|---|---|---|
| sonar | 127k | 无 |
| sonar-pro | 200k | 无 |
| sonar-reasoning | 127k | builtin |
| sonar-reasoning-pro | 200k | builtin |
| sonar-deep-research | 200k | builtin |

#### Cloudflare（全部 128k、D 参数）

glm-4.7-flash / gpt-oss-120b / gpt-oss-20b / llama-4-scout-17b / gemma-4-26b / nemotron-3-120b / qwen3-30b / llama-3.3-70b（无 thinking）；deepseek-r1-distill-qwen-32b（builtin）

#### Bedrock（D 参数为主）

| 模型 id | ctx | thinking |
|---|---|---|
| anthropic.claude-opus-4-7 | 1M | adaptive（T） |
| anthropic.claude-sonnet-4-6 | 200k | budget_tokens（T） |
| anthropic.claude-haiku-4-5 | 200k | 无 |
| deepseek.v3.2 / meta.llama3-3-70b / meta.llama4-scout-17b | 128k | 无 |
| mistral.mistral-large-3 | 128k | 无（F） |

#### 阿里（alibaba / alibaba-intl 相同，T 参数为主）

| 模型 id | ctx | maxOut | thinkingPattern |
|---|---|---|---|
| qwen3.7-max（=`qwen3.7-max-2026-05-20`，另有 06-08 快照） | 1M | 64k | enable_thinking（low/med/high） |
| **qwen3.7-plus**（=`qwen3.7-plus-2026-05-26`） | **1M** | **64k** | enable_thinking；原生视觉-语言多模态智能体（屏幕读取/GUI 操作）（阿里云 2026-06-01 上线） |
| **qwen3.7-flash**（=`qwen3.7-flash-2026-07-15`） | **1M** | **65k** | enable_thinking；$0.03/$0.12（阿里云 2026-07-21 上线） |
| qwen3.6-plus / qwen3.6-flash | 1M | 64k | enable_thinking（low/med/high） |
| deepseek-v4-pro / deepseek-v4-flash | 1M | 384k | thinking_object（low/med/high/max） |
| deepseek-v3.2 | 128k | 32k | thinking_object |
| kimi-k2.6 / kimi-k2.5 | 262k | 98k | thinking_object |
| glm-5.1 | 202k | 131k | enable_thinking |
| glm-5 | 202k | 16k | enable_thinking |
| MiniMax-M2.5 | 196k | 32k | 无（D） |
| **qwen3.8-max**（=`qwen3.8-max-preview` 正式版） | **1M** | **131k** | enable_thinking；2.4T 参数 MoE 旗舰，阿里云 2026-08-03 全球上线（当前 3.8 系列仅 max，3.7 系即最新全系） |

#### MiniMax（minimax / minimax-cn 相同）

| 模型 id | ctx | thinking |
|---|---|---|
| MiniMax-M3 | 1M | adaptive（T） |
| MiniMax-M2.7 / M2.7-highspeed / M2.5 / M2.5-highspeed / M2.1 / M2 / M2-her | 256k | 无（F） |

#### 小米（xiaomi / xiaomi-token-plan 相同）

| 模型 id | ctx | maxOut | thinking |
|---|---|---|---|
| mimo-v2.5-pro / mimo-v2.5 | 1M | 128k | builtin（D） |

#### 智谱（zhipu / zhipu-coding / zai / zai-coding 相同，全部 128k、T 参数、thinking_object、levels low/med/high）

glm-5.1 / glm-5 / glm-5-turbo / glm-4.7 / glm-4.7-flash / glm-4.7-flashx
glm-5.2（**1M ctx / 131k 输出**，thinking_object + 顶层 reasoning_effort，开源；models.dev 2026-06-13）

#### 阶跃（stepfun）

| 模型 id | ctx | thinking |
|---|---|---|
| step-3.7-flash / step-3.5-flash | 256k | thinking_config（low/med/high，T） |
| step-3.5-flash-2603 / step-router | 128k | 无（D） |

#### 腾讯混元（tencent-hunyuan，D 参数）

| 模型 id | ctx | thinking | subOptionFilter |
|---|---|---|---|
| hunyuan-turbos-20250416 / hunyuan-large-20250226 | 32k | 无 | chat |
| hunyuan-t1-20250416 | 32k | builtin（T） | chat |
| hunyuan-mt-20250226 / hunyuan-mt-20250107 | 8k | 无 | translate |

#### 腾讯 Token Plan（tencent-token-plan，D 参数为主）

| 模型 id | ctx | maxOut | thinking |
|---|---|---|---|
| tc-code-latest | — | — | 无（Auto） |
| minimax-m2.5 / minimax-m2.7 | 200k | 128k | 无 |
| glm-5 / glm-5.1 | 200k | 128k | 无 |
| kimi-k2.5 | 256k | 256k | 无 |
| hunyuan-2.0-instruct | 144k | 16k | 无 |
| hunyuan-2.0-thinking | 192k | 64k | builtin（T） |
| hunyuan-t1 | 96k | 64k | builtin（T） |
| hunyuan-turbos / hunyuan-turbo | 48k | 16k | 无 |

#### 火山方舟（volcengine，按计费模式三档模型；官方文档 2026-08-04 更新）

**按量计费（API，`ark.cn-beijing.volces.com/api/v3`）** — 深度思考/文本生成/多模态理解模型：

| 模型 id | ctx | maxOut | 备注 |
|---|---|---|---|
| doubao-seed-2-1-pro-260628 | 256k | 256k | 豆包旗舰 Agent 通用模型 |
| doubao-seed-2-1-turbo-260628 | 256k | 256k | 编程/智能体/多模态 |
| doubao-seed-2-0-lite-260428 | 256k | 128k | 全模态理解（往期模型，未下线） |
| doubao-seed-2-0-mini-260428 | 256k | 128k | 低时延（往期模型，未下线） |
| glm-5-2-260617 | 1024k | 128k | 智谱托管 |
| deepseek-v4-flash-ga-260731 | 1024k | 384k | Flash GA 版 |
| deepseek-v4-pro-260425 | 1024k | 384k | 深度思考 |
| deepseek-v4-flash-260425 | 1024k | 384k | 旧快照 |

**Coding Plan（Base URL `https://ark.cn-beijing.volces.com/api/coding/v3` OpenAI 协议 / `api/coding` Anthropic 协议）**：

| 模型 | ctx | maxOut | 备注 |
|---|---|---|---|
| Auto（ark-code-latest） | — | — | 智能调度，优先体验最新模型 |
| Doubao-Seed-2.1-turbo | 256k | 64k | 多模态视觉理解 |
| Doubao-Seed-2.0-lite | 256k | — | 多模态视觉理解 |
| MiniMax-M3 | 1024k | 128k | 编码+Agent 顶尖 |
| Kimi-K2.7-Code | 256k | 32k | 最新 Coding 模型，文本/图片/视频输入 |
| GLM-5.2 | 1024k | 128k | 1M 长上下文 |
| DeepSeek-V4-Flash | 1024k | 384k | 默认 thinking，可手动关闭；正式版仅 ark-code-latest 方式，预览版 model name `deepseek-v4-flash` |
| DeepSeek-V4-Pro | 1024k | 384k | 尝鲜体验版，默认 thinking |

**Agent Plan（AFP 积分抵扣，Small/Medium/Large/Max 四档）** — 仅文本生成模型（嵌入/生图/生视频/语音模型不列出）：

| 模型 | ctx | maxOut | 档位 |
|---|---|---|---|
| doubao-seed-2.0-mini | 256k | 128k | 全部 |
| doubao-seed-2.0-lite | 256k | 128k | 全部 |
| deepseek-v4-flash | 1024k | 384k | 全部 |
| doubao-seed-2.1-turbo | 256k | 256k | 全部 |
| doubao-seed-evolving | 1024k | 256k | 全部 |
| minimax-m3 | 1024k | 128k | 全部 |
| glm-5.2（glm-latest） | 1024k | 128k | 全部 |
| kimi-k2.7-code | 256k | 32k | 全部 |
| deepseek-v4-pro | 1024k | 384k | 全部（尝鲜） |
| kimi-k3 | 1024k | 128k | Medium 及以上（Small 不可用） |

> 注：均已剔除即将下线模型（doubao-seed-2.0-code、doubao-seed-2.0-pro、doubao-seed-code、minimax-m2.7、kimi-k2.6、doubao-seed-1.x、doubao-1.5 系列、glm-4-7 等）及非 LLM 能力（doubao-embedding-vision 向量化、seedream 生图、seedance 生视频、TTS/ASR、豆包搜索 Harness）。glm-5.2、deepseek-v4-flash/pro、kimi-k3 支持 1M 上下文长会话。
> Responses 支持（§5.2）：250615 及之后版本模型默认支持 `/api/v3/responses`，例外 `doubao-1-5-pro-32k-character-250715`；TPM 保障/精调/智能路由模型不支持。

#### 火山方舟国际（volcengine-intl，同步国内最新模型集；区域/端点以国际控制台为准）

国际版与国内版共享同一方舟模型矩阵（OpenClaw 等生态即按该端点接入），按 §11.3 国内版三档同步更新：

| 档位 | 模型 |
|---|---|
| 按量计费（/api/v3） | doubao-seed-2-1-pro-260628、doubao-seed-2-1-turbo-260628、doubao-seed-2-0-lite-260428、doubao-seed-2-0-mini-260428、glm-5-2-260617、deepseek-v4-flash-ga-260731、deepseek-v4-pro-260425、deepseek-v4-flash-260425 |
| Coding Plan（/api/coding） | Auto（ark-code-latest）、Doubao-Seed-2.1-turbo、Doubao-Seed-2.0-lite、MiniMax-M3、Kimi-K2.7-Code、GLM-5.2、DeepSeek-V4-Flash/Pro |
| Agent Plan | 同国内版 10 个文本生成模型（kimi-k3 仅 Medium+） |

> 剔除规则同国内版：seed 2.0 之前、即将下线（doubao-seed-2.0-code/2.0-pro、doubao-seed-code、minimax-m2.7、kimi-k2.6、glm-4-7、seed-1.x、doubao-1.5 系列）与 gpt-oss-120b 等旧模型均不列入。MintWord 旧表（glm-5.1 / glm-4.7 / kimi-k2.5 / gpt-oss-120b）已废弃。

#### 百度千帆（baidu）

| 模型 id | ctx | maxOut | thinking |
|---|---|---|---|
| ernie-5.0 | 128k | 65k | 无 |
| ernie-4.5-turbo-128k | 128k | 12k | 无 |
| deepseek-v4-pro / v4-flash | 1M | 131k | thinking_object（T） |
| glm-5.1 / glm-5 | 202k | 131k | thinking_object（T） |
| kimi-k2.5 | 262k | 65k | thinking_object（T） |
| minimax-m2.5 | 196k | 131k | 无 |

#### 百度千帆国际（baidu-intl）

ernie-5.0（128k/65k/无）、deepseek-v3.2（128k/32k/thinking_object）、glm-5（202k/131k/thinking_object）、kimi-k2.5（262k/65k/thinking_object）、minimax-m2.5（196k/131k/无）

#### opencode-go（官方支持 18 个模型，2026-08-06）

> 端点 `https://opencode.ai/zen/go/v1`，订阅 API Key（`Authorization: Bearer`），模型 ID 配置格式 `opencode-go/<model-id>`。**官方支持列表 18 个（2026-08-06 订阅文档）**；`/v1/models` 曾实测返回 25 个（含 7 个已下线残留：glm-5、kimi-k2.5、mimo-v2-omni、mimo-v2-pro、minimax-m2.5、qwen3.5-plus、hy3-preview），接入时应以官方列表过滤。协议由模型决定：

| 模型 id（`opencode-go/` 前缀） | 协议 | 端点路径 |
|---|---|---|
| gpt-5.6-luna | **openai_responses** | `/v1/responses` |
| minimax-m3 / minimax-m2.7 | **anthropic_messages** | `/v1/messages` |
| qwen3.8-max / qwen3.7-max / qwen3.7-plus / qwen3.6-plus | **anthropic_messages** | `/v1/messages` |
| grok-4.5 | chat_completions | `/v1/chat/completions` |
| glm-5.2 / glm-5.1 | chat_completions | `/v1/chat/completions` |
| kimi-k3 / kimi-k2.7-code / kimi-k2.6 | chat_completions | `/v1/chat/completions` |
| mimo-v2.5 / mimo-v2.5-pro | chat_completions | `/v1/chat/completions` |
| deepseek-v4-pro / deepseek-v4-flash | chat_completions | `/v1/chat/completions` |
| hy3 | chat_completions | `/v1/chat/completions` |

#### 动态模型厂商（models=[]，由服务端 /models 或用户手动指定）

openai-compatible、anthropic-compatible、fully-custom、chatgpt-plus、opencode-zen、opencode-go、huggingface、azure、openrouter、lmstudio、modelscope、poe、nvidia、ollama、siliconflow-cn、siliconflow-intl

### 11.4 对本项目方案的可复用结论

1. **模型能力表**（§5.7）应直接采用 MintWord 的 `AiModel` 结构（contextWindow / maxOutput / thinkingPattern / thinkingLevels / supportedParams），7 种 `ThinkingPattern` 枚举与 §5.5 适配矩阵一一对应
2. **厂商预设表**（§5.7 可选阶段）可按 `AiProvider` 结构落地：`baseUrl + apiMode + urlEditable + urlMap`，其中 `urlMap`（子选项组合 → baseUrl）是 MintWord 处理多环境/多套餐厂商的关键模式，本项目可扩展为 `sub_options` 配置
3. **urlEditable 语义**与 §5.3 URL 自动识别互补：`urlEditable=false` 时后端仍校验 URL 合法性，`urlEditable=true` 时允许完整端点
4. **认证差异**在 3 种头间切换（Bearer / x-api-key / api-key），本方案 §5.2 已覆盖（`AuthHeader` 可配）

