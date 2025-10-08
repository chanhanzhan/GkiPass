package metrics

import (
	"encoding/json"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/logger"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// EnhancedSystemMonitor 增强版系统监控器
type EnhancedSystemMonitor struct {
	// 基础指标
	cpuUsage     atomic.Value // float64
	cpuLoad1     atomic.Value // float64
	cpuLoad5     atomic.Value // float64
	cpuLoad15    atomic.Value // float64
	cpuCores     atomic.Int32
	memTotal     atomic.Int64
	memUsed      atomic.Int64
	memAvailable atomic.Int64
	memPercent   atomic.Value // float64

	// 磁盘信息
	diskTotal     atomic.Int64
	diskUsed      atomic.Int64
	diskAvailable atomic.Int64
	diskPercent   atomic.Value // float64

	// 网络统计
	netBytesIn    atomic.Int64
	netBytesOut   atomic.Int64
	netPacketsIn  atomic.Int64
	netPacketsOut atomic.Int64
	netErrors     atomic.Int64

	// 连接统计
	tcpConns   atomic.Int32
	udpConns   atomic.Int32
	totalConns atomic.Int32

	// 应用信息
	startTime  time.Time
	goRoutines atomic.Int32
	goVersion  string
	appVersion string

	// 系统信息
	osInfo       string
	architecture string
	hostName     string

	// 性能指标
	responseTime atomic.Value // float64 - 平均响应时间
	maxRespTime  atomic.Value // float64 - 最大响应时间
	minRespTime  atomic.Value // float64 - 最小响应时间
	requestCount atomic.Int64
	errorCount   atomic.Int64

	// 网络接口信息
	interfaces   []EnhancedNetworkInterface
	interfacesMu sync.RWMutex

	// 上次网络统计
	lastNetStats []net.IOCountersStat
	netStatsMu   sync.RWMutex

	// 控制字段
	stopChan chan struct{}
	started  bool
	mu       sync.RWMutex
}

// EnhancedNetworkInterface 增强版网络接口信息
type EnhancedNetworkInterface struct {
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

// NewEnhancedSystemMonitor 创建增强版系统监控器
func NewEnhancedSystemMonitor(appVersion string) *EnhancedSystemMonitor {
	monitor := &EnhancedSystemMonitor{
		startTime:  time.Now(),
		appVersion: appVersion,
		goVersion:  runtime.Version(),
		stopChan:   make(chan struct{}),
	}

	// 初始化原子值
	monitor.cpuUsage.Store(float64(0))
	monitor.cpuLoad1.Store(float64(0))
	monitor.cpuLoad5.Store(float64(0))
	monitor.cpuLoad15.Store(float64(0))
	monitor.memPercent.Store(float64(0))
	monitor.diskPercent.Store(float64(0))
	monitor.responseTime.Store(float64(0))
	monitor.maxRespTime.Store(float64(0))
	monitor.minRespTime.Store(float64(999999))

	// 获取静态系统信息
	if hostInfo, err := host.Info(); err == nil {
		monitor.osInfo = hostInfo.Platform + " " + hostInfo.PlatformVersion
		monitor.architecture = hostInfo.KernelArch
		monitor.hostName = hostInfo.Hostname
	}

	// 获取CPU核心数
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		monitor.cpuCores.Store(int32(cpuInfo[0].Cores))
	}

	return monitor
}

// Start 启动监控
func (m *EnhancedSystemMonitor) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return nil
	}

	m.started = true
	go m.collectLoop()

	logger.Info("增强版系统监控器已启动")
	return nil
}

// Stop 停止监控
func (m *EnhancedSystemMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return
	}

	close(m.stopChan)
	m.started = false

	logger.Info("增强版系统监控器已停止")
}

// collectLoop 数据收集循环
func (m *EnhancedSystemMonitor) collectLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.collectMetrics()
		case <-m.stopChan:
			return
		}
	}
}

// collectMetrics 收集系统指标
func (m *EnhancedSystemMonitor) collectMetrics() {
	// 并发收集各项指标
	var wg sync.WaitGroup

	wg.Add(5)

	// CPU指标
	go func() {
		defer wg.Done()
		m.collectCPUMetrics()
	}()

	// 内存指标
	go func() {
		defer wg.Done()
		m.collectMemoryMetrics()
	}()

	// 磁盘指标
	go func() {
		defer wg.Done()
		m.collectDiskMetrics()
	}()

	// 网络指标
	go func() {
		defer wg.Done()
		m.collectNetworkMetrics()
	}()

	// 连接指标
	go func() {
		defer wg.Done()
		m.collectConnectionMetrics()
	}()

	wg.Wait()

	// 更新应用级指标
	m.goRoutines.Store(int32(runtime.NumGoroutine()))
}

// collectCPUMetrics 收集CPU指标
func (m *EnhancedSystemMonitor) collectCPUMetrics() {
	// CPU使用率
	if cpuPercent, err := cpu.Percent(time.Second, false); err == nil && len(cpuPercent) > 0 {
		m.cpuUsage.Store(cpuPercent[0])
	}

	// CPU负载
	if loadStat, err := load.Avg(); err == nil {
		m.cpuLoad1.Store(loadStat.Load1)
		m.cpuLoad5.Store(loadStat.Load5)
		m.cpuLoad15.Store(loadStat.Load15)
	}
}

// collectMemoryMetrics 收集内存指标
func (m *EnhancedSystemMonitor) collectMemoryMetrics() {
	if vmStat, err := mem.VirtualMemory(); err == nil {
		m.memTotal.Store(int64(vmStat.Total))
		m.memUsed.Store(int64(vmStat.Used))
		m.memAvailable.Store(int64(vmStat.Available))
		m.memPercent.Store(vmStat.UsedPercent)
	}
}

// collectDiskMetrics 收集磁盘指标
func (m *EnhancedSystemMonitor) collectDiskMetrics() {
	if diskStat, err := disk.Usage("/"); err == nil {
		m.diskTotal.Store(int64(diskStat.Total))
		m.diskUsed.Store(int64(diskStat.Used))
		m.diskAvailable.Store(int64(diskStat.Free))
		m.diskPercent.Store(diskStat.UsedPercent)
	}
}

// collectNetworkMetrics 收集网络指标
func (m *EnhancedSystemMonitor) collectNetworkMetrics() {
	// 网络IO统计
	if netStats, err := net.IOCounters(false); err == nil && len(netStats) > 0 {
		m.netStatsMu.Lock()
		if len(m.lastNetStats) > 0 {
			// 计算增量
			deltaIn := int64(netStats[0].BytesRecv - m.lastNetStats[0].BytesRecv)
			deltaOut := int64(netStats[0].BytesSent - m.lastNetStats[0].BytesSent)
			deltaPacketsIn := int64(netStats[0].PacketsRecv - m.lastNetStats[0].PacketsRecv)
			deltaPacketsOut := int64(netStats[0].PacketsSent - m.lastNetStats[0].PacketsSent)

			m.netBytesIn.Add(deltaIn)
			m.netBytesOut.Add(deltaOut)
			m.netPacketsIn.Add(deltaPacketsIn)
			m.netPacketsOut.Add(deltaPacketsOut)
		}
		m.lastNetStats = netStats
		m.netStatsMu.Unlock()
	}

	// 网络接口信息
	if interfaces, err := net.Interfaces(); err == nil {
		m.interfacesMu.Lock()
		m.interfaces = make([]EnhancedNetworkInterface, 0, len(interfaces))

		for _, iface := range interfaces {
			// 获取接口统计
			if stats, err := net.IOCounters(true); err == nil {
				for _, stat := range stats {
					if stat.Name == iface.Name {
						// 获取接口地址
						var ipAddr string
						if addrs := iface.Addrs; len(addrs) > 0 {
							ipAddr = addrs[0].Addr
						}

						netIface := EnhancedNetworkInterface{
							Name:      iface.Name,
							IP:        ipAddr,
							MAC:       iface.HardwareAddr,
							Status:    getInterfaceStatus(iface.Flags),
							Speed:     0, // TODO: 获取接口速度
							RxBytes:   int64(stat.BytesRecv),
							TxBytes:   int64(stat.BytesSent),
							RxPackets: int64(stat.PacketsRecv),
							TxPackets: int64(stat.PacketsSent),
							RxErrors:  int64(stat.Errin),
							TxErrors:  int64(stat.Errout),
						}

						m.interfaces = append(m.interfaces, netIface)
						break
					}
				}
			}
		}
		m.interfacesMu.Unlock()
	}
}

// collectConnectionMetrics 收集连接指标
func (m *EnhancedSystemMonitor) collectConnectionMetrics() {
	// TCP连接数
	if tcpConns, err := net.Connections("tcp"); err == nil {
		m.tcpConns.Store(int32(len(tcpConns)))
	}

	// UDP连接数
	if udpConns, err := net.Connections("udp"); err == nil {
		m.udpConns.Store(int32(len(udpConns)))
	}

	// 总连接数
	total := m.tcpConns.Load() + m.udpConns.Load()
	m.totalConns.Store(total)
}

// getInterfaceStatus 获取接口状态
func getInterfaceStatus(flags []string) string {
	for _, flag := range flags {
		if flag == "up" {
			return "up"
		}
	}
	return "down"
}

// RecordResponseTime 记录响应时间
func (m *EnhancedSystemMonitor) RecordResponseTime(responseTime float64) {
	m.requestCount.Add(1)

	// 更新平均响应时间
	currentAvg := m.responseTime.Load().(float64)
	count := float64(m.requestCount.Load())
	newAvg := (currentAvg*(count-1) + responseTime) / count
	m.responseTime.Store(newAvg)

	// 更新最大响应时间
	currentMax := m.maxRespTime.Load().(float64)
	if responseTime > currentMax {
		m.maxRespTime.Store(responseTime)
	}

	// 更新最小响应时间
	currentMin := m.minRespTime.Load().(float64)
	if responseTime < currentMin {
		m.minRespTime.Store(responseTime)
	}
}

// RecordError 记录错误
func (m *EnhancedSystemMonitor) RecordError() {
	m.errorCount.Add(1)
}

// GetFullSnapshot 获取完整监控快照
func (m *EnhancedSystemMonitor) GetFullSnapshot() *EnhancedMonitorSnapshot {
	m.interfacesMu.RLock()
	interfacesCopy := make([]EnhancedNetworkInterface, len(m.interfaces))
	copy(interfacesCopy, m.interfaces)
	m.interfacesMu.RUnlock()

	// 获取系统启动时间
	bootTime := time.Time{}
	if hostInfo, err := host.Info(); err == nil {
		bootTime = time.Unix(int64(hostInfo.BootTime), 0)
	}

	snapshot := &EnhancedMonitorSnapshot{
		// 时间信息
		Timestamp: time.Now(),
		BootTime:  bootTime,
		Uptime:    int64(time.Since(m.startTime).Seconds()),

		// CPU信息
		CPUUsage:   m.getCPUUsage(),
		CPULoad1m:  m.getCPULoad1(),
		CPULoad5m:  m.getCPULoad5(),
		CPULoad15m: m.getCPULoad15(),
		CPUCores:   int(m.cpuCores.Load()),

		// 内存信息
		MemoryTotal:     m.memTotal.Load(),
		MemoryUsed:      m.memUsed.Load(),
		MemoryAvailable: m.memAvailable.Load(),
		MemoryPercent:   m.getMemoryPercent(),

		// 磁盘信息
		DiskTotal:     m.diskTotal.Load(),
		DiskUsed:      m.diskUsed.Load(),
		DiskAvailable: m.diskAvailable.Load(),
		DiskPercent:   m.getDiskPercent(),

		// 网络信息
		NetBytesIn:    m.netBytesIn.Load(),
		NetBytesOut:   m.netBytesOut.Load(),
		NetPacketsIn:  m.netPacketsIn.Load(),
		NetPacketsOut: m.netPacketsOut.Load(),
		NetErrors:     m.netErrors.Load(),
		Interfaces:    interfacesCopy,

		// 连接信息
		TCPConnections:   int(m.tcpConns.Load()),
		UDPConnections:   int(m.udpConns.Load()),
		TotalConnections: int(m.totalConns.Load()),

		// 性能指标
		AvgResponseTime: m.getAvgResponseTime(),
		MaxResponseTime: m.getMaxResponseTime(),
		MinResponseTime: m.getMinResponseTime(),
		RequestCount:    m.requestCount.Load(),
		ErrorCount:      m.errorCount.Load(),

		// 应用信息
		GoRoutines:   int(m.goRoutines.Load()),
		GoVersion:    m.goVersion,
		AppVersion:   m.appVersion,
		OSInfo:       m.osInfo,
		Architecture: m.architecture,
		HostName:     m.hostName,
	}

	return snapshot
}

// 原子值读取辅助方法
func (m *EnhancedSystemMonitor) getCPUUsage() float64 {
	if val := m.cpuUsage.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

func (m *EnhancedSystemMonitor) getCPULoad1() float64 {
	if val := m.cpuLoad1.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

func (m *EnhancedSystemMonitor) getCPULoad5() float64 {
	if val := m.cpuLoad5.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

func (m *EnhancedSystemMonitor) getCPULoad15() float64 {
	if val := m.cpuLoad15.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

func (m *EnhancedSystemMonitor) getMemoryPercent() float64 {
	if val := m.memPercent.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

func (m *EnhancedSystemMonitor) getDiskPercent() float64 {
	if val := m.diskPercent.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

func (m *EnhancedSystemMonitor) getAvgResponseTime() float64 {
	if val := m.responseTime.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

func (m *EnhancedSystemMonitor) getMaxResponseTime() float64 {
	if val := m.maxRespTime.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

func (m *EnhancedSystemMonitor) getMinResponseTime() float64 {
	if val := m.minRespTime.Load(); val != nil {
		return val.(float64)
	}
	return 0.0
}

// EnhancedMonitorSnapshot 增强版监控快照
type EnhancedMonitorSnapshot struct {
	// 时间信息
	Timestamp time.Time `json:"timestamp"`
	BootTime  time.Time `json:"boot_time"`
	Uptime    int64     `json:"uptime"` // 应用运行时间（秒）

	// CPU信息
	CPUUsage   float64 `json:"cpu_usage"`
	CPULoad1m  float64 `json:"cpu_load_1m"`
	CPULoad5m  float64 `json:"cpu_load_5m"`
	CPULoad15m float64 `json:"cpu_load_15m"`
	CPUCores   int     `json:"cpu_cores"`

	// 内存信息
	MemoryTotal     int64   `json:"memory_total"`
	MemoryUsed      int64   `json:"memory_used"`
	MemoryAvailable int64   `json:"memory_available"`
	MemoryPercent   float64 `json:"memory_percent"`

	// 磁盘信息
	DiskTotal     int64   `json:"disk_total"`
	DiskUsed      int64   `json:"disk_used"`
	DiskAvailable int64   `json:"disk_available"`
	DiskPercent   float64 `json:"disk_percent"`

	// 网络信息
	NetBytesIn    int64                      `json:"net_bytes_in"`
	NetBytesOut   int64                      `json:"net_bytes_out"`
	NetPacketsIn  int64                      `json:"net_packets_in"`
	NetPacketsOut int64                      `json:"net_packets_out"`
	NetErrors     int64                      `json:"net_errors"`
	Interfaces    []EnhancedNetworkInterface `json:"interfaces"`

	// 连接信息
	TCPConnections   int `json:"tcp_connections"`
	UDPConnections   int `json:"udp_connections"`
	TotalConnections int `json:"total_connections"`

	// 性能指标
	AvgResponseTime float64 `json:"avg_response_time"`
	MaxResponseTime float64 `json:"max_response_time"`
	MinResponseTime float64 `json:"min_response_time"`
	RequestCount    int64   `json:"request_count"`
	ErrorCount      int64   `json:"error_count"`
	ErrorRate       float64 `json:"error_rate"` // 错误率

	// 应用信息
	GoRoutines   int    `json:"go_routines"`
	GoVersion    string `json:"go_version"`
	AppVersion   string `json:"app_version"`
	OSInfo       string `json:"os_info"`
	Architecture string `json:"architecture"`
	HostName     string `json:"host_name"`
}

// CalculateErrorRate 计算错误率
func (s *EnhancedMonitorSnapshot) CalculateErrorRate() {
	if s.RequestCount > 0 {
		s.ErrorRate = (float64(s.ErrorCount) / float64(s.RequestCount)) * 100.0
	} else {
		s.ErrorRate = 0.0
	}
}

// ToJSON 转换为JSON字符串
func (s *EnhancedMonitorSnapshot) ToJSON() (string, error) {
	s.CalculateErrorRate()
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
