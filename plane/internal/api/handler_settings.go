package api

import (
	"encoding/json"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"

	"github.com/gin-gonic/gin"
)

// SettingsHandler 设置处理器
type SettingsHandler struct {
	app *App
}

// NewSettingsHandler 创建设置处理器
func NewSettingsHandler(app *App) *SettingsHandler {
	return &SettingsHandler{app: app}
}

// GetCaptchaSettings 获取验证码设置
func (h *SettingsHandler) GetCaptchaSettings(c *gin.Context) {
	setting, err := h.app.DB.DB.SQLite.GetSystemSetting("captcha")
	if err != nil {
		response.InternalError(c, "Failed to get settings")
		return
	}

	if setting == nil {
		// 返回默认设置
		response.Success(c, h.app.Config.Captcha)
		return
	}

	var captchaSettings dbinit.CaptchaSettings
	if err := json.Unmarshal([]byte(setting.Value), &captchaSettings); err != nil {
		response.InternalError(c, "Failed to parse settings")
		return
	}

	response.Success(c, captchaSettings)
}

// UpdateCaptchaSettingsRequest 更新验证码设置请求
type UpdateCaptchaSettingsRequest struct {
	Enabled            bool   `json:"enabled"`
	Type               string `json:"type"`
	EnableLogin        bool   `json:"enable_login"`
	EnableRegister     bool   `json:"enable_register"`
	ImageWidth         int    `json:"image_width"`
	ImageHeight        int    `json:"image_height"`
	CodeLength         int    `json:"code_length"`
	Expiration         int    `json:"expiration"`
	TurnstileSiteKey   string `json:"turnstile_site_key"`
	TurnstileSecretKey string `json:"turnstile_secret_key"`
}

// UpdateCaptchaSettings 更新验证码设置
func (h *SettingsHandler) UpdateCaptchaSettings(c *gin.Context) {
	var req UpdateCaptchaSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")

	captchaSettings := dbinit.CaptchaSettings{
		Enabled:     req.Enabled,
		Type:        req.Type,
		SiteKey:     req.TurnstileSiteKey,
		SecretKey:   req.TurnstileSecretKey,
		ImageWidth:  req.ImageWidth,
		ImageHeight: req.ImageHeight,
		CodeLength:  req.CodeLength,
	}

	valueJSON, err := json.Marshal(captchaSettings)
	if err != nil {
		response.InternalError(c, "Failed to marshal settings")
		return
	}

	setting := &dbinit.SystemSettings{
		Key:         "captcha",
		Value:       string(valueJSON),
		Category:    "captcha",
		UpdatedBy:   userID.(string),
		Description: "验证码配置",
	}

	if err := h.app.DB.DB.SQLite.UpsertSystemSetting(setting); err != nil {
		response.InternalError(c, "Failed to update settings")
		return
	}

	// 同步更新配置
	h.app.Config.Captcha.Enabled = req.Enabled
	h.app.Config.Captcha.Type = req.Type
	h.app.Config.Captcha.EnableLogin = req.EnableLogin
	h.app.Config.Captcha.EnableRegister = req.EnableRegister

	response.SuccessWithMessage(c, "Settings updated successfully", captchaSettings)
}

