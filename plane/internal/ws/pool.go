package ws

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// ConnectionPool WebSocket 连接池
type ConnectionPool struct {
	connections    map[string]*EnhancedNodeConnection
	maxConnections int
	currentCount   atomic.Int32
	mu             sync.RWMutex

	// 统计信息
	stats PoolStats

	// 清理配置
	idleTimeout     time.Duration
	cleanupInterval time.Duration
	stopChan        chan struct{}
}

// PoolStats 连接池统计
type PoolStats struct {
	TotalConnections    atomic.Int64
	ActiveConnections   atomic.Int32
	IdleConnections     atomic.Int32
	RejectedConnections atomic.Int64
	TimeoutConnections  atomic.Int64
	ErrorConnections    atomic.Int64
}

// EnhancedNodeConnection 增强的节点连接
type EnhancedNodeConnection struct {
	*NodeConnection

	// 连接质量指标
	rtt          atomic.Int64 // 往返时延（纳秒）
	packetLoss   atomic.Int64 // 丢包数
	totalPackets atomic.Int64 // 总包数
	bytesIn      atomic.Int64
	bytesOut     atomic.Int64

	// 心跳配置
	heartbeatInterval atomic.Int64 // 心跳间隔（纳秒）
	missedHeartbeats  atomic.Int32 // 连续丢失的心跳数

	// 连接状态
	createdAt      time.Time
	lastActivityAt atomic.Value // time.Time
	idleTime       atomic.Int64 // 空闲时间（纳秒）

	// 质量评分
	qualityScore atomic.Int32 // 0-100
}

// NewConnectionPool 创建连接池
func NewConnectionPool(maxConn int) *ConnectionPool {
	pool := &ConnectionPool{
		connections:     make(map[string]*EnhancedNodeConnection),
		maxConnections:  maxConn,
		idleTimeout:     5 * time.Minute,
		cleanupInterval: 30 * time.Second,
		stopChan:        make(chan struct{}),
	}

	// 启动清理协程
	go pool.cleanupLoop()

	logger.Info("WebSocket连接池已创建",
		zap.Int("max_connections", maxConn))

	return pool
}

// Add 添加连接到池
func (cp *ConnectionPool) Add(nodeID string, conn *NodeConnection) (*EnhancedNodeConnection, error) {
	// 检查连接数限制
	if int(cp.currentCount.Load()) >= cp.maxConnections {
		cp.stats.RejectedConnections.Add(1)
		return nil, fmt.Errorf("连接池已满: %d/%d", cp.currentCount.Load(), cp.maxConnections)
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()

	// 如果已存在，先移除旧连接
	if oldConn, exists := cp.connections[nodeID]; exists {
		oldConn.Close()
		cp.currentCount.Add(-1)
	}

	// 创建增强连接
	enhanced := &EnhancedNodeConnection{
		NodeConnection:    conn,
		createdAt:         time.Now(),
		heartbeatInterval: atomic.Int64{},
	}
	enhanced.heartbeatInterval.Store(int64(30 * time.Second))
	enhanced.lastActivityAt.Store(time.Now())
	enhanced.qualityScore.Store(100) // 初始满分

	cp.connections[nodeID] = enhanced
	cp.currentCount.Add(1)
	cp.stats.TotalConnections.Add(1)
	cp.stats.ActiveConnections.Store(cp.currentCount.Load())

	logger.Debug("连接已添加到池",
		zap.String("nodeID", nodeID),
		zap.Int32("pool_size", cp.currentCount.Load()))

	return enhanced, nil
}

// Remove 从池中移除连接
func (cp *ConnectionPool) Remove(nodeID string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn, exists := cp.connections[nodeID]; exists {
		conn.Close()
		delete(cp.connections, nodeID)
		cp.currentCount.Add(-1)
		cp.stats.ActiveConnections.Store(cp.currentCount.Load())

		logger.Debug("连接已从池中移除",
			zap.String("nodeID", nodeID),
			zap.Int32("pool_size", cp.currentCount.Load()))
	}
}

// Get 获取连接
func (cp *ConnectionPool) Get(nodeID string) (*EnhancedNodeConnection, bool) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	conn, exists := cp.connections[nodeID]
	if exists && conn != nil {
		// 更新活动时间
		conn.RecordActivity()
	}
	return conn, exists
}

// GetAll 获取所有连接
func (cp *ConnectionPool) GetAll() []*EnhancedNodeConnection {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	conns := make([]*EnhancedNodeConnection, 0, len(cp.connections))
	for _, conn := range cp.connections {
		conns = append(conns, conn)
	}
	return conns
}

// GetStats 获取统计信息
func (cp *ConnectionPool) GetStats() PoolStatsSnapshot {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	idleCount := int32(0)
	for _, conn := range cp.connections {
		if conn.IsIdle(30 * time.Second) {
			idleCount++
		}
	}
	cp.stats.IdleConnections.Store(idleCount)

	return PoolStatsSnapshot{
		TotalConnections:    cp.stats.TotalConnections.Load(),
		ActiveConnections:   cp.stats.ActiveConnections.Load(),
		IdleConnections:     idleCount,
		RejectedConnections: cp.stats.RejectedConnections.Load(),
		TimeoutConnections:  cp.stats.TimeoutConnections.Load(),
		ErrorConnections:    cp.stats.ErrorConnections.Load(),
		MaxConnections:      int32(cp.maxConnections),
	}
}

// PoolStatsSnapshot 连接池统计快照
type PoolStatsSnapshot struct {
	TotalConnections    int64
	ActiveConnections   int32
	IdleConnections     int32
	RejectedConnections int64
	TimeoutConnections  int64
	ErrorConnections    int64
	MaxConnections      int32
}

// cleanupLoop 清理循环
func (cp *ConnectionPool) cleanupLoop() {
	ticker := time.NewTicker(cp.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cp.cleanup()
		case <-cp.stopChan:
			return
		}
	}
}

// cleanup 清理空闲和超时的连接
func (cp *ConnectionPool) cleanup() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	toRemove := make([]string, 0)

	for nodeID, conn := range cp.connections {
		// 检查空闲超时
		if conn.IsIdle(cp.idleTimeout) {
			toRemove = append(toRemove, nodeID)
			cp.stats.TimeoutConnections.Add(1)
			logger.Info("清理空闲连接",
				zap.String("nodeID", nodeID),
				zap.Duration("idle", time.Since(conn.lastActivityAt.Load().(time.Time))))
			continue
		}

		// 检查心跳超时（连续丢失5次）
		if conn.missedHeartbeats.Load() >= 5 {
			toRemove = append(toRemove, nodeID)
			cp.stats.TimeoutConnections.Add(1)
			logger.Warn("清理心跳超时连接",
				zap.String("nodeID", nodeID),
				zap.Int32("missed_heartbeats", conn.missedHeartbeats.Load()))
			continue
		}

		// 检查连接质量评分
		if conn.qualityScore.Load() < 20 {
			toRemove = append(toRemove, nodeID)
			cp.stats.ErrorConnections.Add(1)
			logger.Warn("清理低质量连接",
				zap.String("nodeID", nodeID),
				zap.Int32("quality_score", conn.qualityScore.Load()))
		}
	}

	// 移除标记的连接
	for _, nodeID := range toRemove {
		if conn, exists := cp.connections[nodeID]; exists {
			conn.Close()
			delete(cp.connections, nodeID)
			cp.currentCount.Add(-1)
		}
	}

	if len(toRemove) > 0 {
		logger.Info("连接池清理完成",
			zap.Int("cleaned", len(toRemove)),
			zap.Int32("remaining", cp.currentCount.Load()))
	}
}

// Close 关闭连接池
func (cp *ConnectionPool) Close() error {
	close(cp.stopChan)

	cp.mu.Lock()
	defer cp.mu.Unlock()

	for _, conn := range cp.connections {
		conn.Close()
	}

	cp.connections = nil
	logger.Info("连接池已关闭")
	return nil
}

// RecordActivity 记录活动
func (enc *EnhancedNodeConnection) RecordActivity() {
	enc.lastActivityAt.Store(time.Now())
	enc.idleTime.Store(0)
}

// IsIdle 检查是否空闲
func (enc *EnhancedNodeConnection) IsIdle(timeout time.Duration) bool {
	lastActivity := enc.lastActivityAt.Load().(time.Time)
	return time.Since(lastActivity) > timeout
}

// RecordRTT 记录往返时延
func (enc *EnhancedNodeConnection) RecordRTT(rtt time.Duration) {
	enc.rtt.Store(rtt.Nanoseconds())
	enc.adjustHeartbeatInterval(rtt)
	enc.updateQualityScore()
}

// RecordPacket 记录数据包
func (enc *EnhancedNodeConnection) RecordPacket(lost bool) {
	enc.totalPackets.Add(1)
	if lost {
		enc.packetLoss.Add(1)
	}
	enc.updateQualityScore()
}

// RecordTraffic 记录流量
func (enc *EnhancedNodeConnection) RecordTraffic(bytesIn, bytesOut int64) {
	enc.bytesIn.Add(bytesIn)
	enc.bytesOut.Add(bytesOut)
	enc.RecordActivity()
}

// adjustHeartbeatInterval 自适应调整心跳间隔
func (enc *EnhancedNodeConnection) adjustHeartbeatInterval(rtt time.Duration) {
	currentInterval := time.Duration(enc.heartbeatInterval.Load())

	// 根据 RTT 调整心跳间隔
	// RTT < 50ms: 30秒心跳
	// RTT 50-200ms: 20秒心跳
	// RTT > 200ms: 15秒心跳（更频繁检测不稳定连接）
	var newInterval time.Duration
	if rtt < 50*time.Millisecond {
		newInterval = 30 * time.Second
	} else if rtt < 200*time.Millisecond {
		newInterval = 20 * time.Second
	} else {
		newInterval = 15 * time.Second
	}

	if newInterval != currentInterval {
		enc.heartbeatInterval.Store(int64(newInterval))
		logger.Debug("心跳间隔已调整",
			zap.String("nodeID", enc.NodeID),
			zap.Duration("old_interval", currentInterval),
			zap.Duration("new_interval", newInterval),
			zap.Duration("rtt", rtt))
	}
}

// updateQualityScore 更新连接质量评分
func (enc *EnhancedNodeConnection) updateQualityScore() {
	score := 100

	// RTT 影响（0-30分）
	rtt := time.Duration(enc.rtt.Load())
	if rtt > 500*time.Millisecond {
		score -= 30
	} else if rtt > 200*time.Millisecond {
		score -= 20
	} else if rtt > 100*time.Millisecond {
		score -= 10
	}

	// 丢包率影响（0-40分）
	totalPkts := enc.totalPackets.Load()
	if totalPkts > 0 {
		lossRate := float64(enc.packetLoss.Load()) / float64(totalPkts)
		if lossRate > 0.1 { // >10%
			score -= 40
		} else if lossRate > 0.05 { // >5%
			score -= 25
		} else if lossRate > 0.01 { // >1%
			score -= 10
		}
	}

	// 连续丢失心跳影响（0-30分）
	missed := enc.missedHeartbeats.Load()
	if missed > 0 {
		score -= int(missed * 10)
	}

	if score < 0 {
		score = 0
	}

	enc.qualityScore.Store(int32(score))
}

// GetConnectionInfo 获取连接信息
func (enc *EnhancedNodeConnection) GetConnectionInfo() ConnectionInfo {
	lastActivity := enc.lastActivityAt.Load().(time.Time)

	totalPkts := enc.totalPackets.Load()
	lossRate := 0.0
	if totalPkts > 0 {
		lossRate = float64(enc.packetLoss.Load()) / float64(totalPkts) * 100
	}

	return ConnectionInfo{
		NodeID:            enc.NodeID,
		RTT:               time.Duration(enc.rtt.Load()),
		PacketLossRate:    lossRate,
		TotalPackets:      totalPkts,
		BytesIn:           enc.bytesIn.Load(),
		BytesOut:          enc.bytesOut.Load(),
		HeartbeatInterval: time.Duration(enc.heartbeatInterval.Load()),
		MissedHeartbeats:  enc.missedHeartbeats.Load(),
		QualityScore:      enc.qualityScore.Load(),
		CreatedAt:         enc.createdAt,
		LastActivityAt:    lastActivity,
		IdleTime:          time.Since(lastActivity),
		IsAlive:           enc.IsAlive,
	}
}

// ConnectionInfo 连接信息
type ConnectionInfo struct {
	NodeID            string
	RTT               time.Duration
	PacketLossRate    float64
	TotalPackets      int64
	BytesIn           int64
	BytesOut          int64
	HeartbeatInterval time.Duration
	MissedHeartbeats  int32
	QualityScore      int32
	CreatedAt         time.Time
	LastActivityAt    time.Time
	IdleTime          time.Duration
	IsAlive           bool
}
