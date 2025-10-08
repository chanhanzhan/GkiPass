package node

import (
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/auth"
	"gkipass/plane/internal/service"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// NodeHandler 节点处理器
type NodeHandler struct {
	app *types.App
}

// NewNodeHandler 创建节点处理器
func NewNodeHandler(app *types.App) *NodeHandler {
	return &NodeHandler{app: app}
}

// CreateNodeRequest 创建节点请求
type CreateNodeRequest struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type" binding:"required,oneof=client server"`
	IP          string `json:"ip" binding:"required"`
	Port        int    `json:"port" binding:"required"`
	GroupID     string `json:"group_id"` // 可选：节点组ID
	Description string `json:"description"`
}

// Create 创建节点
func (h *NodeHandler) Create(c *gin.Context) {
	var req CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 获取当前用户ID
	userID, _ := c.Get("user_id")

	// 如果指定了组，验证组是否存在且属于当前用户
	if req.GroupID != "" {
		group, err := h.app.DB.DB.SQLite.GetNodeGroup(req.GroupID)
		if err != nil || group == nil {
			response.BadRequest(c, "Node group not found")
			return
		}

		// 权限检查
		userRole, _ := c.Get("role")
		if userRole != "admin" && group.UserID != userID.(string) {
			response.Forbidden(c, "No permission to use this node group")
			return
		}
	}

	// 生成全局唯一的节点标识符（UUID v4）
	nodeID := uuid.New().String()

	node := &dbinit.Node{
		ID:          nodeID,
		Name:        req.Name,
		Type:        req.Type,
		Status:      "offline", // 新建节点默认离线，等待连接
		IP:          req.IP,
		Port:        req.Port,
		APIKey:      uuid.New().String(), // 生成API Key（废弃，用CK替代）
		GroupID:     req.GroupID,
		UserID:      userID.(string),
		LastSeen:    time.Now(),
		Description: req.Description,
	}

	if err := h.app.DB.DB.SQLite.CreateNode(node); err != nil {
		logger.Error("创建节点失败", zap.Error(err))
		response.InternalError(c, "Failed to create node: "+err.Error())
		return
	}

	// 自动为节点生成 Connection Key（30天有效）
	ck := auth.CreateNodeCK(nodeID, 30*24*time.Hour)
	if err := h.app.DB.DB.SQLite.CreateConnectionKey(ck); err != nil {
		logger.Error("生成CK失败", zap.Error(err))
		// 不影响节点创建，只记录错误
	}

	// 自动生成节点证书
	certManager, err := service.NewNodeCertManager(h.app.DB, "./certs")
	if err != nil {
		logger.Warn("初始化证书管理器失败，跳过证书生成", zap.Error(err))
	} else {
		cert, err := certManager.GenerateNodeCert(nodeID, node.Name)
		if err != nil {
			logger.Warn("生成节点证书失败", zap.Error(err))
		} else {
			node.CertID = cert.ID
			// 更新节点记录
			_ = h.app.DB.DB.SQLite.UpdateNode(node)
			logger.Info("✓ 节点证书已生成", zap.String("certID", cert.ID))
		}
	}

	logger.Info("节点已创建",
		zap.String("nodeID", nodeID),
		zap.String("name", node.Name),
		zap.String("groupID", node.GroupID))

	response.SuccessWithMessage(c, "Node created successfully", gin.H{
		"node":           node,
		"connection_key": ck.Key,
		"usage":          "节点启动命令: ./client --token " + ck.Key,
		"expires_at":     ck.ExpiresAt,
		"cert_ready":     node.CertID != "",
	})
}

// ListNodesRequest 列出节点请求
type ListNodesRequest struct {
	Type   string `form:"type"`
	Status string `form:"status"`
	Limit  int    `form:"limit"`
	Offset int    `form:"offset"`
}

// List 列出节点
func (h *NodeHandler) List(c *gin.Context) {
	var req ListNodesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 设置默认值
	if req.Limit == 0 {
		req.Limit = 50
	}

	// 获取当前用户
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")

	nodes, err := h.app.DB.DB.SQLite.ListNodes(req.Type, req.Status, req.Limit, req.Offset)
	if err != nil {
		response.InternalError(c, "Failed to list nodes: "+err.Error())
		return
	}

	// 普通用户只能看到自己的节点
	if userRole != "admin" {
		filteredNodes := []*dbinit.Node{}
		for _, node := range nodes {
			if node.UserID == userID.(string) {
				filteredNodes = append(filteredNodes, node)
			}
		}
		nodes = filteredNodes
	}

	// 如果有Redis，合并实时状态
	if h.app.DB.HasCache() {
		statuses, _ := h.app.DB.Cache.Redis.GetAllNodeStatus()
		for _, node := range nodes {
			if status, exists := statuses[node.ID]; exists {
				if status.Online {
					node.Status = "online"
				}
				node.LastSeen = status.LastHeartbeat
			}
		}
	}

	response.Success(c, gin.H{
		"nodes": nodes,
		"total": len(nodes),
	})
}

// Get 获取节点详情
func (h *NodeHandler) Get(c *gin.Context) {
	id := c.Param("id")

	node, err := h.app.DB.DB.SQLite.GetNode(id)
	if err != nil {
		response.InternalError(c, "Failed to get node: "+err.Error())
		return
	}

	if node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 权限检查：普通用户只能查看自己的节点
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission to access this node")
		return
	}

	// 如果有Redis，获取实时状态
	if h.app.DB.HasCache() {
		status, _ := h.app.DB.Cache.Redis.GetNodeStatus(id)
		if status != nil && status.Online {
			node.Status = "online"
			node.LastSeen = status.LastHeartbeat
		}
	}

	response.Success(c, node)
}

// UpdateNodeRequest 更新节点请求
type UpdateNodeRequest struct {
	Name        string `json:"name"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	GroupID     string `json:"group_id"`
	Status      string `json:"status" binding:"omitempty,oneof=online offline error"`
	Description string `json:"description"`
}

// Update 更新节点
func (h *NodeHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req UpdateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	node, err := h.app.DB.DB.SQLite.GetNode(id)
	if err != nil {
		response.InternalError(c, "Failed to get node: "+err.Error())
		return
	}

	if node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 权限检查：普通用户只能更新自己的节点
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission to update this node")
		return
	}

	// 更新字段
	if req.Name != "" {
		node.Name = req.Name
	}
	if req.IP != "" {
		node.IP = req.IP
	}
	if req.Port > 0 {
		node.Port = req.Port
	}
	if req.GroupID != "" {
		// 验证组是否存在且属于当前用户
		group, err := h.app.DB.DB.SQLite.GetNodeGroup(req.GroupID)
		if err != nil || group == nil {
			response.BadRequest(c, "Node group not found")
			return
		}
		if userRole != "admin" && group.UserID != userID.(string) {
			response.Forbidden(c, "No permission to use this node group")
			return
		}
		node.GroupID = req.GroupID
	}
	if req.Status != "" {
		node.Status = req.Status
	}
	if req.Description != "" {
		node.Description = req.Description
	}

	if err := h.app.DB.DB.SQLite.UpdateNode(node); err != nil {
		response.InternalError(c, "Failed to update node: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "Node updated successfully", node)
}

// Delete 删除节点
func (h *NodeHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	// 获取节点信息进行权限检查
	node, err := h.app.DB.DB.SQLite.GetNode(id)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 权限检查：普通用户只能删除自己的节点
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && node.UserID != userID.(string) {
		response.Forbidden(c, "No permission to delete this node")
		return
	}

	if err := h.app.DB.DB.SQLite.DeleteNode(id); err != nil {
		response.InternalError(c, "Failed to delete node: "+err.Error())
		return
	}

	// 清理Redis缓存
	if h.app.DB.HasCache() {
		_ = h.app.DB.Cache.Redis.Delete("node:status:" + id)
		_ = h.app.DB.Cache.Redis.InvalidatePolicyCache(id)
	}

	response.SuccessWithMessage(c, "Node deleted successfully", nil)
}

// HeartbeatRequest 心跳请求
type HeartbeatRequest struct {
	Load        float64 `json:"load"`
	Connections int     `json:"connections"`
}

// Heartbeat 节点心跳
func (h *NodeHandler) Heartbeat(c *gin.Context) {
	id := c.Param("id")

	var req HeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 更新数据库中的last_seen
	node, err := h.app.DB.DB.SQLite.GetNode(id)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	node.LastSeen = time.Now()
	node.Status = "online"
	_ = h.app.DB.DB.SQLite.UpdateNode(node)

	// 更新Redis实时状态
	if h.app.DB.HasCache() {
		status := &dbinit.NodeStatus{
			NodeID:        id,
			Online:        true,
			LastHeartbeat: time.Now(),
			CurrentLoad:   req.Load,
			Connections:   req.Connections,
		}
		_ = h.app.DB.Cache.Redis.SetNodeStatus(id, status)
	}

	response.Success(c, gin.H{
		"status": "ok",
		"time":   time.Now().Unix(),
	})
}
