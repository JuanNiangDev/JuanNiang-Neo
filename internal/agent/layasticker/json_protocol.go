package layasticker

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func BuildJSONTemplate(template string, request DecisionRequest) ([]byte, error) {
	if strings.TrimSpace(template) == "" {
		return nil, fmt.Errorf("Laya JSON 请求模板不能为空")
	}
	prepared, err := prepareRawPlaceholders(template)
	if err != nil {
		return nil, err
	}
	var document any
	if err := json.Unmarshal([]byte(prepared), &document); err != nil {
		return nil, fmt.Errorf("请求模板不是有效 JSON: %w", err)
	}
	values := map[string]any{
		"user_message":       request.UserMessage,
		"assistant_reply":    request.AssistantReply,
		"message_type":       request.MessageType,
		"model":              request.Model,
		"categories":         request.Categories,
		"enabled_categories": request.EnabledCategories,
		"criteria":           request.Criteria,
		"criteria_map":       request.CriteriaMap,
	}
	replaced, err := replaceTemplateValue(document, values)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(replaced)
	if err != nil {
		return nil, fmt.Errorf("序列化 Laya 请求失败: %w", err)
	}
	return body, nil
}

// ValidateJSONTemplate checks syntax and supported placeholder names without
// making an HTTP request. It is used by configuration updates so malformed
// templates fail before the next chat turn.
func ValidateJSONTemplate(template string) error {
	if strings.TrimSpace(template) == "" {
		return fmt.Errorf("Laya JSON 请求模板不能为空")
	}
	prepared, err := prepareRawPlaceholders(template)
	if err != nil {
		return err
	}
	var document any
	if err := json.Unmarshal([]byte(prepared), &document); err != nil {
		return fmt.Errorf("请求模板不是有效 JSON: %w", err)
	}
	_, err = replaceTemplateValue(document, templateValidationValues())
	return err
}

func templateValidationValues() map[string]any {
	return map[string]any{
		"user_message":       "",
		"assistant_reply":    "",
		"message_type":       "",
		"model":              "",
		"categories":         []Category(nil),
		"enabled_categories": []Category(nil),
		"criteria":           []string(nil),
		"criteria_map":       map[string]string(nil),
	}
}

// prepareRawPlaceholders quotes placeholders that occupy a JSON value position
// so arrays and objects can be represented by {{categories}} without making the
// template itself invalid JSON. Placeholders already inside JSON strings remain
// untouched and are handled as scalar interpolation later.
func prepareRawPlaceholders(template string) (string, error) {
	var out strings.Builder
	inString := false
	escaped := false
	for i := 0; i < len(template); {
		if template[i] == '"' && !escaped {
			inString = !inString
			out.WriteByte(template[i])
			i++
			escaped = false
			continue
		}
		if template[i] == '\\' && inString && !escaped {
			escaped = true
			out.WriteByte(template[i])
			i++
			continue
		}
		if escaped {
			escaped = false
			out.WriteByte(template[i])
			i++
			continue
		}
		if !inString && strings.HasPrefix(template[i:], "{{") {
			end := strings.Index(template[i+2:], "}}")
			if end < 0 {
				return "", fmt.Errorf("请求模板变量缺少结束标记")
			}
			name := strings.TrimSpace(template[i+2 : i+2+end])
			if name == "" {
				return "", fmt.Errorf("请求模板变量名不能为空")
			}
			out.WriteString(`"`)
			out.WriteString(templateToken(name))
			out.WriteString(`"`)
			i += end + 4
			continue
		}
		out.WriteByte(template[i])
		i++
	}
	if inString {
		return "", fmt.Errorf("请求模板字符串未闭合")
	}
	return out.String(), nil
}

func templateToken(name string) string {
	return "__LAYA_TEMPLATE_" + name + "__"
}

func templateTokenName(value string) (string, bool) {
	if !strings.HasPrefix(value, "__LAYA_TEMPLATE_") || !strings.HasSuffix(value, "__") {
		return "", false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(value, "__LAYA_TEMPLATE_"), "__")
	return name, name != ""
}

func replaceTemplateValue(value any, variables map[string]any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			replaced, err := replaceTemplateValue(child, variables)
			if err != nil {
				return nil, err
			}
			v[key] = replaced
		}
		return v, nil
	case []any:
		for i, child := range v {
			replaced, err := replaceTemplateValue(child, variables)
			if err != nil {
				return nil, err
			}
			v[i] = replaced
		}
		return v, nil
	case string:
		return replaceTemplateString(v, variables)
	default:
		return value, nil
	}
}

func replaceTemplateString(value string, variables map[string]any) (any, error) {
	if name, ok := templateTokenName(value); ok {
		variable, exists := variables[name]
		if !exists {
			return nil, fmt.Errorf("未知请求模板变量: %s", name)
		}
		return variable, nil
	}
	if strings.HasPrefix(value, "{{") && strings.HasSuffix(value, "}}") && strings.Count(value, "{{") == 1 {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "{{"), "}}"))
		variable, ok := variables[name]
		if !ok {
			return nil, fmt.Errorf("未知请求模板变量: %s", name)
		}
		return variable, nil
	}
	if !strings.Contains(value, "{{") {
		if !strings.Contains(value, "__LAYA_TEMPLATE_") {
			return value, nil
		}
		var out strings.Builder
		for {
			start := strings.Index(value, "__LAYA_TEMPLATE_")
			if start < 0 {
				out.WriteString(value)
				break
			}
			out.WriteString(value[:start])
			value = value[start:]
			end := strings.Index(value[len("__LAYA_TEMPLATE_"):], "__")
			if end < 0 {
				return nil, fmt.Errorf("请求模板变量标记无效")
			}
			end += len("__LAYA_TEMPLATE_") + 2
			name, ok := templateTokenName(value[:end])
			if !ok {
				return nil, fmt.Errorf("请求模板变量标记无效")
			}
			variable, exists := variables[name]
			if !exists {
				return nil, fmt.Errorf("未知请求模板变量: %s", name)
			}
			if isStructuredTemplateValue(variable) {
				return nil, fmt.Errorf("结构化变量 %s 不能内联到字符串", name)
			}
			out.WriteString(fmt.Sprint(variable))
			value = value[end:]
		}
		return out.String(), nil
	}
	var out strings.Builder
	for {
		start := strings.Index(value, "{{")
		if start < 0 {
			out.WriteString(value)
			break
		}
		out.WriteString(value[:start])
		value = value[start:]
		end := strings.Index(value, "}}")
		if end < 0 {
			return nil, fmt.Errorf("请求模板变量缺少结束标记")
		}
		name := strings.TrimSpace(value[2:end])
		variable, ok := variables[name]
		if !ok {
			return nil, fmt.Errorf("未知请求模板变量: %s", name)
		}
		switch scalar := variable.(type) {
		case string:
			out.WriteString(scalar)
		case nil, bool, float64, float32, int, int32, int64, uint, uint32, uint64:
			out.WriteString(fmt.Sprint(scalar))
		default:
			return nil, fmt.Errorf("结构化变量 %s 不能内联到字符串", name)
		}
		value = value[end+2:]
	}
	return out.String(), nil
}

func isStructuredTemplateValue(value any) bool {
	if value == nil {
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Array || kind == reflect.Slice || kind == reflect.Map || kind == reflect.Struct
}

func ParseJSONPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("JSON Pointer 必须以 / 开头")
	}
	parts := strings.Split(pointer[1:], "/")
	for i, part := range parts {
		var decoded strings.Builder
		for j := 0; j < len(part); j++ {
			if part[j] != '~' {
				decoded.WriteByte(part[j])
				continue
			}
			if j+1 >= len(part) || (part[j+1] != '0' && part[j+1] != '1') {
				return nil, fmt.Errorf("JSON Pointer 转义无效")
			}
			if part[j+1] == '0' {
				decoded.WriteByte('~')
			} else {
				decoded.WriteByte('/')
			}
			j++
		}
		parts[i] = decoded.String()
	}
	return parts, nil
}

func ExtractJSONPointer(document any, pointer string) (any, error) {
	parts, err := ParseJSONPointer(pointer)
	if err != nil {
		return nil, err
	}
	current := document
	for _, part := range parts {
		switch value := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = value[part]
			if !ok {
				return nil, fmt.Errorf("JSON Pointer 字段不存在: %s", part)
			}
		case []any:
			index, parseErr := strconv.Atoi(part)
			if parseErr != nil || index < 0 || index >= len(value) {
				return nil, fmt.Errorf("JSON Pointer 数组索引无效: %s", part)
			}
			current = value[index]
		default:
			return nil, fmt.Errorf("JSON Pointer 无法继续解析")
		}
	}
	return current, nil
}
