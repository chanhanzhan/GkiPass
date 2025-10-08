package service

import (
	//"encoding/json"
	"fmt"
	"sync"
	"time"

	"gkipass/plane/db"
	dbinit "gkipass/plane/db/init"
	"gkipass/plane/pkg/logger"

//	"github.com/google/uuid"
	"go.uber.org/zap"
)

// NodeMonitoringService 节点监控服务
type NodeMonitoringService struct {
	db          *db.Manager
	alertSystem *AlertSystem
	stopChan    chan struct{}
	mu          sync.RWMutex
}

// NewNodeMonitoringService 创建节点监控服务
func NewNodeMonitoringService(dbManager *db.Manager) *NodeMonitoringService {
	return &NodeMonitoringService{
		db:          dbManager,
		alertSystem: NewAlertSystem(),
		stopChan:    make(chan struct{}),
	}
}

// Start 启动监控服务
func (s *NodeMonitoringService) Start() {
	logger.Info("节点监控服务启动")
	
	// 启动数据聚合协程
	go s.dataAggregationLoop()
	
	// 启动告警检查协程
	go s.alertCheckLoop()
	
	// 启动数据清理协程
	go s.dataCleanupLoop()
}

// Stop 停止监控服务
func (s *NodeMonitoringService) Stop() {
	logger.Info("节点监控服务停止")
	close(s.stopChan)
}

// ReportMonitoringData 接收节点上报的监控数据
func (s *NodeMonitoringService) ReportMonitoringData(nodeID string, data *NodeMonitoringReportData) error {
	// 1. 验证节点是否存在
	node, err := s.db.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		return fmt.Errorf("节点不存在: %s", nodeID)
	}

	// 2. 检查节点是否启用监控
	config, err := s.db.DB.SQLite.GetNodeMonitoringConfig(nodeID)
	if err != nil {
		return fmt.Errorf("获取监控配置失败: %w", err)
	}

	// 如果没有配置，创建默认配置
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
		if err := s.db.DB.SQLite.UpsertNodeMonitoringConfig(config); err != nil {
			logger.Error("创建默认监控配置失败", zap.Error(err))
		}
	}

	if !config.MonitoringEnabled {
		return fmt.Errorf("节点监控已禁用")
	}

	// 3. 转换数据格式并存储
	monitoringData := s.convertToMonitoringData(nodeID, data)
	if err := s.db.DB.SQLite.CreateNodeMonitoringData(monitoringData); err != nil {
		logger.Error("存储监控数据失败",
			zap.String("nodeID", nodeID),
			zap.Error(err))
		return err
	}

	// 4. 检查告警条件
	go s.checkAlerts(nodeID, monitoringData)

	// 5. 更新节点状态
	go s.updateNodeStatus(nodeID, data)

	logger.Debug("监控数据已接收",
		zap.String("nodeID", nodeID),
		zap.Float64("cpuUsage", data.SystemInfo.CPUUsage),
		zap.Float64("memoryUsage", data.SystemInfo.MemoryUsagePercent))

	return nil
}

// NodeMonitoringReportData 节点上报的监控数据结构
type NodeMonitoringReportData struct {
	SystemInfo    SystemInfo    `json:"system_info"`
	NetworkStats  NetworkStats  `json:"network_stats"`
	TunnelStats   TunnelStats   `json:"tunnel_stats"`
	Performance   Performance   `json:"performance"`
	AppInfo       AppInfo       `json:"app_info"`
	ConfigVersion string        `json:"config_version"`
}

type SystemInfo struct {
	Uptime             int64     `json:"uptime"`              // 系统运行时间(秒)
	BootTime           time.Time `json:"boot_time"`           // 开机时间
	CPUUsage           float64   `json:"cpu_usage"`           // CPU使用率(0-100)
	CPULoad1m          float64   `json:"cpu_load_1m"`         // 1分钟负载
	CPULoad5m          float64   `json:"cpu_load_5m"`         // 5分钟负载
	CPULoad15m         float64   `json:"cpu_load_15m"`        // 15分钟负载
	CPUCores           int       `json:"cpu_cores"`           // CPU核心数
	MemoryTotal        int64     `json:"memory_total"`        // 总内存(bytes)
	MemoryUsed         int64     `json:"memory_used"`         // 已用内存(bytes)
	MemoryAvailable    int64     `json:"memory_available"`    // 可用内存(bytes)
	MemoryUsagePercent float64   `json:"memory_usage_percent"` // 内存使用率(0-100)
	DiskTotal          int64     `json:"disk_total"`          // 总磁盘空间(bytes)
	DiskUsed           int64     `json:"disk_used"`           // 已用磁盘空间(bytes)
	DiskAvailable      int64     `json:"disk_available"`      // 可用磁盘空间(bytes)
	DiskUsagePercent   float64   `json:"disk_usage_percent"`  // 磁盘使用率(0-100)
}

type NetworkStats struct {
	Interfaces       []NetworkInterface `json:"interfaces"`      // 网络接口信息
	BandwidthIn      int64              `json:"bandwidth_in"`    // 入站带宽(bps)
	BandwidthOut     int64              `json:"bandwidth_out"`   // 出站带宽(bps)
	TCPConnections   int                `json:"tcp_connections"` // TCP连接数
	UDPConnections   int                `json:"udp_connections"` // UDP连接数
	TotalConnections int                `json:"total_connections"` // 总连接数
	TrafficInBytes   int64              `json:"traffic_in_bytes"`  // 入站流量(bytes)
	TrafficOutBytes  int64              `json:"traffic_out_bytes"` // 出站流量(bytes)
	PacketsIn        int64              `json:"packets_in"`        // 入站数据包数
	PacketsOut       int64              `json:"packets_out"`       // 出站数据包数
	ConnectionErrors int                `json:"connection_errors"` // 连接错误数
}

type NetworkInterface struct {
	Name      string `json:"name"`       // 接口名称
	IP        string `json:"ip"`         // IP地址
	MAC       string `json:"mac"`        // MAC地址
	Status    string `json:"status"`     // 状态 up/down
	Speed     int64  `json:"speed"`      // 速度(bps)
	RxBytes   int64  `json:"rx_bytes"`   // 接收字节数
	TxBytes   int64  `json:"tx_bytes"`   // 发送字节数
	RxPackets int64  `json:"rx_packets"` // 接收包数
	TxPackets int64  `json:"tx_packets"` // 发送包数
}

type TunnelStats struct {
	ActiveTunnels int                    `json:"active_tunnels"` // 活跃隧道数
	TunnelErrors  int                    `json:"tunnel_errors"`  // 隧道错误数
	TunnelList    []TunnelStatusInfo     `json:"tunnel_list"`    // 隧道状态列表
}

type TunnelStatusInfo struct {
	TunnelID        string  `json:"tunnel_id"`        // 隧道ID
	Name            string  `json:"name"`             // 隧道名称
	Protocol        string  `json:"protocol"`         // 协议
	LocalPort       int     `json:"local_port"`       // 本地端口
	Status          string  `json:"status"`           // 状态 active/inactive/error
	Connections     int     `json:"connections"`      // 当前连接数
	TrafficIn       int64   `json:"traffic_in"`       // 入站流量
	TrafficOut      int64   `json:"traffic_out"`      // 出站流量
	AvgResponseTime float64 `json:"avg_response_time"` // 平均响应时间(ms)
	ErrorCount      int     `json:"error_count"`      // 错误计数
}

type Performance struct {
	AvgResponseTime float64 `json:"avg_response_time"` // 平均响应时间(ms)
	MaxResponseTime float64 `json:"max_response_time"` // 最大响应时间(ms)
	MinResponseTime float64 `json:"min_response_time"` // 最小响应时间(ms)
	RequestsPerSec  int     `json:"requests_per_sec"`  // 每秒请求数
	ErrorRate       float64 `json:"error_rate"`        // 错误率(0-100)
}

type AppInfo struct {
	Version       string `json:"version"`        // 应用版本
	GoVersion     string `json:"go_version"`     // Go版本
	OSInfo        string `json:"os_info"`        // 操作系统信息
	Architecture  string `json:"architecture"`   // 架构信息
	StartTime     time.Time `json:"start_time"`  // 启动时间
}

// convertToMonitoringData 转换上报数据为监控数据格式
func (s *NodeMonitoringService) convertToMonitoringData(nodeID string, data *NodeMonitoringReportData) *dbinit.NodeMonitoringData {
	// 序列化网络接口信息
	//interfacesJSON, _ := json.Marshal(data.NetworkStats.Interfaces)

	return &dbinit.NodeMonitoringData{
		NodeID:                nodeID,
		Timestamp:             time.Now(),
		SystemUptime:          data.SystemInfo.Uptime,
		CPUUsage:              data.SystemInfo.CPUUsage,
		CPULoad1m:             data.SystemInfo.CPULoad1m,
		CPULoad5m:             data.SystemInfo.CPULoad5m,
		CPULoad15m:            data.SystemInfo.CPULoad15m,
		CPUCores:              data.SystemInfo.CPUCores,
		MemoryTotal:           data.SystemInfo.MemoryTotal,
		MemoryUsed:            data.SystemInfo.MemoryUsed,
		MemoryAvailable:       data.SystemInfo.MemoryAvailable,
		MemoryUsagePercent:    data.SystemInfo.MemoryUsagePercent,
		DiskTotal:             data.SystemInfo.DiskTotal,
		DiskUsed:              data.SystemInfo.DiskUsed,
		DiskAvailable:         data.SystemInfo.DiskAvailable,
		DiskUsagePercent:      data.SystemInfo.DiskUsagePercent,
		BandwidthIn:           data.NetworkStats.BandwidthIn,
		BandwidthOut:          data.NetworkStats.BandwidthOut,
		TCPConnections:        data.NetworkStats.TCPConnections,
		UDPConnections:        data.NetworkStats.UDPConnections,
		ActiveTunnels:         data.TunnelStats.ActiveTunnels,
		TotalConnections:      data.NetworkStats.TotalConnections,
		TrafficInBytes:        data.NetworkStats.TrafficInBytes,
		TrafficOutBytes:       data.NetworkStats.TrafficOutBytes,
		PacketsIn:             data.NetworkStats.PacketsIn,
		PacketsOut:            data.NetworkStats.PacketsOut,
		ConnectionErrors:      data.NetworkStats.ConnectionErrors,
		TunnelErrors:          data.TunnelStats.TunnelErrors,
		AvgResponseTime:       data.Performance.AvgResponseTime,
		MaxResponseTime:       data.Performance.MaxResponseTime,
		MinResponseTime:       data.Performance.MinResponseTime,
	}
}

// updateNodeStatus 更新节点状态
func (s *NodeMonitoringService) updateNodeStatus(nodeID string, data *NodeMonitoringReportData) {
	node, err := s.db.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		return
	}

	// 更新最后心跳时间和状态
	node.Status = "online"
	node.LastSeen = time.Now()
	
	if err := s.db.DB.SQLite.UpdateNode(node); err != nil {
		logger.Error("更新节点状态失败", zap.String("nodeID", nodeID), zap.Error(err))
	}

	// 如果有Redis，更新实时状态
	if s.db.HasCache() {
		status := &dbinit.NodeStatus{
			NodeID:        nodeID,
			Online:        true,
			LastHeartbeat: time.Now(),
			CurrentLoad:   data.SystemInfo.CPUUsage,
			Connections:   data.NetworkStats.TotalConnections,
		}
		_ = s.db.Cache.Redis.SetNodeStatus(nodeID, status)
	}
}

// checkAlerts 检查告警条件
func (s *NodeMonitoringService) checkAlerts(nodeID string, data *dbinit.NodeMonitoringData) {
	// 获取告警规则
	rules, err := s.db.DB.SQLite.ListNodeAlertRules(nodeID)
	if err != nil {
		logger.Error("获取告警规则失败", zap.String("nodeID", nodeID), zap.Error(err))
		return
	}

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		var metricValue float64
		var triggered bool

		// 根据指标类型获取对应值
		switch rule.MetricType {
		case "cpu":
			metricValue = data.CPUUsage
		case "memory":
			metricValue = data.MemoryUsagePercent
		case "disk":
			metricValue = data.DiskUsagePercent
		case "response_time":
			metricValue = data.AvgResponseTime
		case "connections":
			metricValue = float64(data.TotalConnections)
		default:
			continue
		}

		// 检查是否触发告警
		switch rule.Operator {
		case ">":
			triggered = metricValue > rule.ThresholdValue
		case "<":
			triggered = metricValue < rule.ThresholdValue
		case ">=":
			triggered = metricValue >= rule.ThresholdValue
		case "<=":
			triggered = metricValue <= rule.ThresholdValue
		case "=":
			triggered = metricValue == rule.ThresholdValue
		case "!=":
			triggered = metricValue != rule.ThresholdValue
		}

		if triggered {
			s.triggerAlert(rule, nodeID, metricValue)
		}
	}
}

// triggerAlert 触发告警
func (s *NodeMonitoringService) triggerAlert(rule *dbinit.NodeAlertRule, nodeID string, metricValue float64) {
	// 创建告警历史记录
	alert := &dbinit.NodeAlertHistory{
		RuleID:         rule.ID,
		NodeID:         nodeID,
		AlertType:      rule.MetricType,
		Severity:       rule.Severity,
		Message:        fmt.Sprintf("%s: %.2f %s %.2f", rule.RuleName, metricValue, rule.Operator, rule.ThresholdValue),
		Status:         "triggered",
		TriggeredAt:    time.Now(),
	}

	if err := s.db.DB.SQLite.CreateNodeAlertHistory(alert); err != nil {
		logger.Error("创建告警记录失败", zap.Error(err))
		return
	}

	// 发送告警通知
	alertLevel := AlertInfo
	switch rule.Severity {
	case "warning":
		alertLevel = AlertWarning
	case "critical":
		alertLevel = AlertCritical
	}

	node, _ := s.db.DB.SQLite.GetNode(nodeID)
	nodeName := nodeID
	if node != nil {
		nodeName = node.Name
	}

	systemAlert := Alert{
		Level:   alertLevel,
		Title:   fmt.Sprintf("节点告警: %s", nodeName),
		Message: alert.Message,
		Tags:    []string{"node", nodeID, rule.MetricType},
	}

	if err := s.alertSystem.Send(systemAlert); err != nil {
		logger.Error("发送告警通知失败", zap.Error(err))
	}

	logger.Warn("节点告警触发",
		zap.String("nodeID", nodeID),
		zap.String("ruleName", rule.RuleName),
		zap.Float64("value", metricValue),
		zap.Float64("threshold", rule.ThresholdValue))
}

// dataAggregationLoop 数据聚合循环
func (s *NodeMonitoringService) dataAggregationLoop() {
	// 每小时执行一次数据聚合
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.aggregateHourlyData()
		case <-s.stopChan:
			return
		}
	}
}

// alertCheckLoop 告警检查循环
func (s *NodeMonitoringService) alertCheckLoop() {
	// 每分钟检查一次告警状态
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkOfflineNodes()
		case <-s.stopChan:
			return
		}
	}
}

// dataCleanupLoop 数据清理循环
func (s *NodeMonitoringService) dataCleanupLoop() {
	// 每天清理一次过期数据
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupExpiredData()
		case <-s.stopChan:
			return
		}
	}
}

// aggregateHourlyData 聚合小时数据
func (s *NodeMonitoringService) aggregateHourlyData() {
	logger.Info("开始聚合小时监控数据")

	// 获取所有节点
	nodes, err := s.db.DB.SQLite.ListNodes("", "", 1000, 0)
	if err != nil {
		logger.Error("获取节点列表失败", zap.Error(err))
		return
	}

	now := time.Now()
	hourStart := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()-1, 0, 0, 0, now.Location())
	hourEnd := hourStart.Add(1 * time.Hour)

	for _, node := range nodes {
		// 聚合该节点的小时数据
		s.aggregateNodeHourlyData(node.ID, hourStart, hourEnd)
	}
}

// aggregateNodeHourlyData 聚合单个节点的小时数据
func (s *NodeMonitoringService) aggregateNodeHourlyData(nodeID string, from, to time.Time) {
	// 查询小时内的所有监控数据
	dataList, err := s.db.DB.SQLite.ListNodeMonitoringData(nodeID, from, to, 1000)
	if err != nil || len(dataList) == 0 {
		return
	}

	// 计算各项平均值和最大值
	var (
		avgCPU, avgMemory, avgDisk       float64
		avgBandwidthIn, avgBandwidthOut  int64
		avgConnections                   int
		avgResponseTime                  float64
		maxCPU, maxMemory                float64
		maxConnections                   int
		maxResponseTime                  float64
		totalTrafficIn, totalTrafficOut  int64
		totalPacketsIn, totalPacketsOut  int64
		totalErrors                      int
	)

	count := len(dataList)
	for _, data := range dataList {
		avgCPU += data.CPUUsage
		avgMemory += data.MemoryUsagePercent
		avgDisk += data.DiskUsagePercent
		avgBandwidthIn += data.BandwidthIn
		avgBandwidthOut += data.BandwidthOut
		avgConnections += data.TotalConnections
		avgResponseTime += data.AvgResponseTime
		
		if data.CPUUsage > maxCPU {
			maxCPU = data.CPUUsage
		}
		if data.MemoryUsagePercent > maxMemory {
			maxMemory = data.MemoryUsagePercent
		}
		if data.TotalConnections > maxConnections {
			maxConnections = data.TotalConnections
		}
		if data.AvgResponseTime > maxResponseTime {
			maxResponseTime = data.AvgResponseTime
		}
		
		totalTrafficIn += data.TrafficInBytes
		totalTrafficOut += data.TrafficOutBytes
		totalPacketsIn += data.PacketsIn
		totalPacketsOut += data.PacketsOut
		totalErrors += data.ConnectionErrors + data.TunnelErrors
	}

	// 计算平均值
	avgCPU /= float64(count)
	avgMemory /= float64(count)
	avgDisk /= float64(count)
	avgBandwidthIn /= int64(count)
	avgBandwidthOut /= int64(count)
	avgConnections /= count
	avgResponseTime /= float64(count)

	// 创建历史记录
	history := &dbinit.NodePerformanceHistory{
		NodeID:              nodeID,
		Date:                from.Truncate(24 * time.Hour),
		AggregationType:     "hourly",
		AggregationTime:     from,
		AvgCPUUsage:         avgCPU,
		AvgMemoryUsage:      avgMemory,
		AvgDiskUsage:        avgDisk,
		AvgBandwidthIn:      avgBandwidthIn,
		AvgBandwidthOut:     avgBandwidthOut,
		AvgConnections:      avgConnections,
		AvgResponseTime:     avgResponseTime,
		MaxCPUUsage:         maxCPU,
		MaxMemoryUsage:      maxMemory,
		MaxConnections:      maxConnections,
		MaxResponseTime:     maxResponseTime,
		TotalTrafficIn:      totalTrafficIn,
		TotalTrafficOut:     totalTrafficOut,
		TotalPacketsIn:      totalPacketsIn,
		TotalPacketsOut:     totalPacketsOut,
		TotalErrors:         totalErrors,
		UptimeSeconds:       3600, // 1小时
		AvailabilityPercent: 100.0, // 简化计算，后续可以完善
	}

	if err := s.db.DB.SQLite.CreateNodePerformanceHistory(history); err != nil {
		logger.Error("创建性能历史记录失败",
			zap.String("nodeID", nodeID),
			zap.Error(err))
	}
}

// checkOfflineNodes 检查离线节点
func (s *NodeMonitoringService) checkOfflineNodes() {
	// 获取所有节点
	nodes, err := s.db.DB.SQLite.ListNodes("", "", 1000, 0)
	if err != nil {
		return
	}

	now := time.Now()
	for _, node := range nodes {
		// 如果5分钟内没有心跳，认为离线
		if node.Status == "online" && now.Sub(node.LastSeen) > 5*time.Minute {
			node.Status = "offline"
			s.db.DB.SQLite.UpdateNode(node)

			// 发送离线告警
			alert := Alert{
				Level:   AlertWarning,
				Title:   fmt.Sprintf("节点离线: %s", node.Name),
				Message: fmt.Sprintf("节点 %s (%s) 已离线超过5分钟", node.Name, node.ID),
				Tags:    []string{"node", "offline", node.ID},
			}
			s.alertSystem.Send(alert)
		}
	}
}

// cleanupExpiredData 清理过期数据
func (s *NodeMonitoringService) cleanupExpiredData() {
	logger.Info("开始清理过期监控数据")

	// 获取所有有监控配置的节点
	nodes, err := s.db.DB.SQLite.ListNodes("", "", 1000, 0)
	if err != nil {
		return
	}

	for _, node := range nodes {
		config, err := s.db.DB.SQLite.GetNodeMonitoringConfig(node.ID)
		if err != nil || config == nil {
			continue
		}

		// 清理过期的实时数据
		if err := s.db.DB.SQLite.DeleteOldMonitoringData(node.ID, config.DataRetentionDays); err != nil {
			logger.Error("清理监控数据失败",
				zap.String("nodeID", node.ID),
				zap.Error(err))
		}
	}
}

// GetNodeMonitoringStatus 获取节点监控状态概览
func (s *NodeMonitoringService) GetNodeMonitoringStatus(nodeID string) (*NodeMonitoringStatus, error) {
	// 获取最新监控数据
	data, err := s.db.DB.SQLite.GetLatestNodeMonitoringData(nodeID)
	if err != nil {
		return nil, err
	}

	if data == nil {
		return &NodeMonitoringStatus{
			NodeID:    nodeID,
			IsOnline:  false,
			LastSeen:  time.Time{},
			HasData:   false,
		}, nil
	}

	// 获取告警数量
	alerts, _ := s.db.DB.SQLite.ListNodeAlertHistory(nodeID, 10)
	activeAlerts := 0
	for _, alert := range alerts {
		if alert.Status == "triggered" {
			activeAlerts++
		}
	}

	status := &NodeMonitoringStatus{
		NodeID:           nodeID,
		IsOnline:         time.Since(data.Timestamp) < 5*time.Minute,
		LastSeen:         data.Timestamp,
		HasData:          true,
		CPUUsage:         data.CPUUsage,
		MemoryUsage:      data.MemoryUsagePercent,
		DiskUsage:        data.DiskUsagePercent,
		ActiveConnections: data.TotalConnections,
		ActiveTunnels:    data.ActiveTunnels,
		TrafficIn:        data.TrafficInBytes,
		TrafficOut:       data.TrafficOutBytes,
		ResponseTime:     data.AvgResponseTime,
		ActiveAlerts:     activeAlerts,
		Uptime:           data.SystemUptime,
	}

	return status, nil
}

// NodeMonitoringStatus 节点监控状态概览
type NodeMonitoringStatus struct {
	NodeID            string    `json:"node_id"`
	IsOnline          bool      `json:"is_online"`
	LastSeen          time.Time `json:"last_seen"`
	HasData           bool      `json:"has_data"`
	CPUUsage          float64   `json:"cpu_usage"`
	MemoryUsage       float64   `json:"memory_usage"`
	DiskUsage         float64   `json:"disk_usage"`
	ActiveConnections int       `json:"active_connections"`
	ActiveTunnels     int       `json:"active_tunnels"`
	TrafficIn         int64     `json:"traffic_in"`
	TrafficOut        int64     `json:"traffic_out"`
	ResponseTime      float64   `json:"response_time"`
	ActiveAlerts      int       `json:"active_alerts"`
	Uptime            int64     `json:"uptime"`
}

// CheckMonitoringPermission 检查用户是否有监控权限
func (s *NodeMonitoringService) CheckMonitoringPermission(userID, nodeID, requiredLevel string) bool {
	// 管理员拥有所有权限
	user, err := s.db.DB.SQLite.GetUser(userID)
	if err == nil && user != nil && user.Role == "admin" {
		return true
	}

	// 检查节点所有权
	node, err := s.db.DB.SQLite.GetNode(nodeID)
	if err != nil || node == nil {
		return false
	}

	// 节点所有者默认有基础查看权限
	if node.UserID == userID {
		return requiredLevel == "view_basic" || requiredLevel == "view_detailed"
	}

	// 检查明确的权限配置
	perm, err := s.db.DB.SQLite.GetMonitoringPermission(userID, nodeID)
	if err != nil || perm == nil {
		return false
	}

	if !perm.Enabled || perm.PermissionType == "disabled" {
		return false
	}

	// 权限级别检查
	switch requiredLevel {
	case "view_basic":
		return true // 任何非disabled权限都可以查看基础信息
	case "view_detailed":
		return perm.PermissionType == "view_detailed" || perm.PermissionType == "view_system" || perm.PermissionType == "view_network"
	case "view_system":
		return perm.PermissionType == "view_system"
	case "view_network":
		return perm.PermissionType == "view_network" || perm.PermissionType == "view_system"
	}

	return false
}
