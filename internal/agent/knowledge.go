package agent

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/models"
)

// 知识库对话匹配注入条数上限
const knowledgeMatchLimit = 5

// ---------- LRU 缓存 ----------

// knowledgeLRU 知识库检索结果缓存（纯缓存，丢失可重建，不属持久化状态）。
type knowledgeLRU struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List
	items map[string]*list.Element
}

type knowledgeLRUEntry struct {
	key   string
	value []models.KnowledgeItem
}

func newKnowledgeLRU(capacity int) *knowledgeLRU {
	if capacity <= 0 {
		capacity = 50
	}
	return &knowledgeLRU{cap: capacity, ll: list.New(), items: make(map[string]*list.Element)}
}

func (c *knowledgeLRU) Get(key string) ([]models.KnowledgeItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*knowledgeLRUEntry).value, true
}

func (c *knowledgeLRU) Put(key string, val []models.KnowledgeItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*knowledgeLRUEntry).value = val
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&knowledgeLRUEntry{key: key, value: val})
	c.items[key] = el
	if c.ll.Len() > c.cap {
		last := c.ll.Back()
		if last != nil {
			c.ll.Remove(last)
			delete(c.items, last.Value.(*knowledgeLRUEntry).key)
		}
	}
}

func (c *knowledgeLRU) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element)
}

func (c *knowledgeLRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// ---------- 异步关键词提取 ----------

// ExtractKeywordsAsync 异步启动关键词提取（新增/编辑知识后调用，不阻塞消息处理）。
func (h *HagoCenter) ExtractKeywordsAsync(ctx context.Context, itemID string) {
	go func() {
		extractCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := h.extractKnowledgeKeywords(extractCtx, itemID); err != nil {
			log.Error("知识关键词提取失败", "id", itemID, "err", err)
		}
	}()
}

// extractKnowledgeKeywords 用 LLM 从知识内容提取 3~8 个关键词并写回 DB。
func (h *HagoCenter) extractKnowledgeKeywords(ctx context.Context, itemID string) error {
	item, err := h.DAO.Knowledge.GetByID(ctx, itemID)
	if err != nil {
		return err
	}
	llm := h.Providers.SelectModel(provider.ModelTypeText)
	if llm == nil {
		_ = h.DAO.Knowledge.SetKeywordStatus(ctx, itemID, models.KeywordStatusFailed)
		return fmt.Errorf("无可用 Text 模型")
	}

	prompt := fmt.Sprintf(`从以下知识内容中提取 3~8 个代表性关键词（优先领域术语/专有名词，过滤"的/了/是"等通用词）。直接输出 JSON 字符串数组，不要其他内容。

内容：
%s`, item.Content)

	resp, err := llm.Chat(ctx, provider.ChatRequest{
		Messages: []provider.ChatMessage{
			{Role: "system", Content: "你是关键词提取助手，只输出 JSON 数组。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
	})
	if err != nil {
		_ = h.DAO.Knowledge.SetKeywordStatus(ctx, itemID, models.KeywordStatusFailed)
		return err
	}

	keywords := parseKeywordsFromLLM(resp.Message.Content)
	cleaned := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw != "" && len([]rune(kw)) <= 20 {
			cleaned = append(cleaned, kw)
		}
	}
	status := models.KeywordStatusReady
	if len(cleaned) == 0 {
		status = models.KeywordStatusFailed
	}
	if err := h.DAO.Knowledge.SetKeywords(ctx, itemID, cleaned, status); err != nil {
		// 防御：写回失败也把状态标成 failed，避免前端一直显示“提取中”
		_ = h.DAO.Knowledge.SetKeywordStatus(ctx, itemID, models.KeywordStatusFailed)
		return err
	}
	log.Info("知识关键词提取完成", "id", itemID, "keywords", len(cleaned))
	return nil
}

// parseKeywordsFromLLM 解析 LLM 返回的关键词数组（含代码块围栏/格式容错）。
func parseKeywordsFromLLM(content string) []string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var keywords []string
	if err := json.Unmarshal([]byte(content), &keywords); err == nil {
		return keywords
	}
	// 容错：提取所有引号包裹的片段
	re := regexp.MustCompile(`"([^"]+)"`)
	matches := re.FindAllStringSubmatch(content, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// ---------- 对话前检索注入 ----------

// buildKnowledgeContext 对话前模糊匹配知识库，命中内容拼入系统提示词。
// LRU 命中直接返回；未命中走 DB 检索并回写缓存。
func (h *HagoCenter) buildKnowledgeContext(ctx context.Context, msg string) string {
	if h.DAO == nil || h.DAO.Knowledge == nil {
		return ""
	}
	query := preprocessKnowledgeQuery(msg)
	if query == "" {
		return ""
	}
	key := knowledgeQueryKey(query)
	if items, ok := h.knowledgeLRU.Get(key); ok {
		return formatKnowledgeContext(items)
	}
	items, err := h.DAO.Knowledge.Match(ctx, query, knowledgeMatchLimit)
	if err != nil {
		log.Warn("知识库检索失败", "err", err)
		return ""
	}
	h.knowledgeLRU.Put(key, items)
	return formatKnowledgeContext(items)
}

// InvalidateKnowledgeLRU 知识库条目变更后失效缓存。
func (h *HagoCenter) InvalidateKnowledgeLRU() {
	h.knowledgeLRU.Clear()
	log.Info("知识库 LRU 已失效")
}

// preprocessKnowledgeQuery 预处理检索消息：去 CQ 码/URL、压缩空白、截断至 200 字。
func preprocessKnowledgeQuery(msg string) string {
	msg = cqCodeRegexp.ReplaceAllString(msg, " ")
	msg = urlRegexp.ReplaceAllString(msg, " ")
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	runes := []rune(msg)
	if len(runes) > 200 {
		msg = string(runes[:200])
	}
	return msg
}

// knowledgeQueryKey 检索键（LRU key）：小写 + 去空白，避免同义查询不命中。
func knowledgeQueryKey(query string) string {
	return strings.TrimSpace(strings.ToLower(query))
}

// formatKnowledgeContext 组装注入提示词。
func formatKnowledgeContext(items []models.KnowledgeItem) string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("以下是相关知识库内容（供参考）：\n")
	for _, it := range items {
		if strings.TrimSpace(it.Content) == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(strings.TrimSpace(it.Content))
		sb.WriteString("\n")
	}
	if sb.Len() == 0 {
		return ""
	}
	return sb.String()
}

// searchKnowledgeForTool 供 Agent 内置工具 search_knowledge 使用：按关键词主动查询知识库，
// 返回标题 + 内容片段，让 Agent 在对话中按需检索（区别于对话前自动注入的 buildKnowledgeContext）。
func (h *HagoCenter) searchKnowledgeForTool(ctx context.Context, keyword string, limit int) (string, error) {
	if h.DAO == nil || h.DAO.Knowledge == nil {
		return "知识库未初始化", nil
	}
	keyword = strings.TrimSpace(preprocessKnowledgeQuery(keyword))
	if keyword == "" {
		return "请提供搜索关键词", nil
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	items, err := h.DAO.Knowledge.Match(ctx, keyword, limit)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return fmt.Sprintf("知识库中未找到与 %q 相关的内容", keyword), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "按关键词 %q 检索到 %d 条知识：\n", keyword, len(items))
	for _, it := range items {
		content := strings.TrimSpace(it.Content)
		// 截断过长的内容，避免占用过多 token
		runes := []rune(content)
		if len(runes) > 300 {
			content = string(runes[:300]) + "…"
		}
		title := strings.TrimSpace(it.Title)
		if title == "" {
			title = "(无标题)"
		}
		if content == "" {
			continue
		}
		sb.WriteString("- 【")
		sb.WriteString(title)
		sb.WriteString("】 ")
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	if sb.Len() == 0 {
		return fmt.Sprintf("知识库中未找到与 %q 相关的内容", keyword), nil
	}
	return sb.String(), nil
}
