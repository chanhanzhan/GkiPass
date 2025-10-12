package node

import (
	"encoding/json"
	"fmt"
	"time"

	"gkipass/plane/db/sqlite"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// NodeGroupConfigHandler 节点组配置处理器
type NodeGroupConfigHandler struct {
	app *types.App
}

// NewNodeGroupConfigHandler 创建节点组配置处理器
func NewNodeGroupConfigHandler(app *types.App) *NodeGroupConfigHandler {
	return &NodeGroupConfigHandler{app: app}
}

// NodeGroupConfigRequest 节点组配置请求
type NodeGroupConfigRequest struct {
	AllowedProtocols  []string `json:"allowed_protocols"`  // 允许的协议列表
	PortRange         string   `json:"port_range"`         // 端口范围（如 "10000-60000"）
	PortRangeStart    int      `json:"port_range_start"`   // 端口范围起始（前端使用）
	PortRangeEnd      int      `json:"port_range_end"`     // 端口范围结束（前端使用）
	TrafficMultiplier float64  `json:"traffic_multiplier"` // 流量倍率
}

// GetNodeGroupConfig 获取节点组配置
func (h *NodeGroupConfigHandler) GetNodeGroupConfig(c *gin.Context) {
	groupID := c.Param("id")

	// 验证节点组是否存在
	group, err := h.app.DB.DB.SQLite.GetNodeGroup(groupID)
	if err != nil || group == nil {
		response.GinNotFound(c, "Node group not found")
		return
	}

	// 权限检查
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	if role != "admin" && group.UserID != userID.(string) {
		response.GinForbidden(c, "No permission to view this node group config")
		return
	}

	// 获取配置
	config, err := h.app.DB.DB.SQLite.GetNodeGroupConfig(groupID)
	if err != nil {
		logger.Error("获取节点组配置失败", zap.Error(err))
		response.InternalError(c, "Failed to get node group config")
		return
	}

	// 如果配置不存在，返回默认配置
	if config == nil {
		response.GinSuccess(c, gin.H{
			"group_id":           groupID,
			"allowed_protocols":  []string{},
			"port_range":         "10000-60000",
			"port_range_start":   10000,
			"port_range_end":     60000,
			"traffic_multiplier": 1.0,
			"updated_at":         time.Now(),
		})
		return
	}

	// 解析协议列表
	var protocols []string
	if err := json.Unmarshal([]byte(config.AllowedProtocols), &protocols); err != nil {
		protocols = []string{}
	}

	// 解析端口范围（格式："10000-60000"）
	var startPort, endPort int
	fmt.Sscanf(config.PortRange, "%d-%d", &startPort, &endPort)

	response.GinSuccess(c, gin.H{
		"group_id":           config.GroupID,
		"allowed_protocols":  protocols,
		"port_range":         config.PortRange,
		"port_range_start":   startPort,
		"port_range_end":     endPort,
		"traffic_multiplier": config.TrafficMultiplier,
		"updated_at":         config.UpdatedAt,
	})
}

// UpdateNodeGroupConfig 更新节点组配置
func (h *NodeGroupConfigHandler) UpdateNodeGroupConfig(c *gin.Context) {
	groupID := c.Param("id")

	var req NodeGroupConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.GinBadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 验证节点组是否存在
	group, err := h.app.DB.DB.SQLite.GetNodeGroup(groupID)
	if err != nil || group == nil {
		response.GinNotFound(c, "Node group not found")
		return
	}

	// 权限检查
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	if role != "admin" && group.UserID != userID.(string) {
		response.GinForbidden(c, "No permission to update this node group config")
		return
	}

	// 验证参数
	if req.PortRangeStart <= 0 || req.PortRangeEnd <= 0 {
		response.GinBadRequest(c, "Invalid port range")
		return
	}
	if req.PortRangeStart >= req.PortRangeEnd {
		response.GinBadRequest(c, "Port range start must be less than end")
		return
	}
	if req.TrafficMultiplier <= 0 {
		response.GinBadRequest(c, "Traffic multiplier must be greater than 0")
		return
	}

	// 序列化协议列表
	protocolsJSON, err := json.Marshal(req.AllowedProtocols)
	if err != nil {
		response.GinBadRequest(c, "Invalid protocols format")
		return
	}

	// 构造端口范围字符串
	portRange := fmt.Sprintf("%d-%d", req.PortRangeStart, req.PortRangeEnd)

	// 更新配置
	config := &sqlite.NodeGroupConfig{
		ID:                uuid.New().String(),
		GroupID:           groupID,
		AllowedProtocols:  string(protocolsJSON),
		PortRange:         portRange,
		TrafficMultiplier: req.TrafficMultiplier,
		UpdatedAt:         time.Now(),
	}

	if err := h.app.DB.DB.SQLite.UpsertNodeGroupConfig(config); err != nil {
		logger.Error("更新节点组配置失败", zap.Error(err))
		response.InternalError(c, "Failed to update node group config")
		return
	}

	response.SuccessWithMessage(c, "Node group config updated successfully", gin.H{
		"group_id":           config.GroupID,
		"allowed_protocols":  req.AllowedProtocols,
		"port_range":         portRange,
		"port_range_start":   req.PortRangeStart,
		"port_range_end":     req.PortRangeEnd,
		"traffic_multiplier": config.TrafficMultiplier,
		"updated_at":         config.UpdatedAt,
	})
}

// ResetNodeGroupConfig 重置节点组配置为默认值
func (h *NodeGroupConfigHandler) ResetNodeGroupConfig(c *gin.Context) {
	groupID := c.Param("id")

	// 验证节点组是否存在
	group, err := h.app.DB.DB.SQLite.GetNodeGroup(groupID)
	if err != nil || group == nil {
		response.GinNotFound(c, "Node group not found")
		return
	}

	// 权限检查（仅管理员）
	role, _ := c.Get("role")
	if role != "admin" {
		response.GinForbidden(c, "Only admin can reset node group config")
		return
	}

	// 重置为默认配置
	defaultProtocols, _ := json.Marshal([]string{"tcp", "udp"})
	config := &sqlite.NodeGroupConfig{
		ID:                uuid.New().String(),
		GroupID:           groupID,
		AllowedProtocols:  string(defaultProtocols),
		PortRange:         "10000-60000",
		TrafficMultiplier: 1.0,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := h.app.DB.DB.SQLite.UpsertNodeGroupConfig(config); err != nil {
		logger.Error("重置节点组配置失败", zap.Error(err))
		response.InternalError(c, "Failed to reset node group config")
		return
	}

	response.SuccessWithMessage(c, "Node group config reset successfully", gin.H{
		"group_id":           config.GroupID,
		"allowed_protocols":  []string{"tcp", "udp"},
		"port_range":         "10000-60000",
		"port_range_start":   10000,
		"port_range_end":     60000,
		"traffic_multiplier": config.TrafficMultiplier,
	})
}
