package pluggin

import (
	"encoding/json"
	"fmt"

	"JuanNiang-Neo/internal/adapter"
)

// AdapterWrapper 将 adapter.Provider 包装为 SendAdapter 接口。
type AdapterWrapper struct {
	p *adapter.Adapter
}

func WrapAdapter(p *adapter.Adapter) SendAdapter {
	return &AdapterWrapper{p: p}
}

func (a *AdapterWrapper) SendPrivateMsg(userID int64, message any) (int64, error) {
	return a.p.SendPrivateMsg(userID, message)
}

func (a *AdapterWrapper) SendGroupMsg(groupID int64, message any) (int64, error) {
	return a.p.SendGroupMsg(groupID, message)
}

func (a *AdapterWrapper) SendGroupForwardMsg(groupID int64, nodes []adapter.ForwardNode) (int64, error) {
	return a.p.SendGroupForwardMsg(groupID, nodes)
}

func (a *AdapterWrapper) SendPrivateForwardMsg(userID int64, nodes []adapter.ForwardNode) (int64, error) {
	return a.p.SendPrivateForwardMsg(userID, nodes)
}

// GetForwardMsg 读取合并转发卡片的内层消息节点。
func (a *AdapterWrapper) GetForwardMsg(forwardID string) ([]adapter.ForwardNode, error) {
	return a.p.GetForwardMsg(forwardID)
}

func (a *AdapterWrapper) DeleteMsg(messageID int64) error {
	return a.p.DeleteMsg(messageID)
}

func (a *AdapterWrapper) GetMsg(messageID int64) (map[string]any, error) {
	msg, err := a.p.GetMsg(messageID)
	if err != nil {
		return nil, err
	}
	return toMap(msg), nil
}

// GetGroupMsgHistory 获取群历史消息（升序），用于插件在断线重连后补全漏收消息。
func (a *AdapterWrapper) GetGroupMsgHistory(groupID, messageSeq int64, count int) ([]adapter.MessageEvent, error) {
	return a.p.GetGroupMsgHistory(groupID, messageSeq, count)
}

// GetImage 用 QQ 图片文件哈希换一条新的可下载链接（原始 rkey 会过期）。
func (a *AdapterWrapper) GetImage(file string) (*adapter.FileInfo, error) {
	return a.p.GetImage(file)
}

func (a *AdapterWrapper) GetGroupInfo(groupID int64) (map[string]any, error) {
	info, err := a.p.GetGroupInfo(groupID)
	if err != nil {
		return nil, err
	}
	return toMap(info), nil
}

func (a *AdapterWrapper) GetGroupMemberList(groupID int64) ([]map[string]any, error) {
	list, err := a.p.GetGroupMemberList(groupID)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, len(list))
	for i, m := range list {
		result[i] = toMap(m)
	}
	return result, nil
}

func (a *AdapterWrapper) KickGroupMember(groupID, userID int64, rejectAdd bool) error {
	return a.p.KickGroupMember(groupID, userID, rejectAdd)
}

func (a *AdapterWrapper) BanGroupMember(groupID, userID int64, duration int) error {
	return a.p.BanGroupMember(groupID, userID, duration)
}

func (a *AdapterWrapper) SetGroupWholeBan(groupID int64, enable bool) error {
	return a.p.SetGroupWholeBan(groupID, enable)
}

func (a *AdapterWrapper) SetGroupCard(groupID, userID int64, card string) error {
	return a.p.SetGroupCard(groupID, userID, card)
}

func (a *AdapterWrapper) HandleFriendRequest(flag string, approve bool, remark string) error {
	return a.p.HandleFriendRequest(flag, approve, remark)
}

func (a *AdapterWrapper) HandleGroupRequest(flag, subType string, approve bool, reason string) error {
	return a.p.HandleGroupRequest(flag, subType, approve, reason)
}

func (a *AdapterWrapper) GetLoginInfo() (map[string]any, error) {
	info, err := a.p.GetLoginInfo()
	if err != nil {
		return nil, err
	}
	return toMap(info), nil
}

func (a *AdapterWrapper) GetStrangerInfo(userID int64) (map[string]any, error) {
	info, err := a.p.GetStrangerInfo(userID)
	if err != nil {
		return nil, err
	}
	return toMap(info), nil
}

func (a *AdapterWrapper) GetFriendList() ([]map[string]any, error) {
	list, err := a.p.GetFriendList()
	if err != nil {
		return nil, err
	}
	return toMapSlice(list), nil
}

func (a *AdapterWrapper) GetGroupList() ([]map[string]any, error) {
	list, err := a.p.GetGroupList()
	if err != nil {
		return nil, err
	}
	return toMapSlice(list), nil
}

func (a *AdapterWrapper) GetGroupMemberInfo(groupID, userID int64) (map[string]any, error) {
	info, err := a.p.GetGroupMemberInfo(groupID, userID)
	if err != nil {
		return nil, err
	}
	return toMap(info), nil
}

func (a *AdapterWrapper) GetGroupHonorInfo(groupID int64) (map[string]any, error) {
	info, err := a.p.GetGroupHonorInfo(groupID)
	if err != nil {
		return nil, err
	}
	return toMap(info), nil
}

func (a *AdapterWrapper) SendLike(userID int64, times int) error {
	return a.p.SendLike(userID, times)
}

func (a *AdapterWrapper) GetStatus() (map[string]any, error) {
	s := a.p.Status()
	return toMap(s), nil
}

func (a *AdapterWrapper) GetVersionInfo() (map[string]any, error) {
	info, err := a.p.GetVersionInfo()
	if err != nil {
		return nil, err
	}
	return toMap(info), nil
}

func toMap(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("marshal failed: %v", err)}
	}
	var m map[string]any
	json.Unmarshal(data, &m)
	return m
}

func toMapSlice[T any](list []T) []map[string]any {
	result := make([]map[string]any, len(list))
	for i, v := range list {
		result[i] = toMap(v)
	}
	return result
}
