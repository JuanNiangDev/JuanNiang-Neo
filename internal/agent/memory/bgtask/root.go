package bgtask

import "encoding/json"

type BackGroundTaskMetaInfo struct {
	Running  bool
	Metadata json.RawMessage
}

type BackGroundTaskMemory struct {
	taskMemory map[string]BackGroundTaskMetaInfo
}
