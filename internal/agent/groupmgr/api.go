package groupmgr

import (
	"context"

	"JuanNiang-Neo/internal/core/models"
)

// API 层公开方法（Web REST 调用）。

// SyncRAG 全量同步词条 + 样本到 RAG（Web「同步向量库」按钮）。
func (m *Manager) SyncRAG(ctx context.Context) (total, failed int, err error) {
	return m.syncRAG(ctx)
}

// AddWord 新增词条（Web/导入共用）：
//   - 写词条表（幂等 upsert，source=import）
//   - RAG 可用时同步写入样本表（seed）+ RAG upsert，不可用静默跳过
//   - 词库内存缓存热更新
func (m *Manager) AddWord(ctx context.Context, word, category string) (uint, error) {
	if err := m.dao.WordUpsert(ctx, word, category, "import"); err != nil {
		return 0, err
	}
	var sampleID uint
	if m.getRAG() != nil {
		if id, err := m.dao.SampleAdd(ctx, word, category, "seed"); err == nil {
			sampleID = id
			m.upsertRAGSample(ctx, id, word)
		}
	}
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
		if err := m.dao.WordUpsert(ctx, w, category, "import"); err != nil {
			skipped++
			continue
		}
		if m.getRAG() != nil {
			if id, err := m.dao.SampleAdd(ctx, w, category, "seed"); err == nil {
				m.upsertRAGSample(ctx, id, w)
			}
		}
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
