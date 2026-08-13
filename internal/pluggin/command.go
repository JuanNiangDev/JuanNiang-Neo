package pluggin

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// CommandHandler 命令处理函数签名。
//   - args: 命令路径之后的所有参数（已按空格切分）
//   - event: 触发命令的事件上下文
//
// 返回:
//   - consumed: 是否消费此命令（true 则跳过 Agent 处理）
//   - reply: 若非空，则由 PluginEngine 自动回复给用户
//   - err: 错误信息（仅用于日志）
type CommandHandler func(args []string, event EventData) (consumed bool, reply string, err error)

// CommandOpts 命令的元信息，用于 /help 自动生成帮助。
type CommandOpts struct {
	Description string // 一句话描述
	Usage       string // 用法示例（如 "/system provider switch <id>"）
}

// CommandNode 命令树节点。叶节点（Handler != nil）可执行；非叶节点仅作为分组。
type CommandNode struct {
	Name       string
	Opts       CommandOpts
	Handler    CommandHandler
	PluginName string // 注册该命令的插件名
	Children   map[string]*CommandNode
}

// CommandRegistry 多级命令注册表，支持按路径派发与 /help 自动生成。
type CommandRegistry struct {
	mu   sync.RWMutex
	root *CommandNode // 虚拟根节点
}

// NewCommandRegistry 创建空注册表。
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{root: &CommandNode{Children: make(map[string]*CommandNode)}}
}

// Register 注册一条命令路径。
//   - plugin: 注册方插件名（系统命令用 "system"）
//   - path: 命令路径（如 ["system", "provider", "switch"]）；空数组非法
//   - opts: 元信息
//   - handler: 处理函数；nil 表示仅作为分组节点（其子命令的父）
//
// 同一路径重复注册会覆盖旧 handler/opts。
func (r *CommandRegistry) Register(plugin string, path []string, opts CommandOpts, handler CommandHandler) {
	if len(path) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.root
	for i, seg := range path {
		if seg == "" {
			return
		}
		next, ok := cur.Children[seg]
		if !ok {
			next = &CommandNode{Name: seg, Children: make(map[string]*CommandNode)}
			cur.Children[seg] = next
		}
		if i == len(path)-1 {
			next.Opts = opts
			next.Handler = handler
			next.PluginName = plugin
		}
		cur = next
	}
}

// UnregisterPlugin 卸载插件时调用，移除其注册的所有命令。
func (r *CommandRegistry) UnregisterPlugin(plugin string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unregisterPluginNode(r.root, plugin)
}

func (r *CommandRegistry) unregisterPluginNode(n *CommandNode, plugin string) {
	for _, child := range n.Children {
		if child.PluginName == plugin {
			child.Handler = nil
			child.PluginName = ""
		}
		r.unregisterPluginNode(child, plugin)
	}
	// 清理空叶子节点
	for key, child := range n.Children {
		if child.Handler == nil && child.PluginName == "" && len(child.Children) == 0 {
			delete(n.Children, key)
		}
	}
}

// Find 按路径查找节点（不要求是叶节点）。
func (r *CommandRegistry) Find(path []string) *CommandNode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cur := r.root
	for _, seg := range path {
		next, ok := cur.Children[seg]
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

// HasCommand 检查消息是否匹配任何已注册的命令（不执行，仅判断是否有命令）。
func (r *CommandRegistry) HasCommand(raw string) bool {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "/") {
		return false
	}
	tokens := strings.Fields(raw)
	if len(tokens) == 0 {
		return false
	}
	tokens[0] = strings.TrimPrefix(tokens[0], "/")
	if tokens[0] == "" {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	cur := r.root
	var matched bool
	for _, tok := range tokens {
		next, ok := cur.Children[tok]
		if !ok {
			break
		}
		cur = next
		// 叶节点（有 Handler）或分组节点（有子命令）都视为命中，
		// 因为 Dispatch 会对两者产生有效输出（执行命令 / 列出子命令）
		if next.Handler != nil || len(next.Children) > 0 {
			matched = true
		}
	}
	return matched
}

// Dispatch 解析 raw 消息并派发到对应命令。
// raw 应为以 "/" 开头的消息。返回 (consumed, reply, err)。
// 若找不到匹配命令，consumed=false 让上层 fallback 到 on_message。
// Match 匹配命令树但不执行 handler（锁内短临界区，不触碰 Lua）。
// 返回：
//
//	node != nil → 命中可执行 handler，args 为命中路径之后的剩余参数
//	hint != ""  → 停在分组节点，hint 为该节点的子命令提示
//	否则 → 未命中
//
// handler 由调用方在插件 stateMu 下执行（见 PluginEngine.execCommand）——
// 若在锁内直接执行，handler 内动态注册命令会触发读锁升级写锁死锁。
func (r *CommandRegistry) Match(raw string) (node *CommandNode, args []string, hint string) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "/") {
		return nil, nil, ""
	}
	tokens := strings.Fields(raw)
	if len(tokens) == 0 {
		return nil, nil, ""
	}
	// tokens[0] = "/cmd" 形式，去掉前导 "/"
	tokens[0] = strings.TrimPrefix(tokens[0], "/")
	if tokens[0] == "" {
		return nil, nil, ""
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 沿命令树逐级匹配，记录最后一个有 Handler 的节点
	cur := r.root
	var matched *CommandNode
	var matchedPathLen int // 命中的路径 token 数
	for i, tok := range tokens {
		next, ok := cur.Children[tok]
		if !ok {
			break
		}
		cur = next
		if next.Handler != nil {
			matched = next
			matchedPathLen = i + 1
		}
	}

	if matched != nil {
		// args = 命中路径之后的所有 token
		return matched, tokens[matchedPathLen:], ""
	}

	// 没有命中可执行 handler；但若停在某个非根节点，则提示该节点的子命令
	if cur != r.root {
		subs := listChildrenSorted(cur)
		if len(subs) > 0 {
			// 计算实际命中的路径长度
			walk := r.root
			hitLen := 0
			for _, t := range tokens {
				next, ok := walk.Children[t]
				if !ok {
					break
				}
				walk = next
				hitLen++
			}
			lines := []string{fmt.Sprintf("命令 /%s 有以下子命令：", strings.Join(tokens[:hitLen], " "))}
			for _, s := range subs {
				lines = append(lines, formatHelpLine(s))
			}
			return nil, nil, strings.Join(lines, "\n")
		}
	}
	return nil, nil, ""
}

// Dispatch 匹配并执行命令 handler（handler 在锁外执行）。
// 保留旧签名以兼容调用方；新代码建议用 Match + PluginEngine.execCommand，
// 让 handler 在插件 stateMu 下执行以保证 LState 串行。
func (r *CommandRegistry) Dispatch(raw string, event EventData) (consumed bool, reply string, err error) {
	node, args, hint := r.Match(raw)
	if node != nil {
		consumed, reply, err = node.Handler(args, event)
		if !consumed && reply == "" && err == nil {
			return false, "", nil
		}
		return consumed, reply, err
	}
	if hint != "" {
		return true, hint, nil
	}
	return false, "", nil
}

// ListTopLevel 列出所有顶层命令节点（按字母序）。
func (r *CommandRegistry) ListTopLevel() []*CommandNode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return listChildrenSorted(r.root)
}

// ListSubcommands 列出指定路径的子命令。
func (r *CommandRegistry) ListSubcommands(path []string) []*CommandNode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	node := r.root
	for _, seg := range path {
		next, ok := node.Children[seg]
		if !ok {
			return nil
		}
		node = next
	}
	return listChildrenSorted(node)
}

// PluginCommandInfo 单条命令的展示信息。
type PluginCommandInfo struct {
	Path        []string `json:"path"`        // 完整路径, 如 ["system","provider","switch"]
	Description string   `json:"description"` // 描述
	Usage       string   `json:"usage"`       // 用法示例
	IsLeaf      bool     `json:"is_leaf"`     // 是否为可执行命令 (handler != nil)
}

// ListByPlugin 返回指定插件注册的所有命令路径 (递归遍历整棵命令树)。
// 仅返回由该插件注册的节点 (Handler 非 nil 即视为其注册的命令)。
func (r *CommandRegistry) ListByPlugin(plugin string) []PluginCommandInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PluginCommandInfo, 0)
	r.collectByPlugin(r.root, plugin, nil, &out)
	return out
}

func (r *CommandRegistry) collectByPlugin(n *CommandNode, plugin string, prefix []string, out *[]PluginCommandInfo) {
	for _, child := range n.Children {
		curPath := append(append([]string{}, prefix...), child.Name)
		// 节点由指定插件注册且 handler 非空 → 视为该插件的可执行命令
		if child.PluginName == plugin && child.Handler != nil {
			*out = append(*out, PluginCommandInfo{
				Path:        curPath,
				Description: child.Opts.Description,
				Usage:       child.Opts.Usage,
				IsLeaf:      true,
			})
		}
		// 递归子节点
		if len(child.Children) > 0 {
			r.collectByPlugin(child, plugin, curPath, out)
		}
	}
}

// listChildrenSorted 返回子节点列表（按 Name 升序）。
func listChildrenSorted(n *CommandNode) []*CommandNode {
	if n == nil || len(n.Children) == 0 {
		return nil
	}
	out := make([]*CommandNode, 0, len(n.Children))
	for _, c := range n.Children {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// formatHelpLine 将节点格式化为单行帮助文本。
func formatHelpLine(n *CommandNode) string {
	desc := n.Opts.Description
	if desc == "" {
		desc = "(无描述)"
	}
	return fmt.Sprintf("- /%s — %s", n.Name, desc)
}

// FormatHelp 为指定节点生成完整帮助文本（含其子命令）。
// 若 node 为 nil，则生成顶层命令列表。
func (r *CommandRegistry) FormatHelp(path []string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(path) == 0 {
		// 列出顶层
		top := listChildrenSorted(r.root)
		if len(top) == 0 {
			return "暂无可用命令"
		}
		lines := []string{"可用命令："}
		for _, c := range top {
			lines = append(lines, formatHelpLine(c))
		}
		lines = append(lines, "\n使用 /help <命令> 查看该命令的子命令与用法。")
		return strings.Join(lines, "\n")
	}

	node := r.root
	for _, seg := range path {
		next, ok := node.Children[seg]
		if !ok {
			return fmt.Sprintf("未知命令: /%s", strings.Join(path, " "))
		}
		node = next
	}

	var lines []string
	if node.Handler != nil {
		usage := node.Opts.Usage
		if usage == "" {
			usage = "/" + strings.Join(path, " ")
		}
		lines = append(lines, "用法: "+usage)
		if node.Opts.Description != "" {
			lines = append(lines, "描述: "+node.Opts.Description)
		}
	} else {
		lines = append(lines, fmt.Sprintf("/%s 是一个命令分组：", strings.Join(path, " ")))
	}

	subs := listChildrenSorted(node)
	if len(subs) > 0 {
		lines = append(lines, "")
		lines = append(lines, "子命令：")
		for _, c := range subs {
			lines = append(lines, formatHelpLine(c))
		}
	}

	plugin := node.PluginName
	if plugin != "" {
		lines = append(lines, "")
		lines = append(lines, "来源插件: "+plugin)
	}

	return strings.Join(lines, "\n")
}
