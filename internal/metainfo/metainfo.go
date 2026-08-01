// Package metainfo 读取项目根目录的 metainfo.yaml，提供版本等元数据。
// 构建时通过 -ldflags 注入，运行时从文件读取作为 fallback。
package metainfo

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// MetaInfo 项目元数据。
type MetaInfo struct {
	Version   string `yaml:"version"`
	BuildTime string `yaml:"build_time,omitempty"`
	GitCommit string `yaml:"git_commit,omitempty"`
	GoVersion string `yaml:"go_version,omitempty"`
}

var (
	// 构建时通过 ldflags 注入
	//   go build -ldflags "-X JuanNiang-Neo/internal/metainfo.version=1.0.4 ..."
	version   string
	buildTime string
	gitCommit string
	goVersion string

	once     sync.Once
	instance MetaInfo
)

// Get 返回单例 MetaInfo。优先使用 ldflags 注入值，回退到 metainfo.yaml。
func Get() MetaInfo {
	once.Do(initMeta)
	return instance
}

func initMeta() {
	instance = MetaInfo{
		Version:   firstNonEmpty(version, "dev"),
		BuildTime: buildTime,
		GitCommit: gitCommit,
		GoVersion: goVersion,
	}

	// ldflags 未注入 version 时从文件读取
	if instance.Version == "dev" {
		if m, err := loadFromFile("metainfo.yaml"); err == nil && m.Version != "" {
			instance.Version = m.Version
		}
	}
}

func loadFromFile(path string) (*MetaInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m MetaInfo
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
