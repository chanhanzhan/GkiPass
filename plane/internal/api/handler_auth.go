package api

import (
	"time"

	"gkipass/plane/internal/api/middleware"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	app *App
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(app *App) *AuthHandler {
	return &AuthHandler{app: app}
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	CaptchaID   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"expires_at"`
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 验证码检查（如果启用）
	if h.app.Config.Captcha.Enabled && h.app.Config.Captcha.EnableLogin {
		if req.CaptchaID == "" || req.CaptchaCode == "" {
			response.BadRequest(c, "Captcha required")
			return
		}

		// 验证验证码
		if h.app.DB.HasCache() {
			valid, err := h.app.DB.Cache.Redis.VerifyAndDeleteCaptcha(req.CaptchaID, req.CaptchaCode)
			if err != nil || !valid {
				response.BadRequest(c, "Invalid or expired captcha code")
				return
			}
		}
	}

	// 获取用户
	user, err := h.app.DB.DB.SQLite.GetUserByUsername(req.Username)
	if err != nil {
		logger.Error("获取用户失败",
			zap.String("username", req.Username),
			zap.Error(err))
		response.InternalError(c, "Database error")
		return
	}

	if user == nil {
		response.Unauthorized(c, "Invalid username or password")
		return
	}

	// 检查用户是否启用
	if !user.Enabled {
		response.Forbidden(c, "User account is disabled")
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		response.Unauthorized(c, "Invalid username or password")
		return
	}

	// 生成JWT token
	token, err := middleware.GenerateJWT(
		user.ID,
		user.Username,
		user.Role,
		h.app.Config.Auth.JWTSecret,
		h.app.Config.Auth.JWTExpiration,
	)
	if err != nil {
		response.InternalError(c, "Failed to generate token")
		return
	}

	// 更新最后登录时间
	_ = h.app.DB.DB.SQLite.UpdateUserLastLogin(user.ID)

	// 计算过期时间
	expiresAt := time.Now().Add(time.Duration(h.app.Config.Auth.JWTExpiration) * time.Hour)

	response.Success(c, LoginResponse{
		Token:     token,
		UserID:    user.ID,
		Username:  user.Username,
		Role:      user.Role,
		ExpiresAt: expiresAt.Unix(),
	})
}

// Logout 用户登出
func (h *AuthHandler) Logout(c *gin.Context) {
	// 如果有Redis，删除session缓存
	if h.app.DB.HasCache() {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := len(authHeader) > 7
			if parts && authHeader[:7] == "Bearer " {
				token := authHeader[7:]
				_ = h.app.DB.Cache.Redis.DeleteSession(token)
			}
		}
	}

	response.SuccessWithMessage(c, "Logged out successfully", nil)
}
