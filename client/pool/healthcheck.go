package pool

import (
	"context"
	"net"
	"sync"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// HealthChecker 健康检查器
type HealthChecker struct {
	pool          *Pool
	interval      time.Duration
	timeout       time.Duration
	failThreshold int // 连续失败次数阈值
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(pool *Pool, interval, timeout time.Duration) *HealthChecker {
	return &HealthChecker{
		pool:          pool,
		interval:      interval,
		timeout:       timeout,
		failThreshold: 3, // 默认连续3次失败才标记为不健康
		stopChan:      make(chan struct{}),
	}
}

// Start 启动健康检查
func (hc *HealthChecker) Start() {
	hc.wg.Add(1)
	go hc.checkLoop()
	logger.Info("健康检查器已启动",
		zap.String("target", hc.pool.target),
		zap.Duration("interval", hc.interval))
}

// Stop 停止健康检查
func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
	hc.wg.Wait()
	logger.Info("健康检查器已停止")
}

// checkLoop 健康检查循环
func (hc *HealthChecker) checkLoop() {
	defer hc.wg.Done()

	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.performCheck()
		case <-hc.stopChan:
			return
		}
	}
}

// performCheck 执行健康检查
func (hc *HealthChecker) performCheck() {
	hc.pool.mu.RLock()
	conns := make([]*Conn, 0, len(hc.pool.activeConns))
	for _, conn := range hc.pool.activeConns {
		conns = append(conns, conn)
	}
	hc.pool.mu.RUnlock()

	// 检查活跃连接
	for _, conn := range conns {
		if !hc.checkConnection(conn) {
			logger.Warn("连接健康检查失败",
				zap.String("conn_id", conn.ID),
				zap.String("target", hc.pool.target))

			// 关闭不健康的连接
			conn.Conn.Close()
			hc.pool.mu.Lock()
			delete(hc.pool.activeConns, conn.ID)
			hc.pool.mu.Unlock()
		}
	}

	// 检查空闲连接
	select {
	case conn := <-hc.pool.idleConns:
		if !hc.checkConnection(conn) {
			conn.Conn.Close()
		} else {
			// 放回连接池
			select {
			case hc.pool.idleConns <- conn:
			default:
				conn.Conn.Close()
			}
		}
	default:
		// 没有空闲连接
	}
}

// checkConnection 检查单个连接
func (hc *HealthChecker) checkConnection(conn *Conn) bool {
	ctx, cancel := context.WithTimeout(context.Background(), hc.timeout)
	defer cancel()

	// 使用TCP Keep-Alive检查
	if tcpConn, ok := conn.Conn.(*net.TCPConn); ok {
		// 设置读取超时
		tcpConn.SetReadDeadline(time.Now().Add(hc.timeout))
		defer tcpConn.SetReadDeadline(time.Time{})

		// 尝试读取一个字节（如果连接正常，应该阻塞或返回错误）
		var buf [1]byte
		_, err := tcpConn.Read(buf[:])

		if err != nil {
			// 检查是否是超时错误（正常情况）
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return true // 连接健康
			}
			return false // 连接已断开
		}
	}

	// 对于其他类型的连接，尝试设置deadline
	conn.Conn.SetDeadline(time.Now().Add(hc.timeout))
	defer conn.Conn.SetDeadline(time.Time{})

	select {
	case <-ctx.Done():
		return true // 超时但连接未断开，认为健康
	case <-time.After(100 * time.Millisecond):
		return true
	}
}

// AdvancedPool 高级连接池（带连接复用和健康检查）
type AdvancedPool struct {
	*Pool
	healthChecker *HealthChecker
	reuseEnabled  bool
	maxIdleTime   time.Duration
	maxLifetime   time.Duration
}

// NewAdvancedPool 创建高级连接池
func NewAdvancedPool(target string, minConns, maxConns int, config AdvancedPoolConfig) *AdvancedPool {
	basePool := NewPool(target, minConns, maxConns, config.IdleTimeout)

	ap := &AdvancedPool{
		Pool:         basePool,
		reuseEnabled: config.EnableReuse,
		maxIdleTime:  config.MaxIdleTime,
		maxLifetime:  config.MaxLifetime,
	}

	// 创建健康检查器
	if config.EnableHealthCheck {
		ap.healthChecker = NewHealthChecker(basePool, config.HealthCheckInterval, config.HealthCheckTimeout)
	}

	return ap
}

// AdvancedPoolConfig 高级连接池配置
type AdvancedPoolConfig struct {
	IdleTimeout         time.Duration
	EnableReuse         bool
	EnableHealthCheck   bool
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
	MaxIdleTime         time.Duration
	MaxLifetime         time.Duration
}

// DefaultAdvancedPoolConfig 默认高级连接池配置
func DefaultAdvancedPoolConfig() AdvancedPoolConfig {
	return AdvancedPoolConfig{
		IdleTimeout:         5 * time.Minute,
		EnableReuse:         true,
		EnableHealthCheck:   true,
		HealthCheckInterval: 30 * time.Second,
		HealthCheckTimeout:  3 * time.Second,
		MaxIdleTime:         10 * time.Minute,
		MaxLifetime:         1 * time.Hour,
	}
}

// Init 初始化高级连接池
func (ap *AdvancedPool) Init() error {
	if err := ap.Pool.Init(); err != nil {
		return err
	}

	if ap.healthChecker != nil {
		ap.healthChecker.Start()
	}

	logger.Info("高级连接池已初始化",
		zap.String("target", ap.target),
		zap.Bool("reuse_enabled", ap.reuseEnabled),
		zap.Bool("health_check_enabled", ap.healthChecker != nil))

	return nil
}

// Get 获取连接（带连接复用和健康检查）
func (ap *AdvancedPool) Get() (*Conn, error) {
	for {
		conn, err := ap.Pool.Get()
		if err != nil {
			return nil, err
		}

		// 检查连接是否超过最大生命周期
		if ap.maxLifetime > 0 && time.Since(conn.CreatedAt) > ap.maxLifetime {
			logger.Debug("连接已超过最大生命周期",
				zap.String("conn_id", conn.ID),
				zap.Duration("lifetime", time.Since(conn.CreatedAt)))
			conn.Conn.Close()
			continue
		}

		// 检查连接是否超过最大空闲时间
		if ap.maxIdleTime > 0 && time.Since(conn.LastUsed) > ap.maxIdleTime {
			logger.Debug("连接空闲时间过长",
				zap.String("conn_id", conn.ID),
				zap.Duration("idle_time", time.Since(conn.LastUsed)))
			conn.Conn.Close()
			continue
		}

		// 快速健康检查
		if !ap.isConnHealthy(conn) {
			logger.Debug("连接健康检查失败",
				zap.String("conn_id", conn.ID))
			conn.Conn.Close()
			continue
		}

		// 更新使用时间和计数
		conn.LastUsed = time.Now()
		conn.UseCount++

		return conn, nil
	}
}

// isConnHealthy 快速健康检查
func (ap *AdvancedPool) isConnHealthy(conn *Conn) bool {
	if tcpConn, ok := conn.Conn.(*net.TCPConn); ok {
		// 设置很短的超时
		tcpConn.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		defer tcpConn.SetReadDeadline(time.Time{})

		var buf [1]byte
		_, err := tcpConn.Read(buf[:])

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return true
			}
			return false
		}
	}
	return true
}

// Close 关闭高级连接池
func (ap *AdvancedPool) Close() error {
	if ap.healthChecker != nil {
		ap.healthChecker.Stop()
	}
	return ap.Pool.Close()
}

// GetStats 获取统计信息
func (ap *AdvancedPool) GetStats() AdvancedPoolStats {
	baseStats := ap.Pool.Stats()
	return AdvancedPoolStats{
		PoolStats:          baseStats,
		HealthCheckEnabled: ap.healthChecker != nil,
		ReuseEnabled:       ap.reuseEnabled,
		MaxIdleTime:        ap.maxIdleTime,
		MaxLifetime:        ap.maxLifetime,
	}
}

// AdvancedPoolStats 高级连接池统计
type AdvancedPoolStats struct {
	PoolStats
	HealthCheckEnabled bool
	ReuseEnabled       bool
	MaxIdleTime        time.Duration
	MaxLifetime        time.Duration
}

