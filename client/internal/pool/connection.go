package pool

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"gkipass/client/internal/transport"

	"go.uber.org/zap"
)

// MonitoredConnection 被监控的连接包装器
type MonitoredConnection struct {
	net.Conn
	pooledConn *PooledConnection
	pool       *AdaptivePool
}

// NewMonitoredConnection 创建被监控的连接
func NewMonitoredConnection(pooledConn *PooledConnection, pool *AdaptivePool) *MonitoredConnection {
	return &MonitoredConnection{
		Conn:       pooledConn.conn,
		pooledConn: pooledConn,
		pool:       pool,
	}
}

// Read 读取数据并记录统计
func (m *MonitoredConnection) Read(b []byte) (int, error) {
	n, err := m.Conn.Read(b)

	// 记录统计
	if n > 0 {
		m.pooledConn.bytesIn.Add(int64(n))
		m.pooledConn.packetsRecv.Add(1)
	}

	if err != nil {
		m.pooledConn.errorCount.Add(1)
		// 可能的丢包检测
		if isNetworkError(err) {
			m.pooledConn.packetsLost.Add(1)
		}
	}

	return n, err
}

// Write 写入数据并记录统计
func (m *MonitoredConnection) Write(b []byte) (int, error) {
	start := time.Now()
	n, err := m.Conn.Write(b)
	rtt := time.Since(start) * 2 // 简化的RTT估算

	// 记录统计
	if n > 0 {
		m.pooledConn.bytesOut.Add(int64(n))
		m.pooledConn.packetsSent.Add(1)
	}

	if err != nil {
		m.pooledConn.errorCount.Add(1)
		if isNetworkError(err) {
			m.pooledConn.packetsLost.Add(1)
		}
	} else {
		// 记录RTT样本
		m.recordRTT(rtt)
	}

	m.pooledConn.requestCount.Add(1)
	return n, err
}

// Close 关闭连接时释放回池中
func (m *MonitoredConnection) Close() error {
	// 不真正关闭连接，而是释放回池中
	m.pool.ReleaseConnection(m.pooledConn)
	return nil
}

// ForceClose 强制关闭连接
func (m *MonitoredConnection) ForceClose() error {
	return m.Conn.Close()
}

// recordRTT 记录RTT样本
func (m *MonitoredConnection) recordRTT(rtt time.Duration) {
	m.pooledConn.rttMutex.Lock()
	defer m.pooledConn.rttMutex.Unlock()

	m.pooledConn.rttSamples = append(m.pooledConn.rttSamples, rtt)
	if len(m.pooledConn.rttSamples) > 20 {
		// 保持最近20个样本
		copy(m.pooledConn.rttSamples, m.pooledConn.rttSamples[1:])
		m.pooledConn.rttSamples = m.pooledConn.rttSamples[:19]
	}
}

// GetConnectionInfo 获取连接信息
func (m *MonitoredConnection) GetConnectionInfo() map[string]interface{} {
	quality := m.pool.getConnectionQuality(m.pooledConn)

	return map[string]interface{}{
		"id":           m.pooledConn.id,
		"created_at":   m.pooledConn.createdAt,
		"last_used":    m.pooledConn.lastUsed,
		"bytes_in":     m.pooledConn.bytesIn.Load(),
		"bytes_out":    m.pooledConn.bytesOut.Load(),
		"requests":     m.pooledConn.requestCount.Load(),
		"errors":       m.pooledConn.errorCount.Load(),
		"packets_sent": m.pooledConn.packetsSent.Load(),
		"packets_lost": m.pooledConn.packetsLost.Load(),
		"packets_recv": m.pooledConn.packetsRecv.Load(),
		"quality":      quality,
	}
}

// isNetworkError 判断是否为网络错误
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// 简化的网络错误检测
	netErr, ok := err.(net.Error)
	return ok && (netErr.Timeout() || netErr.Temporary())
}

// PoolManager 连接池管理器
type PoolManager struct {
	pools  map[string]*AdaptivePool
	mutex  sync.RWMutex
	logger *zap.Logger
	config *PoolConfig

	// 动态配置
	configWatcher chan *PoolConfig
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// Manager 管理器类型别名
type Manager = PoolManager

// New 创建连接池管理器 (通用接口)
func New(config *PoolConfig) *Manager {
	return NewPoolManager()
}

// NewPoolManager 创建连接池管理器
func NewPoolManager() *PoolManager {
	return &PoolManager{
		pools:         make(map[string]*AdaptivePool),
		logger:        zap.L().Named("pool-manager"),
		config:        DefaultPoolConfig(),
		configWatcher: make(chan *PoolConfig, 1),
	}
}

// NewPoolManagerWithConfig 使用指定配置创建连接池管理器
func NewPoolManagerWithConfig(config *PoolConfig) *PoolManager {
	if err := config.Validate(); err != nil {
		zap.L().Error("连接池配置验证失败", zap.Error(err))
		config = DefaultPoolConfig()
	}

	return &PoolManager{
		pools:         make(map[string]*AdaptivePool),
		logger:        zap.L().Named("pool-manager"),
		config:        config,
		configWatcher: make(chan *PoolConfig, 1),
	}
}

// GetOrCreatePool 获取或创建连接池
func (pm *PoolManager) GetOrCreatePool(targetAddr string, transportType transport.TransportType, transportMgr *transport.Manager) *AdaptivePool {
	poolKey := fmt.Sprintf("%s-%s", string(transportType), targetAddr)

	pm.mutex.RLock()
	if pool, exists := pm.pools[poolKey]; exists {
		pm.mutex.RUnlock()
		return pool
	}
	pm.mutex.RUnlock()

	// 创建新的连接池
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	// 双重检查
	if pool, exists := pm.pools[poolKey]; exists {
		return pool
	}

	pool := NewAdaptivePool(targetAddr, transportType, transportMgr)

	// 应用配置
	if pm.config != nil {
		pm.config.Apply(pool)
		pool.loadBalanceStrategy = ParseLoadBalanceStrategy(pm.config.LoadBalanceStrategy)
		pool.preWarmingEnabled = pm.config.EnablePreWarming
		pool.preWarmingTargets = pm.config.PreWarmingTargets
	}

	pm.pools[poolKey] = pool

	pm.logger.Info("创建新连接池",
		zap.String("key", poolKey),
		zap.String("target", targetAddr),
		zap.String("transport", string(transportType)),
		zap.String("strategy", pool.loadBalanceStrategy.String()))

	return pool
}

// GetConnection 从池中获取连接
func (pm *PoolManager) GetConnection(targetAddr string, transportType transport.TransportType, transportMgr *transport.Manager) (*MonitoredConnection, error) {
	pool := pm.GetOrCreatePool(targetAddr, transportType, transportMgr)

	pooledConn, err := pool.GetConnection()
	if err != nil {
		return nil, err
	}

	return NewMonitoredConnection(pooledConn, pool), nil
}

// Start 启动连接池管理器
func (pm *PoolManager) Start() error {
	pm.ctx, pm.cancel = context.WithCancel(context.Background())

	// 启动配置监控
	pm.wg.Add(1)
	go pm.configWatchLoop()

	pm.logger.Info("✅ 连接池管理器已启动")
	return nil
}

// Stop 停止连接池管理器
func (pm *PoolManager) Stop() error {
	pm.logger.Info("🛑 正在停止连接池管理器")

	if pm.cancel != nil {
		pm.cancel()
	}

	// 停止所有连接池
	if err := pm.StopAll(); err != nil {
		pm.logger.Error("停止连接池失败", zap.Error(err))
	}

	// 等待后台任务结束
	pm.wg.Wait()

	pm.logger.Info("✅ 连接池管理器已停止")
	return nil
}

// StartAll 启动所有连接池
func (pm *PoolManager) StartAll(ctx context.Context) error {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	for _, pool := range pm.pools {
		if err := pool.Start(ctx); err != nil {
			pm.logger.Error("启动连接池失败", zap.Error(err))
			return err
		}
	}

	pm.logger.Info("所有连接池启动完成", zap.Int("count", len(pm.pools)))
	return nil
}

// StopAll 停止所有连接池
func (pm *PoolManager) StopAll() error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	for key, pool := range pm.pools {
		if err := pool.Stop(); err != nil {
			pm.logger.Error("停止连接池失败", zap.String("key", key), zap.Error(err))
		}
	}

	// 清空池映射
	pm.pools = make(map[string]*AdaptivePool)
	pm.logger.Info("所有连接池已停止")
	return nil
}

// GetStats 获取所有连接池统计
func (pm *PoolManager) GetStats() map[string]interface{} {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	stats := map[string]interface{}{
		"pool_count": len(pm.pools),
		"pools":      make(map[string]interface{}),
	}

	for key, pool := range pm.pools {
		stats["pools"].(map[string]interface{})[key] = pool.GetStats()
	}

	return stats
}

// configWatchLoop 配置监控循环
func (pm *PoolManager) configWatchLoop() {
	defer pm.wg.Done()

	for {
		select {
		case newConfig := <-pm.configWatcher:
			pm.applyNewConfig(newConfig)
		case <-pm.ctx.Done():
			return
		}
	}
}

// UpdateConfig 更新配置
func (pm *PoolManager) UpdateConfig(config *PoolConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	select {
	case pm.configWatcher <- config:
		pm.logger.Info("🔄 提交配置更新请求")
		return nil
	default:
		return fmt.Errorf("配置更新渠道忙碁")
	}
}

// applyNewConfig 应用新配置
func (pm *PoolManager) applyNewConfig(newConfig *PoolConfig) {
	pm.logger.Info("🔧 应用新配置")

	pm.mutex.Lock()
	oldConfig := pm.config
	pm.config = newConfig
	pm.mutex.Unlock()

	// 对所有现有连接池应用新配置
	pm.mutex.RLock()
	for key, pool := range pm.pools {
		pm.applyConfigToPool(pool, oldConfig, newConfig)
		pm.logger.Debug("池配置已应用", zap.String("pool", key))
	}
	pm.mutex.RUnlock()

	pm.logger.Info("✅ 配置更新完成")
}

// applyConfigToPool 对单个连接池应用配置
func (pm *PoolManager) applyConfigToPool(pool *AdaptivePool, oldConfig, newConfig *PoolConfig) {
	// 应用基本配置
	newConfig.Apply(pool)

	// 更新负载均衡策略
	if oldConfig.LoadBalanceStrategy != newConfig.LoadBalanceStrategy {
		pool.SetLoadBalanceStrategy(ParseLoadBalanceStrategy(newConfig.LoadBalanceStrategy))
	}

	// 更新预热配置
	pool.preWarmingEnabled = newConfig.EnablePreWarming
	pool.preWarmingTargets = newConfig.PreWarmingTargets

	// 如果启用了预热且之前没启用，执行预热
	if newConfig.EnablePreWarming && !oldConfig.EnablePreWarming {
		go pool.performPreWarming()
	}
}

// GetConfig 获取当前配置
func (pm *PoolManager) GetConfig() *PoolConfig {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	return pm.config
}
