package models

import (
	"database/sql/driver"
	"encoding/json"
)

// ---------- 通用 JSON 字段类型 ----------

// JSONMap 是 map[string]any 的 GORM 兼容类型。
type JSONMap map[string]any

func (j JSONMap) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONMap) Scan(value any) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, j)
}

// JSONSlice 是 []string 的 GORM 兼容类型。
type JSONSlice []string

func (j JSONSlice) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONSlice) Scan(value any) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, j)
}
