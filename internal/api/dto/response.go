package dto

import (
	"time"
)

var (
	OK                  = Response{Status: 0, Info: "OK"}
	ServerInternalErr   = Response{Status: 50000, Info: "服务器内部错误"}
	BindJSONErr         = Response{Status: 40001, Info: "参数格式错误"}
	UserOrPasswordWrong = Response{Status: 40002, Info: "用户名或密码错误"}
	GenTokenFail        = Response{Status: 40003, Info: "token 生成失败"}
	UserNotExists       = Response{Status: 40004, Info: "用户不存在"}
	OriginPasswordWrong = Response{Status: 40005, Info: "原密码错误"}
	UpdatePasswordFail  = Response{Status: 40006, Info: "密码更新失败"}
	InvalidQQNumber     = Response{Status: 40007, Info: "无效的 QQ 号"}
	AdapterNotReady     = Response{Status: 40008, Info: "adapter 未初始化"}
	ProviderNotExist    = Response{Status: 40009, Info: "provider 不存在"}
	MCPNotExist         = Response{Status: 40010, Info: "MCP 服务器不存在"}
	SessionNotExist     = Response{Status: 40011, Info: "Session 不存在"}
	EmptyFileToUpload   = Response{Status: 40012, Info: "缺少上传文件"}
	TempFileCreateFail  = Response{Status: 40013, Info: "临时文件创建失败"}
	WriteFileFail       = Response{Status: 40014, Info: "文件写入失败"}
	InvalidZipFile      = Response{Status: 40015, Info: "无效的 ZIP 文件"}
	InvalidACLID        = Response{Status: 40016, Info: "无效的 ACL ID"}
)

type TokenResp struct {
	Token string `json:"token"`
}

type ErrorDetail struct {
	ErrorDetail string `json:"error_detail"`
}

type AdminQQNumbers struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type ProviderStatus struct {
	Running    bool    `json:"running"`
	ListenAddr string  `json:"listen_addr"`
	SelfID     int64   `json:"self_id"`
	ConnCount  int     `json:"conn_count"`
	ConnIDs    []int64 `json:"conn_ids"`
}
