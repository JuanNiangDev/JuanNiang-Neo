package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

// DevConfig 是 dev.yaml 的完整配置结构。
// 所有字段均为可选；缺失时使用环境变量或内置默认值。
type DevConfig struct {
	Database struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Name     string `yaml:"name"`
	} `yaml:"database"`
	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
		DB       string `yaml:"db"`
	} `yaml:"redis"`
	OneBot11 struct {
		Port   string   `yaml:"port"`
		Token  string   `yaml:"token"`
		Admins []string `yaml:"admins"`
	} `yaml:"onebot11"`
	API struct {
		Addr string `yaml:"addr"`
	} `yaml:"api"`
	Web struct {
		Dir string `yaml:"dir"`
	} `yaml:"web"`
	JWT struct {
		Secret string `yaml:"secret"`
	} `yaml:"jwt"`
	Images struct {
		Dir string `yaml:"dir"`
	} `yaml:"images"`
	Debug struct {
		Enabled   bool   `yaml:"enabled"`
		PprofAddr string `yaml:"pprof_addr"`
	} `yaml:"debug"`
	// Stats 群消息/回复统计（Loki+Promtail 采集通道，独立于主日志 pipeline）。
	Stats struct {
		Enabled    bool   `yaml:"enabled"`
		Path       string `yaml:"path"`
		MaxSizeMB  int    `yaml:"max_size_mb"`
		MaxBackups int    `yaml:"max_backups"`
		MaxAgeDays int    `yaml:"max_age_days"`
		QueueSize  int    `yaml:"queue_size"`
	} `yaml:"stats"`
}

// loadDevConfig 读取 dev.yaml 配置文件。
// 如果文件不存在则返回零值 DevConfig（不报错）。
func loadDevConfig(path string) DevConfig {
	var cfg DevConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = yaml.Unmarshal(data, &cfg)
	return cfg
}

// devEnv 优先从环境变量读取，其次从 dev.yaml 读取，最后使用 fallback 默认值。
func devEnv(envKey string, yamlVal string, fallback string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if yamlVal != "" {
		return yamlVal
	}
	return fallback
}
