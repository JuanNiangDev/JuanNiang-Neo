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
//   - RAG 可用时同步写入样本表（seed）+ RAG upsert，仅真实同步成功才标记 RAGSynced；
//     不可用/失败保持 RAGSynced=false（面板展示未同步，可手动「同步向量库」）
//   - 词库内存缓存热更新
func (m *Manager) AddWord(ctx context.Context, word, category string) (uint, error) {
	id, err := m.dao.WordUpsert(ctx, word, category, "import")
	if err != nil {
		return 0, err
	}
	var sampleID uint
	synced := false
	if m.getRAG() != nil {
		if sid, err := m.dao.SampleAddWithWord(ctx, word, category, "seed", id); err == nil {
			sampleID = sid
			synced = m.upsertRAGSample(ctx, sid, word) == nil
		}
	}
	// 仅当样本真实写入 RAG 成功才标记已同步（区分「客户端存在」与「upsert 成功」）
	_ = m.dao.WordMarkRAGSynced(ctx, id, synced)
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
		synced := false
		if m.getRAG() != nil {
			if sid, err := m.dao.SampleAddWithWord(ctx, w, category, "seed", id); err == nil {
				synced = m.upsertRAGSample(ctx, sid, w) == nil
			}
		}
		_ = m.dao.WordMarkRAGSynced(ctx, id, synced)
		imported++
	}
	_ = m.Reload(ctx)
	return imported, skipped
}

// DeleteWord 删除词条 + 对账清理派生样本与 RAG 向量（双删）。
// 回归：此前只软删 group_mgr_words 行，seed 样本副本与 RAG 向量仍活跃，
// 管理员删词后 RAG 路径照常命中（无法停止检测）。
func (m *Manager) DeleteWord(ctx context.Context, id uint) error {
	w, err := m.dao.WordGet(ctx, id)
	if err != nil {
		return err
	}
	// 按文本匹配派生样本（词条文本即样本文本，幂等唯一）并双删 RAG 向量
	samples, err := m.dao.SampleListByText(ctx, w.Word)
	if err != nil {
		return err
	}
	for _, s := range samples {
		m.deleteRAGSample(ctx, s.ID)
		if err := m.dao.SampleDelete(ctx, s.ID); err != nil {
			log.Warn("词条派生样本删除失败", "sample", s.ID, "err", err)
		}
	}
	if err := m.dao.WordDelete(ctx, id); err != nil {
		return err
	}
	m.invalidateSampleSet()
	return m.Reload(ctx)
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
