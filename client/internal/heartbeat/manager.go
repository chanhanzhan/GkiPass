package heartbeat

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Config 心跳配置
type Config struct {
	// 基本配置
	NodeID     string        `json:"node_id"`
	Interval   time.Duration `json:"interval"`    // 心跳间隔
	Timeout    time.Duration `json:"timeout"`     // 超时时间
	MaxRetries int           `json:"max_retries"` // 最大重试次数

	// 探测配置
	ProbeEnabled  bool          `json:"probe_enabled"`  // 是否启用探测
	ProbeInterval time.Duration `json:"probe_interval"` // 探测间隔
	ProbeSizes    []int         `json:"probe_sizes"`    // 探测载荷大小

	// 统计配置
	StatisticsWindow int           `json:"statistics_window"` // 统计窗口大小
	QualityInterval  time.Duration `json:"quality_interval"`  // 质量计算间隔

	// 健康检查配置
	HealthCheckEnabled bool `json:"health_check_enabled"`
	UnhealthyThreshold int  `json:"unhealthy_threshold"` // 连续失败阈值
}

// DefaultConfig 默认配置
func DefaultConfig(nodeID string) *Config {
	return &Config{
		NodeID:             nodeID,
		Interval:           10 * time.Second,
		Timeout:            5 * time.Second,
		MaxRetries:         3,
		ProbeEnabled:       true,
		ProbeInterval:      30 * time.Second,
		ProbeSizes:         []int{64, 256, 1024},
		StatisticsWindow:   100,
		QualityInterval:    60 * time.Second,
		HealthCheckEnabled: true,
		UnhealthyThreshold: 5,
	}
}

// Connection 连接接口
type Connection interface {
	Send(data []byte) error
	Receive() ([]byte, error)
	RemoteAddr() net.Addr
	Close() error
}

// Manager 心跳管理器
type Manager struct {
	config      *Config
	connections map[string]*ConnectionHandler // 连接ID -> 处理器
	mutex       sync.RWMutex

	// 统计
	totalPingsSent      atomic.Int64
	totalPongsReceived  atomic.Int64
	totalProbesSent     atomic.Int64
	totalProbesReceived atomic.Int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// NewManager 创建心跳管理器
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig("unknown")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		config:      config,
		connections: make(map[string]*ConnectionHandler),
		ctx:         ctx,
		cancel:      cancel,
		logger:      zap.L().Named("heartbeat-manager"),
	}
}

// Start 启动心跳管理器
func (m *Manager) Start() error {
	m.logger.Info("启动心跳管理器",
		zap.String("node_id", m.config.NodeID),
		zap.Duration("interval", m.config.Interval),
		zap.Duration("timeout", m.config.Timeout))

	return nil
}

// Stop 停止心跳管理器
func (m *Manager) Stop() error {
	m.logger.Info("停止心跳管理器")

	// 取消上下文
	if m.cancel != nil {
		m.cancel()
	}

	// 停止所有连接处理器
	m.mutex.Lock()
	handlers := make([]*ConnectionHandler, 0, len(m.connections))
	for _, handler := range m.connections {
		handlers = append(handlers, handler)
	}
	m.mutex.Unlock()

	for _, handler := range handlers {
		handler.Stop()
	}

	// 等待所有协程结束
	m.wg.Wait()

	m.logger.Info("心跳管理器已停止")
	return nil
}

// AddConnection 添加连接
func (m *Manager) AddConnection(connectionID string, conn Connection) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.connections[connectionID]; exists {
		return fmt.Errorf("连接已存在: %s", connectionID)
	}

	handler := NewConnectionHandler(connectionID, conn, m.config, m.ctx)
	m.connections[connectionID] = handler

	// 启动连接处理器
	if err := handler.Start(); err != nil {
		delete(m.connections, connectionID)
		return fmt.Errorf("启动连接处理器失败: %w", err)
	}

	m.logger.Info("添加心跳连接",
		zap.String("connection_id", connectionID),
		zap.String("remote_addr", conn.RemoteAddr().String()))

	return nil
}

// RemoveConnection 移除连接
func (m *Manager) RemoveConnection(connectionID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	handler, exists := m.connections[connectionID]
	if !exists {
		return fmt.Errorf("连接不存在: %s", connectionID)
	}

	handler.Stop()
	delete(m.connections, connectionID)

	m.logger.Info("移除心跳连接", zap.String("connection_id", connectionID))
	return nil
}

// GetConnectionHealth 获取连接健康状态
func (m *Manager) GetConnectionHealth(connectionID string) (*HealthStatus, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	handler, exists := m.connections[connectionID]
	if !exists {
		return nil, fmt.Errorf("连接不存在: %s", connectionID)
	}

	return handler.GetHealthStatus(), nil
}

// GetConnectionQuality 获取连接网络质量
func (m *Manager) GetConnectionQuality(connectionID string) (*NetworkQuality, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	handler, exists := m.connections[connectionID]
	if !exists {
		return nil, fmt.Errorf("连接不存在: %s", connectionID)
	}

	return handler.GetNetworkQuality(), nil
}

// GetAllConnectionsHealth 获取所有连接健康状态
func (m *Manager) GetAllConnectionsHealth() map[string]*HealthStatus {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]*HealthStatus)
	for id, handler := range m.connections {
		result[id] = handler.GetHealthStatus()
	}

	return result
}

// GetStats 获取管理器统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	connectionCount := len(m.connections)
	connectionStats := make(map[string]interface{})

	for id, handler := range m.connections {
		connectionStats[id] = handler.GetStats()
	}
	m.mutex.RUnlock()

	return map[string]interface{}{
		"connection_count":      connectionCount,
		"total_pings_sent":      m.totalPingsSent.Load(),
		"total_pongs_received":  m.totalPongsReceived.Load(),
		"total_probes_sent":     m.totalProbesSent.Load(),
		"total_probes_received": m.totalProbesReceived.Load(),
		"connections":           connectionStats,
		"config": map[string]interface{}{
			"node_id":              m.config.NodeID,
			"interval":             m.config.Interval.String(),
			"timeout":              m.config.Timeout.String(),
			"max_retries":          m.config.MaxRetries,
			"probe_enabled":        m.config.ProbeEnabled,
			"probe_interval":       m.config.ProbeInterval.String(),
			"statistics_window":    m.config.StatisticsWindow,
			"quality_interval":     m.config.QualityInterval.String(),
			"health_check_enabled": m.config.HealthCheckEnabled,
			"unhealthy_threshold":  m.config.UnhealthyThreshold,
		},
	}
}

// ConnectionHandler 连接处理器
type ConnectionHandler struct {
	connectionID string
	connection   Connection
	config       *Config

	// 序列号
	pingSequence  atomic.Uint64
	probeSequence atomic.Uint64

	// 待响应的请求
	pendingPings  map[string]*PendingRequest
	pendingProbes map[string]*PendingRequest
	pendingMutex  sync.RWMutex

	// 统计
	statistics  *StatisticsWindow
	qualityCalc *QualityCalculator
	healthEval  *HealthEvaluator

	// 状态
	lastHeartbeat    time.Time
	consecutiveFails atomic.Int32
	networkQuality   *NetworkQuality
	healthStatus     *HealthStatus
	statusMutex      sync.RWMutex

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// PendingRequest 待响应请求
type PendingRequest struct {
	Message   *Message
	Timestamp time.Time
	Timeout   time.Duration
}

// NewConnectionHandler 创建连接处理器
func NewConnectionHandler(connectionID string, conn Connection, config *Config, parentCtx context.Context) *ConnectionHandler {
	ctx, cancel := context.WithCancel(parentCtx)

	handler := &ConnectionHandler{
		connectionID:  connectionID,
		connection:    conn,
		config:        config,
		pendingPings:  make(map[string]*PendingRequest),
		pendingProbes: make(map[string]*PendingRequest),
		statistics:    NewStatisticsWindow(config.StatisticsWindow),
		qualityCalc:   NewQualityCalculator(),
		healthEval:    NewHealthEvaluator(),
		lastHeartbeat: time.Now(),
		ctx:           ctx,
		cancel:        cancel,
		logger:        zap.L().Named(fmt.Sprintf("heartbeat-handler-%s", connectionID)),
	}

	// 初始化状态
	handler.networkQuality = &NetworkQuality{
		Score:     100.0,
		Grade:     QualityGradeExcellent,
		Timestamp: time.Now(),
	}
	handler.healthStatus = NewHealthStatus()

	return handler
}

// Start 启动连接处理器
func (ch *ConnectionHandler) Start() error {
	ch.logger.Info("启动连接处理器",
		zap.String("connection_id", ch.connectionID),
		zap.String("remote_addr", ch.connection.RemoteAddr().String()))

	// 启动心跳发送协程
	ch.wg.Add(1)
	go func() {
		defer ch.wg.Done()
		ch.heartbeatLoop()
	}()

	// 启动消息接收协程
	ch.wg.Add(1)
	go func() {
		defer ch.wg.Done()
		ch.receiveLoop()
	}()

	// 启动探测协程
	if ch.config.ProbeEnabled {
		ch.wg.Add(1)
		go func() {
			defer ch.wg.Done()
			ch.probeLoop()
		}()
	}

	// 启动质量评估协程
	ch.wg.Add(1)
	go func() {
		defer ch.wg.Done()
		ch.qualityLoop()
	}()

	// 启动超时清理协程
	ch.wg.Add(1)
	go func() {
		defer ch.wg.Done()
		ch.timeoutCleanupLoop()
	}()

	return nil
}

// Stop 停止连接处理器
func (ch *ConnectionHandler) Stop() {
	ch.logger.Info("停止连接处理器", zap.String("connection_id", ch.connectionID))

	if ch.cancel != nil {
		ch.cancel()
	}

	ch.wg.Wait()

	// 关闭连接
	if ch.connection != nil {
		ch.connection.Close()
	}

	ch.logger.Info("连接处理器已停止", zap.String("connection_id", ch.connectionID))
}

// heartbeatLoop 心跳循环
func (ch *ConnectionHandler) heartbeatLoop() {
	ticker := time.NewTicker(ch.config.Interval)
	defer ticker.Stop()

	ch.logger.Debug("启动心跳循环")

	for {
		select {
		case <-ch.ctx.Done():
			ch.logger.Debug("停止心跳循环")
			return
		case <-ticker.C:
			ch.sendPing()
		}
	}
}

// receiveLoop 接收循环
func (ch *ConnectionHandler) receiveLoop() {
	ch.logger.Debug("启动接收循环")

	for {
		select {
		case <-ch.ctx.Done():
			ch.logger.Debug("停止接收循环")
			return
		default:
		}

		// 接收消息
		data, err := ch.connection.Receive()
		if err != nil {
			if ch.ctx.Err() != nil {
				return // 正常关闭
			}
			ch.logger.Error("接收消息失败", zap.Error(err))
			ch.consecutiveFails.Add(1)
			time.Sleep(time.Second) // 避免快速重试
			continue
		}

		// 解码消息
		message, err := DecodeMessage(data)
		if err != nil {
			ch.logger.Error("解码消息失败", zap.Error(err))
			continue
		}

		// 处理消息
		ch.handleMessage(message)
	}
}

// probeLoop 探测循环
func (ch *ConnectionHandler) probeLoop() {
	ticker := time.NewTicker(ch.config.ProbeInterval)
	defer ticker.Stop()

	ch.logger.Debug("启动探测循环")

	for {
		select {
		case <-ch.ctx.Done():
			ch.logger.Debug("停止探测循环")
			return
		case <-ticker.C:
			ch.sendProbes()
		}
	}
}

// sendPing 发送Ping消息
func (ch *ConnectionHandler) sendPing() {
	sequence := ch.pingSequence.Add(1)
	ping := NewPingMessage(ch.config.NodeID, sequence)

	// 编码消息
	data, err := ping.Encode()
	if err != nil {
		ch.logger.Error("编码Ping消息失败", zap.Error(err))
		return
	}

	// 发送消息
	if err := ch.connection.Send(data); err != nil {
		ch.logger.Error("发送Ping消息失败", zap.Error(err))
		ch.consecutiveFails.Add(1)
		return
	}

	// 记录待响应请求
	ch.pendingMutex.Lock()
	ch.pendingPings[ping.ID] = &PendingRequest{
		Message:   ping,
		Timestamp: time.Now(),
		Timeout:   ch.config.Timeout,
	}
	ch.pendingMutex.Unlock()

	ch.logger.Debug("发送Ping消息",
		zap.String("ping_id", ping.ID),
		zap.Uint64("sequence", sequence))
}

// sendProbes 发送探测消息
func (ch *ConnectionHandler) sendProbes() {
	for _, size := range ch.config.ProbeSizes {
		sequence := ch.probeSequence.Add(1)
		probe := NewProbeMessage(ch.config.NodeID, sequence, size)

		// 编码消息
		data, err := probe.Encode()
		if err != nil {
			ch.logger.Error("编码探测消息失败", zap.Error(err))
			continue
		}

		// 发送消息
		if err := ch.connection.Send(data); err != nil {
			ch.logger.Error("发送探测消息失败", zap.Error(err))
			continue
		}

		// 记录待响应请求
		ch.pendingMutex.Lock()
		ch.pendingProbes[probe.ID] = &PendingRequest{
			Message:   probe,
			Timestamp: time.Now(),
			Timeout:   ch.config.Timeout,
		}
		ch.pendingMutex.Unlock()

		ch.logger.Debug("发送探测消息",
			zap.String("probe_id", probe.ID),
			zap.Uint64("sequence", sequence),
			zap.Int("size", size))
	}
}

// handleMessage 处理接收到的消息
func (ch *ConnectionHandler) handleMessage(message *Message) {
	switch message.Type {
	case MessageTypePing:
		ch.handlePing(message)
	case MessageTypePong:
		ch.handlePong(message)
	case MessageTypeProbe:
		ch.handleProbe(message)
	case MessageTypeProbeAck:
		ch.handleProbeAck(message)
	default:
		ch.logger.Warn("未知消息类型",
			zap.String("type", message.Type.String()),
			zap.String("message_id", message.ID))
	}
}

// handlePing 处理Ping消息
func (ch *ConnectionHandler) handlePing(ping *Message) {
	ch.logger.Debug("收到Ping消息",
		zap.String("ping_id", ping.ID),
		zap.Uint64("sequence", ping.Sequence))

	// 发送Pong响应
	pong := NewPongMessage(ch.config.NodeID, ping)
	data, err := pong.Encode()
	if err != nil {
		ch.logger.Error("编码Pong消息失败", zap.Error(err))
		return
	}

	if err := ch.connection.Send(data); err != nil {
		ch.logger.Error("发送Pong消息失败", zap.Error(err))
		return
	}

	ch.logger.Debug("发送Pong响应", zap.String("pong_id", pong.ID))
}

// handlePong 处理Pong消息
func (ch *ConnectionHandler) handlePong(pong *Message) {
	pingID, ok := pong.Metadata["ping_id"].(string)
	if !ok {
		ch.logger.Error("Pong消息缺少ping_id")
		return
	}

	// 查找对应的Ping请求
	ch.pendingMutex.Lock()
	pendingReq, exists := ch.pendingPings[pingID]
	if exists {
		delete(ch.pendingPings, pingID)
	}
	ch.pendingMutex.Unlock()

	if !exists {
		ch.logger.Warn("收到未知Pong响应", zap.String("ping_id", pingID))
		return
	}

	// 计算RTT
	rtt := time.Since(pendingReq.Timestamp)
	probeSize := 0
	if size, ok := pendingReq.Message.Metadata["probe_size"].(int); ok {
		probeSize = size
	}

	// 记录测量结果
	measurement := NewRTTMeasurement(pingID, pendingReq.Message.Sequence, rtt, probeSize, true)
	ch.statistics.AddMeasurement(*measurement)

	// 更新状态
	ch.statusMutex.Lock()
	ch.lastHeartbeat = time.Now()
	ch.consecutiveFails.Store(0) // 重置连续失败计数
	ch.statusMutex.Unlock()

	ch.logger.Debug("收到Pong响应",
		zap.String("ping_id", pingID),
		zap.Duration("rtt", rtt),
		zap.Uint64("sequence", pendingReq.Message.Sequence))
}

// handleProbe 处理探测消息
func (ch *ConnectionHandler) handleProbe(probe *Message) {
	ch.logger.Debug("收到探测消息",
		zap.String("probe_id", probe.ID),
		zap.Int("size", len(probe.Payload)))

	// 发送探测应答
	ack := NewProbeAckMessage(ch.config.NodeID, probe)
	data, err := ack.Encode()
	if err != nil {
		ch.logger.Error("编码探测应答失败", zap.Error(err))
		return
	}

	if err := ch.connection.Send(data); err != nil {
		ch.logger.Error("发送探测应答失败", zap.Error(err))
		return
	}

	ch.logger.Debug("发送探测应答", zap.String("ack_id", ack.ID))
}

// handleProbeAck 处理探测应答
func (ch *ConnectionHandler) handleProbeAck(ack *Message) {
	probeID, ok := ack.Metadata["probe_id"].(string)
	if !ok {
		ch.logger.Error("探测应答缺少probe_id")
		return
	}

	// 查找对应的探测请求
	ch.pendingMutex.Lock()
	pendingReq, exists := ch.pendingProbes[probeID]
	if exists {
		delete(ch.pendingProbes, probeID)
	}
	ch.pendingMutex.Unlock()

	if !exists {
		ch.logger.Warn("收到未知探测应答", zap.String("probe_id", probeID))
		return
	}

	// 计算RTT
	rtt := time.Since(pendingReq.Timestamp)
	probeSize := len(pendingReq.Message.Payload)

	// 记录测量结果
	measurement := NewRTTMeasurement(probeID, pendingReq.Message.Sequence, rtt, probeSize, true)
	ch.statistics.AddMeasurement(*measurement)

	ch.logger.Debug("收到探测应答",
		zap.String("probe_id", probeID),
		zap.Duration("rtt", rtt),
		zap.Int("probe_size", probeSize))
}

// qualityLoop 质量评估循环
func (ch *ConnectionHandler) qualityLoop() {
	ticker := time.NewTicker(ch.config.QualityInterval)
	defer ticker.Stop()

	ch.logger.Debug("启动质量评估循环")

	for {
		select {
		case <-ch.ctx.Done():
			ch.logger.Debug("停止质量评估循环")
			return
		case <-ticker.C:
			ch.evaluateQuality()
		}
	}
}

// evaluateQuality 评估网络质量
func (ch *ConnectionHandler) evaluateQuality() {
	rttStats := ch.statistics.GetRTTStatistics()
	lossStats := ch.statistics.GetPacketLossStatistics()
	jitterStats := ch.statistics.GetJitterStatistics()

	// 计算网络质量
	quality := ch.qualityCalc.CalculateNetworkQuality(
		rttStats, lossStats, jitterStats, ch.config.QualityInterval)

	// 评估健康状态
	health := ch.healthEval.EvaluateHealth(
		quality,
		ch.lastHeartbeat,
		int(ch.consecutiveFails.Load()),
		ch.config.Interval,
	)

	// 更新状态
	ch.statusMutex.Lock()
	ch.networkQuality = quality
	ch.healthStatus = health
	ch.statusMutex.Unlock()

	ch.logger.Debug("评估网络质量",
		zap.Float64("quality_score", quality.Score),
		zap.String("quality_grade", quality.Grade.String()),
		zap.Float64("health_score", health.HealthScore),
		zap.Bool("is_healthy", health.IsHealthy))
}

// timeoutCleanupLoop 超时清理循环
func (ch *ConnectionHandler) timeoutCleanupLoop() {
	ticker := time.NewTicker(ch.config.Timeout)
	defer ticker.Stop()

	ch.logger.Debug("启动超时清理循环")

	for {
		select {
		case <-ch.ctx.Done():
			ch.logger.Debug("停止超时清理循环")
			return
		case <-ticker.C:
			ch.cleanupTimeouts()
		}
	}
}

// cleanupTimeouts 清理超时请求
func (ch *ConnectionHandler) cleanupTimeouts() {
	now := time.Now()

	ch.pendingMutex.Lock()
	defer ch.pendingMutex.Unlock()

	// 清理超时的Ping请求
	for id, req := range ch.pendingPings {
		if now.Sub(req.Timestamp) > req.Timeout {
			// 记录超时测量结果
			measurement := NewRTTMeasurement(id, req.Message.Sequence, 0, 0, false)
			ch.statistics.AddMeasurement(*measurement)

			delete(ch.pendingPings, id)
			ch.consecutiveFails.Add(1)

			ch.logger.Debug("Ping请求超时",
				zap.String("ping_id", id),
				zap.Duration("elapsed", now.Sub(req.Timestamp)))
		}
	}

	// 清理超时的探测请求
	for id, req := range ch.pendingProbes {
		if now.Sub(req.Timestamp) > req.Timeout {
			// 记录超时测量结果
			measurement := NewRTTMeasurement(id, req.Message.Sequence, 0, len(req.Message.Payload), false)
			ch.statistics.AddMeasurement(*measurement)

			delete(ch.pendingProbes, id)

			ch.logger.Debug("探测请求超时",
				zap.String("probe_id", id),
				zap.Duration("elapsed", now.Sub(req.Timestamp)))
		}
	}
}

// GetNetworkQuality 获取网络质量
func (ch *ConnectionHandler) GetNetworkQuality() *NetworkQuality {
	ch.statusMutex.RLock()
	defer ch.statusMutex.RUnlock()

	return ch.networkQuality
}

// GetHealthStatus 获取健康状态
func (ch *ConnectionHandler) GetHealthStatus() *HealthStatus {
	ch.statusMutex.RLock()
	defer ch.statusMutex.RUnlock()

	return ch.healthStatus
}

// GetStats 获取连接统计信息
func (ch *ConnectionHandler) GetStats() map[string]interface{} {
	ch.pendingMutex.RLock()
	pendingPings := len(ch.pendingPings)
	pendingProbes := len(ch.pendingProbes)
	ch.pendingMutex.RUnlock()

	ch.statusMutex.RLock()
	lastHeartbeat := ch.lastHeartbeat
	consecutiveFails := ch.consecutiveFails.Load()
	networkQuality := ch.networkQuality
	healthStatus := ch.healthStatus
	ch.statusMutex.RUnlock()

	return map[string]interface{}{
		"connection_id":     ch.connectionID,
		"remote_addr":       ch.connection.RemoteAddr().String(),
		"last_heartbeat":    lastHeartbeat,
		"consecutive_fails": consecutiveFails,
		"pending_pings":     pendingPings,
		"pending_probes":    pendingProbes,
		"ping_sequence":     ch.pingSequence.Load(),
		"probe_sequence":    ch.probeSequence.Load(),
		"statistics_window": ch.statistics.GetSize(),
		"network_quality":   networkQuality,
		"health_status":     healthStatus,
	}
}





