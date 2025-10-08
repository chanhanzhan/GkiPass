package user

import (
	"time"

	dbinit "gkipass/plane/db/init"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/types"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// StatisticsHandler 统计处理器
type StatisticsHandler struct {
	app *types.App
}

// NewStatisticsHandler 创建统计处理器
func NewStatisticsHandler(app *types.App) *StatisticsHandler {
	return &StatisticsHandler{app: app}
}

// GetNodeStats 获取节点统计
func (h *StatisticsHandler) GetNodeStats(c *gin.Context) {
	nodeID := c.Param("id")
	fromStr := c.DefaultQuery("from", time.Now().Add(-24*time.Hour).Format(time.RFC3339))
	toStr := c.DefaultQuery("to", time.Now().Format(time.RFC3339))

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

	stats, err := h.app.DB.DB.SQLite.GetStatistics(nodeID, from, to)
	if err != nil {
		response.InternalError(c, "Failed to get statistics: "+err.Error())
		return
	}

	// 计算汇总数据
	var totalBytesIn, totalBytesOut, totalPacketsIn, totalPacketsOut int64
	var avgConnections, avgLatency float64

	if len(stats) > 0 {
		for _, s := range stats {
			totalBytesIn += s.BytesIn
			totalBytesOut += s.BytesOut
			totalPacketsIn += s.PacketsIn
			totalPacketsOut += s.PacketsOut
			avgConnections += float64(s.Connections)
			avgLatency += s.AvgLatency
		}
		avgConnections /= float64(len(stats))
		avgLatency /= float64(len(stats))
	}

	response.Success(c, gin.H{
		"node_id": nodeID,
		"from":    from,
		"to":      to,
		"summary": gin.H{
			"total_bytes_in":    totalBytesIn,
			"total_bytes_out":   totalBytesOut,
			"total_packets_in":  totalPacketsIn,
			"total_packets_out": totalPacketsOut,
			"avg_connections":   avgConnections,
			"avg_latency":       avgLatency,
		},
		"data": stats,
	})
}

// GetOverview 获取总览统计
func (h *StatisticsHandler) GetOverview(c *gin.Context) {
	// 获取所有节点
	nodes, err := h.app.DB.DB.SQLite.ListNodes("", "", 1000, 0)
	if err != nil {
		response.InternalError(c, "Failed to get nodes: "+err.Error())
		return
	}

	// 统计节点状态
	var onlineCount, offlineCount, errorCount int
	for _, node := range nodes {
		switch node.Status {
		case "online":
			onlineCount++
		case "offline":
			offlineCount++
		case "error":
			errorCount++
		}
	}

	// 如果有Redis，获取实时流量
	var totalTrafficIn, totalTrafficOut int64
	if h.app.DB.HasCache() {
		for _, node := range nodes {
			bytesIn, bytesOut, _ := h.app.DB.Cache.Redis.GetTraffic(node.ID)
			totalTrafficIn += bytesIn
			totalTrafficOut += bytesOut
		}
	}

	// 获取策略数量
	policies, _ := h.app.DB.DB.SQLite.ListPolicies("", nil)
	enabledPolicies := 0
	for _, p := range policies {
		if p.Enabled {
			enabledPolicies++
		}
	}

	// 获取证书数量
	certs, _ := h.app.DB.DB.SQLite.ListCertificates("", nil)
	activeCerts := 0
	for _, cert := range certs {
		if !cert.Revoked && time.Now().Before(cert.NotAfter) {
			activeCerts++
		}
	}

	response.Success(c, gin.H{
		"nodes": gin.H{
			"total":   len(nodes),
			"online":  onlineCount,
			"offline": offlineCount,
			"error":   errorCount,
		},
		"policies": gin.H{
			"total":   len(policies),
			"enabled": enabledPolicies,
		},
		"certificates": gin.H{
			"total":  len(certs),
			"active": activeCerts,
		},
		"traffic": gin.H{
			"total_in":  totalTrafficIn,
			"total_out": totalTrafficOut,
		},
	})
}

// GetAdminOverview 获取管理员总览统计
func (h *StatisticsHandler) GetAdminOverview(c *gin.Context) {
	// 获取用户总数
	var totalUsers int
	err := h.app.DB.DB.SQLite.Get().QueryRow("SELECT COUNT(*) FROM users").Scan(&totalUsers)
	if err != nil {
		response.InternalError(c, "Failed to get users count: "+err.Error())
		return
	}

	// 获取所有节点
	nodes, err := h.app.DB.DB.SQLite.ListNodes("", "", 10000, 0)
	if err != nil {
		response.InternalError(c, "Failed to get nodes: "+err.Error())
		return
	}

	// 获取所有隧道
	tunnels, err := h.app.DB.DB.SQLite.ListTunnels("", nil)
	if err != nil {
		response.InternalError(c, "Failed to get tunnels: "+err.Error())
		return
	}

	// 获取活跃订阅数
	var activeSubscriptions int
	err = h.app.DB.DB.SQLite.Get().QueryRow("SELECT COUNT(*) FROM user_subscriptions WHERE status = 'active'").Scan(&activeSubscriptions)
	if err != nil {
		activeSubscriptions = 0
	}

	response.Success(c, gin.H{
		"total_users":         totalUsers,
		"total_nodes":         len(nodes),
		"total_tunnels":       len(tunnels),
		"total_subscriptions": activeSubscriptions,
	})
}

// ReportStatsRequest 上报统计请求
type ReportStatsRequest struct {
	NodeID         string  `json:"node_id" binding:"required"`
	BytesIn        int64   `json:"bytes_in"`
	BytesOut       int64   `json:"bytes_out"`
	PacketsIn      int64   `json:"packets_in"`
	PacketsOut     int64   `json:"packets_out"`
	Connections    int     `json:"connections"`
	ActiveSessions int     `json:"active_sessions"`
	ErrorCount     int     `json:"error_count"`
	AvgLatency     float64 `json:"avg_latency"`
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsage    int64   `json:"memory_usage"`
}

// ReportStats 节点上报统计
func (h *StatisticsHandler) ReportStats(c *gin.Context) {
	var req ReportStatsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 保存到数据库
	stats := &dbinit.Statistics{
		ID:             uuid.New().String(),
		NodeID:         req.NodeID,
		Timestamp:      time.Now(),
		BytesIn:        req.BytesIn,
		BytesOut:       req.BytesOut,
		PacketsIn:      req.PacketsIn,
		PacketsOut:     req.PacketsOut,
		Connections:    req.Connections,
		ActiveSessions: req.ActiveSessions,
		ErrorCount:     req.ErrorCount,
		AvgLatency:     req.AvgLatency,
		CPUUsage:       req.CPUUsage,
		MemoryUsage:    req.MemoryUsage,
	}

	if err := h.app.DB.DB.SQLite.CreateStatistics(stats); err != nil {
		response.InternalError(c, "Failed to save statistics: "+err.Error())
		return
	}

	// 更新Redis实时流量
	if h.app.DB.HasCache() {
		_ = h.app.DB.Cache.Redis.IncrementTraffic(req.NodeID, req.BytesIn, req.BytesOut)
	}

	response.SuccessWithMessage(c, "Statistics reported successfully", nil)
}
