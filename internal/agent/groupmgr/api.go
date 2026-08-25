package groupmgr

import (
	"context"
	"strconv"

	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/core/ragtag"
)

// API 层公开方法（Web REST 调用）。

// SyncRAG 全量同步词条 + 样本到 RAG（Web「同步向量库」按钮）。
func (m *Manager) SyncRAG(ctx context.Context) (total, failed int, err error) {
	return m.syncRAG(ctx)
}

// SyncRAGProgress 带进度回调的全量同步（SSE 流式同步进度展示用）：
// 每批处理后回调 onProgress(done, failed)，回调返回错误（如客户端断开）即中止。
func (m *Manager) SyncRAGProgress(ctx context.Context, onProgress func(done, failed int) error) (total, failed int, err error) {
	return m.syncRAGProgress(ctx, onProgress)
}

// sampleCategoryByWord 词条分类 → 样本类别（样本契约：ad / sensitive；
// 灰色/黑色词统一归 ad，敏感词映射 sensitive）。
func sampleCategoryByWord(category string) string {
	if category == "sensitive" {
		return "sensitive"
	}
	return "ad"
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
	if _, err := m.upsertRAGPhrase(ctx, id, text, listType); err != nil {
		log.Warn("语录写入 RAG 失败（可手动同步）", "phrase", id, "list", listType, "err", err)
	}
	m.invalidateSampleSet()
	return id, nil
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
		if sid, err := m.dao.SampleAddWithWord(ctx, word, sampleCategoryByWord(category), "seed", id); err == nil {
			sampleID = sid
			synced, _ = m.upsertRAGSample(ctx, sid, word)
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
			if sid, err := m.dao.SampleAddWithWord(ctx, w, sampleCategoryByWord(category), "seed", id); err == nil {
				synced, _ = m.upsertRAGSample(ctx, sid, w)
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

// deleteRAGPhrase 删除语录向量（双删，RAG 不可用静默跳过）。按集合选择 tag 前缀。
func (m *Manager) deleteRAGPhrase(ctx context.Context, sampleID uint, listType string) {
	cli := m.getRAG()
	if cli == nil {
		return
	}
	tag := ragtag.Sample(u32s(sampleID))
	if listType == "white" {
		tag = ragtag.WhitePhrase(u32s(sampleID))
	}
	if err := cli.Delete(ctx, tag); err != nil {
		log.Warn("语录从 RAG 删除失败", "phrase", sampleID, "list", listType, "err", err)
	}
	m.invalidateSampleSet()
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
	m.deleteRAGPhrase(ctx, id, listType)
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
