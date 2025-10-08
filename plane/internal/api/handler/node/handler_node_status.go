package node

import (
	"encoding/json"
	"fmt"
	"time"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NodeStatusHandler 节点状态处理器
type NodeStatusHandler struct {
	app *types.App
}

// NewNodeStatusHandler 创建节点状态处理器
func NewNodeStatusHandler(app *types.App) *NodeStatusHandler {
	return &NodeStatusHandler{app: app}
}

// NodeStatusResponse 节点状态响应
type NodeStatusResponse struct {
	// 基本信息
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	GroupID     string `json:"group_id"`
	GroupName   string `json:"group_name"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Description string `json:"description"`

	// 状态信息
	Status   string    `json:"status"` // online/offline
	IsOnline bool      `json:"is_online"`
	LastSeen time.Time `json:"last_seen"`
	Uptime   int64     `json:"uptime"` // 运行时间（秒）

	// 配置信息
	MaxBandwidth   int64 `json:"max_bandwidth"`   // 最大带宽 (bps)
	MaxConnections int   `json:"max_connections"` // 最大连接数
	MaxTunnels     int   `json:"max_tunnels"`     // 最大隧道数

	// 实时统计
	CurrentLoad        float64 `json:"current_load"`        // 当前负载 (0-100%)
	CurrentConnections int     `json:"current_connections"` // 当前连接数
	CurrentTunnels     int     `json:"current_tunnels"`     // 当前隧道数

	// 流量统计
	TrafficIn    int64   `json:"traffic_in"`    // 入站流量（字节）
	TrafficOut   int64   `json:"traffic_out"`   // 出站流量（字节）
	TrafficTotal int64   `json:"traffic_total"` // 总流量
	BandwidthIn  float64 `json:"bandwidth_in"`  // 入站带宽 (Mbps)
	BandwidthOut float64 `json:"bandwidth_out"` // 出站带宽 (Mbps)

	// 时间信息
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetNodeStatus 获取单个节点状态
func (h *NodeStatusHandler) GetNodeStatus(c *gin.Context) {
	nodeID := c.Param("id")

	// 获取节点信息
	node, err := h.app.DB.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		response.NotFound(c, "Node not found")
		return
	}

	// 获取节点组信息
	group, _ := h.app.DB.DB.SQLite.GetNodeGroup(node.GroupID)
	groupName := ""
	if group != nil {
		groupName = group.Name
	}

	// 判断节点是否在线（5分钟内有心跳）
	isOnline := node.Status == "online" && time.Since(node.LastSeen) < 5*time.Minute

	// 计算运行时间
	uptime := int64(0)
	if isOnline {
		uptime = int64(time.Since(node.LastSeen).Seconds())
	}

	// 获取节点统计信息（从 Redis 缓存）
	stats := h.getNodeStats(nodeID)

	// 构建响应
	statusResponse := NodeStatusResponse{
		// 基本信息
		ID:          node.ID,
		Name:        node.Name,
		Type:        node.Type,
		GroupID:     node.GroupID,
		GroupName:   groupName,
		IP:          node.IP,
		Port:        node.Port,
		Description: node.Description,

		// 状态信息
		Status:   node.Status,
		IsOnline: isOnline,
		LastSeen: node.LastSeen,
		Uptime:   uptime,

		// 配置信息
		MaxBandwidth:   1000 * 1024 * 1024, // 1Gbps 默认
		MaxConnections: 10000,
		MaxTunnels:     1000,

		// 实时统计
		CurrentLoad:        stats.Load,
		CurrentConnections: stats.Connections,
		CurrentTunnels:     stats.Tunnels,

		// 流量统计
		TrafficIn:    stats.TrafficIn,
		TrafficOut:   stats.TrafficOut,
		TrafficTotal: stats.TrafficIn + stats.TrafficOut,
		BandwidthIn:  stats.BandwidthIn,
		BandwidthOut: stats.BandwidthOut,

		// 时间信息
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}

	response.Success(c, statusResponse)
}

// ListNodesStatus 列出所有节点状态
func (h *NodeStatusHandler) ListNodesStatus(c *gin.Context) {
	groupID := c.Query("group_id")
	status := c.Query("status")

	// 获取节点列表
	nodes, err := h.app.DB.DB.SQLite.ListNodes(groupID, status, 0, 1000)
	if err != nil {
		logger.Error("获取节点列表失败", zap.Error(err))
		response.InternalError(c, "Failed to list nodes")
		return
	}

	// 构建响应
	statusList := make([]NodeStatusResponse, 0, len(nodes))
	for _, node := range nodes {
		// 获取节点组信息
		group, _ := h.app.DB.DB.SQLite.GetNodeGroup(node.GroupID)
		groupName := ""
		if group != nil {
			groupName = group.Name
		}

		// 判断在线状态
		isOnline := node.Status == "online" && time.Since(node.LastSeen) < 5*time.Minute

		// 获取统计信息
		stats := h.getNodeStats(node.ID)

		statusList = append(statusList, NodeStatusResponse{
			ID:                 node.ID,
			Name:               node.Name,
			Type:               node.Type,
			GroupID:            node.GroupID,
			GroupName:          groupName,
			IP:                 node.IP,
			Port:               node.Port,
			Status:             node.Status,
			IsOnline:           isOnline,
			LastSeen:           node.LastSeen,
			CurrentLoad:        stats.Load,
			CurrentConnections: stats.Connections,
			TrafficIn:          stats.TrafficIn,
			TrafficOut:         stats.TrafficOut,
			TrafficTotal:       stats.TrafficIn + stats.TrafficOut,
			CreatedAt:          node.CreatedAt,
		})
	}

	response.Success(c, gin.H{
		"nodes": statusList,
		"total": len(statusList),
	})
}

// NodeStats 节点统计信息
type NodeStats struct {
	Load         float64 // 负载
	Connections  int     // 连接数
	Tunnels      int     // 隧道数
	TrafficIn    int64   // 入站流量
	TrafficOut   int64   // 出站流量
	BandwidthIn  float64 // 入站带宽
	BandwidthOut float64 // 出站带宽
}

// getNodeStats 从 Redis 获取节点统计信息
func (h *NodeStatusHandler) getNodeStats(nodeID string) NodeStats {
	stats := NodeStats{}

	if !h.app.DB.HasCache() {
		return stats
	}

	// 从 Redis 获取实时统计数据
	key := fmt.Sprintf("node:stats:%s", nodeID)
	var data string
	if err := h.app.DB.Cache.Redis.Get(key, &data); err != nil {
		return stats
	}

	// 解析统计数据
	var wsStats struct {
		Load         float64 `json:"load"`
		CPUUsage     float64 `json:"cpu_usage"`
		MemoryUsage  float64 `json:"memory_usage"`
		Connections  int     `json:"connections"`
		Tunnels      int     `json:"tunnels"`
		TrafficIn    int64   `json:"traffic_in"`
		TrafficOut   int64   `json:"traffic_out"`
		BandwidthIn  float64 `json:"bandwidth_in"`
		BandwidthOut float64 `json:"bandwidth_out"`
	}

	if err := json.Unmarshal([]byte(data), &wsStats); err != nil {
		return stats
	}

	stats.Load = wsStats.Load
	stats.Connections = wsStats.Connections
	stats.Tunnels = wsStats.Tunnels
	stats.TrafficIn = wsStats.TrafficIn
	stats.TrafficOut = wsStats.TrafficOut
	stats.BandwidthIn = wsStats.BandwidthIn
	stats.BandwidthOut = wsStats.BandwidthOut

	return stats
}

// GetNodesByGroup 按组获取节点状态
func (h *NodeStatusHandler) GetNodesByGroup(c *gin.Context) {
	groupID := c.Param("group_id")

	// 获取组信息
	group, err := h.app.DB.DB.SQLite.GetNodeGroup(groupID)
	if err != nil || group == nil {
		response.NotFound(c, "Node group not found")
		return
	}

	// 获取组内节点
	nodes, err := h.app.DB.DB.SQLite.ListNodes(groupID, "", 0, 1000)
	if err != nil {
		response.InternalError(c, "Failed to list nodes")
		return
	}

	// 统计在线/离线节点
	onlineCount := 0
	offlineCount := 0
	totalTraffic := int64(0)

	statusList := make([]NodeStatusResponse, 0, len(nodes))
	for _, node := range nodes {
		isOnline := node.Status == "online" && time.Since(node.LastSeen) < 5*time.Minute

		if isOnline {
			onlineCount++
		} else {
			offlineCount++
		}

		stats := h.getNodeStats(node.ID)
		totalTraffic += stats.TrafficIn + stats.TrafficOut

		statusList = append(statusList, NodeStatusResponse{
			ID:          node.ID,
			Name:        node.Name,
			Type:        node.Type,
			Status:      node.Status,
			IsOnline:    isOnline,
			LastSeen:    node.LastSeen,
			CurrentLoad: stats.Load,
			TrafficIn:   stats.TrafficIn,
			TrafficOut:  stats.TrafficOut,
		})
	}

	response.Success(c, gin.H{
		"group": gin.H{
			"id":   group.ID,
			"name": group.Name,
			"type": group.Type,
		},
		"nodes": statusList,
		"summary": gin.H{
			"total":         len(nodes),
			"online":        onlineCount,
			"offline":       offlineCount,
			"total_traffic": totalTraffic,
		},
	})
}
