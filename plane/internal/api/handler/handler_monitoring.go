package handler

import (
	"strconv"
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/service"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// MonitoringHandler 监控处理器
type MonitoringHandler struct {
	app               *types.App
	monitoringService *service.NodeMonitoringService
}

// NewMonitoringHandler 创建监控处理器
func NewMonitoringHandler(app *types.App) *MonitoringHandler {
	return &MonitoringHandler{
		app:               app,
		monitoringService: service.NewNodeMonitoringService(app.DB),
	}
}

// ReportNodeMonitoringData 接收节点监控数据上报
func (h *MonitoringHandler) ReportNodeMonitoringData(c *gin.Context) {
	nodeID := c.Param("node_id")
	if nodeID == "" {
		nodeID = c.Query("node_id")
	}

	var data service.NodeMonitoringReportData
	if err := c.ShouldBindJSON(&data); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 验证节点权限（通过CK或API Key）
	if !h.validateNodeAccess(c, nodeID) {
		response.Unauthorized(c, "Invalid node credentials")
		return
	}

	// 处理监控数据
	if err := h.monitoringService.ReportMonitoringData(nodeID, &data); err != nil {
		logger.Error("处理监控数据失败",
			zap.String("nodeID", nodeID),
			zap.Error(err))
		response.InternalError(c, "Failed to process monitoring data")
		return
	}

	response.Success(c, gin.H{
		"status":    "success",
		"timestamp": time.Now(),
		"message":   "Monitoring data received",
	})
}

// GetNodeMonitoringStatus 获取节点监控状态
func (h *MonitoringHandler) GetNodeMonitoringStatus(c *gin.Context) {
	nodeID := c.Param("id")
	userID, _ := c.Get("user_id")

	// 权限检查
	if !h.monitoringService.CheckMonitoringPermission(userID.(string), nodeID, "view_basic") {
		response.Forbidden(c, "No permission to view monitoring data")
		return
	}

	status, err := h.monitoringService.GetNodeMonitoringStatus(nodeID)
	if err != nil {
		response.InternalError(c, "Failed to get monitoring status")
		return
	}

	response.Success(c, status)
}

// GetNodeMonitoringData 获取节点详细监控数据
func (h *MonitoringHandler) GetNodeMonitoringData(c *gin.Context) {
	nodeID := c.Param("id")
	userID, _ := c.Get("user_id")

	// 权限检查
	if !h.monitoringService.CheckMonitoringPermission(userID.(string), nodeID, "view_detailed") {
		response.Forbidden(c, "No permission to view detailed monitoring data")
		return
	}

	// 解析时间范围
	fromStr := c.DefaultQuery("from", time.Now().Add(-24*time.Hour).Format(time.RFC3339))
	toStr := c.DefaultQuery("to", time.Now().Format(time.RFC3339))
	limitStr := c.DefaultQuery("limit", "100")

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		response.BadRequest(c, "Invalid from time format")
		return
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		response.BadRequest(c, "Invalid to time format")
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 1000 {
		limit = 100
	}

	// 获取监控数据
	data, err := h.app.DB.DB.SQLite.ListNodeMonitoringData(nodeID, from, to, limit)
	if err != nil {
		response.InternalError(c, "Failed to get monitoring data")
		return
	}

	response.Success(c, gin.H{
		"node_id":    nodeID,
		"from":       from,
		"to":         to,
		"data_count": len(data),
		"data":       data,
	})
}

// GetNodePerformanceHistory 获取节点性能历史
func (h *MonitoringHandler) GetNodePerformanceHistory(c *gin.Context) {
	nodeID := c.Param("id")
	userID, _ := c.Get("user_id")

	// 权限检查
	if !h.monitoringService.CheckMonitoringPermission(userID.(string), nodeID, "view_detailed") {
		response.Forbidden(c, "No permission to view performance history")
		return
	}

	// 解析参数
	aggregationType := c.DefaultQuery("type", "hourly")
	fromStr := c.DefaultQuery("from", time.Now().AddDate(0, 0, -7).Format("2006-01-02"))
	toStr := c.DefaultQuery("to", time.Now().Format("2006-01-02"))

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		response.BadRequest(c, "Invalid from date format")
		return
	}

	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		response.BadRequest(c, "Invalid to date format")
		return
	}

	// 获取性能历史数据
	history, err := h.app.DB.DB.SQLite.GetNodePerformanceHistory(nodeID, aggregationType, from, to)
	if err != nil {
		response.InternalError(c, "Failed to get performance history")
		return
	}

	response.Success(c, gin.H{
		"node_id":          nodeID,
		"aggregation_type": aggregationType,
		"from":             from,
		"to":               to,
		"data_count":       len(history),
		"data":             history,
	})
}

// GetNodeMonitoringConfig 获取节点监控配置
func (h *MonitoringHandler) GetNodeMonitoringConfig(c *gin.Context) {
	nodeID := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	// 权限检查 - 只有管理员和节点所有者可以查看配置
	node, err := h.app.DB.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	if role != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission to view monitoring config")
		return
	}

	config, err := h.app.DB.DB.SQLite.GetNodeMonitoringConfig(nodeID)
	if err != nil {
		response.InternalError(c, "Failed to get monitoring config")
		return
	}

	// 如果没有配置，返回默认值
	if config == nil {
		config = &dbinit.NodeMonitoringConfig{
			NodeID:               nodeID,
			MonitoringEnabled:    true,
			ReportInterval:       60,
			CollectSystemInfo:    true,
			CollectNetworkStats:  true,
			CollectTunnelStats:   true,
			CollectPerformance:   true,
			DataRetentionDays:    30,
			AlertCPUThreshold:    80.0,
			AlertMemoryThreshold: 80.0,
			AlertDiskThreshold:   90.0,
		}
	}

	response.Success(c, config)
}

// UpdateNodeMonitoringConfig 更新节点监控配置
func (h *MonitoringHandler) UpdateNodeMonitoringConfig(c *gin.Context) {
	nodeID := c.Param("id")
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	// 权限检查 - 只有管理员可以更新配置
	if role != "admin" {
		response.Forbidden(c, "Only admin can update monitoring config")
		return
	}

	var req dbinit.NodeMonitoringConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	req.NodeID = nodeID
	if err := h.app.DB.DB.SQLite.UpsertNodeMonitoringConfig(&req); err != nil {
		logger.Error("更新监控配置失败",
			zap.String("nodeID", nodeID),
			zap.Error(err))
		response.InternalError(c, "Failed to update monitoring config")
		return
	}

	logger.Info("节点监控配置已更新",
		zap.String("nodeID", nodeID),
		zap.String("updatedBy", userID.(string)))

	response.SuccessWithMessage(c, "Monitoring config updated", &req)
}

// ListNodeMonitoringOverview 获取监控概览（管理员）
func (h *MonitoringHandler) ListNodeMonitoringOverview(c *gin.Context) {
	role, _ := c.Get("role")
	userID, _ := c.Get("user_id")

	// 获取节点列表
	var nodes []*dbinit.Node
	var err error

	if role == "admin" {
		// 管理员可以看到所有节点
		nodes, err = h.app.DB.DB.SQLite.ListNodes("", "", 1000, 0)
	} else {
		// 普通用户只能看到自己的节点
		nodes, err = h.app.DB.DB.SQLite.ListNodes("", "", 1000, 0)
		if err == nil {
			var userNodes []*dbinit.Node
			for _, node := range nodes {
				if node.UserID == userID.(string) {
					userNodes = append(userNodes, node)
				}
			}
			nodes = userNodes
		}
	}

	if err != nil {
		response.InternalError(c, "Failed to get nodes")
		return
	}

	// 获取每个节点的监控状态
	var overview []gin.H
	onlineCount := 0
	totalNodes := len(nodes)
	totalAlerts := 0

	for _, node := range nodes {
		status, err := h.monitoringService.GetNodeMonitoringStatus(node.ID)
		if err != nil {
			continue
		}

		if status.IsOnline {
			onlineCount++
		}
		totalAlerts += status.ActiveAlerts

		nodeInfo := gin.H{
			"node_id":            node.ID,
			"node_name":          node.Name,
			"node_type":          node.Type,
			"group_id":           node.GroupID,
			"is_online":          status.IsOnline,
			"last_seen":          status.LastSeen,
			"has_monitoring":     status.HasData,
			"cpu_usage":          status.CPUUsage,
			"memory_usage":       status.MemoryUsage,
			"disk_usage":         status.DiskUsage,
			"active_connections": status.ActiveConnections,
			"active_tunnels":     status.ActiveTunnels,
			"response_time":      status.ResponseTime,
			"active_alerts":      status.ActiveAlerts,
			"uptime":             status.Uptime,
		}

		overview = append(overview, nodeInfo)
	}

	response.Success(c, gin.H{
		"summary": gin.H{
			"total_nodes":   totalNodes,
			"online_nodes":  onlineCount,
			"offline_nodes": totalNodes - onlineCount,
			"total_alerts":  totalAlerts,
		},
		"nodes": overview,
	})
}

// GetNodeAlerts 获取节点告警列表
func (h *MonitoringHandler) GetNodeAlerts(c *gin.Context) {
	nodeID := c.Param("id")
	userID, _ := c.Get("user_id")

	// 权限检查
	if !h.monitoringService.CheckMonitoringPermission(userID.(string), nodeID, "view_basic") {
		response.Forbidden(c, "No permission to view alerts")
		return
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	alerts, err := h.app.DB.DB.SQLite.ListNodeAlertHistory(nodeID, limit)
	if err != nil {
		response.InternalError(c, "Failed to get alerts")
		return
	}

	response.Success(c, gin.H{
		"node_id": nodeID,
		"alerts":  alerts,
		"total":   len(alerts),
	})
}

// CreateMonitoringPermission 创建监控权限（管理员）
func (h *MonitoringHandler) CreateMonitoringPermission(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req struct {
		TargetUserID   string `json:"user_id" binding:"required"`
		NodeID         string `json:"node_id"` // 可为空表示全局配置
		PermissionType string `json:"permission_type" binding:"required"`
		Enabled        bool   `json:"enabled"`
		Description    string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 验证权限类型
	validTypes := map[string]bool{
		"view_basic": true, "view_detailed": true,
		"view_system": true, "view_network": true, "disabled": true,
	}
	if !validTypes[req.PermissionType] {
		response.BadRequest(c, "Invalid permission type")
		return
	}

	// 如果指定了节点，验证节点是否存在
	if req.NodeID != "" {
		node, err := h.app.DB.DB.SQLite.GetNode(req.NodeID)
		if err != nil || node == nil {
			response.NotFound(c, "Node not found")
			return
		}
	}

	// 验证目标用户是否存在
	targetUser, err := h.app.DB.DB.SQLite.GetUser(req.TargetUserID)
	if err != nil || targetUser == nil {
		response.NotFound(c, "Target user not found")
		return
	}

	permission := &dbinit.MonitoringPermission{
		UserID:         req.TargetUserID,
		NodeID:         req.NodeID,
		PermissionType: req.PermissionType,
		Enabled:        req.Enabled,
		CreatedBy:      userID.(string),
		Description:    req.Description,
	}

	if err := h.app.DB.DB.SQLite.CreateMonitoringPermission(permission); err != nil {
		logger.Error("创建监控权限失败", zap.Error(err))
		response.InternalError(c, "Failed to create monitoring permission")
		return
	}

	logger.Info("监控权限已创建",
		zap.String("targetUserID", req.TargetUserID),
		zap.String("nodeID", req.NodeID),
		zap.String("permissionType", req.PermissionType),
		zap.String("createdBy", userID.(string)))

	response.SuccessWithMessage(c, "Monitoring permission created", permission)
}

// ListMonitoringPermissions 列出监控权限（管理员）
func (h *MonitoringHandler) ListMonitoringPermissions(c *gin.Context) {
	targetUserID := c.Query("user_id")

	permissions, err := h.app.DB.DB.SQLite.ListMonitoringPermissions(targetUserID)
	if err != nil {
		response.InternalError(c, "Failed to list monitoring permissions")
		return
	}

	// 添加用户和节点信息
	var permissionsWithInfo []gin.H
	for _, perm := range permissions {
		permInfo := gin.H{
			"id":              perm.ID,
			"user_id":         perm.UserID,
			"node_id":         perm.NodeID,
			"permission_type": perm.PermissionType,
			"enabled":         perm.Enabled,
			"created_by":      perm.CreatedBy,
			"created_at":      perm.CreatedAt,
			"updated_at":      perm.UpdatedAt,
			"description":     perm.Description,
		}

		// 添加用户信息
		if user, err := h.app.DB.DB.SQLite.GetUser(perm.UserID); err == nil && user != nil {
			permInfo["username"] = user.Username
		}

		// 添加节点信息
		if perm.NodeID != "" {
			if node, err := h.app.DB.DB.SQLite.GetNode(perm.NodeID); err == nil && node != nil {
				permInfo["node_name"] = node.Name
			}
		} else {
			permInfo["node_name"] = "全局配置"
		}

		permissionsWithInfo = append(permissionsWithInfo, permInfo)
	}

	response.Success(c, gin.H{
		"permissions": permissionsWithInfo,
		"total":       len(permissionsWithInfo),
	})
}

// GetMyMonitoringPermissions 获取我的监控权限
func (h *MonitoringHandler) GetMyMonitoringPermissions(c *gin.Context) {
	userID, _ := c.Get("user_id")

	permissions, err := h.app.DB.DB.SQLite.ListMonitoringPermissions(userID.(string))
	if err != nil {
		response.InternalError(c, "Failed to get monitoring permissions")
		return
	}

	response.Success(c, gin.H{
		"permissions": permissions,
		"total":       len(permissions),
	})
}

// validateNodeAccess 验证节点访问权限（用于数据上报）
func (h *MonitoringHandler) validateNodeAccess(c *gin.Context, nodeID string) bool {
	// 从Header获取认证信息
	apiKey := c.GetHeader("X-API-Key")
	connectionKey := c.GetHeader("X-Connection-Key")

	if apiKey != "" {
		// 通过API Key验证
		node, err := h.app.DB.DB.SQLite.GetNodeByAPIKey(apiKey)
		return err == nil && node != nil && node.ID == nodeID
	}

	if connectionKey != "" {
		// 通过Connection Key验证
		ck, err := h.app.DB.DB.SQLite.GetConnectionKeyByKey(connectionKey)
		return err == nil && ck != nil && ck.NodeID == nodeID && ck.Type == "node"
	}

	return false
}

// NodeMonitoringSummary 节点监控汇总（用于Dashboard）
func (h *MonitoringHandler) NodeMonitoringSummary(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	var nodes []*dbinit.Node
	var err error

	// 根据角色获取节点列表
	if role == "admin" {
		nodes, err = h.app.DB.DB.SQLite.ListNodes("", "", 1000, 0)
	} else {
		nodes, err = h.app.DB.DB.SQLite.ListNodes("", "", 1000, 0)
		if err == nil {
			var userNodes []*dbinit.Node
			for _, node := range nodes {
				if node.UserID == userID.(string) {
					userNodes = append(userNodes, node)
				}
			}
			nodes = userNodes
		}
	}

	if err != nil {
		response.InternalError(c, "Failed to get nodes")
		return
	}

	// 统计数据
	var summary struct {
		TotalNodes       int     `json:"total_nodes"`
		OnlineNodes      int     `json:"online_nodes"`
		MonitoredNodes   int     `json:"monitored_nodes"`
		TotalAlerts      int     `json:"total_alerts"`
		AvgCPUUsage      float64 `json:"avg_cpu_usage"`
		AvgMemoryUsage   float64 `json:"avg_memory_usage"`
		TotalConnections int     `json:"total_connections"`
		TotalTraffic     int64   `json:"total_traffic"`
	}

	summary.TotalNodes = len(nodes)
	var totalCPU, totalMemory float64
	var monitoredCount int

	for _, node := range nodes {
		status, err := h.monitoringService.GetNodeMonitoringStatus(node.ID)
		if err != nil {
			continue
		}

		if status.IsOnline {
			summary.OnlineNodes++
		}

		if status.HasData {
			summary.MonitoredNodes++
			monitoredCount++
			totalCPU += status.CPUUsage
			totalMemory += status.MemoryUsage
			summary.TotalConnections += status.ActiveConnections
			summary.TotalTraffic += status.TrafficIn + status.TrafficOut
		}

		summary.TotalAlerts += status.ActiveAlerts
	}

	if monitoredCount > 0 {
		summary.AvgCPUUsage = totalCPU / float64(monitoredCount)
		summary.AvgMemoryUsage = totalMemory / float64(monitoredCount)
	}

	response.Success(c, summary)
}

