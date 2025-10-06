package api

import (
	"fmt"
	"time"

	"gkipass/plane/db/sqlite"
	"gkipass/plane/internal/api/response"
	"gkipass/plane/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TrafficStatsHandler 流量统计处理器
type TrafficStatsHandler struct {
	app *App
}

// NewTrafficStatsHandler 创建流量统计处理器
func NewTrafficStatsHandler(app *App) *TrafficStatsHandler {
	return &TrafficStatsHandler{app: app}
}

// ListTrafficStatsResponse 流量统计列表响应
type ListTrafficStatsResponse struct {
	Data  []*TrafficStatWithDetails `json:"data"`
	Total int                       `json:"total"`
}

// TrafficStatWithDetails 带详情的流量统计
type TrafficStatWithDetails struct {
	*sqlite.TunnelTrafficStat
	TunnelName     string `json:"tunnel_name"`
	EntryGroupName string `json:"entry_group_name"`
	ExitGroupName  string `json:"exit_group_name"`
}

// ListTrafficStats 列出流量统计
func (h *TrafficStatsHandler) ListTrafficStats(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	// 管理员可以查看所有用户的数据
	queryUserID := userID.(string)
	if role == "admin" && c.Query("user_id") != "" {
		queryUserID = c.Query("user_id")
	}

	tunnelID := c.Query("tunnel_id")
	page := 1
	limit := 50

	if p := c.Query("page"); p != "" {
		if val, err := intParse(p); err == nil {
			page = val
		}
	}
	if l := c.Query("limit"); l != "" {
		if val, err := intParse(l); err == nil && val > 0 && val <= 200 {
			limit = val
		}
	}

	offset := (page - 1) * limit

	// 查询流量统计
	stats, total, err := h.app.DB.DB.SQLite.ListTunnelTrafficStats(queryUserID, tunnelID, limit, offset)
	if err != nil {
		logger.Error("查询流量统计失败", zap.Error(err))
		response.InternalError(c, "Failed to list traffic stats")
		return
	}

	// 填充详情
	result := make([]*TrafficStatWithDetails, 0, len(stats))
	for _, stat := range stats {
		detail := &TrafficStatWithDetails{
			TunnelTrafficStat: stat,
		}

		// 获取隧道名称
		if tunnel, _ := h.app.DB.DB.SQLite.GetTunnel(stat.TunnelID); tunnel != nil {
			detail.TunnelName = tunnel.Name
		}

		// 获取节点组名称
		if entryGroup, _ := h.app.DB.DB.SQLite.GetNodeGroup(stat.EntryGroupID); entryGroup != nil {
			detail.EntryGroupName = entryGroup.Name
		}
		if exitGroup, _ := h.app.DB.DB.SQLite.GetNodeGroup(stat.ExitGroupID); exitGroup != nil {
			detail.ExitGroupName = exitGroup.Name
		}

		result = append(result, detail)
	}

	response.Success(c, ListTrafficStatsResponse{
		Data:  result,
		Total: total,
	})
}

// GetTrafficSummary 获取流量汇总
func (h *TrafficStatsHandler) GetTrafficSummary(c *gin.Context) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")

	// 管理员可以查看所有用户的数据
	queryUserID := userID.(string)
	if role == "admin" && c.Query("user_id") != "" {
		queryUserID = c.Query("user_id")
	}

	tunnelID := c.Query("tunnel_id")

	// 解析日期范围
	startDateStr := c.DefaultQuery("start_date", time.Now().AddDate(0, 0, -30).Format("2006-01-02"))
	endDateStr := c.DefaultQuery("end_date", time.Now().Format("2006-01-02"))

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		response.BadRequest(c, "Invalid start_date format")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		response.BadRequest(c, "Invalid end_date format")
		return
	}

	// 获取汇总数据
	trafficIn, trafficOut, billedIn, billedOut, err := h.app.DB.DB.SQLite.GetTunnelTrafficSummary(
		queryUserID, tunnelID, startDate, endDate)
	if err != nil {
		logger.Error("获取流量汇总失败", zap.Error(err))
		response.InternalError(c, "Failed to get traffic summary")
		return
	}

	response.Success(c, gin.H{
		"traffic_in":         trafficIn,
		"traffic_out":        trafficOut,
		"billed_traffic_in":  billedIn,
		"billed_traffic_out": billedOut,
		"total_traffic":      trafficIn + trafficOut,
		"total_billed":       billedIn + billedOut,
		"start_date":         startDate,
		"end_date":           endDate,
	})
}

// ReportTrafficRequest 上报流量请求
type ReportTrafficRequest struct {
	TunnelID   string `json:"tunnel_id" binding:"required"`
	TrafficIn  int64  `json:"traffic_in"`
	TrafficOut int64  `json:"traffic_out"`
}

// ReportTraffic 上报流量（节点使用）
func (h *TrafficStatsHandler) ReportTraffic(c *gin.Context) {
	var req ReportTrafficRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// 获取隧道信息
	tunnel, err := h.app.DB.DB.SQLite.GetTunnel(req.TunnelID)
	if err != nil || tunnel == nil {
		response.NotFound(c, "Tunnel not found")
		return
	}

	// 创建今日流量统计记录
	today := time.Now().Truncate(24 * time.Hour)
	stat := &sqlite.TunnelTrafficStat{
		ID:               uuid.New().String(),
		TunnelID:         req.TunnelID,
		UserID:           tunnel.UserID,
		EntryGroupID:     tunnel.EntryGroupID,
		ExitGroupID:      tunnel.ExitGroupID,
		TrafficIn:        req.TrafficIn,
		TrafficOut:       req.TrafficOut,
		BilledTrafficIn:  req.TrafficIn,  // TODO: 根据倍率计算
		BilledTrafficOut: req.TrafficOut, // TODO: 根据倍率计算
		Date:             today,
		CreatedAt:        time.Now(),
	}

	if err := h.app.DB.DB.SQLite.CreateTunnelTrafficStat(stat); err != nil {
		logger.Error("创建流量统计失败", zap.Error(err))
		response.InternalError(c, "Failed to create traffic stat")
		return
	}

	response.SuccessWithMessage(c, "Traffic reported successfully", nil)
}

// intParse 解析整数
func intParse(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
