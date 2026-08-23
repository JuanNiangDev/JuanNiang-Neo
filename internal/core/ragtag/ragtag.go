// Package ragtag 派生 RAG-Service 的 tag（UUID v5 确定性映射）。
//
// 背景：知识与长期记忆共用同一 RAG-Service 实例（tag 必须是 UUID，无法加前缀），
// 为避免两个集合的向量互相污染检索，用固定命名空间 + 前缀做 UUID v5 派生：
//   - 知识：v5(ragNS, "k:"+itemID)
//   - 记忆：v5(ragNS, "m:"+itemID)
//
// 派生函数确定性、可复现：写入/检索/删除/手动同步都用同一函数，无需额外映射字段
// （表结构零改动）。检索侧用"本地条目 ID 全量 → 派生 tag set"过滤出本集合的命中。
package ragtag

import "github.com/google/uuid"

// ragNS 固定命名空间（任意常量 UUID，保证派生结果跨实例稳定）。
var ragNS = uuid.MustParse("e8c5c747-3a2b-4c1f-9d5e-0a6b7c8d9e0f")

// Knowledge 返回知识条目的 RAG tag。
func Knowledge(itemID string) uuid.UUID {
	return uuid.NewSHA1(ragNS, []byte("k:"+itemID))
}

// Memory 返回长期记忆条目的 RAG tag。
func Memory(itemID string) uuid.UUID {
	return uuid.NewSHA1(ragNS, []byte("m:"+itemID))
}

// Word 返回群管理违规关键词条的 RAG tag（与 k:/m:/s: 前缀互不污染）。
func Word(itemID string) uuid.UUID {
	return uuid.NewSHA1(ragNS, []byte("w:"+itemID))
}

// Sample 返回群管理违规样本的 RAG tag（学习闭环/种子/导入样本）。
func Sample(itemID string) uuid.UUID {
	return uuid.NewSHA1(ragNS, []byte("s:"+itemID))
}
