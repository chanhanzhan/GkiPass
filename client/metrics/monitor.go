package metrics

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// SystemMonitor 系统监控器
type SystemMonitor struct {
	cpuUsage    atomic.Value // float64
	memUsage    atomic.Int64
	netIn       atomic.Int64
	netOut      atomic.Int64
	connections atomic.Int64
	goroutines  atomic.Int32

	lastNetStats net.IOCountersStat
	mu           sync.RWMutex
}

// NewSystemMonitor 创建系统监控器
func NewSystemMonitor() *SystemMonitor {
	sm := &SystemMonitor{}
	sm.cpuUsage.Store(float64(0))
	return sm
}

// Start 启动监控
func (sm *SystemMonitor) Start() {
	go sm.collectLoop()
}

// collectLoop 采集循环
func (sm *SystemMonitor) collectLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		sm.collect()
	}
}

// collect 采集指标
func (sm *SystemMonitor) collect() {
	// CPU使用率
	if cpuPercent, err := cpu.Percent(time.Second, false); err == nil && len(cpuPercent) > 0 {
		sm.cpuUsage.Store(cpuPercent[0])
	}

	// 内存使用
	if vmStat, err := mem.VirtualMemory(); err == nil {
		sm.memUsage.Store(int64(vmStat.Used))
	}

	// 网络流量
	if netStats, err := net.IOCounters(false); err == nil && len(netStats) > 0 {
		sm.mu.Lock()
		if sm.lastNetStats.BytesRecv > 0 {
			sm.netIn.Store(int64(netStats[0].BytesRecv - sm.lastNetStats.BytesRecv))
			sm.netOut.Store(int64(netStats[0].BytesSent - sm.lastNetStats.BytesSent))
		}
		sm.lastNetStats = netStats[0]
		sm.mu.Unlock()
	}

	// Goroutine数量
	sm.goroutines.Store(int32(runtime.NumGoroutine()))
}

// GetCPUUsage 获取CPU使用率
func (sm *SystemMonitor) GetCPUUsage() float64 {
	if val := sm.cpuUsage.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

// GetMemoryUsage 获取内存使用
func (sm *SystemMonitor) GetMemoryUsage() int64 {
	return sm.memUsage.Load()
}

// GetNetworkIO 获取网络IO
func (sm *SystemMonitor) GetNetworkIO() (bytesIn, bytesOut int64) {
	return sm.netIn.Load(), sm.netOut.Load()
}

// SetConnections 设置连接数
func (sm *SystemMonitor) SetConnections(count int64) {
	sm.connections.Store(count)
}

// GetConnections 获取连接数
func (sm *SystemMonitor) GetConnections() int64 {
	return sm.connections.Load()
}

// GetGoroutines 获取Goroutine数量
func (sm *SystemMonitor) GetGoroutines() int32 {
	return sm.goroutines.Load()
}

// GetSnapshot 获取监控快照
func (sm *SystemMonitor) GetSnapshot() MonitorSnapshot {
	netIn, netOut := sm.GetNetworkIO()
	return MonitorSnapshot{
		CPUUsage:    sm.GetCPUUsage(),
		MemUsage:    sm.GetMemoryUsage(),
		NetIn:       netIn,
		NetOut:      netOut,
		Connections: sm.GetConnections(),
		Goroutines:  sm.GetGoroutines(),
		Timestamp:   time.Now(),
	}
}

// MonitorSnapshot 监控快照
type MonitorSnapshot struct {
	CPUUsage    float64   `json:"cpu_usage"`
	MemUsage    int64     `json:"mem_usage"`
	NetIn       int64     `json:"net_in"`
	NetOut      int64     `json:"net_out"`
	Connections int64     `json:"connections"`
	Goroutines  int32     `json:"goroutines"`
	Timestamp   time.Time `json:"timestamp"`
}






