// Package imgstore 图床文件存储。
//
// 所有图片以 <id>.img 平铺存储在 rootDir（默认 data/imgs）下，元数据（名称/虚拟文件夹/
// MIME/大小）由 Postgres 的 image_assets 表管理。id 为 uuid，天然无路径穿越风险，
// 这里再额外做一层防御校验。
package imgstore

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store 图床文件存储。
type Store struct {
	rootDir string
}

// New 创建图床存储。rootDir 为空时使用默认 "data/imgs"。
func New(rootDir string) *Store {
	if rootDir == "" {
		rootDir = "data/imgs"
	}
	return &Store{rootDir: rootDir}
}

// RootDir 返回存储根目录。
func (s *Store) RootDir() string { return s.rootDir }

// EnsureDir 确保存储目录存在。
func (s *Store) EnsureDir() error {
	return os.MkdirAll(s.rootDir, 0o755)
}

// ErrInvalidID 非法图片 ID。
var ErrInvalidID = errors.New("invalid image id")

func (s *Store) filePath(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return "", fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	return filepath.Join(s.rootDir, id+".img"), nil
}

// Save 写入图片文件。
func (s *Store) Save(id string, data []byte) error {
	path, err := s.filePath(id)
	if err != nil {
		return err
	}
	if err := s.EnsureDir(); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Read 读取图片原始字节。
func (s *Store) Read(id string) ([]byte, error) {
	path, err := s.filePath(id)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// LoadBase64 读取图片并返回 base64 编码（不含前缀；发送消息时拼成 "base64://" + 返回值）。
func (s *Store) LoadBase64(id string) (string, error) {
	data, err := s.Read(id)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// Delete 删除图片文件；文件不存在时视为已删除（返回 nil）。
func (s *Store) Delete(id string) error {
	path, err := s.filePath(id)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Exists 判断图片文件是否存在。
func (s *Store) Exists(id string) bool {
	path, err := s.filePath(id)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
