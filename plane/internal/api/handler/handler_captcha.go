package handler

import (
	"crypto/rand"
	"strings"
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mojocn/base64Captcha"
)

// CaptchaHandler 验证码处理器
type CaptchaHandler struct {
	app *types.App
}

// NewCaptchaHandler 创建验证码处理器
func NewCaptchaHandler(app *types.App) *CaptchaHandler {
	return &CaptchaHandler{app: app}
}

// CaptchaResponse 验证码响应
type CaptchaResponse struct {
	CaptchaID string `json:"captcha_id"`
	ImageData string `json:"image_data"` // base64编码的图片
}

// GenerateImageCaptcha 生成图片验证码
func (h *CaptchaHandler) GenerateImageCaptcha(c *gin.Context) {
	// 检查是否启用验证码
	if !h.app.Config.Captcha.Enabled {
		response.GinBadRequest(c, "Captcha is disabled")
		return
	}

	// 生成验证码配置
	driver := base64Captcha.NewDriverDigit(
		h.app.Config.Captcha.ImageHeight,
		h.app.Config.Captcha.ImageWidth,
		h.app.Config.Captcha.CodeLength,
		0.7, // 最大倾斜度
		80,  // 点数
	)

	captchaID := uuid.New().String()
	code := generateRandomCode(h.app.Config.Captcha.CodeLength)

	// 生成验证码
	item, err := driver.DrawCaptcha(code)
	if err != nil {
		response.InternalError(c, "Failed to generate captcha")
		return
	}

	// 转换为base64
	imageData := item.EncodeB64string()

	// 存储验证码到Redis
	if h.app.DB.HasCache() {
		session := &dbinit.CaptchaSession{
			ID:        captchaID,
			Code:      strings.ToLower(code),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Duration(h.app.Config.Captcha.Expiration) * time.Second),
		}
		if err := h.app.DB.Cache.Redis.SetCaptchaSession(session,
			time.Duration(h.app.Config.Captcha.Expiration)*time.Second); err != nil {
			response.InternalError(c, "Failed to store captcha")
			return
		}
	}

	response.GinSuccess(c, CaptchaResponse{
		CaptchaID: captchaID,
		ImageData: imageData,
	})
}

// VerifyCaptchaRequest 验证验证码请求
type VerifyCaptchaRequest struct {
	CaptchaID string `json:"captcha_id" binding:"required"`
	Code      string `json:"code" binding:"required"`
}

// VerifyCaptcha 验证验证码
func (h *CaptchaHandler) VerifyCaptcha(c *gin.Context) {
	var req VerifyCaptchaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if !h.app.Config.Captcha.Enabled {
		response.GinSuccess(c, gin.H{"valid": true})
		return
	}

	if !h.app.DB.HasCache() {
		response.GinBadRequest(c, "Captcha verification unavailable")
		return
	}

	valid, err := h.app.DB.Cache.Redis.VerifyAndDeleteCaptcha(req.CaptchaID, strings.ToLower(req.Code))
	if err != nil {
		response.InternalError(c, "Failed to verify captcha")
		return
	}

	response.GinSuccess(c, gin.H{"valid": valid})
}

// GetCaptchaConfig 获取验证码配置（公开接口）
func (h *CaptchaHandler) GetCaptchaConfig(c *gin.Context) {
	response.GinSuccess(c, gin.H{
		"enabled":            h.app.Config.Captcha.Enabled,
		"type":               h.app.Config.Captcha.Type,
		"enable_login":       h.app.Config.Captcha.EnableLogin,
		"enable_register":    h.app.Config.Captcha.EnableRegister,
		"turnstile_site_key": h.app.Config.Captcha.TurnstileSiteKey,
	})
}

// generateRandomCode 生成随机验证码
func generateRandomCode(length int) string {
	const charset = "0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "123456" // fallback
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}
