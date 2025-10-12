package ports

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// PortType 端口类型
type PortType int

const (
	PortTypeTCP PortType = iota
	PortTypeUDP
	PortTypeBoth
)

// String 返回端口类型名称
func (pt PortType) String() string {
	switch pt {
	case PortTypeTCP:
		return "TCP"
	case PortTypeUDP:
		return "UDP"
	case PortTypeBoth:
		return "BOTH"
	default:
		return "UNKNOWN"
	}
}

// PortStatus 端口状态
type PortStatus int

const (
	PortStatusAvailable PortStatus = iota
	PortStatusOccupied
	PortStatusReserved
	PortStatusConflict
)

// String 返回端口状态名称
func (ps PortStatus) String() string {
	switch ps {
	case PortStatusAvailable:
		return "AVAILABLE"
	case PortStatusOccupied:
		return "OCCUPIED"
	case PortStatusReserved:
		return "RESERVED"
	case PortStatusConflict:
		return "CONFLICT"
	default:
		return "UNKNOWN"
	}
}

// PortInfo 端口信息
type PortInfo struct {
	Port        uint16                 `json:"port"`
	Type        PortType               `json:"type"`
	Status      PortStatus             `json:"status"`
	Owner       string                 `json:"owner"`      // 端口占用者
	ProcessID   int                    `json:"process_id"` // 进程ID
	ProcessName string                 `json:"process_name"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// NewPortInfo 创建端口信息
func NewPortInfo(port uint16, portType PortType, owner string) *PortInfo {
	now := time.Now()
	return &PortInfo{
		Port:      port,
		Type:      portType,
		Status:    PortStatusAvailable,
		Owner:     owner,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]interface{}),
	}
}

// PortListener 端口监听器
type PortListener struct {
	Port      uint16         `json:"port"`
	Type      PortType       `json:"type"`
	TCPConn   net.Listener   `json:"-"`
	UDPConn   *net.UDPConn   `json:"-"`
	Handler   func(net.Conn) `json:"-"`
	Owner     string         `json:"owner"`
	CreatedAt time.Time      `json:"created_at"`
	Active    atomic.Bool    `json:"-"`
}

// NewPortListener 创建端口监听器
func NewPortListener(port uint16, portType PortType, owner string, handler func(net.Conn)) *PortListener {
	return &PortListener{
		Port:      port,
		Type:      portType,
		Handler:   handler,
		Owner:     owner,
		CreatedAt: time.Now(),
	}
}

// Start 启动监听器
func (pl *PortListener) Start() error {
	if pl.Active.Load() {
		return fmt.Errorf("监听器已启动")
	}

	var err error

	// 启动TCP监听
	if pl.Type == PortTypeTCP || pl.Type == PortTypeBoth {
		addr := fmt.Sprintf(":%d", pl.Port)
		pl.TCPConn, err = net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("启动TCP监听失败: %w", err)
		}

		go pl.handleTCPConnections()
	}

	// 启动UDP监听
	if pl.Type == PortTypeUDP || pl.Type == PortTypeBoth {
		addr := fmt.Sprintf(":%d", pl.Port)
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			if pl.TCPConn != nil {
				pl.TCPConn.Close()
			}
			return fmt.Errorf("解析UDP地址失败: %w", err)
		}

		pl.UDPConn, err = net.ListenUDP("udp", udpAddr)
		if err != nil {
			if pl.TCPConn != nil {
				pl.TCPConn.Close()
			}
			return fmt.Errorf("启动UDP监听失败: %w", err)
		}

		go pl.handleUDPConnections()
	}

	pl.Active.Store(true)
	return nil
}

// Stop 停止监听器
func (pl *PortListener) Stop() error {
	if !pl.Active.Load() {
		return nil
	}

	pl.Active.Store(false)

	if pl.TCPConn != nil {
		pl.TCPConn.Close()
	}

	if pl.UDPConn != nil {
		pl.UDPConn.Close()
	}

	return nil
}

// handleTCPConnections 处理TCP连接
func (pl *PortListener) handleTCPConnections() {
	for pl.Active.Load() {
		conn, err := pl.TCPConn.Accept()
		if err != nil {
			if !pl.Active.Load() {
				return // 正常关闭
			}
			continue
		}

		if pl.Handler != nil {
			go pl.Handler(conn)
		} else {
			conn.Close()
		}
	}
}

// handleUDPConnections 处理UDP连接
func (pl *PortListener) handleUDPConnections() {
	// UDP连接处理的简化实现
	buffer := make([]byte, 65536)

	for pl.Active.Load() {
		_, _, err := pl.UDPConn.ReadFromUDP(buffer)
		if err != nil {
			if !pl.Active.Load() {
				return // 正常关闭
			}
			continue
		}

		// 这里可以添加UDP数据处理逻辑
	}
}

// ManagerConfig 端口管理器配置
type ManagerConfig struct {
	// 端口范围
	MinPort       uint16   `json:"min_port"`
	MaxPort       uint16   `json:"max_port"`
	ReservedPorts []uint16 `json:"reserved_ports"`

	// 检测配置
	ScanEnabled  bool          `json:"scan_enabled"`
	ScanInterval time.Duration `json:"scan_interval"`
	ScanTimeout  time.Duration `json:"scan_timeout"`

	// 冲突处理
	ConflictPolicy ConflictPolicy `json:"conflict_policy"`
	RetryAttempts  int            `json:"retry_attempts"`
	RetryDelay     time.Duration  `json:"retry_delay"`

	// 性能配置
	MaxListeners int `json:"max_listeners"`
}

// ConflictPolicy 冲突处理策略
type ConflictPolicy int

const (
	ConflictPolicyError ConflictPolicy = iota // 报错
	ConflictPolicyRetry                       // 重试其他端口
	ConflictPolicyForce                       // 强制占用
)

// String 返回冲突策略名称
func (cp ConflictPolicy) String() string {
	switch cp {
	case ConflictPolicyError:
		return "ERROR"
	case ConflictPolicyRetry:
		return "RETRY"
	case ConflictPolicyForce:
		return "FORCE"
	default:
		return "UNKNOWN"
	}
}

// DefaultManagerConfig 默认管理器配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		MinPort:        10000,
		MaxPort:        65535,
		ReservedPorts:  []uint16{22, 80, 443, 8080, 9090},
		ScanEnabled:    true,
		ScanInterval:   30 * time.Second,
		ScanTimeout:    5 * time.Second,
		ConflictPolicy: ConflictPolicyRetry,
		RetryAttempts:  10,
		RetryDelay:     100 * time.Millisecond,
		MaxListeners:   1000,
	}
}

// Manager 端口管理器
type Manager struct {
	config    *ManagerConfig
	ports     map[uint16]*PortInfo     // 端口信息
	listeners map[uint16]*PortListener // 活跃监听器
	mutex     sync.RWMutex

	// 统计
	totalPorts    atomic.Int64
	occupiedPorts atomic.Int64
	conflictCount atomic.Int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// NewManager 创建端口管理器
func NewManager(config *ManagerConfig) *Manager {
	if config == nil {
		config = DefaultManagerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		config:    config,
		ports:     make(map[uint16]*PortInfo),
		listeners: make(map[uint16]*PortListener),
		ctx:       ctx,
		cancel:    cancel,
		logger:    zap.L().Named("port-manager"),
	}
}

// Start 启动端口管理器
func (m *Manager) Start() error {
	m.logger.Info("启动端口管理器",
		zap.Uint16("min_port", m.config.MinPort),
		zap.Uint16("max_port", m.config.MaxPort),
		zap.Bool("scan_enabled", m.config.ScanEnabled))

	// 初始化保留端口
	m.initReservedPorts()

	// 启动端口扫描
	if m.config.ScanEnabled {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.scanLoop()
		}()
	}

	return nil
}

// Stop 停止端口管理器
func (m *Manager) Stop() error {
	m.logger.Info("停止端口管理器")

	// 取消上下文
	if m.cancel != nil {
		m.cancel()
	}

	// 停止所有监听器
	m.mutex.Lock()
	listeners := make([]*PortListener, 0, len(m.listeners))
	for _, listener := range m.listeners {
		listeners = append(listeners, listener)
	}
	m.mutex.Unlock()

	for _, listener := range listeners {
		listener.Stop()
	}

	// 等待协程结束
	m.wg.Wait()

	m.logger.Info("端口管理器已停止")
	return nil
}

// AllocatePort 分配端口
func (m *Manager) AllocatePort(portType PortType, owner string, preferredPort uint16) (uint16, error) {
	if preferredPort != 0 {
		// 尝试分配指定端口
		if port, err := m.allocateSpecificPort(preferredPort, portType, owner); err == nil {
			return port, nil
		}

		// 如果分配失败且策略是报错，则返回错误
		if m.config.ConflictPolicy == ConflictPolicyError {
			return 0, fmt.Errorf("端口 %d 不可用", preferredPort)
		}
	}

	// 自动分配端口
	return m.allocateAvailablePort(portType, owner)
}

// allocateSpecificPort 分配指定端口
func (m *Manager) allocateSpecificPort(port uint16, portType PortType, owner string) (uint16, error) {
	if !m.isPortInRange(port) {
		return 0, fmt.Errorf("端口 %d 不在允许范围内", port)
	}

	if m.isReservedPort(port) {
		return 0, fmt.Errorf("端口 %d 是保留端口", port)
	}

	// 检查端口可用性
	if !m.isPortAvailable(port, portType) {
		if m.config.ConflictPolicy == ConflictPolicyForce {
			m.logger.Warn("强制占用端口", zap.Uint16("port", port))
		} else {
			return 0, fmt.Errorf("端口 %d 不可用", port)
		}
	}

	// 分配端口
	m.mutex.Lock()
	defer m.mutex.Unlock()

	portInfo := NewPortInfo(port, portType, owner)
	portInfo.Status = PortStatusOccupied
	m.ports[port] = portInfo

	m.occupiedPorts.Add(1)

	m.logger.Info("分配端口",
		zap.Uint16("port", port),
		zap.String("type", portType.String()),
		zap.String("owner", owner))

	return port, nil
}

// allocateAvailablePort 分配可用端口
func (m *Manager) allocateAvailablePort(portType PortType, owner string) (uint16, error) {
	for attempt := 0; attempt < m.config.RetryAttempts; attempt++ {
		// 从范围内随机选择端口
		port := m.generateRandomPort()

		if m.isReservedPort(port) {
			continue
		}

		if m.isPortAvailable(port, portType) {
			allocatedPort, err := m.allocateSpecificPort(port, portType, owner)
			if err == nil {
				return allocatedPort, nil
			}
		}

		// 延迟重试
		if attempt < m.config.RetryAttempts-1 {
			time.Sleep(m.config.RetryDelay)
		}
	}

	return 0, fmt.Errorf("无法分配可用端口，已尝试 %d 次", m.config.RetryAttempts)
}

// ReleasePort 释放端口
func (m *Manager) ReleasePort(port uint16, owner string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	portInfo, exists := m.ports[port]
	if !exists {
		return fmt.Errorf("端口 %d 未被分配", port)
	}

	if portInfo.Owner != owner {
		return fmt.Errorf("端口 %d 不属于 %s", port, owner)
	}

	// 停止监听器
	if listener, exists := m.listeners[port]; exists {
		listener.Stop()
		delete(m.listeners, port)
	}

	delete(m.ports, port)
	m.occupiedPorts.Add(-1)

	m.logger.Info("释放端口",
		zap.Uint16("port", port),
		zap.String("owner", owner))

	return nil
}

// StartListener 启动端口监听
func (m *Manager) StartListener(port uint16, portType PortType, owner string, handler func(net.Conn)) error {
	if len(m.listeners) >= m.config.MaxListeners {
		return fmt.Errorf("监听器数量已达上限: %d", m.config.MaxListeners)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查端口是否已被占用
	if _, exists := m.listeners[port]; exists {
		return fmt.Errorf("端口 %d 已有监听器", port)
	}

	// 创建监听器
	listener := NewPortListener(port, portType, owner, handler)

	// 启动监听
	if err := listener.Start(); err != nil {
		return fmt.Errorf("启动监听器失败: %w", err)
	}

	m.listeners[port] = listener

	// 更新端口信息
	if portInfo, exists := m.ports[port]; exists {
		portInfo.Status = PortStatusOccupied
	} else {
		portInfo := NewPortInfo(port, portType, owner)
		portInfo.Status = PortStatusOccupied
		m.ports[port] = portInfo
		m.occupiedPorts.Add(1)
	}

	m.logger.Info("启动端口监听",
		zap.Uint16("port", port),
		zap.String("type", portType.String()),
		zap.String("owner", owner))

	return nil
}

// StopListener 停止端口监听
func (m *Manager) StopListener(port uint16, owner string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	listener, exists := m.listeners[port]
	if !exists {
		return fmt.Errorf("端口 %d 没有监听器", port)
	}

	if listener.Owner != owner {
		return fmt.Errorf("端口 %d 监听器不属于 %s", port, owner)
	}

	// 停止监听器
	if err := listener.Stop(); err != nil {
		return fmt.Errorf("停止监听器失败: %w", err)
	}

	delete(m.listeners, port)

	// 更新端口状态
	if portInfo, exists := m.ports[port]; exists {
		portInfo.Status = PortStatusAvailable
	}

	m.logger.Info("停止端口监听",
		zap.Uint16("port", port),
		zap.String("owner", owner))

	return nil
}

// isPortInRange 检查端口是否在范围内
func (m *Manager) isPortInRange(port uint16) bool {
	return port >= m.config.MinPort && port <= m.config.MaxPort
}

// isReservedPort 检查是否是保留端口
func (m *Manager) isReservedPort(port uint16) bool {
	for _, reserved := range m.config.ReservedPorts {
		if port == reserved {
			return true
		}
	}
	return false
}

// isPortAvailable 检查端口是否可用
func (m *Manager) isPortAvailable(port uint16, portType PortType) bool {
	// 检查TCP端口
	if portType == PortTypeTCP || portType == PortTypeBoth {
		if !m.checkTCPPortAvailable(port) {
			return false
		}
	}

	// 检查UDP端口
	if portType == PortTypeUDP || portType == PortTypeBoth {
		if !m.checkUDPPortAvailable(port) {
			return false
		}
	}

	return true
}

// checkTCPPortAvailable 检查TCP端口是否可用
func (m *Manager) checkTCPPortAvailable(port uint16) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// checkUDPPortAvailable 检查UDP端口是否可用
func (m *Manager) checkUDPPortAvailable(port uint16) bool {
	addr := fmt.Sprintf(":%d", port)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return false
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// generateRandomPort 生成随机端口
func (m *Manager) generateRandomPort() uint16 {
	// 简化实现：使用当前时间戳
	now := time.Now().UnixNano()
	port := uint16(now%(int64(m.config.MaxPort-m.config.MinPort)+1)) + m.config.MinPort
	return port
}

// initReservedPorts 初始化保留端口
func (m *Manager) initReservedPorts() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, port := range m.config.ReservedPorts {
		portInfo := NewPortInfo(port, PortTypeBoth, "system")
		portInfo.Status = PortStatusReserved
		m.ports[port] = portInfo
	}

	m.totalPorts.Store(int64(len(m.ports)))

	m.logger.Debug("初始化保留端口",
		zap.Int("count", len(m.config.ReservedPorts)))
}

// scanLoop 端口扫描循环
func (m *Manager) scanLoop() {
	ticker := time.NewTicker(m.config.ScanInterval)
	defer ticker.Stop()

	m.logger.Debug("启动端口扫描循环")

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Debug("停止端口扫描循环")
			return
		case <-ticker.C:
			m.scanPorts()
		}
	}
}

// scanPorts 扫描端口状态
func (m *Manager) scanPorts() {
	m.mutex.RLock()
	portsToScan := make([]uint16, 0, len(m.ports))
	for port := range m.ports {
		portsToScan = append(portsToScan, port)
	}
	m.mutex.RUnlock()

	for _, port := range portsToScan {
		m.scanPort(port)
	}
}

// scanPort 扫描单个端口
func (m *Manager) scanPort(port uint16) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	portInfo, exists := m.ports[port]
	if !exists {
		return
	}

	// 检查端口实际状态
	actuallyAvailable := m.isPortAvailable(port, portInfo.Type)

	switch portInfo.Status {
	case PortStatusOccupied:
		if actuallyAvailable {
			// 端口被标记为占用但实际可用，可能是进程退出了
			if _, hasListener := m.listeners[port]; !hasListener {
				portInfo.Status = PortStatusAvailable
				m.occupiedPorts.Add(-1)
				m.logger.Debug("端口状态更新为可用",
					zap.Uint16("port", port))
			}
		}
	case PortStatusAvailable:
		if !actuallyAvailable {
			// 端口被标记为可用但实际被占用，可能是外部进程占用
			portInfo.Status = PortStatusConflict
			m.conflictCount.Add(1)
			m.logger.Warn("发现端口冲突",
				zap.Uint16("port", port))
		}
	}

	portInfo.UpdatedAt = time.Now()
}

// GetPortInfo 获取端口信息
func (m *Manager) GetPortInfo(port uint16) (*PortInfo, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	info, exists := m.ports[port]
	if !exists {
		return nil, false
	}

	// 返回副本
	infoCopy := *info
	return &infoCopy, true
}

// GetAllPorts 获取所有端口信息
func (m *Manager) GetAllPorts() map[uint16]*PortInfo {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[uint16]*PortInfo)
	for port, info := range m.ports {
		infoCopy := *info
		result[port] = &infoCopy
	}

	return result
}

// GetAvailablePorts 获取可用端口列表
func (m *Manager) GetAvailablePorts(count int) []uint16 {
	var available []uint16

	for port := m.config.MinPort; port <= m.config.MaxPort && len(available) < count; port++ {
		if !m.isReservedPort(port) && m.isPortAvailable(port, PortTypeBoth) {
			available = append(available, port)
		}
	}

	return available
}

// GetStats 获取管理器统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	listenerCount := len(m.listeners)
	m.mutex.RUnlock()

	return map[string]interface{}{
		"total_ports":    m.totalPorts.Load(),
		"occupied_ports": m.occupiedPorts.Load(),
		"conflict_count": m.conflictCount.Load(),
		"listener_count": listenerCount,
		"config": map[string]interface{}{
			"min_port":        m.config.MinPort,
			"max_port":        m.config.MaxPort,
			"reserved_ports":  len(m.config.ReservedPorts),
			"scan_enabled":    m.config.ScanEnabled,
			"scan_interval":   m.config.ScanInterval.String(),
			"conflict_policy": m.config.ConflictPolicy.String(),
			"max_listeners":   m.config.MaxListeners,
		},
	}
}





