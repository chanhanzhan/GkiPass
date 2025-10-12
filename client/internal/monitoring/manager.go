package monitoring

import (
	"context"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"
)

// Manager 监控管理器
type Manager struct {
	logger *zap.Logger

	// 系统统计
	systemStats  *SystemStats
	networkStats *NetworkStats
	appStats     *ApplicationStats

	// 状态管理
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 配置
	collectInterval time.Duration
	retentionPeriod time.Duration

	// 历史数据
	history struct {
		mutex   sync.RWMutex
		samples []StatsSample
		maxSize int
	}
}

// StatsSample 统计样本
type StatsSample struct {
	Timestamp    time.Time                `json:"timestamp"`
	SystemStats  SystemStatsSnapshot      `json:"system_stats"`
	NetworkStats NetworkStatsSnapshot     `json:"network_stats"`
	AppStats     ApplicationStatsSnapshot `json:"app_stats"`
}

// SystemStats 系统统计
type SystemStats struct {
	process    *process.Process
	startTime  time.Time
	cpuPercent atomic.Value // float64
	memUsage   atomic.Int64 // bytes

	// 计数器
	cpuSamples    atomic.Int64
	memorySamples atomic.Int64
}

// SystemStatsSnapshot 系统统计快照
type SystemStatsSnapshot struct {
	CPUPercent     float64   `json:"cpu_percent"`
	MemoryUsage    int64     `json:"memory_usage"`
	MemoryPercent  float64   `json:"memory_percent"`
	GoroutineCount int       `json:"goroutine_count"`
	HeapUsage      int64     `json:"heap_usage"`
	StackUsage     int64     `json:"stack_usage"`
	GCCount        uint32    `json:"gc_count"`
	LastGCTime     time.Time `json:"last_gc_time"`
	Uptime         int64     `json:"uptime_seconds"`
}

// NetworkStats 网络统计
type NetworkStats struct {
	bytesIn        atomic.Int64
	bytesOut       atomic.Int64
	packetsIn      atomic.Int64
	packetsOut     atomic.Int64
	connectionsIn  atomic.Int64
	connectionsOut atomic.Int64
	errors         atomic.Int64
}

// NetworkStatsSnapshot 网络统计快照
type NetworkStatsSnapshot struct {
	BytesIn        int64          `json:"bytes_in"`
	BytesOut       int64          `json:"bytes_out"`
	PacketsIn      int64          `json:"packets_in"`
	PacketsOut     int64          `json:"packets_out"`
	ConnectionsIn  int64          `json:"connections_in"`
	ConnectionsOut int64          `json:"connections_out"`
	Errors         int64          `json:"errors"`
	Bandwidth      BandwidthStats `json:"bandwidth"`
}

// BandwidthStats 带宽统计
type BandwidthStats struct {
	InboundBps  float64 `json:"inbound_bps"`
	OutboundBps float64 `json:"outbound_bps"`
}

// ApplicationStats 应用统计
type ApplicationStats struct {
	// 连接统计
	activeConnections atomic.Int64
	totalConnections  atomic.Int64
	failedConnections atomic.Int64

	// 协议统计
	protocolCounts map[string]*atomic.Int64
	protocolMutex  sync.RWMutex

	// 处理统计
	requestsProcessed atomic.Int64
	requestsFailed    atomic.Int64
	avgResponseTime   atomic.Int64 // nanoseconds

	// 流量统计
	tunnelBytesIn     atomic.Int64
	tunnelBytesOut    atomic.Int64
	tunnelConnections atomic.Int64
}

// ApplicationStatsSnapshot 应用统计快照
type ApplicationStatsSnapshot struct {
	ActiveConnections int64            `json:"active_connections"`
	TotalConnections  int64            `json:"total_connections"`
	FailedConnections int64            `json:"failed_connections"`
	ProtocolCounts    map[string]int64 `json:"protocol_counts"`
	RequestsProcessed int64            `json:"requests_processed"`
	RequestsFailed    int64            `json:"requests_failed"`
	AvgResponseTime   int64            `json:"avg_response_time_ns"`
	TunnelBytesIn     int64            `json:"tunnel_bytes_in"`
	TunnelBytesOut    int64            `json:"tunnel_bytes_out"`
	TunnelConnections int64            `json:"tunnel_connections"`
	SuccessRate       float64          `json:"success_rate"`
}

// NewManager 创建监控管理器 (通用接口)
func NewManager() (*Manager, error) {
	return New()
}

// New 创建监控管理器
func New() (*Manager, error) {
	// 获取当前进程
	pid := int32(os.Getpid())
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		logger:          zap.L().Named("monitoring"),
		collectInterval: 10 * time.Second,
		retentionPeriod: 24 * time.Hour,
		systemStats: &SystemStats{
			process:   proc,
			startTime: time.Now(),
		},
		networkStats: &NetworkStats{},
		appStats: &ApplicationStats{
			protocolCounts: make(map[string]*atomic.Int64),
		},
	}

	// 初始化历史数据
	manager.history.maxSize = int(manager.retentionPeriod / manager.collectInterval)
	manager.history.samples = make([]StatsSample, 0, manager.history.maxSize)

	return manager, nil
}

// Start 启动监控管理器
func (m *Manager) Start() error {
	m.ctx, m.cancel = context.WithCancel(context.Background())

	m.logger.Info("📊 监控管理器启动",
		zap.Duration("collect_interval", m.collectInterval),
		zap.Duration("retention_period", m.retentionPeriod))

	// 启动统计收集协程
	m.wg.Add(3)
	go m.collectSystemStats()
	go m.collectNetworkStats()
	go m.collectAndStore()

	return nil
}

// Stop 停止监控管理器
func (m *Manager) Stop() error {
	m.logger.Info("🔄 正在停止监控管理器...")

	if m.cancel != nil {
		m.cancel()
	}

	m.wg.Wait()

	m.logger.Info("✅ 监控管理器已停止")
	return nil
}

// collectSystemStats 收集系统统计
func (m *Manager) collectSystemStats() {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.updateSystemStats()
		case <-m.ctx.Done():
			return
		}
	}
}

// updateSystemStats 更新系统统计
func (m *Manager) updateSystemStats() {
	// CPU使用率
	if cpuPercents, err := cpu.Percent(0, false); err == nil && len(cpuPercents) > 0 {
		m.systemStats.cpuPercent.Store(cpuPercents[0])
		m.systemStats.cpuSamples.Add(1)
	}

	// 内存使用
	if memInfo, err := mem.VirtualMemory(); err == nil {
		m.systemStats.memUsage.Store(int64(memInfo.Used))
		m.systemStats.memorySamples.Add(1)
	}
}

// collectNetworkStats 收集网络统计
func (m *Manager) collectNetworkStats() {
	defer m.wg.Done()

	var lastStats []net.IOCountersStat
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if stats, err := net.IOCounters(false); err == nil && len(stats) > 0 {
				if len(lastStats) > 0 {
					// 计算差值
					deltaBytes := int64(stats[0].BytesRecv - lastStats[0].BytesRecv)
					m.networkStats.bytesIn.Add(deltaBytes)

					deltaBytes = int64(stats[0].BytesSent - lastStats[0].BytesSent)
					m.networkStats.bytesOut.Add(deltaBytes)

					deltaPackets := int64(stats[0].PacketsRecv - lastStats[0].PacketsRecv)
					m.networkStats.packetsIn.Add(deltaPackets)

					deltaPackets = int64(stats[0].PacketsSent - lastStats[0].PacketsSent)
					m.networkStats.packetsOut.Add(deltaPackets)
				}
				lastStats = stats
			}
		case <-m.ctx.Done():
			return
		}
	}
}

// collectAndStore 收集并存储统计快照
func (m *Manager) collectAndStore() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.collectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sample := m.createStatsSample()
			m.storeStatsSample(sample)
		case <-m.ctx.Done():
			return
		}
	}
}

// createStatsSample 创建统计样本
func (m *Manager) createStatsSample() StatsSample {
	return StatsSample{
		Timestamp:    time.Now(),
		SystemStats:  m.getSystemStatsSnapshot(),
		NetworkStats: m.getNetworkStatsSnapshot(),
		AppStats:     m.getApplicationStatsSnapshot(),
	}
}

// storeStatsSample 存储统计样本
func (m *Manager) storeStatsSample(sample StatsSample) {
	m.history.mutex.Lock()
	defer m.history.mutex.Unlock()

	// 添加新样本
	m.history.samples = append(m.history.samples, sample)

	// 如果超过最大容量，移除最旧的样本
	if len(m.history.samples) > m.history.maxSize {
		copy(m.history.samples, m.history.samples[1:])
		m.history.samples = m.history.samples[:m.history.maxSize]
	}
}

// getSystemStatsSnapshot 获取系统统计快照
func (m *Manager) getSystemStatsSnapshot() SystemStatsSnapshot {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	var cpuPercent float64
	if val := m.systemStats.cpuPercent.Load(); val != nil {
		cpuPercent = val.(float64)
	}

	// 获取内存信息
	var memPercent float64
	if memInfo, err := mem.VirtualMemory(); err == nil {
		memPercent = memInfo.UsedPercent
	}

	// 计算运行时间
	uptime := int64(time.Since(m.systemStats.startTime).Seconds())

	return SystemStatsSnapshot{
		CPUPercent:     cpuPercent,
		MemoryUsage:    m.systemStats.memUsage.Load(),
		MemoryPercent:  memPercent,
		GoroutineCount: runtime.NumGoroutine(),
		HeapUsage:      int64(memStats.HeapInuse),
		StackUsage:     int64(memStats.StackInuse),
		GCCount:        memStats.NumGC,
		LastGCTime:     time.Unix(0, int64(memStats.LastGC)),
		Uptime:         uptime,
	}
}

// getNetworkStatsSnapshot 获取网络统计快照
func (m *Manager) getNetworkStatsSnapshot() NetworkStatsSnapshot {
	bytesIn := m.networkStats.bytesIn.Load()
	bytesOut := m.networkStats.bytesOut.Load()

	// 计算带宽（简化版本，基于最近的数据）
	bandwidth := BandwidthStats{
		InboundBps:  float64(bytesIn) / m.collectInterval.Seconds(),
		OutboundBps: float64(bytesOut) / m.collectInterval.Seconds(),
	}

	return NetworkStatsSnapshot{
		BytesIn:        bytesIn,
		BytesOut:       bytesOut,
		PacketsIn:      m.networkStats.packetsIn.Load(),
		PacketsOut:     m.networkStats.packetsOut.Load(),
		ConnectionsIn:  m.networkStats.connectionsIn.Load(),
		ConnectionsOut: m.networkStats.connectionsOut.Load(),
		Errors:         m.networkStats.errors.Load(),
		Bandwidth:      bandwidth,
	}
}

// getApplicationStatsSnapshot 获取应用统计快照
func (m *Manager) getApplicationStatsSnapshot() ApplicationStatsSnapshot {
	m.appStats.protocolMutex.RLock()
	protocolCounts := make(map[string]int64)
	for protocol, counter := range m.appStats.protocolCounts {
		protocolCounts[protocol] = counter.Load()
	}
	m.appStats.protocolMutex.RUnlock()

	// 计算成功率
	total := m.appStats.requestsProcessed.Load()
	failed := m.appStats.requestsFailed.Load()
	var successRate float64
	if total > 0 {
		successRate = float64(total-failed) / float64(total)
	}

	return ApplicationStatsSnapshot{
		ActiveConnections: m.appStats.activeConnections.Load(),
		TotalConnections:  m.appStats.totalConnections.Load(),
		FailedConnections: m.appStats.failedConnections.Load(),
		ProtocolCounts:    protocolCounts,
		RequestsProcessed: total,
		RequestsFailed:    failed,
		AvgResponseTime:   m.appStats.avgResponseTime.Load(),
		TunnelBytesIn:     m.appStats.tunnelBytesIn.Load(),
		TunnelBytesOut:    m.appStats.tunnelBytesOut.Load(),
		TunnelConnections: m.appStats.tunnelConnections.Load(),
		SuccessRate:       successRate,
	}
}

// GetStats 获取统计信息
func (m *Manager) GetStats() map[string]interface{} {
	current := m.createStatsSample()

	m.history.mutex.RLock()
	historyCount := len(m.history.samples)
	m.history.mutex.RUnlock()

	return map[string]interface{}{
		"current":          current,
		"history_count":    historyCount,
		"retention_period": m.retentionPeriod.String(),
		"collect_interval": m.collectInterval.String(),
	}
}

// GetHistoricalStats 获取历史统计
func (m *Manager) GetHistoricalStats(since time.Time) []StatsSample {
	m.history.mutex.RLock()
	defer m.history.mutex.RUnlock()

	var result []StatsSample
	for _, sample := range m.history.samples {
		if sample.Timestamp.After(since) {
			result = append(result, sample)
		}
	}

	return result
}

// RecordConnection 记录连接
func (m *Manager) RecordConnection(active bool) {
	if active {
		m.appStats.activeConnections.Add(1)
	} else {
		m.appStats.activeConnections.Add(-1)
	}
	m.appStats.totalConnections.Add(1)
}

// RecordConnectionFailed 记录连接失败
func (m *Manager) RecordConnectionFailed() {
	m.appStats.failedConnections.Add(1)
}

// RecordProtocol 记录协议使用
func (m *Manager) RecordProtocol(protocol string) {
	m.appStats.protocolMutex.Lock()
	defer m.appStats.protocolMutex.Unlock()

	if counter, exists := m.appStats.protocolCounts[protocol]; exists {
		counter.Add(1)
	} else {
		counter := &atomic.Int64{}
		counter.Add(1)
		m.appStats.protocolCounts[protocol] = counter
	}
}

// RecordRequest 记录请求处理
func (m *Manager) RecordRequest(success bool, responseTime time.Duration) {
	m.appStats.requestsProcessed.Add(1)
	if !success {
		m.appStats.requestsFailed.Add(1)
	}

	// 更新平均响应时间（简化算法）
	currentAvg := m.appStats.avgResponseTime.Load()
	newAvg := (currentAvg + responseTime.Nanoseconds()) / 2
	m.appStats.avgResponseTime.Store(newAvg)
}

// RecordTunnelTraffic 记录隧道流量
func (m *Manager) RecordTunnelTraffic(bytesIn, bytesOut int64) {
	m.appStats.tunnelBytesIn.Add(bytesIn)
	m.appStats.tunnelBytesOut.Add(bytesOut)
}

// RecordTunnelConnection 记录隧道连接
func (m *Manager) RecordTunnelConnection() {
	m.appStats.tunnelConnections.Add(1)
}

// RecordNetworkTraffic 记录网络流量
func (m *Manager) RecordNetworkTraffic(bytesIn, bytesOut int64) {
	m.networkStats.bytesIn.Add(bytesIn)
	m.networkStats.bytesOut.Add(bytesOut)
}

// RecordNetworkError 记录网络错误
func (m *Manager) RecordNetworkError() {
	m.networkStats.errors.Add(1)
}
