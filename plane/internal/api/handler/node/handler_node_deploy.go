package node

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// NodeDeployHandler 节点部署处理器
type NodeDeployHandler struct {
	app *types.App
}

// NewNodeDeployHandler 创建节点部署处理器
func NewNodeDeployHandler(app *types.App) *NodeDeployHandler {
	return &NodeDeployHandler{app: app}
}

// DeployNodeRequest 部署节点请求
type DeployNodeRequest struct {
	Name         string `json:"name"`          // 服务器名（可选）
	ConnectionIP string `json:"connection_ip"` // 连接IP或域名（可选）
	ExitNetwork  string `json:"exit_network"`  // 出口网络（可选）
	DebugMode    bool   `json:"debug_mode"`    // 调试模式
}

// CreateNodeResponse 创建节点响应
type CreateNodeResponse struct {
	ID              string    `json:"id"`
	GroupID         string    `json:"group_id"`
	Name            string    `json:"name,omitempty"`
	DeploymentToken string    `json:"deployment_token"`
	ConnectionIP    string    `json:"connection_ip,omitempty"`
	ExitNetwork     string    `json:"exit_network,omitempty"`
	DebugMode       bool      `json:"debug_mode"`
	Status          string    `json:"status"`
	InstallCommand  string    `json:"install_command"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateNode 创建节点并生成部署Token
func (h *NodeDeployHandler) CreateNode(c *gin.Context) {
	groupID := c.Param("id")

	var req DeployNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 验证节点组是否存在
	group, err := h.app.DB.DB.SQLite.GetNodeGroup(groupID)
	if err != nil || group == nil {
		response.NotFound(c, "Node group not found")
		return
	}

	// 权限检查
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	if role != "admin" && group.UserID != userID.(string) {
		response.Forbidden(c, "No permission to deploy nodes in this group")
		return
	}

	// 生成节点ID和部署Token
	nodeID := uuid.New().String()
	deploymentToken := generateDeploymentToken()

	// 创建节点记录
	now := time.Now()
	node := &dbinit.Node{
		ID:          nodeID,
		Name:        req.Name,
		Type:        group.Type,
		Status:      "pending",
		IP:          req.ConnectionIP,
		Port:        443, // 默认端口
		GroupID:     groupID,
		UserID:      group.UserID,
		CreatedAt:   now,
		UpdatedAt:   now,
		Description: fmt.Sprintf("Deployment pending - Token: %s", deploymentToken[:8]+"..."),
	}

	if err := h.app.DB.DB.SQLite.CreateNode(node); err != nil {
		logger.Error("创建节点失败", zap.Error(err))
		response.InternalError(c, "Failed to create node")
		return
	}

	// 存储部署Token（使用APIKey字段临时存储，后续可以改用专门的表）
	node.APIKey = deploymentToken
	if err := h.app.DB.DB.SQLite.UpdateNode(node); err != nil {
		logger.Error("保存部署Token失败", zap.Error(err))
	}

	// 生成安装命令
	serverURL := fmt.Sprintf("https://%s", c.Request.Host)

	installCmd := generateInstallCommand(serverURL, deploymentToken, nodeID, req)

	// 构建响应
	resp := CreateNodeResponse{
		ID:              nodeID,
		GroupID:         groupID,
		Name:            req.Name,
		DeploymentToken: deploymentToken,
		ConnectionIP:    req.ConnectionIP,
		ExitNetwork:     req.ExitNetwork,
		DebugMode:       req.DebugMode,
		Status:          "pending",
		InstallCommand:  installCmd,
		CreatedAt:       now,
	}

	response.SuccessWithMessage(c, "Node created successfully", resp)
}

// RegisterNodeRequest 节点注册请求（服务器端调用）
type RegisterNodeRequest struct {
	NodeID          string                 `json:"node_id" binding:"required"`
	DeploymentToken string                 `json:"deployment_token" binding:"required"`
	ServerInfo      map[string]interface{} `json:"server_info"`
}

// RegisterNode 节点注册
func (h *NodeDeployHandler) RegisterNode(c *gin.Context) {
	var req RegisterNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 获取节点
	node, err := h.app.DB.DB.SQLite.GetNode(req.NodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 验证部署Token
	if node.APIKey != req.DeploymentToken {
		response.Unauthorized(c, "Invalid deployment token")
		return
	}

	// 检查节点状态
	if node.Status != "pending" {
		response.BadRequest(c, "Node already registered")
		return
	}

	// 更新节点状态
	now := time.Now()
	node.Status = "online"
	node.LastSeen = now

	// 从ServerInfo中提取公网IP
	if publicIP, ok := req.ServerInfo["public_ip"].(string); ok && publicIP != "" {
		node.IP = publicIP
	}

	// 保存服务器信息
	if req.ServerInfo != nil {
		serverInfoJSON, _ := json.Marshal(req.ServerInfo)
		node.Tags = string(serverInfoJSON)
	}

	node.UpdatedAt = now

	if err := h.app.DB.DB.SQLite.UpdateNode(node); err != nil {
		logger.Error("更新节点状态失败", zap.Error(err))
		response.InternalError(c, "Failed to register node")
		return
	}

	// 清除部署Token（已使用）
	node.APIKey = ""
	h.app.DB.DB.SQLite.UpdateNode(node)

	// 获取节点组配置
	groupConfig, _ := h.app.DB.DB.SQLite.GetNodeGroupConfig(node.GroupID)

	response.SuccessWithMessage(c, "Node registered successfully", gin.H{
		"node_id": node.ID,
		"config": gin.H{
			"group_config": groupConfig,
			"node_config": gin.H{
				"id":       node.ID,
				"group_id": node.GroupID,
				"type":     node.Type,
			},
		},
	})
}

// NodeHeartbeatRequest 节点心跳请求
type NodeHeartbeatRequest struct {
	DeploymentToken string                 `json:"deployment_token"`
	Status          map[string]interface{} `json:"status"`
}

// NodeHeartbeat 节点心跳
func (h *NodeDeployHandler) NodeHeartbeat(c *gin.Context) {
	nodeID := c.Param("id")

	var req NodeHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 获取节点
	node, err := h.app.DB.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 更新最后心跳时间
	now := time.Now()
	node.LastSeen = now
	node.Status = "online"

	// 更新状态信息
	if req.Status != nil {
		statusJSON, _ := json.Marshal(req.Status)
		node.Tags = string(statusJSON)
	}

	node.UpdatedAt = now

	if err := h.app.DB.DB.SQLite.UpdateNode(node); err != nil {
		logger.Error("更新节点心跳失败", zap.Error(err))
		response.InternalError(c, "Failed to update heartbeat")
		return
	}

	response.Success(c, gin.H{
		"status":    "ok",
		"timestamp": now,
	})
}

// ListNodesInGroup 列出节点组内的节点
func (h *NodeDeployHandler) ListNodesInGroup(c *gin.Context) {
	groupID := c.Param("id")

	// 验证节点组是否存在
	group, err := h.app.DB.DB.SQLite.GetNodeGroup(groupID)
	if err != nil || group == nil {
		response.NotFound(c, "Node group not found")
		return
	}

	// 权限检查
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	if role != "admin" && group.UserID != userID.(string) {
		response.Forbidden(c, "No permission to view nodes in this group")
		return
	}

	// 获取节点列表
	nodes, err := h.app.DB.DB.SQLite.GetNodesInGroup(groupID)
	if err != nil {
		logger.Error("获取节点列表失败", zap.Error(err))
		response.InternalError(c, "Failed to list nodes")
		return
	}

	// 统计节点状态
	var pending, online, offline int
	for _, node := range nodes {
		switch node.Status {
		case "pending":
			pending++
		case "online":
			// 检查是否超过5分钟未心跳
			if time.Since(node.LastSeen) > 5*time.Minute {
				offline++
			} else {
				online++
			}
		case "offline":
			offline++
		}
	}

	response.Success(c, gin.H{
		"data":    nodes,
		"total":   len(nodes),
		"pending": pending,
		"online":  online,
		"offline": offline,
	})
}

// generateDeploymentToken 生成部署Token
func generateDeploymentToken() string {
	// 格式：gkipass_deploy_{timestamp}_{random}
	timestamp := time.Now().Unix()

	// 生成32字节随机数
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)
	randomStr := hex.EncodeToString(randomBytes)

	return fmt.Sprintf("gkipass_deploy_%d_%s", timestamp, randomStr)
}

// generateInstallCommand 生成安装命令
func generateInstallCommand(serverURL, token, nodeID string, req DeployNodeRequest) string {
	baseCmd := fmt.Sprintf("curl -sSL https://dl.relayx.cc/install.sh | bash -s -- -s %s -t %s -n %s",
		serverURL, token, nodeID)

	// 添加可选参数
	if req.Name != "" {
		baseCmd += fmt.Sprintf(" --name \"%s\"", req.Name)
	}
	if req.ConnectionIP != "" {
		baseCmd += fmt.Sprintf(" --ip %s", req.ConnectionIP)
	}
	if req.ExitNetwork != "" {
		baseCmd += fmt.Sprintf(" --interface %s", req.ExitNetwork)
	}
	if req.DebugMode {
		baseCmd += " --debug"
	}

	return baseCmd
}
