package resource

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// NetworkResourceManager 网络资源管理器
type NetworkResourceManager struct {
	config *NetworkResourceConfig
	logger *zap.Logger

	// 连接池
	pools      map[string]*ConnectionPool
	poolsMutex sync.RWMutex

	// 连接监控
	activeConns map[string]*ManagedConnection
	connsMutex  sync.RWMutex

	// 统计信息
	stats struct {
		totalConnections  atomic.Int64
		activeConnections atomic.Int64
		pooledConnections atomic.Int64
		connectionReuse   atomic.Int64
		connectionTimeout atomic.Int64
		connectionErrors  atomic.Int64
	}

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NetworkResourceConfig 网络资源配置
type NetworkResourceConfig struct {
	// 连接池配置
	MaxPoolSize          int `json:"max_pool_size"`
	MinPoolSize          int `json:"min_pool_size"`
	MaxIdleConnections   int `json:"max_idle_connections"`
	MaxActiveConnections int `json:"max_active_connections"`

	// 超时配置
	ConnectionTimeout time.Duration `json:"connection_timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout"`
	MaxLifetime       time.Duration `json:"max_lifetime"`
	KeepAliveTimeout  time.Duration `json:"keep_alive_timeout"`

	// 健康检查
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	HealthCheckTimeout  time.Duration `json:"health_check_timeout"`
	MaxRetries          int           `json:"max_retries"`

	// 监控配置
	EnableMetrics   bool          `json:"enable_metrics"`
	MetricsInterval time.Duration `json:"metrics_interval"`
}

// DefaultNetworkResourceConfig 默认网络资源配置
func DefaultNetworkResourceConfig() *NetworkResourceConfig {
	return &NetworkResourceConfig{
		MaxPoolSize:          100,
		MinPoolSize:          5,
		MaxIdleConnections:   20,
		MaxActiveConnections: 80,
		ConnectionTimeout:    30 * time.Second,
		IdleTimeout:          5 * time.Minute,
		MaxLifetime:          1 * time.Hour,
		KeepAliveTimeout:     30 * time.Second,
		HealthCheckInterval:  1 * time.Minute,
		HealthCheckTimeout:   5 * time.Second,
		MaxRetries:           3,
		EnableMetrics:        true,
		MetricsInterval:      30 * time.Second,
	}
}

// ConnectionPool 连接池
type ConnectionPool struct {
	target  string
	network string
	config  *NetworkResourceConfig
	logger  *zap.Logger

	// 连接存储
	idle   []*ManagedConnection
	active map[string]*ManagedConnection
	mutex  sync.RWMutex

	// 统计
	stats struct {
		created  atomic.Int64
		closed   atomic.Int64
		reused   atomic.Int64
		timeouts atomic.Int64
		errors   atomic.Int64
	}

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ManagedConnection 托管连接
type ManagedConnection struct {
	id        string
	conn      net.Conn
	target    string
	network   string
	createdAt time.Time
	lastUsed  atomic.Int64
	useCount  atomic.Int64
	healthy   atomic.Bool
	inUse     atomic.Bool
	pool      *ConnectionPool

	// 健康检查
	lastHealthCheck atomic.Int64
	healthErrors    atomic.Int32
}

// NewNetworkResourceManager 创建网络资源管理器
func NewNetworkResourceManager(config *NetworkResourceConfig) *NetworkResourceManager {
	if config == nil {
		config = DefaultNetworkResourceConfig()
	}

	return &NetworkResourceManager{
		config:      config,
		logger:      zap.L().Named("network-resource-manager"),
		pools:       make(map[string]*ConnectionPool),
		activeConns: make(map[string]*ManagedConnection),
	}
}

// Start 启动网络资源管理器
func (nrm *NetworkResourceManager) Start(ctx context.Context) error {
	nrm.ctx, nrm.cancel = context.WithCancel(ctx)

	// 启动监控协程
	if nrm.config.EnableMetrics {
		nrm.wg.Add(1)
		go nrm.metricsLoop()
	}

	// 启动健康检查协程
	nrm.wg.Add(1)
	go nrm.healthCheckLoop()

	// 启动清理协程
	nrm.wg.Add(1)
	go nrm.cleanupLoop()

	nrm.logger.Info("🌐 网络资源管理器启动",
		zap.Int("max_pool_size", nrm.config.MaxPoolSize),
		zap.Int("max_active_connections", nrm.config.MaxActiveConnections),
		zap.Duration("connection_timeout", nrm.config.ConnectionTimeout))

	return nil
}

// Stop 停止网络资源管理器
func (nrm *NetworkResourceManager) Stop() error {
	nrm.logger.Info("🛑 正在停止网络资源管理器...")

	if nrm.cancel != nil {
		nrm.cancel()
	}

	// 关闭所有连接池
	nrm.poolsMutex.Lock()
	for _, pool := range nrm.pools {
		pool.Close()
	}
	nrm.poolsMutex.Unlock()

	// 等待协程结束
	nrm.wg.Wait()

	nrm.logger.Info("✅ 网络资源管理器已停止")
	return nil
}

// GetConnection 获取连接
func (nrm *NetworkResourceManager) GetConnection(network, target string) (*ManagedConnection, error) {
	poolKey := fmt.Sprintf("%s://%s", network, target)

	// 获取或创建连接池
	pool := nrm.getOrCreatePool(poolKey, network, target)

	// 从池中获取连接
	conn, err := pool.Get()
	if err != nil {
		nrm.stats.connectionErrors.Add(1)
		return nil, err
	}

	// 注册活跃连接
	nrm.registerActiveConnection(conn)

	nrm.stats.activeConnections.Add(1)
	return conn, nil
}

// ReturnConnection 归还连接
func (nrm *NetworkResourceManager) ReturnConnection(conn *ManagedConnection) error {
	if conn == nil {
		return fmt.Errorf("连接不能为空")
	}

	// 从活跃连接中移除
	nrm.unregisterActiveConnection(conn)

	// 归还到池中
	err := conn.pool.Put(conn)
	if err != nil {
		return err
	}

	nrm.stats.activeConnections.Add(-1)
	return nil
}

// getOrCreatePool 获取或创建连接池
func (nrm *NetworkResourceManager) getOrCreatePool(poolKey, network, target string) *ConnectionPool {
	nrm.poolsMutex.RLock()
	if pool, exists := nrm.pools[poolKey]; exists {
		nrm.poolsMutex.RUnlock()
		return pool
	}
	nrm.poolsMutex.RUnlock()

	nrm.poolsMutex.Lock()
	defer nrm.poolsMutex.Unlock()

	// 双重检查
	if pool, exists := nrm.pools[poolKey]; exists {
		return pool
	}

	// 创建新的连接池
	pool := NewConnectionPool(network, target, nrm.config)
	pool.Start(nrm.ctx)
	nrm.pools[poolKey] = pool

	nrm.logger.Debug("创建连接池",
		zap.String("pool_key", poolKey),
		zap.String("network", network),
		zap.String("target", target))

	return pool
}

// registerActiveConnection 注册活跃连接
func (nrm *NetworkResourceManager) registerActiveConnection(conn *ManagedConnection) {
	nrm.connsMutex.Lock()
	nrm.activeConns[conn.id] = conn
	nrm.connsMutex.Unlock()
}

// unregisterActiveConnection 注销活跃连接
func (nrm *NetworkResourceManager) unregisterActiveConnection(conn *ManagedConnection) {
	nrm.connsMutex.Lock()
	delete(nrm.activeConns, conn.id)
	nrm.connsMutex.Unlock()
}

// metricsLoop 指标循环
func (nrm *NetworkResourceManager) metricsLoop() {
	defer nrm.wg.Done()

	ticker := time.NewTicker(nrm.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-nrm.ctx.Done():
			return
		case <-ticker.C:
			nrm.logMetrics()
		}
	}
}

// healthCheckLoop 健康检查循环
func (nrm *NetworkResourceManager) healthCheckLoop() {
	defer nrm.wg.Done()

	ticker := time.NewTicker(nrm.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-nrm.ctx.Done():
			return
		case <-ticker.C:
			nrm.performHealthChecks()
		}
	}
}

// cleanupLoop 清理循环
func (nrm *NetworkResourceManager) cleanupLoop() {
	defer nrm.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-nrm.ctx.Done():
			return
		case <-ticker.C:
			nrm.performCleanup()
		}
	}
}

// performHealthChecks 执行健康检查
func (nrm *NetworkResourceManager) performHealthChecks() {
	nrm.connsMutex.RLock()
	connections := make([]*ManagedConnection, 0, len(nrm.activeConns))
	for _, conn := range nrm.activeConns {
		connections = append(connections, conn)
	}
	nrm.connsMutex.RUnlock()

	for _, conn := range connections {
		if conn.shouldHealthCheck() {
			go conn.performHealthCheck()
		}
	}
}

// performCleanup 执行清理
func (nrm *NetworkResourceManager) performCleanup() {
	nrm.poolsMutex.RLock()
	pools := make([]*ConnectionPool, 0, len(nrm.pools))
	for _, pool := range nrm.pools {
		pools = append(pools, pool)
	}
	nrm.poolsMutex.RUnlock()

	for _, pool := range pools {
		pool.cleanup()
	}
}

// logMetrics 记录指标
func (nrm *NetworkResourceManager) logMetrics() {
	stats := nrm.GetStats()

	nrm.logger.Info("📊 网络资源统计",
		zap.Int64("total_connections", stats["total_connections"].(int64)),
		zap.Int64("active_connections", stats["active_connections"].(int64)),
		zap.Int64("pooled_connections", stats["pooled_connections"].(int64)),
		zap.Int64("connection_reuse", stats["connection_reuse"].(int64)),
		zap.Int("active_pools", len(nrm.pools)))
}

// GetStats 获取统计信息
func (nrm *NetworkResourceManager) GetStats() map[string]interface{} {
	nrm.poolsMutex.RLock()
	poolStats := make(map[string]interface{})
	totalPooled := int64(0)
	for key, pool := range nrm.pools {
		stats := pool.GetStats()
		poolStats[key] = stats
		totalPooled += stats["idle_count"].(int64)
	}
	nrm.poolsMutex.RUnlock()

	return map[string]interface{}{
		"total_connections":  nrm.stats.totalConnections.Load(),
		"active_connections": nrm.stats.activeConnections.Load(),
		"pooled_connections": totalPooled,
		"connection_reuse":   nrm.stats.connectionReuse.Load(),
		"connection_timeout": nrm.stats.connectionTimeout.Load(),
		"connection_errors":  nrm.stats.connectionErrors.Load(),
		"active_pools":       len(nrm.pools),
		"pool_stats":         poolStats,
		"config":             nrm.config,
	}
}

// NewConnectionPool 创建连接池
func NewConnectionPool(network, target string, config *NetworkResourceConfig) *ConnectionPool {
	return &ConnectionPool{
		target:  target,
		network: network,
		config:  config,
		logger:  zap.L().Named("connection-pool").With(zap.String("target", target)),
		idle:    make([]*ManagedConnection, 0, config.MaxIdleConnections),
		active:  make(map[string]*ManagedConnection),
	}
}

// Start 启动连接池
func (cp *ConnectionPool) Start(ctx context.Context) {
	cp.ctx, cp.cancel = context.WithCancel(ctx)

	// 预热连接池
	cp.wg.Add(1)
	go cp.warmup()
}

// Get 获取连接
func (cp *ConnectionPool) Get() (*ManagedConnection, error) {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	// 尝试从空闲连接中获取
	for len(cp.idle) > 0 {
		conn := cp.idle[len(cp.idle)-1]
		cp.idle = cp.idle[:len(cp.idle)-1]

		// 检查连接是否仍然有效
		if conn.isValid() {
			conn.inUse.Store(true)
			conn.lastUsed.Store(time.Now().Unix())
			conn.useCount.Add(1)
			cp.active[conn.id] = conn
			cp.stats.reused.Add(1)
			return conn, nil
		}

		// 连接无效，关闭它
		conn.close()
	}

	// 检查活跃连接数限制
	if len(cp.active) >= cp.config.MaxActiveConnections {
		return nil, fmt.Errorf("连接池已满，活跃连接数: %d", len(cp.active))
	}

	// 创建新连接
	conn, err := cp.createConnection()
	if err != nil {
		cp.stats.errors.Add(1)
		return nil, err
	}

	conn.inUse.Store(true)
	cp.active[conn.id] = conn
	cp.stats.created.Add(1)
	return conn, nil
}

// Put 归还连接
func (cp *ConnectionPool) Put(conn *ManagedConnection) error {
	if conn == nil {
		return fmt.Errorf("连接不能为空")
	}

	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	// 从活跃连接中移除
	delete(cp.active, conn.id)
	conn.inUse.Store(false)

	// 检查连接是否仍然有效
	if !conn.isValid() || len(cp.idle) >= cp.config.MaxIdleConnections {
		conn.close()
		return nil
	}

	// 加入空闲连接
	cp.idle = append(cp.idle, conn)
	return nil
}

// createConnection 创建连接
func (cp *ConnectionPool) createConnection() (*ManagedConnection, error) {
	// 创建带超时的连接
	dialer := &net.Dialer{
		Timeout: cp.config.ConnectionTimeout,
	}

	conn, err := dialer.DialContext(cp.ctx, cp.network, cp.target)
	if err != nil {
		return nil, fmt.Errorf("创建连接失败: %w", err)
	}

	// 创建托管连接
	managedConn := &ManagedConnection{
		id:        fmt.Sprintf("conn-%s-%d", cp.target, time.Now().UnixNano()),
		conn:      conn,
		target:    cp.target,
		network:   cp.network,
		createdAt: time.Now(),
		pool:      cp,
	}

	managedConn.lastUsed.Store(time.Now().Unix())
	managedConn.healthy.Store(true)
	managedConn.lastHealthCheck.Store(time.Now().Unix())

	return managedConn, nil
}

// warmup 预热连接池
func (cp *ConnectionPool) warmup() {
	defer cp.wg.Done()

	for i := 0; i < cp.config.MinPoolSize; i++ {
		conn, err := cp.createConnection()
		if err != nil {
			cp.logger.Warn("预热连接失败", zap.Error(err))
			continue
		}

		cp.mutex.Lock()
		cp.idle = append(cp.idle, conn)
		cp.mutex.Unlock()
	}

	cp.logger.Debug("连接池预热完成",
		zap.Int("min_size", cp.config.MinPoolSize),
		zap.Int("created", len(cp.idle)))
}

// cleanup 清理连接池
func (cp *ConnectionPool) cleanup() {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	now := time.Now()

	// 清理过期的空闲连接
	validIdle := cp.idle[:0]
	for _, conn := range cp.idle {
		if conn.isExpired(now) {
			conn.close()
		} else {
			validIdle = append(validIdle, conn)
		}
	}
	cp.idle = validIdle

	// 检查活跃连接
	for id, conn := range cp.active {
		if conn.isExpired(now) && !conn.inUse.Load() {
			delete(cp.active, id)
			conn.close()
		}
	}
}

// Close 关闭连接池
func (cp *ConnectionPool) Close() {
	if cp.cancel != nil {
		cp.cancel()
	}

	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	// 关闭所有空闲连接
	for _, conn := range cp.idle {
		conn.close()
	}
	cp.idle = nil

	// 关闭所有活跃连接
	for _, conn := range cp.active {
		conn.close()
	}
	cp.active = make(map[string]*ManagedConnection)

	cp.wg.Wait()
}

// GetStats 获取连接池统计
func (cp *ConnectionPool) GetStats() map[string]interface{} {
	cp.mutex.RLock()
	idleCount := len(cp.idle)
	activeCount := len(cp.active)
	cp.mutex.RUnlock()

	return map[string]interface{}{
		"target":       cp.target,
		"network":      cp.network,
		"idle_count":   int64(idleCount),
		"active_count": int64(activeCount),
		"created":      cp.stats.created.Load(),
		"closed":       cp.stats.closed.Load(),
		"reused":       cp.stats.reused.Load(),
		"timeouts":     cp.stats.timeouts.Load(),
		"errors":       cp.stats.errors.Load(),
	}
}

// ManagedConnection 方法

// isValid 检查连接是否有效
func (mc *ManagedConnection) isValid() bool {
	if !mc.healthy.Load() {
		return false
	}

	// 检查连接是否关闭
	if mc.conn == nil {
		return false
	}

	// 检查是否超过最大生命周期
	if time.Since(mc.createdAt) > mc.pool.config.MaxLifetime {
		return false
	}

	return true
}

// isExpired 检查连接是否过期
func (mc *ManagedConnection) isExpired(now time.Time) bool {
	// 检查空闲超时
	lastUsed := time.Unix(mc.lastUsed.Load(), 0)
	if now.Sub(lastUsed) > mc.pool.config.IdleTimeout {
		return true
	}

	// 检查最大生命周期
	if now.Sub(mc.createdAt) > mc.pool.config.MaxLifetime {
		return true
	}

	return false
}

// shouldHealthCheck 是否应该进行健康检查
func (mc *ManagedConnection) shouldHealthCheck() bool {
	lastCheck := time.Unix(mc.lastHealthCheck.Load(), 0)
	return time.Since(lastCheck) > mc.pool.config.HealthCheckInterval
}

// performHealthCheck 执行健康检查
func (mc *ManagedConnection) performHealthCheck() {
	mc.lastHealthCheck.Store(time.Now().Unix())

	// 简单的健康检查：尝试设置读取超时
	if mc.conn != nil {
		if err := mc.conn.SetReadDeadline(time.Now().Add(mc.pool.config.HealthCheckTimeout)); err != nil {
			mc.healthErrors.Add(1)
			if mc.healthErrors.Load() > 3 {
				mc.healthy.Store(false)
			}
		} else {
			mc.healthErrors.Store(0)
			mc.healthy.Store(true)
		}
	}
}

// close 关闭连接
func (mc *ManagedConnection) close() {
	if mc.conn != nil {
		mc.conn.Close()
		mc.conn = nil
	}
	mc.healthy.Store(false)
}

// GetConnection 获取底层连接
func (mc *ManagedConnection) GetConnection() net.Conn {
	mc.lastUsed.Store(time.Now().Unix())
	return mc.conn
}

// GetID 获取连接ID
func (mc *ManagedConnection) GetID() string {
	return mc.id
}

// GetStats 获取连接统计
func (mc *ManagedConnection) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"id":            mc.id,
		"target":        mc.target,
		"network":       mc.network,
		"created_at":    mc.createdAt,
		"last_used":     time.Unix(mc.lastUsed.Load(), 0),
		"use_count":     mc.useCount.Load(),
		"healthy":       mc.healthy.Load(),
		"in_use":        mc.inUse.Load(),
		"health_errors": mc.healthErrors.Load(),
	}
}
