package user

import (
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/middleware"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// UserHandler 用户处理器
type UserHandler struct {
	app *types.App
}

// NewUserHandler 创建用户处理器
func NewUserHandler(app *types.App) *UserHandler {
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
		logger.Warn("注册请求参数错误", zap.Error(err))
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	logger.Info("用户注册请求",
		zap.String("username", req.Username),
		zap.String("email", req.Email))

	// 验证码检查（如果启用）
	if h.app.Config.Captcha.Enabled && h.app.Config.Captcha.EnableRegister {
		if req.CaptchaID == "" || req.CaptchaCode == "" {
			logger.Warn("缺少验证码", zap.String("username", req.Username))
			response.GinBadRequest(c, "Captcha required")
			return
		}

		// 验证验证码
		if h.app.DB.HasCache() {
			valid, err := h.app.DB.Cache.Redis.VerifyAndDeleteCaptcha(req.CaptchaID, req.CaptchaCode)
			if err != nil || !valid {
				logger.Warn("验证码无效或已过期",
					zap.String("username", req.Username),
					zap.Error(err))
				response.GinBadRequest(c, "Invalid or expired captcha code")
				return
			}
		}
	}

	// 检查用户名是否已存在
	existingUser, err := h.app.DB.DB.SQLite.GetUserByUsername(req.Username)
	if err != nil {
		logger.Error("检查用户名失败",
			zap.String("username", req.Username),
			zap.Error(err))
		response.InternalError(c, "Database error")
		return
	}
	if existingUser != nil {
		logger.Warn("用户名已存在",
			zap.String("username", req.Username))
		response.GinBadRequest(c, "Username already exists")
		return
	}

	// 检查邮箱是否已存在
	existingEmail, err := h.app.DB.DB.SQLite.GetUserByEmail(req.Email)
	if err != nil {
		logger.Error("检查邮箱失败",
			zap.String("email", req.Email),
			zap.Error(err))
		response.InternalError(c, "Database error")
		return
	}
	if existingEmail != nil {
		logger.Warn("邮箱已存在",
			zap.String("email", req.Email),
			zap.String("existingUserID", existingEmail.ID),
			zap.String("existingUsername", existingEmail.Username))
		response.GinBadRequest(c, "Email already exists")
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
		response.GinNotFound(c, "User not found")
		return
	}

	response.GinSuccess(c, gin.H{
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
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	user, err := h.app.DB.DB.SQLite.GetUser(userID.(string))
	if err != nil || user == nil {
		response.GinNotFound(c, "User not found")
		return
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		response.GinUnauthorized(c, "Invalid old password")
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

	response.GinSuccess(c, safeUsers)
}

// GetCurrentUser 获取当前用户完整信息（包括订阅、钱包、权限等）
func (h *UserHandler) GetCurrentUser(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	// 获取用户基本信息
	user, err := h.app.DB.DB.SQLite.GetUser(userID.(string))
	if err != nil || user == nil {
		response.GinNotFound(c, "User not found")
		return
	}

	// 构建返回数据
	userData := gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"role":       user.Role,
		"is_admin":   user.Role == "admin",
		"enabled":    user.Enabled,
		"provider":   user.Provider,
		"created_at": user.CreatedAt,
		"last_login": user.LastLoginAt,
	}

	// 获取订阅信息
	subscription, err := h.app.DB.DB.SQLite.GetUserSubscription(userID.(string))
	if err == nil && subscription != nil {
		userData["subscription"] = gin.H{
			"id":            subscription.ID,
			"plan_id":       subscription.PlanID,
			"status":        subscription.Status,
			"start_at":      subscription.StartAt,
			"expires_at":    subscription.ExpiresAt,
			"traffic":       subscription.Traffic,
			"used_traffic":  subscription.UsedTraffic,
			"max_tunnels":   subscription.MaxTunnels,
			"max_bandwidth": subscription.MaxBandwidth,
		}
	} else {
		userData["subscription"] = nil
	}

	// 获取钱包信息
	wallet, err := h.app.DB.DB.SQLite.GetWalletByUserID(userID.(string))
	if err == nil && wallet != nil {
		userData["wallet"] = gin.H{
			"id":      wallet.ID,
			"balance": wallet.Balance,
			"frozen":  wallet.Frozen,
		}
	} else {
		userData["wallet"] = gin.H{
			"id":      "",
			"balance": 0,
			"frozen":  0,
		}
	}

	// 添加权限信息
	permissions := []string{}
	if role.(string) == "admin" {
		permissions = []string{
			"user:read", "user:write", "user:delete",
			"node:read", "node:write", "node:delete",
			"tunnel:read", "tunnel:write", "tunnel:delete",
			"plan:read", "plan:write", "plan:delete",
			"announcement:read", "announcement:write", "announcement:delete",
			"settings:read", "settings:write",
			"statistics:read",
			"admin:access",
		}
	} else {
		permissions = []string{
			"tunnel:read", "tunnel:write", "tunnel:delete:own",
			"node:read:own", "node:write:own", "node:delete:own",
			"profile:read", "profile:write",
			"wallet:read", "subscription:read",
		}
	}
	userData["permissions"] = permissions

	response.GinSuccess(c, userData)
}

// ToggleUserStatus 启用/禁用用户（管理员）
func (h *UserHandler) ToggleUserStatus(c *gin.Context) {
	targetUserID := c.Param("id")
	currentUserID, _ := c.Get("user_id")

	// 不能禁用自己
	if targetUserID == currentUserID.(string) {
		response.GinBadRequest(c, "Cannot disable your own account")
		return
	}

	user, err := h.app.DB.DB.SQLite.GetUser(targetUserID)
	if err != nil || user == nil {
		response.GinNotFound(c, "User not found")
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

// UpdateUserRoleRequest 更新角色请求
type UpdateUserRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin user"`
}

// UpdateUserRole 更新用户角色（管理员）
func (h *UserHandler) UpdateUserRole(c *gin.Context) {
	targetUserID := c.Param("id")
	currentUserID, _ := c.Get("user_id")

	var req UpdateUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 不能修改自己的角色
	if targetUserID == currentUserID.(string) {
		response.GinBadRequest(c, "Cannot modify your own role")
		return
	}

	user, err := h.app.DB.DB.SQLite.GetUser(targetUserID)
	if err != nil || user == nil {
		response.GinNotFound(c, "User not found")
		return
	}

	// 更新角色
	if err := h.app.DB.DB.SQLite.UpdateUserRole(targetUserID, req.Role); err != nil {
		response.InternalError(c, "Failed to update user role")
		return
	}

	logger.Info("用户角色已更新",
		zap.String("userID", targetUserID),
		zap.String("newRole", req.Role))

	response.SuccessWithMessage(c, "User role updated", gin.H{
		"user_id": targetUserID,
		"role":    req.Role,
	})
}

// DeleteUser 删除用户（管理员）
func (h *UserHandler) DeleteUser(c *gin.Context) {
	targetUserID := c.Param("id")
	currentUserID, _ := c.Get("user_id")

	// 不能删除自己
	if targetUserID == currentUserID.(string) {
		response.GinBadRequest(c, "Cannot delete your own account")
		return
	}

	user, err := h.app.DB.DB.SQLite.GetUser(targetUserID)
	if err != nil || user == nil {
		response.GinNotFound(c, "User not found")
		return
	}

	// 删除用户
	if err := h.app.DB.DB.SQLite.DeleteUser(targetUserID); err != nil {
		response.InternalError(c, "Failed to delete user")
		return
	}

	logger.Info("用户已删除", zap.String("userID", targetUserID))
	response.SuccessWithMessage(c, "User deleted successfully", nil)
}

// GetUserPermissions 获取用户权限详情
func (h *UserHandler) GetUserPermissions(c *gin.Context) {
	role, _ := c.Get("role")
	userID, _ := c.Get("user_id")

	// 构建权限详情
	permissionDetails := gin.H{
		"user_id":     userID,
		"role":        role,
		"permissions": map[string]interface{}{},
	}

	if role.(string) == "admin" {
		permissionDetails["permissions"] = map[string]interface{}{
			"user": map[string]bool{
				"read":   true,
				"create": true,
				"update": true,
				"delete": true,
			},
			"node": map[string]bool{
				"read":   true,
				"create": true,
				"update": true,
				"delete": true,
			},
			"tunnel": map[string]bool{
				"read":   true,
				"create": true,
				"update": true,
				"delete": true,
			},
			"plan": map[string]bool{
				"read":   true,
				"create": true,
				"update": true,
				"delete": true,
			},
			"announcement": map[string]bool{
				"read":   true,
				"create": true,
				"update": true,
				"delete": true,
			},
			"settings": map[string]bool{
				"read":   true,
				"update": true,
			},
			"statistics": map[string]bool{
				"read": true,
			},
		}
		permissionDetails["admin"] = true
		permissionDetails["can_access_admin_panel"] = true
	} else {
		permissionDetails["permissions"] = map[string]interface{}{
			"tunnel": map[string]bool{
				"read":       true,
				"create":     true,
				"update_own": true,
				"delete_own": true,
			},
			"node": map[string]bool{
				"read_own":   true,
				"create":     true,
				"update_own": true,
				"delete_own": true,
			},
			"profile": map[string]bool{
				"read":   true,
				"update": true,
			},
			"wallet": map[string]bool{
				"read": true,
			},
			"subscription": map[string]bool{
				"read": true,
			},
		}
		permissionDetails["admin"] = false
		permissionDetails["can_access_admin_panel"] = false
	}

	response.GinSuccess(c, permissionDetails)
}
