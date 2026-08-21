package pluggin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ====================================================================
// 插件商店客户端（Plugin Store）
// ====================================================================

const storeConfigFile = "data/plugin_store.json"

// 内置默认镜像源列表（按优先级顺序，第一个成功即命中）。
var defaultStoreMirrors = []string{
	"https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}",
	"https://ghproxy.net/https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}",
	"https://gh-proxy.com/https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}",
	"https://raw.gitmirror.com/{owner}/{repo}/{branch}/{path}",
	"https://cdn.jsdelivr.net/gh/{owner}/{repo}@{branch}/{path}",
}

// StoreConfig 存储配置（持久化到 data/plugin_store.json）。
type StoreConfig struct {
	RepoOwner      string   `json:"repo_owner"`
	RepoName       string   `json:"repo_name"`
	Branch         string   `json:"branch"`
	Mirrors        []string `json:"mirrors"`
	SelectedMirror string   `json:"selected_mirror,omitempty"` // 手动选择的镜像源（空 = 默认自动按顺序尝试）
}

// StoreClient 插件商店客户端。
type StoreClient struct {
	basePath   string
	config     StoreConfig
	httpClient *http.Client
}

// NewStoreClient 创建商店客户端。basePath 为 data 目录。
func NewStoreClient(basePath string) *StoreClient {
	sc := &StoreClient{
		basePath:   basePath,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
	sc.loadConfig()
	return sc
}

func (sc *StoreClient) configFilePath() string {
	return filepath.Join(sc.basePath, "plugin_store.json")
}

func (sc *StoreClient) loadConfig() {
	defaults := StoreConfig{
		RepoOwner: "JuanNiangDev",
		RepoName:  "JuanNiang-Plugins",
		Branch:    "main",
	}
	data, err := os.ReadFile(sc.configFilePath())
	if err != nil {
		sc.config = defaults
		return
	}
	_ = json.Unmarshal(data, &sc.config)
	if sc.config.RepoOwner == "" {
		sc.config.RepoOwner = defaults.RepoOwner
	}
	if sc.config.RepoName == "" {
		sc.config.RepoName = defaults.RepoName
	}
	if sc.config.Branch == "" {
		sc.config.Branch = defaults.Branch
	}
}

func (sc *StoreClient) saveConfig() error {
	data, err := json.MarshalIndent(sc.config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(sc.configFilePath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(sc.configFilePath(), data, 0o644)
}

func (sc *StoreClient) SetConfig(cfg StoreConfig) error {
	if cfg.RepoOwner == "" || cfg.RepoName == "" {
		return fmt.Errorf("repo_owner 与 repo_name 不能为空")
	}
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	// 未指定手动镜像时保留当前选择，避免保存仓库配置时被清空
	if cfg.SelectedMirror == "" {
		cfg.SelectedMirror = sc.config.SelectedMirror
	}
	sc.config = cfg
	return sc.saveConfig()
}

// SelectMirror 手动指定生效镜像源（mirror 为空表示恢复默认自动选择）。
func (sc *StoreClient) SelectMirror(mirror string) error {
	if mirror != "" {
		found := false
		for _, m := range sc.ListMirrors() {
			if m == mirror {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("镜像不存在于可用列表")
		}
	}
	sc.config.SelectedMirror = mirror
	return sc.saveConfig()
}

func (sc *StoreClient) GetConfig() StoreConfig {
	return sc.config
}

func (sc *StoreClient) AddMirror(mirror string) error {
	mirror = strings.TrimSpace(mirror)
	if mirror == "" {
		return fmt.Errorf("镜像地址不能为空")
	}
	if !strings.Contains(mirror, "{path}") {
		return fmt.Errorf("镜像地址必须包含 {path} 占位符")
	}
	for _, m := range sc.config.Mirrors {
		if m == mirror {
			return fmt.Errorf("镜像已存在")
		}
	}
	sc.config.Mirrors = append(sc.config.Mirrors, mirror)
	return sc.saveConfig()
}

func (sc *StoreClient) RemoveMirror(mirror string) error {
	idx := -1
	for i, m := range sc.config.Mirrors {
		if m == mirror {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("镜像不存在")
	}
	sc.config.Mirrors = append(sc.config.Mirrors[:idx], sc.config.Mirrors[idx+1:]...)
	return sc.saveConfig()
}

func (sc *StoreClient) ListMirrors() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(defaultStoreMirrors)+len(sc.config.Mirrors))
	all := append(append([]string{}, defaultStoreMirrors...), sc.config.Mirrors...)
	for _, m := range all {
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

func (sc *StoreClient) resolveURL(template, path string) string {
	r := strings.NewReplacer(
		"{owner}", sc.config.RepoOwner,
		"{repo}", sc.config.RepoName,
		"{branch}", sc.config.Branch,
		"{path}", path,
	)
	return r.Replace(template)
}

func (sc *StoreClient) fetch(path string) ([]byte, error) {
	// 手动选择了镜像源时只使用该镜像，不做自动切换
	if sc.config.SelectedMirror != "" {
		u := sc.resolveURL(sc.config.SelectedMirror, path)
		data, err := sc.fetchOne(u)
		if err != nil {
			return nil, fmt.Errorf("镜像源 %s 不可用: %w", sc.config.SelectedMirror, err)
		}
		return data, nil
	}
	mirrors := sc.ListMirrors()
	var lastErr error
	for _, m := range mirrors {
		u := sc.resolveURL(m, path)
		data, err := sc.fetchOne(u)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("无可用镜像源")
	}
	return nil, fmt.Errorf("所有镜像源均失败: %w", lastErr)
}

func (sc *StoreClient) fetchOne(rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "JuanNiang-Neo/plugin-store")
	resp, err := sc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 20<<20))
}

// StorePluginEntry 商店插件条目。
type StorePluginEntry struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Image       string `json:"image,omitempty"`
	HasConfig   bool   `json:"has_config,omitempty"`
	HasReadme   bool   `json:"has_readme,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

// StorePluginIndex 分片索引。
type StorePluginIndex struct {
	Total     int      `json:"total"`
	Chunks    []string `json:"chunks"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// List 拉取商店插件列表（合并所有分片）。
func (sc *StoreClient) List() ([]StorePluginEntry, error) {
	idxData, err := sc.fetch("plugins.json")
	if err != nil {
		return nil, fmt.Errorf("拉取插件索引失败: %w", err)
	}
	var idx StorePluginIndex
	if err := json.Unmarshal(idxData, &idx); err != nil {
		return nil, fmt.Errorf("解析插件索引失败: %w", err)
	}
	var out []StorePluginEntry
	for _, chunk := range idx.Chunks {
		data, err := sc.fetch("metadata/" + chunk)
		if err != nil {
			continue
		}
		var entries []StorePluginEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			continue
		}
		out = append(out, entries...)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (sc *StoreClient) GetReadmeRaw(pluginPath string) (string, error) {
	data, err := sc.fetch("plugins/" + pluginPath + "/README.md")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (sc *StoreClient) GetAvatarRaw(pluginPath string) ([]byte, error) {
	return sc.fetch("plugins/" + pluginPath + "/avatar.png")
}

func (sc *StoreClient) DownloadPlugin(path string) ([]byte, error) {
	return sc.fetch("dist/" + path + ".zip")
}

// TestMirror 测试指定镜像源是否可用（拉取 plugins.json 并返回耗时）。
func (sc *StoreClient) TestMirror(mirror string) (time.Duration, error) {
	u := sc.resolveURL(mirror, "plugins.json")
	start := time.Now()
	_, err := sc.fetchOne(u)
	return time.Since(start), err
}

// pluginDirNameRe 插件目录名白名单：仅字母/数字/下划线/连字符。
// dirName 会拼进 basePath（data/pluggins/<dirName>）且安装前会 RemoveAll
// 该目录，必须拒绝 ".."、路径分隔符等片段，否则恶意 zip 可推断出
// 逃逸目录名实现任意目录删除（zip-slip 变种）。
var pluginDirNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// lastPathSegment 取路径末段（兼容反斜杠分隔符），用于从商店 path
// （如 "plugins/welcome"）提取插件目录名。
func lastPathSegment(p string) string {
	return path.Base(filepath.ToSlash(p))
}

// ErrUnsafeZipEntry 标识 zip 包含非法条目（逃逸路径/特殊文件），
// 调用方可据此映射"插件包不安全"类错误。
var ErrUnsafeZipEntry = errors.New("zip 包含非法条目")

// extractZipTo 将 zip 全部条目解压到 destDir（可剥离 rootPrefix 前缀）。
// 逐条执行 zip-slip 校验；任一条目非法或任一 I/O 失败立即返回错误，
// 不静默跳过、不落半成品。
func extractZipTo(reader *zip.Reader, rootPrefix, destDir string) error {
	for _, f := range reader.File {
		// 去掉 rootPrefix 前缀，落到插件目录根
		rel := filepath.ToSlash(f.Name)
		if rootPrefix != "" && strings.HasPrefix(rel, rootPrefix) {
			rel = strings.TrimPrefix(rel, rootPrefix)
		}
		if rel == "" {
			continue
		}
		// 防 zip-slip：解压目标必须仍在 destDir 内，逃逸条目（"../"、
		// 绝对路径等）直接中止安装，而不是静默写到插件目录之外。
		target := filepath.Join(destDir, rel)
		if r, err := filepath.Rel(destDir, target); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: 逃逸路径条目 %q", ErrUnsafeZipEntry, f.Name)
		}
		// 拒绝符号链接等特殊条目：解压只落普通文件与目录
		if f.Mode()&(os.ModeSymlink|os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0 {
			return fmt.Errorf("%w: 特殊文件条目 %q", ErrUnsafeZipEntry, f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("创建目录失败 %q: %w", f.Name, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("创建父目录失败 %q: %w", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("读取 zip 条目失败 %q: %w", f.Name, err)
		}
		dst, err := os.Create(target)
		if err != nil {
			_ = rc.Close()
			return fmt.Errorf("创建文件失败 %q: %w", f.Name, err)
		}
		if _, err := io.Copy(dst, rc); err != nil {
			_ = rc.Close()
			_ = dst.Close()
			return fmt.Errorf("写入文件失败 %q: %w", f.Name, err)
		}
		_ = rc.Close()
		_ = dst.Close()
	}
	return nil
}

// StageZipExtract 在 destDir 同级创建唯一暂存目录（同文件系统，rename 原子），
// 将 zip 全量校验解压到暂存目录，返回暂存目录路径。
// 任一条目非法或 I/O 失败时清理暂存目录并返回错误。
func StageZipExtract(reader *zip.Reader, rootPrefix, destDir string) (string, error) {
	staging, err := os.MkdirTemp(filepath.Dir(destDir), ".staging-")
	if err != nil {
		return "", fmt.Errorf("创建暂存目录失败: %w", err)
	}
	_ = os.Chmod(staging, 0o755)
	if err := extractZipTo(reader, rootPrefix, staging); err != nil {
		_ = os.RemoveAll(staging)
		return "", err
	}
	return staging, nil
}

// CommitStagedPlugin 将暂存目录替换为正式插件目录（原子 rename）。
// 旧目录先改名备份；rename 失败时恢复旧目录，旧插件不丢。
// 返回备份路径（无旧版时空串）。
func CommitStagedPlugin(destDir, staging string) (backup string, err error) {
	hadOld := false
	if _, statErr := os.Stat(destDir); statErr == nil {
		hadOld = true
		backup = staging + "-backup"
		if err := os.Rename(destDir, backup); err != nil {
			_ = os.RemoveAll(staging)
			return "", fmt.Errorf("备份旧插件目录失败: %w", err)
		}
	}
	if err := os.Rename(staging, destDir); err != nil {
		_ = os.RemoveAll(staging)
		if hadOld {
			if rbErr := os.Rename(backup, destDir); rbErr != nil {
				return "", fmt.Errorf("落位新插件目录失败且恢复旧目录也失败: install=%v, restore=%v", err, rbErr)
			}
		}
		return "", fmt.Errorf("落位新插件目录失败: %w", err)
	}
	return backup, nil
}

// RollbackStagedPlugin 插件加载失败时回滚：移除新目录，恢复备份的旧版本。
// 全新安装（无备份）时保留新文件，便于排查加载失败原因。
func RollbackStagedPlugin(destDir, backup string) {
	if backup == "" {
		return
	}
	_ = os.RemoveAll(destDir)
	if err := os.Rename(backup, destDir); err != nil {
		log.Error("回滚旧版插件目录失败", "dest", destDir, "err", err)
	}
}

// InstallPluginZip 将商店下载的 zip 暂存解压、全量校验后原子安装到
// data/pluggins/<name>/ 并加载。zip 内要求包含 pluggin.yaml（可在根或子目录）。
// zip 内容不可信：目录名与每个条目都过安全校验，防 zip-slip 逃逸。
// 校验或解压失败时旧插件目录保持原样；升级场景加载失败自动回滚旧版。
func InstallPluginZip(pe *PluginEngine, name string, data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("无效的 zip: %w", err)
	}
	// 路径片段中的父路径部分，末段仍非法（如 ".."）则整体拒绝。
	dirName := lastPathSegment(name)
	var rootPrefix string
	for _, f := range reader.File {
		if f.Name == "pluggin.yaml" || filepath.Base(f.Name) == "pluggin.yaml" {
			d := filepath.ToSlash(filepath.Dir(f.Name))
			if d == "." {
				dirName = lastPathSegment(name)
				rootPrefix = ""
			} else {
				dirName = path.Base(d)
				rootPrefix = d + "/"
			}
			break
		}
	}
	if !pluginDirNameRe.MatchString(dirName) {
		return "", fmt.Errorf("插件目录名不合法: %q", dirName)
	}

	destDir := filepath.Join(pe.basePath, dirName)

	// 暂存解压 + 全量校验：失败时旧目录原样保留
	staging, err := StageZipExtract(reader, rootPrefix, destDir)
	if err != nil {
		return "", err
	}

	// 原子替换：旧目录备份 → 新目录上位
	backup, err := CommitStagedPlugin(destDir, staging)
	if err != nil {
		return "", err
	}

	// 升级场景：旧版已在引擎中加载，先卸载使其磁盘新文件能被 Load 读取
	// （Load 对已加载插件直接返回 already loaded）。
	wasLoaded := false
	if err := pe.Unload(dirName); err != nil {
		if !strings.Contains(err.Error(), "not loaded") {
			RollbackStagedPlugin(destDir, backup)
			return dirName, fmt.Errorf("卸载旧版插件失败: %w", err)
		}
	} else {
		wasLoaded = true
	}

	if err := pe.Load(dirName); err != nil {
		// 升级场景回滚旧版并尽量恢复引擎状态；全新安装保留文件便于排查
		RollbackStagedPlugin(destDir, backup)
		if wasLoaded {
			// 目录已回滚为旧版，重载旧版恢复引擎运行状态
			if reErr := pe.Load(dirName); reErr != nil {
				log.Error("回滚后重载旧版插件失败", "name", dirName, "err", reErr)
			}
		}
		return dirName, fmt.Errorf("加载插件失败: %w", err)
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return dirName, nil
}
