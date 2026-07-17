package adapter

import "encoding/json"

// --- 消息段 ---

// Segment 表示 OneBot11 消息段 (text/image/at/face 等)。
type Segment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// --- 事件类型 ---

// Event 是所有 OneBot11 事件的通用表示。
// PostType 决定具体填充哪个子类型字段。
type Event struct {
	PostType string          `json:"post_type"` // message / notice / request / meta_event
	Time     int64           `json:"time"`
	SelfID   int64           `json:"self_id"`
	Raw      json.RawMessage `json:"-"` // 原始 JSON

	// 消息事件
	Message *MessageEvent `json:"message,omitempty"`
	// 通知事件
	Notice *NoticeEvent `json:"notice,omitempty"`
	// 请求事件
	Request *RequestEvent `json:"request,omitempty"`
	// 元事件
	Meta *MetaEvent `json:"meta_event,omitempty"`
}

// MessageEvent 消息事件。
type MessageEvent struct {
	MessageType string    `json:"message_type"` // private / group
	SubType     string    `json:"sub_type"`     // friend / normal / anonymous / group_self
	MessageID   int64     `json:"message_id"`
	UserID      int64     `json:"user_id"`
	GroupID     int64     `json:"group_id,omitempty"`
	Message     []Segment `json:"message"`     // 结构化消息段
	RawMessage  string    `json:"raw_message"` // CQ 码格式原文
	Font        int       `json:"font"`
	Sender      struct {
		UserID   int64  `json:"user_id"`
		Nickname string `json:"nickname"`
		Sex      string `json:"sex"`
		Age      int    `json:"age"`
		Card     string `json:"card"`
	} `json:"sender"`
}

// NoticeEvent 通知事件 (群成员变动、戳一戳等)。
type NoticeEvent struct {
	NoticeType string `json:"notice_type"` // group_upload / group_admin / group_decrease / group_increase / group_ban / friend_add / group_recall / friend_recall / notify
	SubType    string `json:"sub_type"`
	UserID     int64  `json:"user_id"`
	GroupID    int64  `json:"group_id"`
	OperatorID int64  `json:"operator_id"`
	TargetID   int64  `json:"target_id"`
	File       *struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		BusID int64  `json:"busid"`
	} `json:"file,omitempty"`
	Duration int64 `json:"duration,omitempty"` // 禁言时长(秒)
}

// RequestEvent 请求事件 (好友/群申请)。
type RequestEvent struct {
	RequestType string `json:"request_type"` // friend / group
	SubType     string `json:"sub_type"`     // add / invite
	UserID      int64  `json:"user_id"`
	GroupID     int64  `json:"group_id"`
	Comment     string `json:"comment"`
	Flag        string `json:"flag"`
}

// MetaEvent 元事件 (生命周期、心跳)。
type MetaEvent struct {
	MetaEventType string `json:"meta_event_type"` // lifecycle / heartbeat
	SubType       string `json:"sub_type"`        // enable / disable / connect
	Status        any    `json:"status"`
	Interval      int64  `json:"interval"`
}

// --- API 类型 ---

type APIRequest struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params"`
	Echo   string         `json:"echo,omitempty"`
}

type APIResponse struct {
	Status  string `json:"status"`
	RetCode int64  `json:"retcode"`
	Data    any    `json:"data"`
	Echo    string `json:"echo,omitempty"`
	Msg     string `json:"msg,omitempty"`
}

// --- API 响应实体 ---

type LoginInfo struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
}

type StrangerInfo struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Sex      string `json:"sex"`
	Age      int    `json:"age"`
}

type FriendInfo struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Remark   string `json:"remark"`
}

type GroupInfo struct {
	GroupID        int64  `json:"group_id"`
	GroupName      string `json:"group_name"`
	MemberCount    int    `json:"member_count"`
	MaxMemberCount int    `json:"max_member_count"`
}

type GroupMemberInfo struct {
	GroupID         int64  `json:"group_id"`
	UserID          int64  `json:"user_id"`
	Nickname        string `json:"nickname"`
	Card            string `json:"card"`
	Sex             string `json:"sex"`
	Age             int    `json:"age"`
	Area            string `json:"area"`
	JoinTime        int64  `json:"join_time"`
	LastSentTime    int64  `json:"last_sent_time"`
	Level           string `json:"level"`
	Role            string `json:"role"` // owner / admin / member
	Unfriendly      bool   `json:"unfriendly"`
	Title           string `json:"title"`
	TitleExpireTime int64  `json:"title_expire_time"`
	CardChangeable  bool   `json:"card_changeable"`
}

type GroupHonorInfo struct {
	GroupID          int64               `json:"group_id"`
	CurrentTalkative TalkativeHonor      `json:"current_talkative"`
	TalkativeList    []TalkativeHonor    `json:"talkative_list"`
	PerformerList    []PerformerHonor    `json:"performer_list"`
	LegendList       []LegendHonor       `json:"legend_list"`
	StrongNewbieList []StrongNewbieHonor `json:"strong_newbie_list"`
	EmotionList      []EmotionHonor      `json:"emotion_list"`
}

type TalkativeHonor struct {
	UserID      int64  `json:"user_id"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	DayCount    int    `json:"day_count"`
	Description string `json:"description"`
}

type PerformerHonor struct {
	UserID      int64  `json:"user_id"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
}

type LegendHonor struct {
	UserID      int64  `json:"user_id"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
}

type StrongNewbieHonor struct {
	UserID      int64  `json:"user_id"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
}

type EmotionHonor struct {
	UserID      int64  `json:"user_id"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
}

type FileInfo struct {
	File string `json:"file"`
	URL  string `json:"url,omitempty"`
}

type Status struct {
	Online bool `json:"online"`
	Good   bool `json:"good"`
}

type VersionInfo struct {
	AppName         string `json:"app_name"`
	AppVersion      string `json:"app_version"`
	ProtocolVersion string `json:"protocol_version"`
}

type MessageData struct {
	MessageID int64 `json:"message_id"`
}

type Cookies struct {
	Cookies string `json:"cookies"`
}

type CSRF struct {
	Token int `json:"token"`
}

type Credentials struct {
	Cookies string `json:"cookies"`
	Token   int    `json:"token"`
}

type ForwardNode struct {
	ID      int64  `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Uin     int64  `json:"uin,omitempty"`
	Content any    `json:"content,omitempty"`
}

// ProviderStatus 适配器运行状态。
type ProviderStatus struct {
	Running    bool    `json:"running"`
	ListenAddr string  `json:"listen_addr"`
	SelfID     int64   `json:"self_id,omitempty"`
	ConnCount  int     `json:"conn_count"`
	ConnIDs    []int64 `json:"conn_ids,omitempty"`
}
