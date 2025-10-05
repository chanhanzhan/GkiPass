package api

import (
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/middleware"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// UserHandler 用户处理器
type UserHandler struct {
	app *App
}

// NewUserHandler 创建用户处理器
func NewUserHandler(app *App) *UserHandler {
	return &UserHandler{app: app}
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=50"`
	Password    string `json:"password" binding:"required,min=6"`
	Email       string `json:"email" binding:"required,email"`
	CaptchaID   string `json:"captcha_id"`
	CaptchaCode string `json:"captcha_code"`
}

// Register 用户注册
func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 验证码检查（如果启用）
	if h.app.Config.Captcha.Enabled && h.app.Config.Captcha.EnableRegister {
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

	// 检查用户名是否已存在
	existingUser, _ := h.app.DB.DB.SQLite.GetUserByUsername(req.Username)
	if existingUser != nil {
		response.BadRequest(c, "Username already exists")
		return
	}

	// 检查邮箱是否已存在
	existingEmail, _ := h.app.DB.DB.SQLite.GetUserByEmail(req.Email)
	if existingEmail != nil {
		response.BadRequest(c, "Email already exists")
		return
	}

	// 检查是否为首个用户（自动成为管理员）
	userCount, err := h.app.DB.DB.SQLite.GetUserCount()
	if err != nil {
		logger.Error("获取用户数量失败", zap.Error(err))
		response.InternalError(c, "Database error")
		return
	}

	isFirstUser := userCount == 0
	role := "user"
	if isFirstUser {
		role = "admin"
		logger.Info("🎉 首个用户注册，自动设置为管理员", zap.String("username", req.Username))
	}

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("密码加密失败", zap.Error(err))
		response.InternalError(c, "Failed to hash password")
		return
	}

	// 创建用户
	user := &dbinit.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Provider:     "local", // 本地注册用户
		Role:         role,
		Enabled:      true,
		CreatedAt:    time.Now(),
	}

	if err := h.app.DB.DB.SQLite.CreateUser(user); err != nil {
		logger.Error("创建用户失败", zap.Error(err))
		response.InternalError(c, "Failed to create user")
		return
	}

	logger.Info("用户注册成功",
		zap.String("userID", user.ID),
		zap.String("username", user.Username),
		zap.String("role", user.Role))

	// 生成JWT token（自动登录）
	token, err := middleware.GenerateJWT(
		user.ID,
		user.Username,
		user.Role,
		h.app.Config.Auth.JWTSecret,
		h.app.Config.Auth.JWTExpiration,
	)
	if err != nil {
		logger.Error("生成token失败", zap.Error(err))
		response.InternalError(c, "Failed to generate token")
		return
	}

	// 计算过期时间
	expiresAt := time.Now().Add(time.Duration(h.app.Config.Auth.JWTExpiration) * time.Hour)

	response.SuccessWithMessage(c, "User registered successfully", gin.H{
		"token":         token,
		"user_id":       user.ID,
		"username":      user.Username,
		"role":          user.Role,
		"expires_at":    expiresAt.Unix(),
		"is_first_user": isFirstUser,
	})
}

// GetProfile 获取用户信息
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")

	user, err := h.app.DB.DB.SQLite.GetUser(userID.(string))
	if err != nil || user == nil {
		response.NotFound(c, "User not found")
		return
	}

	response.Success(c, gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"role":       user.Role,
		"enabled":    user.Enabled,
		"created_at": user.CreatedAt,
		"last_login": user.LastLoginAt,
	})
}

// UpdatePasswordRequest 修改密码请求
type UpdatePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// UpdatePassword 修改密码
func (h *UserHandler) UpdatePassword(c *gin.Context) {
	var req UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	user, err := h.app.DB.DB.SQLite.GetUser(userID.(string))
	if err != nil || user == nil {
		response.NotFound(c, "User not found")
		return
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		response.Unauthorized(c, "Invalid old password")
		return
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		response.InternalError(c, "Failed to hash password")
		return
	}

	// 更新密码
	if err := h.app.DB.DB.SQLite.UpdateUserPassword(user.ID, string(hashedPassword)); err != nil {
		response.InternalError(c, "Failed to update password")
		return
	}

	logger.Info("用户密码已更新", zap.String("userID", user.ID))
	response.SuccessWithMessage(c, "Password updated successfully", nil)
}

// ListUsers 列出所有用户（管理员）
func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.app.DB.DB.SQLite.ListUsers("", nil)
	if err != nil {
		response.InternalError(c, "Failed to list users")
		return
	}

	// 移除敏感信息
	safeUsers := make([]gin.H, 0, len(users))
	for _, user := range users {
		safeUsers = append(safeUsers, gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"enabled":    user.Enabled,
			"created_at": user.CreatedAt,
			"last_login": user.LastLoginAt,
		})
	}

	response.Success(c, safeUsers)
}

// ToggleUserStatus 启用/禁用用户（管理员）
func (h *UserHandler) ToggleUserStatus(c *gin.Context) {
	targetUserID := c.Param("id")
	currentUserID, _ := c.Get("user_id")

	// 不能禁用自己
	if targetUserID == currentUserID.(string) {
		response.BadRequest(c, "Cannot disable your own account")
		return
	}

	user, err := h.app.DB.DB.SQLite.GetUser(targetUserID)
	if err != nil || user == nil {
		response.NotFound(c, "User not found")
		return
	}

	newStatus := !user.Enabled
	if err := h.app.DB.DB.SQLite.UpdateUserStatus(targetUserID, newStatus); err != nil {
		response.InternalError(c, "Failed to update user status")
		return
	}

	logger.Info("用户状态已更新",
		zap.String("userID", targetUserID),
		zap.Bool("enabled", newStatus))

	response.SuccessWithMessage(c, "User status updated", gin.H{
		"user_id": targetUserID,
		"enabled": newStatus,
	})
}
