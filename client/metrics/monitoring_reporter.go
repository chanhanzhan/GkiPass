package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"gkipass/client/core"
	"gkipass/client/logger"
	"gkipass/client/ws"

	"go.uber.org/zap"
)

// MonitoringReporter 监控数据上报器
type MonitoringReporter struct {
	nodeID         string
	planeURL       string
	connectionKey  string
	systemMonitor  *EnhancedSystemMonitor
	tunnelManager  *core.TunnelManager
	wsClient       *ws.Client
	reportInterval time.Duration
	httpClient     *http.Client
	stopChan       chan struct{}
	started        bool
	mu             sync.RWMutex
}

// NewMonitoringReporter 创建监控上报器
func NewMonitoringReporter(nodeID, planeURL, connectionKey string, systemMonitor *EnhancedSystemMonitor, tunnelManager *core.TunnelManager, wsClient *ws.Client) *MonitoringReporter {
	return &MonitoringReporter{
		nodeID:         nodeID,
		planeURL:       planeURL,
		connectionKey:  connectionKey,
		systemMonitor:  systemMonitor,
		tunnelManager:  tunnelManager,
		wsClient:       wsClient,
		reportInterval: 60 * time.Second, // 默认60秒上报一次
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		stopChan: make(chan struct{}),
	}
}

// Start 启动上报器
func (r *MonitoringReporter) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return nil
	}

	r.started = true
	go r.reportLoop()

	logger.Info("监控数据上报器已启动",
		zap.String("nodeID", r.nodeID),
		zap.Duration("interval", r.reportInterval))

	return nil
}

// Stop 停止上报器
func (r *MonitoringReporter) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.started {
		return
	}

	close(r.stopChan)
	r.started = false

	logger.Info("监控数据上报器已停止")
}

// SetReportInterval 设置上报间隔
func (r *MonitoringReporter) SetReportInterval(interval time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if interval < 10*time.Second {
		interval = 10 * time.Second // 最小10秒
	}
	if interval > 300*time.Second {
		interval = 300 * time.Second // 最大5分钟
	}

	r.reportInterval = interval
	logger.Info("监控上报间隔已更新", zap.Duration("interval", interval))
}

// reportLoop 上报循环
func (r *MonitoringReporter) reportLoop() {
	ticker := time.NewTicker(r.reportInterval)
	defer ticker.Stop()

	// 立即执行一次上报
	r.doReport()

	for {
		select {
		case <-ticker.C:
			r.doReport()
		case <-r.stopChan:
			return
		}
	}
}

// doReport 执行上报
func (r *MonitoringReporter) doReport() {
	startTime := time.Now()

	// 获取系统快照
	snapshot := r.systemMonitor.GetFullSnapshot()

	// 获取隧道统计
	tunnelStats := r.getTunnelStats()

	// 构建上报数据
	reportData := MonitoringReportData{
		NodeID:    r.nodeID,
		Timestamp: time.Now(),
		SystemInfo: SystemInfo{
			Uptime:             snapshot.Uptime,
			BootTime:           snapshot.BootTime,
			CPUUsage:           snapshot.CPUUsage,
			CPULoad1m:          snapshot.CPULoad1m,
			CPULoad5m:          snapshot.CPULoad5m,
			CPULoad15m:         snapshot.CPULoad15m,
			CPUCores:           snapshot.CPUCores,
			MemoryTotal:        snapshot.MemoryTotal,
			MemoryUsed:         snapshot.MemoryUsed,
			MemoryAvailable:    snapshot.MemoryAvailable,
			MemoryUsagePercent: snapshot.MemoryPercent,
			DiskTotal:          snapshot.DiskTotal,
			DiskUsed:           snapshot.DiskUsed,
			DiskAvailable:      snapshot.DiskAvailable,
			DiskUsagePercent:   snapshot.DiskPercent,
		},
		NetworkStats: NetworkStats{
			Interfaces:       convertEnhancedInterfaces(snapshot.Interfaces),
			BandwidthIn:      snapshot.NetBytesIn,
			BandwidthOut:     snapshot.NetBytesOut,
			TCPConnections:   snapshot.TCPConnections,
			UDPConnections:   snapshot.UDPConnections,
			TotalConnections: snapshot.TotalConnections,
			TrafficInBytes:   snapshot.NetBytesIn,
			TrafficOutBytes:  snapshot.NetBytesOut,
			PacketsIn:        snapshot.NetPacketsIn,
			PacketsOut:       snapshot.NetPacketsOut,
			ConnectionErrors: 0, // TODO: 实现连接错误统计
		},
		TunnelStats: tunnelStats,
		Performance: Performance{
			AvgResponseTime: snapshot.AvgResponseTime,
			MaxResponseTime: snapshot.MaxResponseTime,
			MinResponseTime: snapshot.MinResponseTime,
			RequestsPerSec:  int(snapshot.RequestCount / max(snapshot.Uptime, 1)),
			ErrorRate:       snapshot.ErrorRate,
		},
		AppInfo: AppInfo{
			Version:      snapshot.AppVersion,
			GoVersion:    snapshot.GoVersion,
			OSInfo:       snapshot.OSInfo,
			Architecture: snapshot.Architecture,
			StartTime:    r.systemMonitor.startTime,
		},
		ConfigVersion: "1.0", // TODO: 从配置管理器获取
	}

	// 发送数据
	var err error
	if r.wsClient != nil && r.wsClient.IsConnected() {
		err = r.sendViaWebSocket(reportData)
		if err != nil {
			logger.Warn("WebSocket上报失败，尝试HTTP", zap.Error(err))
			err = r.sendViaHTTP(reportData)
		}
	} else {
		err = r.sendViaHTTP(reportData)
	}

	duration := time.Since(startTime)

	if err != nil {
		logger.Error("监控数据上报失败",
			zap.String("nodeID", r.nodeID),
			zap.Duration("duration", duration),
			zap.Error(err))
	} else {
		logger.Debug("监控数据上报成功",
			zap.String("nodeID", r.nodeID),
			zap.Duration("duration", duration),
			zap.Float64("cpuUsage", reportData.SystemInfo.CPUUsage),
			zap.Float64("memUsage", reportData.SystemInfo.MemoryUsagePercent),
			zap.Int("tunnels", reportData.TunnelStats.ActiveTunnels))
	}
}

// sendViaWebSocket 通过WebSocket发送数据
func (r *MonitoringReporter) sendViaWebSocket(data MonitoringReportData) error {
	msg, err := ws.NewMessage(ws.MsgTypeMonitoringReport, data)
	if err != nil {
		return fmt.Errorf("创建WebSocket消息失败: %w", err)
	}

	return r.wsClient.Send(msg)
}

// sendViaHTTP 通过HTTP发送数据
func (r *MonitoringReporter) sendViaHTTP(data MonitoringReportData) error {
	url := fmt.Sprintf("%s/api/v1/monitoring/report/%s", r.planeURL, r.nodeID)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化数据失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Connection-Key", r.connectionKey)
	req.Header.Set("User-Agent", "GKIPass-Client/1.0")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器响应错误: %d", resp.StatusCode)
	}

	return nil
}

// getTunnelStats 获取隧道统计
func (r *MonitoringReporter) getTunnelStats() TunnelStats {
	if r.tunnelManager == nil {
		return TunnelStats{
			ActiveTunnels: 0,
			TunnelErrors:  0,
			TunnelList:    []TunnelStatusInfo{},
		}
	}

	// 获取所有规则和统计
	rules := r.tunnelManager.ListRules()
	allStats := r.tunnelManager.GetAllStats()

	activeTunnels := 0
	tunnelErrors := 0
	tunnelList := make([]TunnelStatusInfo, 0, len(rules))

	for _, rule := range rules {
		isActive := rule.Enabled
		if isActive {
			activeTunnels++
		}

		// 获取对应的统计数据
		stats := allStats[rule.TunnelID]
		errorCount := 0
		connections := 0
		trafficIn := int64(0)
		trafficOut := int64(0)

		if stats != nil {
			errorCount = int(stats.Errors.Load())
			connections = int(stats.Connections.Load())
			trafficIn = stats.TrafficIn.Load()
			trafficOut = stats.TrafficOut.Load()
		}

		tunnelInfo := TunnelStatusInfo{
			TunnelID:        rule.TunnelID,
			Name:            rule.Name,
			Protocol:        rule.Protocol,
			LocalPort:       rule.LocalPort,
			Status:          getStatusString(isActive),
			Connections:     connections,
			TrafficIn:       trafficIn,
			TrafficOut:      trafficOut,
			AvgResponseTime: 0, // TODO: 实现响应时间统计
			ErrorCount:      errorCount,
		}

		tunnelErrors += tunnelInfo.ErrorCount
		tunnelList = append(tunnelList, tunnelInfo)
	}

	return TunnelStats{
		ActiveTunnels: activeTunnels,
		TunnelErrors:  tunnelErrors,
		TunnelList:    tunnelList,
	}
}

// 监控上报数据结构定义
type MonitoringReportData struct {
	NodeID        string       `json:"node_id"`
	Timestamp     time.Time    `json:"timestamp"`
	SystemInfo    SystemInfo   `json:"system_info"`
	NetworkStats  NetworkStats `json:"network_stats"`
	TunnelStats   TunnelStats  `json:"tunnel_stats"`
	Performance   Performance  `json:"performance"`
	AppInfo       AppInfo      `json:"app_info"`
	ConfigVersion string       `json:"config_version"`
}

type SystemInfo struct {
	Uptime             int64     `json:"uptime"`
	BootTime           time.Time `json:"boot_time"`
	CPUUsage           float64   `json:"cpu_usage"`
	CPULoad1m          float64   `json:"cpu_load_1m"`
	CPULoad5m          float64   `json:"cpu_load_5m"`
	CPULoad15m         float64   `json:"cpu_load_15m"`
	CPUCores           int       `json:"cpu_cores"`
	MemoryTotal        int64     `json:"memory_total"`
	MemoryUsed         int64     `json:"memory_used"`
	MemoryAvailable    int64     `json:"memory_available"`
	MemoryUsagePercent float64   `json:"memory_usage_percent"`
	DiskTotal          int64     `json:"disk_total"`
	DiskUsed           int64     `json:"disk_used"`
	DiskAvailable      int64     `json:"disk_available"`
	DiskUsagePercent   float64   `json:"disk_usage_percent"`
}

type NetworkStats struct {
	Interfaces       []NetworkInterface `json:"interfaces"`
	BandwidthIn      int64              `json:"bandwidth_in"`
	BandwidthOut     int64              `json:"bandwidth_out"`
	TCPConnections   int                `json:"tcp_connections"`
	UDPConnections   int                `json:"udp_connections"`
	TotalConnections int                `json:"total_connections"`
	TrafficInBytes   int64              `json:"traffic_in_bytes"`
	TrafficOutBytes  int64              `json:"traffic_out_bytes"`
	PacketsIn        int64              `json:"packets_in"`
	PacketsOut       int64              `json:"packets_out"`
	ConnectionErrors int                `json:"connection_errors"`
}

type NetworkInterface struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Status    string `json:"status"`
	Speed     int64  `json:"speed"`
	RxBytes   int64  `json:"rx_bytes"`
	TxBytes   int64  `json:"tx_bytes"`
	RxPackets int64  `json:"rx_packets"`
	TxPackets int64  `json:"tx_packets"`
	RxErrors  int64  `json:"rx_errors"`
	TxErrors  int64  `json:"tx_errors"`
}

type TunnelStats struct {
	ActiveTunnels int                `json:"active_tunnels"`
	TunnelErrors  int                `json:"tunnel_errors"`
	TunnelList    []TunnelStatusInfo `json:"tunnel_list"`
}

type TunnelStatusInfo struct {
	TunnelID        string  `json:"tunnel_id"`
	Name            string  `json:"name"`
	Protocol        string  `json:"protocol"`
	LocalPort       int     `json:"local_port"`
	Status          string  `json:"status"`
	Connections     int     `json:"connections"`
	TrafficIn       int64   `json:"traffic_in"`
	TrafficOut      int64   `json:"traffic_out"`
	AvgResponseTime float64 `json:"avg_response_time"`
	ErrorCount      int     `json:"error_count"`
}

type Performance struct {
	AvgResponseTime float64 `json:"avg_response_time"`
	MaxResponseTime float64 `json:"max_response_time"`
	MinResponseTime float64 `json:"min_response_time"`
	RequestsPerSec  int     `json:"requests_per_sec"`
	ErrorRate       float64 `json:"error_rate"`
}

type AppInfo struct {
	Version      string    `json:"version"`
	GoVersion    string    `json:"go_version"`
	OSInfo       string    `json:"os_info"`
	Architecture string    `json:"architecture"`
	StartTime    time.Time `json:"start_time"`
}

// convertEnhancedInterfaces 转换增强版网络接口为普通接口
func convertEnhancedInterfaces(enhanced []EnhancedNetworkInterface) []NetworkInterface {
	result := make([]NetworkInterface, len(enhanced))
	for i, iface := range enhanced {
		result[i] = NetworkInterface{
			Name:      iface.Name,
			IP:        iface.IP,
			MAC:       iface.MAC,
			Status:    iface.Status,
			Speed:     iface.Speed,
			RxBytes:   iface.RxBytes,
			TxBytes:   iface.TxBytes,
			RxPackets: iface.RxPackets,
			TxPackets: iface.TxPackets,
			RxErrors:  iface.RxErrors,
			TxErrors:  iface.TxErrors,
		}
	}
	return result
}

// getStatusString 获取状态字符串
func getStatusString(active bool) string {
	if active {
		return "active"
	}
	return "inactive"
}

// max 辅助函数
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
