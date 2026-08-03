package caller

// ImageType 图片格式
type ImageType string

const (
	ImageTypeJPEG ImageType = "jpeg"
	ImageTypePNG  ImageType = "png"
)

// ScaleLevel 设备像素比级别
type ScaleLevel string

const (
	ScaleNormal ScaleLevel = "normal" // 1.0
	ScaleHigh   ScaleLevel = "high"   // 1.3
	ScaleUltra  ScaleLevel = "ultra"  // 1.8
)

// Animation 动画处理
type Animation string

const (
	AnimationAllow    Animation = "allow"
	AnimationDisabled Animation = "disabled"
)

// Caret 光标处理
type Caret string

const (
	CaretHide Caret = "hide"
	CaretShow Caret = "initial"
)

// ────────────────────── 请求模型 ──────────────────────

type GenerateOptions struct {
	Timeout           float64    `json:"timeout,omitempty"`
	Type              ImageType  `json:"type,omitempty"`
	Quality           int        `json:"quality,omitempty"`         // 仅 JPEG 有效
	OmitBackground    bool       `json:"omit_background,omitempty"` // 透明背景 (PNG)
	FullPage          *bool      `json:"full_page,omitempty"`       // 默认 true
	ViewportWidth     int        `json:"viewport_width,omitempty"`
	ViewportHeight    int        `json:"viewport_height,omitempty"`
	Scale             string     `json:"scale,omitempty"` // "css" | "device"
	Animations        Animation  `json:"animations,omitempty"`
	Caret             Caret      `json:"caret,omitempty"`
	DeviceScaleFactor ScaleLevel `json:"device_scale_factor_level,omitempty"`
}

type GenerateRequest struct {
	HTML         string           `json:"html,omitempty"`
	Template     string           `json:"tmpl,omitempty"`
	TemplateData map[string]any   `json:"tmpldata,omitempty"`
	AsJSON       bool             `json:"json"`
	Options      *GenerateOptions `json:"options,omitempty"`
}

// ────────────────────── 响应模型 ──────────────────────

type GenerateResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID string `json:"id"`
	} `json:"data"`
}
