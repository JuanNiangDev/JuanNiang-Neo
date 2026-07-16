package provider

type Platform string

type MsgType string

const (
	PlatformOneBot11 Platform = "onebot11"
)

const (
	MsgTypeText  MsgType = "text"
	MsgTypeImage MsgType = "image"
	MsgTypeFile  MsgType = "file"
	MsgTypeEvent MsgType = "event"
	MsgTypeVoice MsgType = "voice"
)

type Message struct {
	ID          string
	Platform    Platform
	MessageType MsgType
	Content     string
	SenderID    string
	SenderName  string
	GroupID     string
	RawMessage  any
}
