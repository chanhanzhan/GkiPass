package handler

import (
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/service"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TunnelHandler 隧道处理器
type TunnelHandler struct {
	app           *types.App
	tunnelService *service.TunnelService
}

// NewTunnelHandler 创建隧道处理器
func NewTunnelHandler(app *types.App) *TunnelHandler {
	planService := service.NewPlanService(app.DB)
	return &TunnelHandler{
		app:           app,
		tunnelService: service.NewTunnelService(app.DB, planService),
	}
}

// CreateTunnelRequest 创建隧道请求
type CreateTunnelRequest struct {
	Name         string                `json:"name" binding:"required"`
	Protocol     string                `json:"protocol" binding:"required,oneof=tcp udp http https"`
	EntryGroupID string                `json:"entry_group_id" binding:"required"`
	ExitGroupID  string                `json:"exit_group_id" binding:"required"`
	LocalPort    int                   `json:"local_port" binding:"required,min=1024,max=65535"`
	Targets      []dbinit.TunnelTarget `json:"targets" binding:"required,min=1"`
	Enabled      bool                  `json:"enabled"`
	Description  string                `json:"description"`
}

// Create 创建隧道
func (h *TunnelHandler) Create(c *gin.Context) {
	var req CreateTunnelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userID, _ := c.Get("user_id")

	tunnel := &dbinit.Tunnel{
		UserID:       userID.(string),
		Name:         req.Name,
		Protocol:     req.Protocol,
		EntryGroupID: req.EntryGroupID,
		ExitGroupID:  req.ExitGroupID,
		LocalPort:    req.LocalPort,
		Enabled:      req.Enabled,
		Description:  req.Description,
	}

	if err := h.tunnelService.CreateTunnel(tunnel, req.Targets); err != nil {
		logger.Error("创建隧道失败", zap.Error(err))
		response.BadRequest(c, err.Error())
		return
	}

	// 解析目标用于返回
	targets, _ := service.ParseTargetsFromString(tunnel.Targets)

	response.SuccessWithMessage(c, "Tunnel created successfully", gin.H{
		"tunnel":  tunnel,
		"targets": targets,
	})
}

// List 列出隧道
func (h *TunnelHandler) List(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	// 管理员可以查看所有隧道
	queryUserID := userID.(string)
	if role == "admin" && c.Query("user_id") != "" {
		queryUserID = c.Query("user_id")
	}

	var enabled *bool
	if enabledStr := c.Query("enabled"); enabledStr != "" {
		val := enabledStr == "true"
		enabled = &val
	}

	tunnels, err := h.tunnelService.ListTunnels(queryUserID, enabled)
	if err != nil {
		logger.Error("列出隧道失败", zap.Error(err))
		response.InternalError(c, "Failed to list tunnels")
		return
	}

	response.Success(c, tunnels)
}

// Get 获取隧道详情
func (h *TunnelHandler) Get(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	tunnel, err := h.tunnelService.GetTunnel(id)
	if err != nil {
		response.InternalError(c, "Failed to get tunnel")
		return
	}
	if tunnel == nil {
		response.NotFound(c, "Tunnel not found")
		return
	}

	// 权限检查
	if role != "admin" && tunnel.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	response.Success(c, tunnel)
}

// Update 更新隧道
func (h *TunnelHandler) Update(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	var req CreateTunnelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 获取现有隧道
	tunnel, err := h.tunnelService.GetTunnel(id)
	if err != nil || tunnel == nil {
		response.NotFound(c, "Tunnel not found")
		return
	}

	// 权限检查
	if role != "admin" && tunnel.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	// 更新字段
	tunnel.Name = req.Name
	tunnel.Protocol = req.Protocol
	tunnel.EntryGroupID = req.EntryGroupID
	tunnel.ExitGroupID = req.ExitGroupID
	tunnel.LocalPort = req.LocalPort
	tunnel.Enabled = req.Enabled
	tunnel.Description = req.Description

	if err := h.tunnelService.UpdateTunnel(tunnel, req.Targets); err != nil {
		logger.Error("更新隧道失败", zap.Error(err))
		response.InternalError(c, "Failed to update tunnel")
		return
	}

	// 解析目标用于返回
	targets, _ := service.ParseTargetsFromString(tunnel.Targets)

	response.SuccessWithMessage(c, "Tunnel updated successfully", gin.H{
		"tunnel":  tunnel,
		"targets": targets,
	})
}

// Delete 删除隧道
func (h *TunnelHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	// 获取隧道
	tunnel, err := h.tunnelService.GetTunnel(id)
	if err != nil || tunnel == nil {
		response.NotFound(c, "Tunnel not found")
		return
	}

	// 权限检查
	if role != "admin" && tunnel.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	if err := h.tunnelService.DeleteTunnel(id, userID.(string)); err != nil {
		logger.Error("删除隧道失败", zap.String("id", id), zap.Error(err))
		response.InternalError(c, "Failed to delete tunnel")
		return
	}

	response.SuccessWithMessage(c, "Tunnel deleted successfully", nil)
}

// Toggle 切换隧道状态
func (h *TunnelHandler) Toggle(c *gin.Context) {
	id := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	// 获取隧道
	tunnel, err := h.tunnelService.GetTunnel(id)
	if err != nil || tunnel == nil {
		response.NotFound(c, "Tunnel not found")
		return
	}

	// 权限检查
	if role != "admin" && tunnel.UserID != userID.(string) {
		response.Forbidden(c, "No permission")
		return
	}

	// 切换状态
	if err := h.tunnelService.ToggleTunnel(id, !tunnel.Enabled); err != nil {
		response.InternalError(c, "Failed to toggle tunnel")
		return
	}

	response.SuccessWithMessage(c, "Tunnel toggled successfully", gin.H{
		"id":      id,
		"enabled": !tunnel.Enabled,
	})
}
