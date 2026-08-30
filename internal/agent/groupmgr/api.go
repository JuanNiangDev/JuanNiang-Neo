package groupmgr

import (
	"context"
	"fmt"
	"strconv"

	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/core/ragtag"
)

// API 层公开方法（Web REST 调用）。

// SyncRAG 全量同步违禁语录到 RAG（Web「同步向量库」按钮）。
// 关键词词库不入 RAG（仅内存兜底，从 go:embed txt 加载）。
func (m *Manager) SyncRAG(ctx context.Context) (total, failed int, err error) {
	return m.syncRAG(ctx)
}

// SyncRAGProgress 带进度回调的全量同步（SSE 流式同步进度展示用）：
// 每批处理后回调 onProgress(done, failed)，回调返回错误（如客户端断开）即中止。
func (m *Manager) SyncRAGProgress(ctx context.Context, onProgress func(done, failed int) error) (total, failed int, err error) {
	return m.syncRAGProgress(ctx, onProgress)
}

// AddPhrase 新增违禁语录（黑/白名单）：写语录表 + RAG 双向同步（可用时）。
// 返回语录 ID；RAG 未配置/失败不影响入库（面板可手动同步）。
func (m *Manager) AddPhrase(ctx context.Context, text, category, listType string) (uint, error) {
	if listType != "white" {
		listType = "black"
	}
	id, err := m.dao.SampleAddPhrase(ctx, text, category, "import", listType)
	if err != nil {
		return 0, err
	}
	// 仅真实写入 RAG 成功才标记已同步（区分「客户端存在」与「upsert 成功」）
	synced, _ := m.upsertRAGPhrase(ctx, id, text, listType)
	_ = m.dao.SampleMarkRAGSynced(ctx, id, synced)
	if !synced {
		log.Warn("语录写入 RAG 失败（可手动同步）", "phrase", id, "list", listType)
	}
	m.invalidateSampleSet()
	return id, nil
}

// AddWord 已废弃：关键词词库仅从 go:embed txt 加载到内存作兜底，不入 DB/RAG/samples，不可 Web 修改。
// 保留方法签名仅为向后兼容旧 API 调用，调用一律返回错误。
func (m *Manager) AddWord(ctx context.Context, word, category string) (uint, error) {
	return 0, fmt.Errorf("关键词词库已改为只读内存兜底（从 txt 加载），不再支持新增")
}

// ImportWords 已废弃：关键词词库仅从 go:embed txt 加载到内存作兜底，不入 DB/RAG/samples，不可 Web 修改。
// 保留方法签名仅为向后兼容旧 API 调用，调用一律跳过（导入 0 条，全部 skipped）。
func (m *Manager) ImportWords(ctx context.Context, lines []string, category string) (imported, skipped int) {
	return 0, len(lines)
}

// DeleteWord 已废弃：关键词词库仅从 go:embed txt 加载到内存作兜底，不入 DB/RAG/samples，不可 Web 修改。
// 保留方法签名仅为向后兼容旧 API 调用，调用一律返回错误。
func (m *Manager) DeleteWord(ctx context.Context, id uint) error {
	return fmt.Errorf("关键词词库已改为只读内存兜底（从 txt 加载），不再支持删除")
}

// deleteRAGPhrase 删除语录向量（双删）。按集合选择 tag 前缀。
// RAG 未配置/不可用时返回 nil（无向量可删，视同成功）；删除失败返回 error（调用方决定是否保留 PG 行）。
func (m *Manager) deleteRAGPhrase(ctx context.Context, sampleID uint, listType string) error {
	cli := m.getRAG()
	if cli == nil {
		return nil
	}
	tag := ragtag.Sample(u32s(sampleID))
	if listType == "white" {
		tag = ragtag.WhitePhrase(u32s(sampleID))
	}
	if err := cli.Delete(ctx, ragtag.ScoopGroupMgr, tag); err != nil {
		log.Warn("语录从 RAG 删除失败", "phrase", sampleID, "list", listType, "err", err)
		return err
	}
	m.invalidateSampleSet()
	return nil
}

// DeleteSample 删除语录（双删 RAG 向量，按黑白集合选 tag；不可用静默跳过）。
func (m *Manager) DeleteSample(ctx context.Context, id uint) error {
	// 先查集合类型（决定 RAG tag 前缀），再删除
	var listType string
	if list, err := m.dao.SampleListAll(ctx); err == nil {
		for _, s := range list {
			if s.ID == id {
				listType = s.ListType
				break
			}
		}
	}
	if err := m.dao.SampleDelete(ctx, id); err != nil {
		return err
	}
	if listType == "" {
		listType = "black"
	}
	// 手动删除语义：RAG 删除失败仅告警，不阻塞 PG 删除（用户主动删除，残留向量由对账清理）
	_ = m.deleteRAGPhrase(ctx, id, listType)
	return nil
}

// Samples 样本列表。
func (m *Manager) Samples(ctx context.Context) ([]models.GroupMgrSample, error) {
	return m.dao.SampleListAll(ctx)
}

// SyncAdminsFromAdapter 把 Adapter.Admins（系统管理员 QQ）合并到群管理手动管理员表
// （去重，已存在跳过）。返回新增数量。面板「从 Adapter 同步管理员」按钮。
func (m *Manager) SyncAdminsFromAdapter(ctx context.Context, admins []string) (int, error) {
	existing, err := m.dao.AdminList(ctx)
	if err != nil {
		return 0, err
	}
	have := make(map[int64]bool, len(existing))
	for _, a := range existing {
		have[a.QQ] = true
	}
	added := 0
	for _, s := range admins {
		qq, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil || qq <= 0 || have[qq] {
			continue
		}
		if err := m.dao.AdminAdd(ctx, qq); err != nil {
			log.Warn("管理员同步添加失败", "qq", qq, "err", err)
			continue
		}
		have[qq] = true
		added++
	}
	if added > 0 {
		_ = m.Reload(ctx)
	}
	return added, nil
}
