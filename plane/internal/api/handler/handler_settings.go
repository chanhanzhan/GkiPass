package handler

import (
	"encoding/json"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SettingsHandler 设置处理器
type SettingsHandler struct {
	app *types.App
}

// NewSettingsHandler 创建设置处理器
func NewSettingsHandler(app *types.App) *SettingsHandler {
	return &SettingsHandler{app: app}
}

// GetCaptchaSettings 获取验证码设置
func (h *SettingsHandler) GetCaptchaSettings(c *gin.Context) {
	// 如果数据库未初始化或表不存在，直接返回配置文件中的设置
	if h.app.DB == nil || h.app.DB.DB == nil || h.app.DB.DB.SQLite == nil {
		response.GinSuccess(c, h.app.Config.Captcha)
		return
	}

	setting, err := h.app.DB.DB.SQLite.GetSystemSetting("captcha")
	if err != nil {
		// 如果出错（例如表不存在），记录日志并返回默认配置
		logger.Warn("Failed to get captcha settings from database, using config defaults", zap.Error(err))
		response.GinSuccess(c, h.app.Config.Captcha)
		return
	}

	if setting == nil {
		// 返回默认设置
		response.GinSuccess(c, h.app.Config.Captcha)
		return
	}

	var captchaSettings dbinit.CaptchaSettings
	if err := json.Unmarshal([]byte(setting.Value), &captchaSettings); err != nil {
		logger.Error("Failed to parse captcha settings", zap.Error(err))
		response.GinSuccess(c, h.app.Config.Captcha)
		return
	}

	response.GinSuccess(c, captchaSettings)
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
		response.GinBadRequest(c, "Invalid request: "+err.Error())
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

// GeneralSettings 通用设置结构
type GeneralSettings struct {
	SiteName                 string `json:"site_name"`
	SiteDescription          string `json:"site_description"`
	SiteLogo                 string `json:"site_logo"`
	AllowRegister            bool   `json:"allow_register"`
	RequireEmailVerification bool   `json:"require_email_verification"`
}

// GetGeneralSettings 获取通用设置
func (h *SettingsHandler) GetGeneralSettings(c *gin.Context) {
	defaultSettings := GeneralSettings{
		SiteName:                 "GKI Pass",
		SiteDescription:          "企业级双向隧道控制平台",
		SiteLogo:                 "",
		AllowRegister:            true,
		RequireEmailVerification: false,
	}

	if h.app.DB == nil || h.app.DB.DB == nil || h.app.DB.DB.SQLite == nil {
		response.GinSuccess(c, defaultSettings)
		return
	}

	setting, err := h.app.DB.DB.SQLite.GetSystemSetting("general")
	if err != nil {
		logger.Warn("Failed to get general settings from database, using defaults", zap.Error(err))
		response.GinSuccess(c, defaultSettings)
		return
	}

	if setting == nil {
		response.GinSuccess(c, defaultSettings)
		return
	}

	var generalSettings GeneralSettings
	if err := json.Unmarshal([]byte(setting.Value), &generalSettings); err != nil {
		logger.Error("Failed to parse general settings", zap.Error(err))
		response.GinSuccess(c, defaultSettings)
		return
	}

	response.GinSuccess(c, generalSettings)
}

// UpdateGeneralSettingsRequest 更新通用设置请求
type UpdateGeneralSettingsRequest struct {
	SiteName                 string `json:"site_name"`
	SiteDescription          string `json:"site_description"`
	SiteLogo                 string `json:"site_logo"`
	AllowRegister            bool   `json:"allow_register"`
	RequireEmailVerification bool   `json:"require_email_verification"`
}

// UpdateGeneralSettings 更新通用设置
func (h *SettingsHandler) UpdateGeneralSettings(c *gin.Context) {
	var req UpdateGeneralSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")

	generalSettings := GeneralSettings{
		SiteName:                 req.SiteName,
		SiteDescription:          req.SiteDescription,
		SiteLogo:                 req.SiteLogo,
		AllowRegister:            req.AllowRegister,
		RequireEmailVerification: req.RequireEmailVerification,
	}

	valueJSON, err := json.Marshal(generalSettings)
	if err != nil {
		response.InternalError(c, "Failed to marshal settings")
		return
	}

	setting := &dbinit.SystemSettings{
		Key:         "general",
		Value:       string(valueJSON),
		Category:    "general",
		UpdatedBy:   userID.(string),
		Description: "通用系统设置",
	}

	if err := h.app.DB.DB.SQLite.UpsertSystemSetting(setting); err != nil {
		response.InternalError(c, "Failed to update settings")
		return
	}

	response.SuccessWithMessage(c, "General settings updated successfully", generalSettings)
}

// SecuritySettings 安全设置结构
type SecuritySettings struct {
	PasswordMinLength        int  `json:"password_min_length"`
	PasswordRequireUppercase bool `json:"password_require_uppercase"`
	PasswordRequireLowercase bool `json:"password_require_lowercase"`
	PasswordRequireNumber    bool `json:"password_require_number"`
	PasswordRequireSpecial   bool `json:"password_require_special"`
	LoginMaxAttempts         int  `json:"login_max_attempts"`
	LoginLockoutDuration     int  `json:"login_lockout_duration"` // 分钟
	Enable2FA                bool `json:"enable_2fa"`
	SessionTimeout           int  `json:"session_timeout"` // 小时
}

// GetSecuritySettings 获取安全设置
func (h *SettingsHandler) GetSecuritySettings(c *gin.Context) {
	defaultSettings := SecuritySettings{
		PasswordMinLength:        6,
		PasswordRequireUppercase: false,
		PasswordRequireLowercase: false,
		PasswordRequireNumber:    false,
		PasswordRequireSpecial:   false,
		LoginMaxAttempts:         5,
		LoginLockoutDuration:     30,
		Enable2FA:                false,
		SessionTimeout:           24,
	}

	if h.app.DB == nil || h.app.DB.DB == nil || h.app.DB.DB.SQLite == nil {
		response.GinSuccess(c, defaultSettings)
		return
	}

	setting, err := h.app.DB.DB.SQLite.GetSystemSetting("security")
	if err != nil {
		logger.Warn("Failed to get security settings from database, using defaults", zap.Error(err))
		response.GinSuccess(c, defaultSettings)
		return
	}

	if setting == nil {
		response.GinSuccess(c, defaultSettings)
		return
	}

	var securitySettings SecuritySettings
	if err := json.Unmarshal([]byte(setting.Value), &securitySettings); err != nil {
		logger.Error("Failed to parse security settings", zap.Error(err))
		response.GinSuccess(c, defaultSettings)
		return
	}

	response.GinSuccess(c, securitySettings)
}

// UpdateSecuritySettings 更新安全设置
func (h *SettingsHandler) UpdateSecuritySettings(c *gin.Context) {
	var req SecuritySettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")

	valueJSON, err := json.Marshal(req)
	if err != nil {
		response.InternalError(c, "Failed to marshal settings")
		return
	}

	setting := &dbinit.SystemSettings{
		Key:         "security",
		Value:       string(valueJSON),
		Category:    "security",
		UpdatedBy:   userID.(string),
		Description: "安全设置",
	}

	if err := h.app.DB.DB.SQLite.UpsertSystemSetting(setting); err != nil {
		response.InternalError(c, "Failed to update settings")
		return
	}

	response.SuccessWithMessage(c, "Security settings updated successfully", req)
}

// NotificationSettings 通知设置结构
type NotificationSettings struct {
	EmailEnabled         bool   `json:"email_enabled"`
	EmailHost            string `json:"email_host"`
	EmailPort            int    `json:"email_port"`
	EmailUsername        string `json:"email_username"`
	EmailPassword        string `json:"email_password"`
	EmailFrom            string `json:"email_from"`
	SystemNotifyEnabled  bool   `json:"system_notify_enabled"`
	WebhookEnabled       bool   `json:"webhook_enabled"`
	WebhookURL           string `json:"webhook_url"`
	NotifyOnUserRegister bool   `json:"notify_on_user_register"`
	NotifyOnPayment      bool   `json:"notify_on_payment"`
	NotifyOnNodeOffline  bool   `json:"notify_on_node_offline"`
}

// GetNotificationSettings 获取通知设置
func (h *SettingsHandler) GetNotificationSettings(c *gin.Context) {
	defaultSettings := NotificationSettings{
		EmailEnabled:         false,
		EmailHost:            "smtp.example.com",
		EmailPort:            587,
		EmailUsername:        "",
		EmailPassword:        "",
		EmailFrom:            "noreply@example.com",
		SystemNotifyEnabled:  true,
		WebhookEnabled:       false,
		WebhookURL:           "",
		NotifyOnUserRegister: true,
		NotifyOnPayment:      true,
		NotifyOnNodeOffline:  true,
	}

	if h.app.DB == nil || h.app.DB.DB == nil || h.app.DB.DB.SQLite == nil {
		response.GinSuccess(c, defaultSettings)
		return
	}

	setting, err := h.app.DB.DB.SQLite.GetSystemSetting("notification")
	if err != nil {
		logger.Warn("Failed to get notification settings from database, using defaults", zap.Error(err))
		response.GinSuccess(c, defaultSettings)
		return
	}

	if setting == nil {
		response.GinSuccess(c, defaultSettings)
		return
	}

	var notificationSettings NotificationSettings
	if err := json.Unmarshal([]byte(setting.Value), &notificationSettings); err != nil {
		logger.Error("Failed to parse notification settings", zap.Error(err))
		response.GinSuccess(c, defaultSettings)
		return
	}

	response.GinSuccess(c, notificationSettings)
}

// UpdateNotificationSettings 更新通知设置
func (h *SettingsHandler) UpdateNotificationSettings(c *gin.Context) {
	var req NotificationSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 验证邮件设置
	if req.EmailEnabled {
		if req.EmailHost == "" || req.EmailFrom == "" {
			response.GinBadRequest(c, "Email host and from address are required when email is enabled")
			return
		}
		if req.EmailPort < 1 || req.EmailPort > 65535 {
			response.GinBadRequest(c, "Invalid email port")
			return
		}
	}

	userID, _ := c.Get("user_id")

	valueJSON, err := json.Marshal(req)
	if err != nil {
		response.InternalError(c, "Failed to marshal settings")
		return
	}

	setting := &dbinit.SystemSettings{
		Key:         "notification",
		Value:       string(valueJSON),
		Category:    "notification",
		UpdatedBy:   userID.(string),
		Description: "通知设置",
	}

	if err := h.app.DB.DB.SQLite.UpsertSystemSetting(setting); err != nil {
		response.InternalError(c, "Failed to update settings")
		return
	}

	logger.Info("更新通知设置",
		zap.Bool("email_enabled", req.EmailEnabled),
		zap.Bool("system_notify_enabled", req.SystemNotifyEnabled))

	response.SuccessWithMessage(c, "Notification settings updated successfully", req)
}
