package handler

import (
	"strconv"
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AnnouncementHandler 公告处理器
type AnnouncementHandler struct {
	app *types.App
}

// NewAnnouncementHandler 创建公告处理器
func NewAnnouncementHandler(app *types.App) *AnnouncementHandler {
	return &AnnouncementHandler{app: app}
}

// ListActiveAnnouncements 获取有效公告列表（用户）
func (h *AnnouncementHandler) ListActiveAnnouncements(c *gin.Context) {
	announcements, err := h.app.DB.DB.SQLite.ListActiveAnnouncements()
	if err != nil {
		response.InternalError(c, "Failed to list announcements")
		return
	}

	response.GinSuccess(c, announcements)
}

// GetAnnouncement 获取公告详情
func (h *AnnouncementHandler) GetAnnouncement(c *gin.Context) {
	id := c.Param("id")

	announcement, err := h.app.DB.DB.SQLite.GetAnnouncement(id)
	if err != nil {
		response.InternalError(c, "Failed to get announcement")
		return
	}

	if announcement == nil {
		response.GinNotFound(c, "Announcement not found")
		return
	}

	response.GinSuccess(c, announcement)
}

// ListAll 获取所有公告（管理员）
func (h *AnnouncementHandler) ListAll(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	announcements, total, err := h.app.DB.DB.SQLite.ListAnnouncements(page, limit)
	if err != nil {
		response.InternalError(c, "Failed to list announcements")
		return
	}

	response.GinSuccess(c, gin.H{
		"data":        announcements,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": (total + limit - 1) / limit,
	})
}

// CreateAnnouncementRequest 创建公告请求
type CreateAnnouncementRequest struct {
	Title     string `json:"title" binding:"required"`
	Content   string `json:"content" binding:"required"`
	Type      string `json:"type" binding:"required"`
	Priority  string `json:"priority"`
	Enabled   bool   `json:"enabled"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
}

// Create 创建公告（管理员）
func (h *AnnouncementHandler) Create(c *gin.Context) {
	var req CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		response.GinBadRequest(c, "Invalid start_time format")
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		response.GinBadRequest(c, "Invalid end_time format")
		return
	}

	if req.Priority == "" {
		req.Priority = "normal"
	}

	announcement := &dbinit.Announcement{
		ID:        uuid.New().String(),
		Title:     req.Title,
		Content:   req.Content,
		Type:      req.Type,
		Priority:  req.Priority,
		Enabled:   req.Enabled,
		StartTime: startTime,
		EndTime:   endTime,
		CreatedBy: userID.(string),
	}

	if err := h.app.DB.DB.SQLite.CreateAnnouncement(announcement); err != nil {
		response.InternalError(c, "Failed to create announcement")
		return
	}

	response.SuccessWithMessage(c, "Announcement created", announcement)
}

// Update 更新公告（管理员）
func (h *AnnouncementHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		response.GinBadRequest(c, "Invalid start_time format")
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		response.GinBadRequest(c, "Invalid end_time format")
		return
	}

	announcement := &dbinit.Announcement{
		ID:        id,
		Title:     req.Title,
		Content:   req.Content,
		Type:      req.Type,
		Priority:  req.Priority,
		Enabled:   req.Enabled,
		StartTime: startTime,
		EndTime:   endTime,
	}

	if err := h.app.DB.DB.SQLite.UpdateAnnouncement(announcement); err != nil {
		response.InternalError(c, "Failed to update announcement")
		return
	}

	response.SuccessWithMessage(c, "Announcement updated", announcement)
}

// Delete 删除公告（管理员）
func (h *AnnouncementHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.app.DB.DB.SQLite.DeleteAnnouncement(id); err != nil {
		response.InternalError(c, "Failed to delete announcement")
		return
	}

	response.SuccessWithMessage(c, "Announcement deleted", nil)
}
