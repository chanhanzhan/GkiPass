package node

import (
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// NodeGroupHandler 节点组处理器
type NodeGroupHandler struct {
	app *types.App
}

// NewNodeGroupHandler 创建节点组处理器
func NewNodeGroupHandler(app *types.App) *NodeGroupHandler {
	return &NodeGroupHandler{app: app}
}

// CreateNodeGroupRequest 创建节点组请求
type CreateNodeGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type" binding:"required,oneof=entry exit"`
	Description string `json:"description"`
}

// Create 创建节点组
func (h *NodeGroupHandler) Create(c *gin.Context) {
	var req CreateNodeGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 获取当前用户ID
	userID, _ := c.Get("user_id")

	group := &dbinit.NodeGroup{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Type:        req.Type,
		UserID:      userID.(string),
		Description: req.Description,
	}

	if err := h.app.DB.DB.SQLite.CreateNodeGroup(group); err != nil {
		response.InternalError(c, "Failed to create node group: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "Node group created successfully", group)
}

// List 列出节点组
func (h *NodeGroupHandler) List(c *gin.Context) {
	groupType := c.Query("type")

	// 获取当前用户ID
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")

	// 管理员可以看到所有组，普通用户只能看到自己的
	var uid string
	if userRole == "admin" {
		uid = c.Query("user_id") // 管理员可以指定用户ID
	} else {
		uid = userID.(string)
	}

	groups, err := h.app.DB.DB.SQLite.ListNodeGroups(uid, groupType)
	if err != nil {
		response.InternalError(c, "Failed to list node groups: "+err.Error())
		return
	}

	response.GinSuccess(c, gin.H{
		"groups": groups,
		"total":  len(groups),
	})
}

// Get 获取节点组详情
func (h *NodeGroupHandler) Get(c *gin.Context) {
	id := c.Param("id")

	group, err := h.app.DB.DB.SQLite.GetNodeGroup(id)
	if err != nil {
		response.InternalError(c, "Failed to get node group: "+err.Error())
		return
	}

	if group == nil {
		response.GinNotFound(c, "Node group not found")
		return
	}

	// 权限检查：普通用户只能查看自己的组
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && group.UserID != userID.(string) {
		response.GinForbidden(c, "No permission to access this node group")
		return
	}

	// 获取组内节点列表
	nodes, _ := h.app.DB.DB.SQLite.GetNodesInGroup(id)

	response.GinSuccess(c, gin.H{
		"group": group,
		"nodes": nodes,
	})
}

// UpdateNodeGroupRequest 更新节点组请求
type UpdateNodeGroupRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type" binding:"omitempty,oneof=entry exit"`
	Description string `json:"description"`
}

// Update 更新节点组
func (h *NodeGroupHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req UpdateNodeGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	group, err := h.app.DB.DB.SQLite.GetNodeGroup(id)
	if err != nil {
		response.InternalError(c, "Failed to get node group: "+err.Error())
		return
	}

	if group == nil {
		response.GinNotFound(c, "Node group not found")
		return
	}

	// 权限检查：普通用户只能更新自己的组
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && group.UserID != userID.(string) {
		response.GinForbidden(c, "No permission to update this node group")
		return
	}

	// 更新字段
	if req.Name != "" {
		group.Name = req.Name
	}
	if req.Type != "" {
		group.Type = req.Type
	}
	if req.Description != "" {
		group.Description = req.Description
	}

	if err := h.app.DB.DB.SQLite.UpdateNodeGroup(group); err != nil {
		response.InternalError(c, "Failed to update node group: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "Node group updated successfully", group)
}

// Delete 删除节点组
func (h *NodeGroupHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	group, err := h.app.DB.DB.SQLite.GetNodeGroup(id)
	if err != nil || group == nil {
		response.GinNotFound(c, "Node group not found")
		return
	}

	// 权限检查：普通用户只能删除自己的组
	userID, _ := c.Get("user_id")
	userRole, _ := c.Get("role")
	if userRole != "admin" && group.UserID != userID.(string) {
		response.GinForbidden(c, "No permission to delete this node group")
		return
	}

	// 检查组内是否还有节点
	if group.NodeCount > 0 {
		response.GinBadRequest(c, "Cannot delete node group with nodes, please remove nodes first")
		return
	}

	if err := h.app.DB.DB.SQLite.DeleteNodeGroup(id); err != nil {
		response.InternalError(c, "Failed to delete node group: "+err.Error())
		return
	}

	response.SuccessWithMessage(c, "Node group deleted successfully", nil)
}
