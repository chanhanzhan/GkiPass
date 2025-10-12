package multiplex

import (
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Manager 多路复用管理器
type Manager struct {
	sessions   map[string]*Session
	sessionsMu sync.RWMutex
	config     *SessionConfig
	logger     *zap.Logger

	// 统计信息
	stats struct {
		totalSessions    int64
		activeSessions   int64
		totalStreams     int64
		activeStreams    int64
		bytesTransferred int64
	}
}

// New 创建多路复用管理器 (通用接口)
func New() *Manager {
	return NewManager(nil)
}

// NewManager 创建多路复用管理器
func NewManager(config *SessionConfig) *Manager {
	if config == nil {
		config = DefaultSessionConfig()
	}

	return &Manager{
		sessions: make(map[string]*Session),
		config:   config,
		logger:   zap.L().Named("multiplex-manager"),
	}
}

// CreateSession 创建新会话
func (m *Manager) CreateSession(conn net.Conn, isClient bool) (*Session, error) {
	sessionKey := fmt.Sprintf("%s-%s-%t",
		conn.LocalAddr().String(),
		conn.RemoteAddr().String(),
		isClient)

	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	// 检查是否已存在
	if existing, exists := m.sessions[sessionKey]; exists {
		return existing, nil
	}

	// 创建新会话
	session := NewSession(conn, m.config, isClient)
	m.sessions[sessionKey] = session

	// 启动会话
	if err := session.Start(); err != nil {
		delete(m.sessions, sessionKey)
		return nil, err
	}

	m.stats.totalSessions++
	m.stats.activeSessions++

	m.logger.Info("创建多路复用会话",
		zap.String("key", sessionKey),
		zap.Bool("is_client", isClient))

	// 监控会话状态
	go m.monitorSession(sessionKey, session)

	return session, nil
}

// GetSession 获取会话
func (m *Manager) GetSession(sessionKey string) (*Session, bool) {
	m.sessionsMu.RLock()
	defer m.sessionsMu.RUnlock()

	session, exists := m.sessions[sessionKey]
	return session, exists
}

// GetOrCreateSession 获取或创建会话
func (m *Manager) GetOrCreateSession(conn net.Conn, isClient bool) (*Session, error) {
	sessionKey := fmt.Sprintf("%s-%s-%t",
		conn.LocalAddr().String(),
		conn.RemoteAddr().String(),
		isClient)

	// 先尝试获取现有会话
	if session, exists := m.GetSession(sessionKey); exists && !session.IsClosed() {
		return session, nil
	}

	// 创建新会话
	return m.CreateSession(conn, isClient)
}

// CloseSession 关闭会话
func (m *Manager) CloseSession(sessionKey string) error {
	m.sessionsMu.Lock()
	session, exists := m.sessions[sessionKey]
	if exists {
		delete(m.sessions, sessionKey)
		m.stats.activeSessions--
	}
	m.sessionsMu.Unlock()

	if exists {
		return session.Close()
	}

	return nil
}

// CloseAllSessions 关闭所有会话
func (m *Manager) CloseAllSessions() error {
	m.sessionsMu.Lock()
	sessions := make(map[string]*Session)
	for k, v := range m.sessions {
		sessions[k] = v
	}
	m.sessions = make(map[string]*Session)
	m.stats.activeSessions = 0
	m.sessionsMu.Unlock()

	var lastErr error
	for key, session := range sessions {
		if err := session.Close(); err != nil {
			m.logger.Error("关闭会话失败",
				zap.String("key", key),
				zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}

// monitorSession 监控会话状态
func (m *Manager) monitorSession(sessionKey string, session *Session) {
	defer func() {
		m.sessionsMu.Lock()
		delete(m.sessions, sessionKey)
		m.stats.activeSessions--
		m.sessionsMu.Unlock()

		m.logger.Info("会话已移除", zap.String("key", sessionKey))
	}()

	// 等待会话关闭
	<-session.ctx.Done()
}

// GetStats 获取管理器统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.sessionsMu.RLock()
	activeSessions := len(m.sessions)

	// 收集所有会话的统计信息
	var totalActiveStreams int64
	var totalBytesTransferred int64
	sessionStats := make(map[string]interface{})

	for key, session := range m.sessions {
		stats := session.GetStats()
		sessionStats[key] = stats

		if activeStreams, ok := stats["active_streams"].(int); ok {
			totalActiveStreams += int64(activeStreams)
		}

		if bytesSent, ok := stats["bytes_sent"].(int64); ok {
			totalBytesTransferred += bytesSent
		}

		if bytesReceived, ok := stats["bytes_received"].(int64); ok {
			totalBytesTransferred += bytesReceived
		}
	}
	m.sessionsMu.RUnlock()

	return map[string]interface{}{
		"total_sessions":    m.stats.totalSessions,
		"active_sessions":   int64(activeSessions),
		"active_streams":    totalActiveStreams,
		"bytes_transferred": totalBytesTransferred,
		"session_stats":     sessionStats,
		"config": map[string]interface{}{
			"initial_window_size":    m.config.InitialWindowSize,
			"max_frame_size":         m.config.MaxFrameSize,
			"max_concurrent_streams": m.config.MaxConcurrentStreams,
			"stream_idle_timeout":    m.config.StreamIdleTimeout.String(),
			"ping_interval":          m.config.PingInterval.String(),
			"enable_flow_control":    m.config.EnableFlowControl,
		},
	}
}

// Start 启动多路复用管理器
func (m *Manager) Start() error {
	m.logger.Info("启动多路复用管理器")
	return nil
}

// Stop 停止多路复用管理器
func (m *Manager) Stop() error {
	m.logger.Info("停止多路复用管理器")
	return m.CloseAllSessions()
}

// MultiplexedConn 多路复用连接包装器
type MultiplexedConn struct {
	stream  *Stream
	session *Session
}

// NewMultiplexedConn 创建多路复用连接
func NewMultiplexedConn(conn net.Conn, isClient bool, config *SessionConfig) (*MultiplexedConn, error) {
	manager := NewManager(config)
	session, err := manager.CreateSession(conn, isClient)
	if err != nil {
		return nil, err
	}

	stream, err := session.OpenStream()
	if err != nil {
		session.Close()
		return nil, err
	}

	return &MultiplexedConn{
		stream:  stream,
		session: session,
	}, nil
}

// Read 实现 net.Conn 接口
func (mc *MultiplexedConn) Read(b []byte) (n int, err error) {
	return mc.stream.Read(b)
}

// Write 实现 net.Conn 接口
func (mc *MultiplexedConn) Write(b []byte) (n int, err error) {
	return mc.stream.Write(b)
}

// Close 实现 net.Conn 接口
func (mc *MultiplexedConn) Close() error {
	return mc.stream.Close()
}

// LocalAddr 实现 net.Conn 接口
func (mc *MultiplexedConn) LocalAddr() net.Addr {
	return mc.session.conn.LocalAddr()
}

// RemoteAddr 实现 net.Conn 接口
func (mc *MultiplexedConn) RemoteAddr() net.Addr {
	return mc.session.conn.RemoteAddr()
}

// SetDeadline 实现 net.Conn 接口
func (mc *MultiplexedConn) SetDeadline(t time.Time) error {
	// 多路复用连接的超时处理较复杂，这里简化实现
	return nil
}

// SetReadDeadline 实现 net.Conn 接口
func (mc *MultiplexedConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline 实现 net.Conn 接口
func (mc *MultiplexedConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// GetStream 获取底层流
func (mc *MultiplexedConn) GetStream() *Stream {
	return mc.stream
}

// GetSession 获取底层会话
func (mc *MultiplexedConn) GetSession() *Session {
	return mc.session
}

// OpenNewStream 在同一会话上打开新流
func (mc *MultiplexedConn) OpenNewStream() (*Stream, error) {
	return mc.session.OpenStream()
}

// AcceptStream 接受新流
func (mc *MultiplexedConn) AcceptStream() (*Stream, error) {
	return mc.session.AcceptStream()
}

// Ping 发送Ping
func (mc *MultiplexedConn) Ping(data []byte) error {
	return mc.session.Ping(data)
}

// GetStats 获取连接统计信息
func (mc *MultiplexedConn) GetStats() map[string]interface{} {
	sessionStats := mc.session.GetStats()
	streamStats := mc.stream.GetStats()

	return map[string]interface{}{
		"session": sessionStats,
		"stream":  streamStats,
	}
}

// ConnectionPool 多路复用连接池
type ConnectionPool struct {
	connections map[string]*MultiplexedConn
	mutex       sync.RWMutex
	config      *SessionConfig
	logger      *zap.Logger
}

// NewConnectionPool 创建连接池
func NewConnectionPool(config *SessionConfig) *ConnectionPool {
	return &ConnectionPool{
		connections: make(map[string]*MultiplexedConn),
		config:      config,
		logger:      zap.L().Named("multiplex-pool"),
	}
}

// GetConnection 获取到指定地址的连接
func (cp *ConnectionPool) GetConnection(address string, dialer func() (net.Conn, error)) (*MultiplexedConn, error) {
	cp.mutex.RLock()
	if conn, exists := cp.connections[address]; exists {
		if !conn.session.IsClosed() {
			cp.mutex.RUnlock()
			return conn, nil
		}
	}
	cp.mutex.RUnlock()

	// 创建新连接
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	// 双重检查
	if conn, exists := cp.connections[address]; exists {
		if !conn.session.IsClosed() {
			return conn, nil
		}
		delete(cp.connections, address)
	}

	// 拨号建立连接
	rawConn, err := dialer()
	if err != nil {
		return nil, err
	}

	// 创建多路复用连接
	conn, err := NewMultiplexedConn(rawConn, true, cp.config)
	if err != nil {
		rawConn.Close()
		return nil, err
	}

	cp.connections[address] = conn
	cp.logger.Info("创建新的多路复用连接", zap.String("address", address))

	// 监控连接状态
	go cp.monitorConnection(address, conn)

	return conn, nil
}

// monitorConnection 监控连接状态
func (cp *ConnectionPool) monitorConnection(address string, conn *MultiplexedConn) {
	<-conn.session.ctx.Done()

	cp.mutex.Lock()
	delete(cp.connections, address)
	cp.mutex.Unlock()

	cp.logger.Info("多路复用连接已移除", zap.String("address", address))
}

// CloseAll 关闭所有连接
func (cp *ConnectionPool) CloseAll() error {
	cp.mutex.Lock()
	connections := make(map[string]*MultiplexedConn)
	for k, v := range cp.connections {
		connections[k] = v
	}
	cp.connections = make(map[string]*MultiplexedConn)
	cp.mutex.Unlock()

	var lastErr error
	for address, conn := range connections {
		if err := conn.session.Close(); err != nil {
			cp.logger.Error("关闭连接失败",
				zap.String("address", address),
				zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}

// GetStats 获取连接池统计信息
func (cp *ConnectionPool) GetStats() map[string]interface{} {
	cp.mutex.RLock()
	defer cp.mutex.RUnlock()

	stats := map[string]interface{}{
		"active_connections": len(cp.connections),
		"connections":        make(map[string]interface{}),
	}

	for address, conn := range cp.connections {
		stats["connections"].(map[string]interface{})[address] = conn.GetStats()
	}

	return stats
}
