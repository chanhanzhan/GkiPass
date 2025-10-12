package relay

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// RelayConfig 中继配置
type RelayConfig struct {
	// 缓冲区配置
	BufferSize int `json:"buffer_size"`

	// 超时配置
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`

	// 性能配置
	MaxConnections int  `json:"max_connections"`
	KeepAlive      bool `json:"keep_alive"`

	// 错误处理
	RetryAttempts int           `json:"retry_attempts"`
	RetryDelay    time.Duration `json:"retry_delay"`
}

// DefaultRelayConfig 默认中继配置
func DefaultRelayConfig() *RelayConfig {
	return &RelayConfig{
		BufferSize:     32768, // 32KB
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    5 * time.Minute,
		MaxConnections: 10000,
		KeepAlive:      true,
		RetryAttempts:  3,
		RetryDelay:     1 * time.Second,
	}
}

// RelaySession 中继会话
type RelaySession struct {
	ID     string
	ConnA  net.Conn
	ConnB  net.Conn
	Config *RelayConfig

	// 统计信息
	BytesAtoB    atomic.Int64
	BytesBtoA    atomic.Int64
	StartTime    time.Time
	LastActivity atomic.Int64 // Unix timestamp

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed atomic.Bool

	logger *zap.Logger
}

// NewRelaySession 创建中继会话
func NewRelaySession(id string, connA, connB net.Conn, config *RelayConfig) *RelaySession {
	if config == nil {
		config = DefaultRelayConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	session := &RelaySession{
		ID:        id,
		ConnA:     connA,
		ConnB:     connB,
		Config:    config,
		StartTime: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
		logger:    zap.L().Named(fmt.Sprintf("relay-session-%s", id)),
	}

	session.updateActivity()
	return session
}

// Start 启动中继会话
func (rs *RelaySession) Start() error {
	if rs.closed.Load() {
		return fmt.Errorf("会话已关闭")
	}

	rs.logger.Info("启动中继会话",
		zap.String("conn_a", rs.ConnA.RemoteAddr().String()),
		zap.String("conn_b", rs.ConnB.RemoteAddr().String()))

	// 设置连接属性
	if rs.Config.KeepAlive {
		rs.setKeepAlive(rs.ConnA)
		rs.setKeepAlive(rs.ConnB)
	}

	// 启动双向数据转发
	rs.wg.Add(2)
	go func() {
		defer rs.wg.Done()
		rs.relay(rs.ConnA, rs.ConnB, "A->B", &rs.BytesAtoB)
	}()

	go func() {
		defer rs.wg.Done()
		rs.relay(rs.ConnB, rs.ConnA, "B->A", &rs.BytesBtoA)
	}()

	rs.logger.Debug("中继会话启动完成")
	return nil
}

// Stop 停止中继会话
func (rs *RelaySession) Stop() error {
	if rs.closed.Swap(true) {
		return nil // 已经关闭
	}

	rs.logger.Debug("停止中继会话")

	// 取消上下文
	if rs.cancel != nil {
		rs.cancel()
	}

	// 关闭连接
	if rs.ConnA != nil {
		rs.ConnA.Close()
	}
	if rs.ConnB != nil {
		rs.ConnB.Close()
	}

	// 等待协程结束
	rs.wg.Wait()

	duration := time.Since(rs.StartTime)
	rs.logger.Info("中继会话已停止",
		zap.Duration("duration", duration),
		zap.Int64("bytes_a_to_b", rs.BytesAtoB.Load()),
		zap.Int64("bytes_b_to_a", rs.BytesBtoA.Load()))

	return nil
}

// relay 执行数据中继
func (rs *RelaySession) relay(src, dst net.Conn, direction string, counter *atomic.Int64) {
	defer rs.logger.Debug("停止数据中继", zap.String("direction", direction))

	buffer := make([]byte, rs.Config.BufferSize)

	for !rs.closed.Load() {
		select {
		case <-rs.ctx.Done():
			return
		default:
		}

		// 设置读取超时
		if rs.Config.ReadTimeout > 0 {
			src.SetReadDeadline(time.Now().Add(rs.Config.ReadTimeout))
		}

		// 读取数据
		n, err := src.Read(buffer)
		if err != nil {
			if err == io.EOF {
				rs.logger.Debug("连接关闭", zap.String("direction", direction))
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// 读取超时，检查是否空闲超时
				if rs.isIdleTimeout() {
					rs.logger.Debug("空闲超时", zap.String("direction", direction))
					rs.Stop()
				}
				continue
			} else {
				rs.logger.Error("读取数据失败",
					zap.String("direction", direction),
					zap.Error(err))
			}
			rs.Stop()
			return
		}

		if n > 0 {
			// 设置写入超时
			if rs.Config.WriteTimeout > 0 {
				dst.SetWriteDeadline(time.Now().Add(rs.Config.WriteTimeout))
			}

			// 写入数据
			written, err := dst.Write(buffer[:n])
			if err != nil {
				rs.logger.Error("写入数据失败",
					zap.String("direction", direction),
					zap.Error(err))
				rs.Stop()
				return
			}

			counter.Add(int64(written))
			rs.updateActivity()

			rs.logger.Debug("转发数据",
				zap.String("direction", direction),
				zap.Int("bytes", written))
		}
	}
}

// setKeepAlive 设置连接保持活跃
func (rs *RelaySession) setKeepAlive(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}
}

// updateActivity 更新活动时间
func (rs *RelaySession) updateActivity() {
	rs.LastActivity.Store(time.Now().Unix())
}

// isIdleTimeout 检查是否空闲超时
func (rs *RelaySession) isIdleTimeout() bool {
	if rs.Config.IdleTimeout <= 0 {
		return false
	}

	lastActivity := time.Unix(rs.LastActivity.Load(), 0)
	return time.Since(lastActivity) > rs.Config.IdleTimeout
}

// GetStats 获取会话统计信息
func (rs *RelaySession) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"id":            rs.ID,
		"start_time":    rs.StartTime,
		"duration":      time.Since(rs.StartTime).Seconds(),
		"bytes_a_to_b":  rs.BytesAtoB.Load(),
		"bytes_b_to_a":  rs.BytesBtoA.Load(),
		"total_bytes":   rs.BytesAtoB.Load() + rs.BytesBtoA.Load(),
		"last_activity": time.Unix(rs.LastActivity.Load(), 0),
		"closed":        rs.closed.Load(),
		"conn_a":        rs.ConnA.RemoteAddr().String(),
		"conn_b":        rs.ConnB.RemoteAddr().String(),
	}
}

// RelayManager 中继管理器
type RelayManager struct {
	config   *RelayConfig
	sessions map[string]*RelaySession
	mutex    sync.RWMutex

	// 统计
	totalSessions   atomic.Int64
	activeSessions  atomic.Int64
	totalBytesRelay atomic.Int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// NewRelayManager 创建中继管理器
func NewRelayManager(config *RelayConfig) *RelayManager {
	if config == nil {
		config = DefaultRelayConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &RelayManager{
		config:   config,
		sessions: make(map[string]*RelaySession),
		ctx:      ctx,
		cancel:   cancel,
		logger:   zap.L().Named("relay-manager"),
	}
}

// Start 启动中继管理器
func (rm *RelayManager) Start() error {
	rm.logger.Info("启动中继管理器",
		zap.Int("max_connections", rm.config.MaxConnections),
		zap.Int("buffer_size", rm.config.BufferSize))

	// 启动清理协程
	rm.wg.Add(1)
	go func() {
		defer rm.wg.Done()
		rm.cleanupLoop()
	}()

	return nil
}

// Stop 停止中继管理器
func (rm *RelayManager) Stop() error {
	rm.logger.Info("停止中继管理器")

	// 取消上下文
	if rm.cancel != nil {
		rm.cancel()
	}

	// 停止所有会话
	rm.mutex.Lock()
	sessions := make([]*RelaySession, 0, len(rm.sessions))
	for _, session := range rm.sessions {
		sessions = append(sessions, session)
	}
	rm.mutex.Unlock()

	for _, session := range sessions {
		session.Stop()
	}

	// 等待协程结束
	rm.wg.Wait()

	rm.logger.Info("中继管理器已停止")
	return nil
}

// CreateRelay 创建中继连接
func (rm *RelayManager) CreateRelay(sessionID string, connA, connB net.Conn) (*RelaySession, error) {
	// 检查连接数限制
	if int(rm.activeSessions.Load()) >= rm.config.MaxConnections {
		return nil, fmt.Errorf("连接数已达上限: %d", rm.config.MaxConnections)
	}

	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	// 检查会话是否已存在
	if _, exists := rm.sessions[sessionID]; exists {
		return nil, fmt.Errorf("会话已存在: %s", sessionID)
	}

	// 创建中继会话
	session := NewRelaySession(sessionID, connA, connB, rm.config)
	rm.sessions[sessionID] = session

	// 启动会话
	if err := session.Start(); err != nil {
		delete(rm.sessions, sessionID)
		return nil, fmt.Errorf("启动中继会话失败: %w", err)
	}

	rm.totalSessions.Add(1)
	rm.activeSessions.Add(1)

	rm.logger.Info("创建中继会话",
		zap.String("session_id", sessionID),
		zap.String("conn_a", connA.RemoteAddr().String()),
		zap.String("conn_b", connB.RemoteAddr().String()))

	return session, nil
}

// RemoveRelay 移除中继连接
func (rm *RelayManager) RemoveRelay(sessionID string) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	session, exists := rm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	// 停止会话
	session.Stop()

	// 更新统计
	rm.totalBytesRelay.Add(session.BytesAtoB.Load() + session.BytesBtoA.Load())

	delete(rm.sessions, sessionID)
	rm.activeSessions.Add(-1)

	rm.logger.Info("移除中继会话", zap.String("session_id", sessionID))
	return nil
}

// GetSession 获取中继会话
func (rm *RelayManager) GetSession(sessionID string) (*RelaySession, bool) {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	session, exists := rm.sessions[sessionID]
	return session, exists
}

// GetAllSessions 获取所有中继会话
func (rm *RelayManager) GetAllSessions() map[string]*RelaySession {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]*RelaySession)
	for id, session := range rm.sessions {
		result[id] = session
	}

	return result
}

// RelayTCP 简单的TCP中继函数
func (rm *RelayManager) RelayTCP(sessionID, targetAddr string, clientConn net.Conn) error {
	// 连接到目标服务器
	serverConn, err := rm.connectWithRetry(targetAddr)
	if err != nil {
		return fmt.Errorf("连接目标服务器失败: %w", err)
	}

	// 创建中继会话
	_, err = rm.CreateRelay(sessionID, clientConn, serverConn)
	if err != nil {
		serverConn.Close()
		return fmt.Errorf("创建中继会话失败: %w", err)
	}

	return nil
}

// connectWithRetry 带重试的连接
func (rm *RelayManager) connectWithRetry(addr string) (net.Conn, error) {
	var lastErr error

	for attempt := 0; attempt < rm.config.RetryAttempts; attempt++ {
		conn, err := net.DialTimeout("tcp", addr, rm.config.WriteTimeout)
		if err == nil {
			return conn, nil
		}

		lastErr = err
		if attempt < rm.config.RetryAttempts-1 {
			time.Sleep(rm.config.RetryDelay)
		}
	}

	return nil, fmt.Errorf("连接失败，已重试 %d 次: %w", rm.config.RetryAttempts, lastErr)
}

// cleanupLoop 清理循环
func (rm *RelayManager) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	rm.logger.Debug("启动中继清理循环")

	for {
		select {
		case <-rm.ctx.Done():
			rm.logger.Debug("停止中继清理循环")
			return
		case <-ticker.C:
			rm.cleanup()
		}
	}
}

// cleanup 清理已关闭的会话
func (rm *RelayManager) cleanup() {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	var closedSessions []string

	for sessionID, session := range rm.sessions {
		if session.closed.Load() {
			closedSessions = append(closedSessions, sessionID)
		} else if session.isIdleTimeout() {
			rm.logger.Debug("会话空闲超时", zap.String("session_id", sessionID))
			session.Stop()
			closedSessions = append(closedSessions, sessionID)
		}
	}

	// 移除已关闭的会话
	for _, sessionID := range closedSessions {
		if session, exists := rm.sessions[sessionID]; exists {
			rm.totalBytesRelay.Add(session.BytesAtoB.Load() + session.BytesBtoA.Load())
			delete(rm.sessions, sessionID)
			rm.activeSessions.Add(-1)
		}
	}

	if len(closedSessions) > 0 {
		rm.logger.Debug("清理中继会话", zap.Int("count", len(closedSessions)))
	}
}

// GetStats 获取管理器统计信息
func (rm *RelayManager) GetStats() map[string]interface{} {
	rm.mutex.RLock()
	activeCount := len(rm.sessions)
	sessionStats := make(map[string]interface{})

	for id, session := range rm.sessions {
		sessionStats[id] = session.GetStats()
	}
	rm.mutex.RUnlock()

	return map[string]interface{}{
		"total_sessions":  rm.totalSessions.Load(),
		"active_sessions": activeCount,
		"total_bytes":     rm.totalBytesRelay.Load(),
		"sessions":        sessionStats,
		"config": map[string]interface{}{
			"buffer_size":     rm.config.BufferSize,
			"read_timeout":    rm.config.ReadTimeout.String(),
			"write_timeout":   rm.config.WriteTimeout.String(),
			"idle_timeout":    rm.config.IdleTimeout.String(),
			"max_connections": rm.config.MaxConnections,
			"keep_alive":      rm.config.KeepAlive,
			"retry_attempts":  rm.config.RetryAttempts,
			"retry_delay":     rm.config.RetryDelay.String(),
		},
	}
}

// SimpleRelay 简单的静态中继函数
func SimpleRelay(connA, connB net.Conn, bufferSize int) error {
	var wg sync.WaitGroup
	var once sync.Once

	copyData := func(dst, src net.Conn, direction string) {
		defer wg.Done()

		buffer := make([]byte, bufferSize)
		_, err := io.CopyBuffer(dst, src, buffer)
		if err != nil && err != io.EOF {
			zap.L().Debug("数据复制结束",
				zap.String("direction", direction),
				zap.Error(err))
		}

		// 关闭连接（只执行一次）
		once.Do(func() {
			connA.Close()
			connB.Close()
		})
	}

	wg.Add(2)
	go copyData(connB, connA, "A->B")
	go copyData(connA, connB, "B->A")

	wg.Wait()
	return nil
}





