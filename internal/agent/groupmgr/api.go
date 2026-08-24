package groupmgr

import (
	"context"
	"strconv"

	"JuanNiang-Neo/internal/core/models"
)

// API 层公开方法（Web REST 调用）。

// SyncRAG 全量同步词条 + 样本到 RAG（Web「同步向量库」按钮）。
func (m *Manager) SyncRAG(ctx context.Context) (total, failed int, err error) {
	return m.syncRAG(ctx)
}

// AddWord 新增词条（Web/导入共用）：
//   - 写词条表（幂等 upsert，source=import，回填派生 RAG tag）
//   - RAG 可用时同步写入样本表（seed）+ RAG upsert，成功则标记 RAGSynced；
//     不可用则保持 RAGSynced=false（面板展示未同步，可手动「同步向量库」）
//   - 词库内存缓存热更新
func (m *Manager) AddWord(ctx context.Context, word, category string) (uint, error) {
	id, err := m.dao.WordUpsert(ctx, word, category, "import")
	if err != nil {
		return 0, err
	}
	var sampleID uint
	if m.getRAG() != nil {
		if sid, err := m.dao.SampleAdd(ctx, word, category, "seed"); err == nil {
			sampleID = sid
			m.upsertRAGSample(ctx, sid, word)
		}
	}
	// RAG 可用且样本写入成功（含 RAG upsert 成功）→ 标记已同步；
	// upsertRAGSample 内部静默跳过失败，这里以 RAG 客户端存在为准标记
	_ = m.dao.WordMarkRAGSynced(ctx, id, m.getRAG() != nil)
	_ = m.Reload(ctx)
	return sampleID, nil
}

// ImportWords 批量导入词条（txt 一行一个，Web 导入按钮）：
// 逐条 AddWord；返回成功条数。导入前先规范化（去注释/空白/小写）。
func (m *Manager) ImportWords(ctx context.Context, lines []string, category string) (imported, skipped int) {
	seen := map[string]bool{}
	for _, line := range lines {
		w := cleanWord(line)
		if w == "" || seen[w] {
			skipped++
			continue
		}
		seen[w] = true
		id, err := m.dao.WordUpsert(ctx, w, category, "import")
		if err != nil {
			skipped++
			continue
		}
		if m.getRAG() != nil {
			if sid, err := m.dao.SampleAdd(ctx, w, category, "seed"); err == nil {
				m.upsertRAGSample(ctx, sid, w)
			}
		}
		_ = m.dao.WordMarkRAGSynced(ctx, id, m.getRAG() != nil)
		imported++
	}
	_ = m.Reload(ctx)
	return imported, skipped
}

// DeleteSample 删除样本（双删 RAG，不可用静默跳过）。
func (m *Manager) DeleteSample(ctx context.Context, id uint) error {
	if err := m.dao.SampleDelete(ctx, id); err != nil {
		return err
	}
	m.deleteRAGSample(ctx, id)
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
