package pluggin

import (
	"errors"
	"strings"
	"testing"
	"time"

	"JuanNiang-Neo/internal/adapter"
)

// ====================================================================
// 新增插件 API 回归：
//  1. jn.onebot11.get_forward_msg_sync(forward_id) —— 读取合并转发内层节点
//     （daily_analysis 需要把群里搬运的转发内容算进总结语料）
//  2. jn.llm.vision_url(provider, url, prompt) —— 用 image_model Provider
//     描述网络图片（图片内容此前对总结模型完全不可见）
// ====================================================================

// 本文件的断言助手（不依赖其它测试文件，独立可编译）
func fvGroupsSnapshot(t *testing.T, adp *fakeAdapter) []string {
	t.Helper()
	adp.mu.Lock()
	defer adp.mu.Unlock()
	return append([]string(nil), adp.groups...)
}

func fvGroupsContain(t *testing.T, adp *fakeAdapter, substr string) bool {
	t.Helper()
	for _, g := range fvGroupsSnapshot(t, adp) {
		if strings.Contains(g, substr) {
			return true
		}
	}
	return false
}

func fvWaitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

func forwardNodesFixture() []adapter.ForwardNode {
	return []adapter.ForwardNode{
		{
			Name: "张三", Uin: 1001,
			Content: []any{
				map[string]any{"type": "text", "data": map[string]any{"text": "第一条转发正文"}},
			},
		},
		{
			Name: "李四", Uin: 1002,
			Content: []any{
				map[string]any{"type": "text", "data": map[string]any{"text": "第二条转发正文"}},
			},
		},
	}
}

func TestGetForwardMsgSyncBinding(t *testing.T) {
	pe, adp := newMiniTestEngine(t, &fakeLLM{})
	adp.forwardNodes = forwardNodesFixture()

	L := loadMiniPlugin(t, pe, "fwdread", `
local jn = require("jn")
function on_message(event)
    local nodes, err = jn.onebot11.get_forward_msg_sync("FWD-1")
    if err then
        jn.onebot11.send_group_msg(100, "ERR:" .. tostring(err))
        return false, nil
    end
    local first = nodes[1] or {}
    local second = nodes[2] or {}
    jn.onebot11.send_group_msg(100, string.format("N=%d names=%s/%s", #nodes,
        tostring(first.name), tostring(second.name)))
    return false, nil
end
`)
	runOnMessage(t, pe, L, 10001, 20001, "hi", "90001")

	fvWaitFor(t, func() bool { return len(fvGroupsSnapshot(t, adp)) > 0 })
	if !fvGroupsContain(t, adp, "N=2 names=张三/李四") {
		t.Fatalf("转发节点读取异常: %v", fvGroupsSnapshot(t, adp))
	}
}

func TestGetForwardMsgSyncBindingError(t *testing.T) {
	pe, adp := newMiniTestEngine(t, &fakeLLM{})
	adp.forwardErr = errors.New("forward not found")

	L := loadMiniPlugin(t, pe, "fwderr", `
local jn = require("jn")
function on_message(event)
    local nodes, err = jn.onebot11.get_forward_msg_sync("BAD")
    if nodes ~= nil then
        jn.onebot11.send_group_msg(100, "UNEXPECTED_NODES")
    elseif err and err ~= "" then
        jn.onebot11.send_group_msg(100, "ERR_OK")
    end
    return false, nil
end
`)
	runOnMessage(t, pe, L, 10001, 20001, "hi", "90002")

	fvWaitFor(t, func() bool { return fvGroupsContain(t, adp, "ERR_OK") })
	if !fvGroupsContain(t, adp, "ERR_OK") {
		t.Fatalf("失败应返回 nil+err: %v", fvGroupsSnapshot(t, adp))
	}
}

func TestVisionURLBinding(t *testing.T) {
	llm := &fakeLLM{visionReply: "一张橘猫的截图"}
	pe, adp := newMiniTestEngine(t, llm)

	L := loadMiniPlugin(t, pe, "visiontest", `
local jn = require("jn")
function on_message(event)
    local text, err = jn.llm.vision_url("wb-api-image", "https://example.com/a.jpg", "描述这张图")
    if err then
        jn.onebot11.send_group_msg(100, "ERR:" .. tostring(err))
        return false, nil
    end
    jn.onebot11.send_group_msg(100, "DESC:" .. tostring(text))
    return false, nil
end
`)
	runOnMessage(t, pe, L, 10001, 20001, "hi", "90003")

	fvWaitFor(t, func() bool { return fvGroupsContain(t, adp, "DESC:一张橘猫的截图") })
	if !fvGroupsContain(t, adp, "DESC:一张橘猫的截图") {
		t.Fatalf("视觉描述未返回: %v", fvGroupsSnapshot(t, adp))
	}
	llm.mu.Lock()
	defer llm.mu.Unlock()
	if llm.visionCalls != 1 || llm.visionProvider != "wb-api-image" || llm.visionURL != "https://example.com/a.jpg" {
		t.Fatalf("视觉调用参数异常: calls=%d provider=%q url=%q", llm.visionCalls, llm.visionProvider, llm.visionURL)
	}
}

func TestVisionURLBindingError(t *testing.T) {
	llm := &fakeLLM{visionErr: errors.New("no image provider")}
	pe, adp := newMiniTestEngine(t, llm)

	L := loadMiniPlugin(t, pe, "visionerr", `
local jn = require("jn")
function on_message(event)
    local text, err = jn.llm.vision_url("", "https://example.com/b.png", "描述")
    if text ~= nil then
        jn.onebot11.send_group_msg(100, "UNEXPECTED_TEXT")
    elseif err then
        jn.onebot11.send_group_msg(100, "ERR_OK")
    end
    return false, nil
end
`)
	runOnMessage(t, pe, L, 10001, 20001, "hi", "90004")

	fvWaitFor(t, func() bool { return fvGroupsContain(t, adp, "ERR_OK") })
	if !fvGroupsContain(t, adp, "ERR_OK") {
		t.Fatalf("视觉失败应返回 nil+err: %v", fvGroupsSnapshot(t, adp))
	}
}

func TestGetImageBinding(t *testing.T) {
	pe, adp := newMiniTestEngine(t, &fakeLLM{})
	L := loadMiniPlugin(t, pe, "getimagetest", `
local jn = require("jn")
function on_message(event)
    local info, err = jn.onebot11.get_image("ABC.jpg")
    if info and info.file == "ABC.jpg" and info.url == "https://img.example/ABC.jpg" then
        jn.onebot11.send_group_msg(100, "IMAGE_OK")
    else
        jn.onebot11.send_group_msg(100, "IMAGE_ERR:" .. tostring(err))
    end
    return false, nil
end
`)
	runOnMessage(t, pe, L, 10001, 20001, "hi", "90005")

	fvWaitFor(t, func() bool { return fvGroupsContain(t, adp, "IMAGE_OK") })
	if !fvGroupsContain(t, adp, "IMAGE_OK") {
		t.Fatalf("get_image 应返回图片信息: %v", fvGroupsSnapshot(t, adp))
	}
}
