package api

import (
	"strconv"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// NotificationHandler 通知处理器
type NotificationHandler struct {
	app *App
}

// NewNotificationHandler 创建通知处理器
func NewNotificationHandler(app *App) *NotificationHandler {
	return &NotificationHandler{app: app}
}

// List 获取通知列表
func (h *NotificationHandler) List(c *gin.Context) {
	userID, _ := c.Get("user_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	notifications, total, err := h.app.DB.DB.SQLite.ListNotifications(userID.(string), page, limit)
	if err != nil {
		response.InternalError(c, "Failed to list notifications")
		return
	}

	response.Success(c, gin.H{
		"data":        notifications,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": (total + limit - 1) / limit,
	})
}

// MarkAsRead 标记为已读
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	id := c.Param("id")

	if err := h.app.DB.DB.SQLite.MarkNotificationAsRead(id); err != nil {
		response.InternalError(c, "Failed to mark as read")
		return
	}

	response.SuccessWithMessage(c, "Marked as read", nil)
}

// MarkAllAsRead 全部标记为已读
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	userID, _ := c.Get("user_id")

	if err := h.app.DB.DB.SQLite.MarkAllNotificationsAsRead(userID.(string)); err != nil {
		response.InternalError(c, "Failed to mark all as read")
		return
	}

	response.SuccessWithMessage(c, "All marked as read", nil)
}

// Delete 删除通知
func (h *NotificationHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.app.DB.DB.SQLite.DeleteNotification(id); err != nil {
		response.InternalError(c, "Failed to delete notification")
		return
	}

	response.SuccessWithMessage(c, "Notification deleted", nil)
}

// CreateNotificationRequest 创建通知请求
type CreateNotificationRequest struct {
	UserID   string `json:"user_id"` // 空表示全局通知
	Type     string `json:"type" binding:"required"`
	Title    string `json:"title" binding:"required"`
	Content  string `json:"content" binding:"required"`
	Link     string `json:"link"`
	Priority string `json:"priority"`
}

// Create 创建通知（管理员）
func (h *NotificationHandler) Create(c *gin.Context) {
	var req CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if req.Priority == "" {
		req.Priority = "normal"
	}

	notification := &dbinit.Notification{
		ID:       uuid.New().String(),
		UserID:   req.UserID,
		Type:     req.Type,
		Title:    req.Title,
		Content:  req.Content,
		Link:     req.Link,
		IsRead:   false,
		Priority: req.Priority,
	}

	if err := h.app.DB.DB.SQLite.CreateNotification(notification); err != nil {
		response.InternalError(c, "Failed to create notification")
		return
	}

	response.SuccessWithMessage(c, "Notification created", notification)
}

