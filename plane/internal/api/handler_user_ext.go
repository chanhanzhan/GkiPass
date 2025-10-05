package api

import (
	"gkipass/plane/internal/api/response"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UpdateUserRole 更新用户角色（仅管理员）
func (h *UserHandler) UpdateUserRole(c *gin.Context) {
	userID := c.Param("id")

	var req struct {
		Role string `json:"role" binding:"required,oneof=admin user"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 更新用户角色
	_, err := h.app.DB.DB.SQLite.Get().Exec("UPDATE users SET role = ? WHERE id = ?", req.Role, userID)
	if err != nil {
		response.InternalError(c, "Failed to update user role")
		return
	}

	logger.Info("用户角色已更新", zap.String("userID", userID), zap.String("role", req.Role))
	response.SuccessWithMessage(c, "User role updated successfully", nil)
}

// DeleteUser 删除用户（仅管理员）
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	// 获取当前用户ID（不能删除自己）
	currentUserID, _ := c.Get("user_id")
	if userID == currentUserID {
		response.BadRequest(c, "Cannot delete yourself")
		return
	}

	// 删除用户
	if err := h.app.DB.DB.SQLite.DeleteUser(userID); err != nil {
		response.InternalError(c, "Failed to delete user")
		return
	}

	logger.Info("用户已删除", zap.String("userID", userID))
	response.SuccessWithMessage(c, "User deleted successfully", nil)
}
