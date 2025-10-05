package ws

import (
	"encoding/json"
	"fmt"
	"time"

	"gkipass/plane/db"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// NodeStats 节点统计数据
type NodeStats struct {
	NodeID         string    `json:"node_id"`
	Timestamp      time.Time `json:"timestamp"`
	Load           float64   `json:"load"`            // CPU 负载 (0-100%)
	CPUUsage       float64   `json:"cpu_usage"`       // CPU 使用率 (0-100%)
	MemoryUsage    float64   `json:"memory_usage"`    // 内存使用率 (0-100%)
	MemoryTotal    int64     `json:"memory_total"`    // 总内存 (字节)
	MemoryUsed     int64     `json:"memory_used"`     // 已用内存 (字节)
	Connections    int       `json:"connections"`     // 当前连接数
	Tunnels        int       `json:"tunnels"`         // 当前隧道数
	TrafficIn      int64     `json:"traffic_in"`      // 入站流量 (字节)
	TrafficOut     int64     `json:"traffic_out"`     // 出站流量 (字节)
	BandwidthIn    float64   `json:"bandwidth_in"`    // 入站带宽 (Mbps)
	BandwidthOut   float64   `json:"bandwidth_out"`   // 出站带宽 (Mbps)
	PacketsIn      int64     `json:"packets_in"`      // 入站数据包数
	PacketsOut     int64     `json:"packets_out"`     // 出站数据包数
	ErrorCount     int       `json:"error_count"`     // 错误计数
	ActiveSessions int       `json:"active_sessions"` // 活跃会话数
}

// StatsHandler 统计数据处理器
type StatsHandler struct {
	db *db.Manager
}

// NewStatsHandler 创建统计处理器
func NewStatsHandler(dbManager *db.Manager) *StatsHandler {
	return &StatsHandler{
		db: dbManager,
	}
}

// HandleStatsReport 处理统计数据上报
func (h *StatsHandler) HandleStatsReport(nodeConn *NodeConnection, data json.RawMessage) {
	var stats NodeStats
	if err := json.Unmarshal(data, &stats); err != nil {
		logger.Error("解析统计数据失败", zap.Error(err))
		return
	}

	stats.NodeID = nodeConn.NodeID
	stats.Timestamp = time.Now()

	// 存储到 Redis（实时数据，5分钟过期）
	if err := h.storeStatsToRedis(&stats); err != nil {
		logger.Error("存储统计数据到Redis失败", zap.Error(err))
	}

	// 存储到 SQLite（历史数据）
	if err := h.storeStatsToSQLite(&stats); err != nil {
		logger.Error("存储统计数据到SQLite失败", zap.Error(err))
	}

	logger.Debug("收到节点统计数据",
		zap.String("nodeID", stats.NodeID),
		zap.Float64("load", stats.Load),
		zap.Int("connections", stats.Connections),
		zap.Int64("trafficIn", stats.TrafficIn),
		zap.Int64("trafficOut", stats.TrafficOut))
}

// storeStatsToRedis 存储统计数据到 Redis
func (h *StatsHandler) storeStatsToRedis(stats *NodeStats) error {
	if !h.db.HasCache() {
		return fmt.Errorf("Redis 不可用")
	}

	key := fmt.Sprintf("node:stats:%s", stats.NodeID)

	// 序列化数据
	data, err := json.Marshal(stats)
	if err != nil {
		return err
	}

	// 存储到 Redis，5分钟过期
	return h.db.Cache.Redis.Set(key, string(data), 5*time.Minute)
}

// storeStatsToSQLite 存储统计数据到 SQLite
func (h *StatsHandler) storeStatsToSQLite(stats *NodeStats) error {
	// 创建统计记录
	statRecord := &struct {
		ID             string
		NodeID         string
		Timestamp      time.Time
		BytesIn        int64
		BytesOut       int64
		PacketsIn      int64
		PacketsOut     int64
		Connections    int
		ActiveSessions int
		ErrorCount     int
		AvgLatency     float64
		CPUUsage       float64
		MemoryUsage    int64
	}{
		ID:             fmt.Sprintf("%s-%d", stats.NodeID, stats.Timestamp.Unix()),
		NodeID:         stats.NodeID,
		Timestamp:      stats.Timestamp,
		BytesIn:        stats.TrafficIn,
		BytesOut:       stats.TrafficOut,
		PacketsIn:      stats.PacketsIn,
		PacketsOut:     stats.PacketsOut,
		Connections:    stats.Connections,
		ActiveSessions: stats.ActiveSessions,
		ErrorCount:     stats.ErrorCount,
		AvgLatency:     0, // 待实现
		CPUUsage:       stats.CPUUsage,
		MemoryUsage:    stats.MemoryUsed,
	}

	// 插入数据库
	query := `
		INSERT OR REPLACE INTO statistics 
		(id, node_id, timestamp, bytes_in, bytes_out, packets_in, packets_out, 
		 connections, active_sessions, error_count, avg_latency, cpu_usage, memory_usage)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := h.db.DB.SQLite.GetDB().Exec(query,
		statRecord.ID, statRecord.NodeID, statRecord.Timestamp,
		statRecord.BytesIn, statRecord.BytesOut, statRecord.PacketsIn, statRecord.PacketsOut,
		statRecord.Connections, statRecord.ActiveSessions, statRecord.ErrorCount,
		statRecord.AvgLatency, statRecord.CPUUsage, statRecord.MemoryUsage)

	return err
}

// GetNodeStats 从 Redis 获取节点统计数据
func (h *StatsHandler) GetNodeStats(nodeID string) (*NodeStats, error) {
	if !h.db.HasCache() {
		return nil, fmt.Errorf("Redis 不可用")
	}

	key := fmt.Sprintf("node:stats:%s", nodeID)

	var data string
	if err := h.db.Cache.Redis.Get(key, &data); err != nil {
		return nil, err
	}

	var stats NodeStats
	if err := json.Unmarshal([]byte(data), &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

// GetNodeStatsHistory 获取节点历史统计数据
func (h *StatsHandler) GetNodeStatsHistory(nodeID string, from, to time.Time, limit int) ([]NodeStats, error) {
	query := `
		SELECT node_id, timestamp, bytes_in, bytes_out, packets_in, packets_out,
		       connections, active_sessions, error_count, cpu_usage, memory_usage
		FROM statistics
		WHERE node_id = ? AND timestamp BETWEEN ? AND ?
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := h.db.DB.SQLite.GetDB().Query(query, nodeID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var statsList []NodeStats
	for rows.Next() {
		var stats NodeStats
		err := rows.Scan(
			&stats.NodeID, &stats.Timestamp,
			&stats.TrafficIn, &stats.TrafficOut,
			&stats.PacketsIn, &stats.PacketsOut,
			&stats.Connections, &stats.ActiveSessions,
			&stats.ErrorCount, &stats.CPUUsage, &stats.MemoryUsed,
		)
		if err != nil {
			continue
		}
		statsList = append(statsList, stats)
	}

	return statsList, nil
}

// AggregateStats 聚合统计数据（按小时）
func (h *StatsHandler) AggregateStats(nodeID string, hours int) error {
	// 1. 计算时间范围
	now := time.Now()
	cutoff := now.Add(-time.Duration(hours) * time.Hour)
	
	// 2. 查询原始数据
	query := `
		SELECT 
			strftime('%Y-%m-%d %H:00:00', timestamp) as hour,
			SUM(bytes_in) as total_in,
			SUM(bytes_out) as total_out,
			AVG(connections) as avg_connections,
			AVG(cpu_usage) as avg_cpu,
			AVG(memory_usage) as avg_memory
		FROM statistics
		WHERE node_id = ? AND timestamp >= ?
		GROUP BY strftime('%Y-%m-%d %H:00:00', timestamp)
	`
	
	rows, err := h.db.DB.SQLite.GetDB().Query(query, nodeID, cutoff)
	if err != nil {
		return err
	}
	defer rows.Close()
	
	// 3. 处理聚合数据
	for rows.Next() {
		var hour string
		var totalIn, totalOut int64
		var avgConn, avgCPU, avgMem float64
		
		if err := rows.Scan(&hour, &totalIn, &totalOut, &avgConn, &avgCPU, &avgMem); err != nil {
			continue
		}
		
		// 保存聚合数据到单独的表或Redis
		logger.Debug("统计数据聚合",
			zap.String("nodeID", nodeID),
			zap.String("hour", hour),
			zap.Int64("trafficIn", totalIn),
			zap.Int64("trafficOut", totalOut))
	}
	
	return nil
}

// CleanupOldStats 清理旧的统计数据
func (h *StatsHandler) CleanupOldStats(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)

	query := `DELETE FROM statistics WHERE timestamp < ?`
	result, err := h.db.DB.SQLite.GetDB().Exec(query, cutoff)
	if err != nil {
		return err
	}

	affected, _ := result.RowsAffected()
	logger.Info("清理旧统计数据完成",
		zap.Int("days", days),
		zap.Int64("deleted", affected))

	return nil
}

