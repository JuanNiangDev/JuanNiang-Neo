package groupmgr

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
)

// 命令话术（平移自旧插件，多套随机）。
var (
	pardonTemplates = struct {
		ok, whitelisted, denied, usage []string
	}{
		ok: []string{
			"好啦，%d 的禁言已解除，违规记录也清空咯～不过 TA 还在正常检测范围内哦～",
			"收到！%d 解禁 + 违规清零完成，卷娘会继续盯着 TA 的～",
			"%d 这次先放过：解除禁言、清空违规记录～下次再犯可就要按规矩来咯～",
		},
		whitelisted: []string{"%d 本来就在白名单里，不用豁免啦～卷娘顺手帮 TA 解了禁言、清了记录～"},
		denied: []string{
			"只有管理员才能用豁免哦～",
			"豁免是管理员的专属技能啦，你不行哦～",
		},
		usage: []string{"用法：/豁免 QQ号 或 /豁免 @某人 哦～"},
	}

	whitelistTemplates = struct {
		ok, denied, usage, already []string
	}{
		ok: []string{
			"好啦，%d 已经被卷娘记到白名单小本本上啦，违规记录也清空咯～",
			"收到！%d 已加入白名单，以后可以放心发言，卷娘不会管 TA 啦，违规记录已清空～",
			"%d 获得免死金牌一枚！卷娘已将 TA 加入白名单，并清空了违规记录哦～",
		},
		denied: []string{
			"只有管理员才能用白名单哦～",
			"白名单是管理员的专属技能啦，你不行哦～",
		},
		usage:   []string{"用法：/白名单 QQ号 或 /白名单 @某人 哦～"},
		already: []string{"%d 早就在卷娘的白名单里啦～"},
	}

	unexemptTemplates = struct {
		ok, notFound, usage []string
	}{
		ok: []string{
			"好啦，%d 已移出白名单，回归正常检测咯～",
			"收到！%d 已从白名单移除，之后会正常检测啦～",
			"免死金牌收回！%d 移出白名单，卷娘继续盯着 TA 哦～",
		},
		notFound: []string{"%d 本来就不在白名单里啦～"},
		usage:    []string{"用法：/解除豁免 QQ号 或 /解除豁免 @某人 哦～"},
	}
)

func pick(msgs []string, fmtArg ...any) string {
	m := msgs[rand.Intn(len(msgs))]
	if len(fmtArg) > 0 {
		return fmt.Sprintf(m, fmtArg...)
	}
	return m
}

var atQQRe = regexp.MustCompile(`\[CQ:at,qq=(\d+)`)

// ParseTargetQQ 解析 /白名单 /豁免 参数（QQ 号或 @某人）。
func ParseTargetQQ(args []string) int64 {
	if len(args) == 0 {
		return 0
	}
	raw := args[0]
	if m := atQQRe.FindStringSubmatch(raw); m != nil {
		qq, _ := strconv.ParseInt(m[1], 10, 64)
		return qq
	}
	qq, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return qq
}

// IsCommandAdmin 命令权限校验（系统/手动管理员 + 群角色 owner/admin）。
func (m *Manager) IsCommandAdmin(groupID, userID int64, admins []string) bool {
	return m.isGroupAdmin(userID, admins, groupID)
}

// CommandGroupStats /groupstats 统计文本。
func (m *Manager) CommandGroupStats(groupID int64) string {
	return m.StatsText(context.Background(), groupID)
}

// CommandPardon /豁免：解除禁言 + 清空违规记录（不加入白名单）。
func (m *Manager) CommandPardon(groupID, targetQQ int64) string {
	if m.isWhitelisted(context.Background(), targetQQ) {
		m.unmuteAndClear(groupID, targetQQ)
		return pick(pardonTemplates.whitelisted, targetQQ)
	}
	m.unmuteAndClear(groupID, targetQQ)
	return pick(pardonTemplates.ok, targetQQ)
}

// CommandWhitelist /白名单：加入白名单 + 清违规（全局语义）+ 解禁言。
func (m *Manager) CommandWhitelist(groupID, targetQQ int64) string {
	ctx := context.Background()
	if m.isWhitelisted(ctx, targetQQ) {
		m.unmuteAndClear(groupID, targetQQ)
		return pick(whitelistTemplates.already, targetQQ)
	}
	if err := m.dao.WlAdd(ctx, targetQQ); err != nil {
		log.Warn("白名单写入失败", "qq", targetQQ, "err", err)
	}
	_ = m.Reload(ctx)
	// 白名单是全局豁免：清空该用户全部群的违规记录（含当前群）+ 解除当前群禁言
	if _, err := m.dao.ViolationClearUserAll(ctx, targetQQ); err != nil {
		log.Warn("违规记录全局清除失败", "qq", targetQQ, "err", err)
	}
	m.unbanOnly(groupID, targetQQ)
	return pick(whitelistTemplates.ok, targetQQ)
}

// CommandUnexempt /解除豁免：移出白名单，恢复检测。
func (m *Manager) CommandUnexempt(targetQQ int64) string {
	ctx := context.Background()
	if !m.isWhitelisted(ctx, targetQQ) {
		return pick(unexemptTemplates.notFound, targetQQ)
	}
	if err := m.dao.WlDelete(ctx, targetQQ); err != nil {
		log.Warn("白名单移除失败", "qq", targetQQ, "err", err)
	}
	_ = m.Reload(ctx)
	return pick(unexemptTemplates.ok, targetQQ)
}

// CommandPardonDenied / 白名单权限拒绝话术（闭包层判断后用）。
func CommandPardonDenied() string    { return pick(pardonTemplates.denied) }
func CommandPardonUsage() string     { return pick(pardonTemplates.usage) }
func CommandWhitelistDenied() string { return pick(whitelistTemplates.denied) }
func CommandWhitelistUsage() string  { return pick(whitelistTemplates.usage) }
func CommandUnexemptUsage() string   { return pick(unexemptTemplates.usage) }

// unmuteAndClear /豁免：清空指定群的违规记录 + 解禁言（不加入白名单）。
// 按群清除：避免 /豁免 把该用户在其它群的三级惩罚阶梯一并清零。
func (m *Manager) unmuteAndClear(groupID, targetQQ int64) {
	ctx := context.Background()
	if _, err := m.dao.ViolationClearUser(ctx, groupID, targetQQ); err != nil {
		log.Warn("违规记录清除失败", "qq", targetQQ, "group", groupID, "err", err)
	}
	m.unbanOnly(groupID, targetQQ)
}

// unbanOnly 仅解除禁言（duration=0 为 OneBot11 规范解禁语义）。
// 不依赖 shut_up_timestamp 判断——部分实现该字段缺失/失效；对未禁言成员是无害 no-op。
func (m *Manager) unbanOnly(groupID, targetQQ int64) {
	if err := m.adp.BanGroupMember(groupID, targetQQ, 0); err != nil {
		log.Warn("解除禁言失败", "qq", targetQQ, "group", groupID, "err", err)
	}
}
