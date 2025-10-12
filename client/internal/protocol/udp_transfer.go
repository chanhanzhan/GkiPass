package protocol

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// UDPSessionOptions UDP会话选项
type UDPSessionOptions struct {
	// 连接超时
	SessionTimeout time.Duration
	IdleTimeout    time.Duration

	// 缓冲区大小
	BufferSize int

	// 流量控制
	RateLimit   int // 每秒包数限制
	PacketLimit int // 最大包大小

	// 统计和监控
	CollectStats bool
	LogTraffic   bool
}

// DefaultUDPSessionOptions 默认UDP会话选项
func DefaultUDPSessionOptions() *UDPSessionOptions {
	return &UDPSessionOptions{
		SessionTimeout: 5 * time.Minute,
		IdleTimeout:    60 * time.Second,
		BufferSize:     65536, // 64KB
		RateLimit:      0,     // 无限制
		PacketLimit:    8192,  // 8KB
		CollectStats:   true,
		LogTraffic:     false,
	}
}

// UDPSessionStats UDP会话统计
type UDPSessionStats struct {
	// 会话信息
	SessionID    string
	CreatedAt    time.Time
	LastActivity atomic.Int64 // Unix时间戳

	// 流量统计
	PacketsIn  atomic.Int64
	PacketsOut atomic.Int64
	BytesIn    atomic.Int64
	BytesOut   atomic.Int64

	// 错误计数
	ReadErrors    atomic.Int64
	WriteErrors   atomic.Int64
	TimeoutErrors atomic.Int64
}

// UDPSession UDP会话
type UDPSession struct {
	// 会话标识
	sessionID  string
	clientAddr *net.UDPAddr
	serverAddr *net.UDPAddr

	// 连接
	clientConn *net.UDPConn
	serverConn *net.UDPConn

	// 配置
	options *UDPSessionOptions

	// 统计
	stats UDPSessionStats

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed atomic.Bool

	// 日志
	logger *zap.Logger
}

// NewUDPSession 创建UDP会话
func NewUDPSession(clientConn *net.UDPConn, clientAddr, serverAddr *net.UDPAddr, options *UDPSessionOptions) *UDPSession {
	if options == nil {
		options = DefaultUDPSessionOptions()
	}

	ctx, cancel := context.WithCancel(context.Background())

	sessionID := fmt.Sprintf("%s-%s", clientAddr.String(), serverAddr.String())

	session := &UDPSession{
		sessionID:  sessionID,
		clientAddr: clientAddr,
		serverAddr: serverAddr,
		clientConn: clientConn,
		options:    options,
		ctx:        ctx,
		cancel:     cancel,
		logger:     zap.L().Named("udp-session"),
	}

	// 初始化统计
	session.stats.SessionID = sessionID
	session.stats.CreatedAt = time.Now()
	session.stats.LastActivity.Store(time.Now().Unix())

	return session
}

// Start 启动UDP会话
func (s *UDPSession) Start() error {
	// 连接到目标服务器
	var err error
	s.serverConn, err = net.DialUDP("udp", nil, s.serverAddr)
	if err != nil {
		return fmt.Errorf("连接UDP服务器失败: %w", err)
	}

	s.logger.Debug("UDP会话启动",
		zap.String("session_id", s.sessionID),
		zap.String("client", s.clientAddr.String()),
		zap.String("server", s.serverAddr.String()))

	// 启动转发协程
	s.wg.Add(2)
	go s.clientToServer()
	go s.serverToClient()

	// 启动超时检查
	go s.checkTimeout()

	return nil
}

// Stop 停止UDP会话
func (s *UDPSession) Stop() {
	if s.closed.Swap(true) {
		return // 已经关闭
	}

	s.logger.Debug("停止UDP会话",
		zap.String("session_id", s.sessionID))

	// 取消上下文
	s.cancel()

	// 关闭服务器连接
	if s.serverConn != nil {
		s.serverConn.Close()
	}

	// 等待协程结束
	s.wg.Wait()

	s.logger.Debug("UDP会话已停止",
		zap.String("session_id", s.sessionID),
		zap.Int64("packets_in", s.stats.PacketsIn.Load()),
		zap.Int64("packets_out", s.stats.PacketsOut.Load()),
		zap.Int64("bytes_in", s.stats.BytesIn.Load()),
		zap.Int64("bytes_out", s.stats.BytesOut.Load()))
}

// clientToServer 客户端到服务器数据转发
func (s *UDPSession) clientToServer() {
	defer s.wg.Done()

	buffer := make([]byte, s.options.BufferSize)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			// 从客户端读取数据
			s.clientConn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, addr, err := s.clientConn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				s.logger.Error("从客户端读取UDP数据失败",
					zap.String("session_id", s.sessionID),
					zap.Error(err))
				s.stats.ReadErrors.Inc()
				continue
			}

			// 检查地址是否匹配
			if addr.String() != s.clientAddr.String() {
				s.logger.Debug("忽略非客户端地址的UDP数据",
					zap.String("session_id", s.sessionID),
					zap.String("expected", s.clientAddr.String()),
					zap.String("actual", addr.String()))
				continue
			}

			// 更新活动时间
			s.stats.LastActivity.Store(time.Now().Unix())

			// 发送到服务器
			_, err = s.serverConn.Write(buffer[:n])
			if err != nil {
				s.logger.Error("发送UDP数据到服务器失败",
					zap.String("session_id", s.sessionID),
					zap.Error(err))
				s.stats.WriteErrors.Inc()
				continue
			}

			// 更新统计
			s.stats.PacketsIn.Inc()
			s.stats.BytesIn.Add(int64(n))

			if s.options.LogTraffic {
				s.logger.Debug("UDP数据: 客户端->服务器",
					zap.String("session_id", s.sessionID),
					zap.Int("bytes", n),
					zap.String("client", s.clientAddr.String()),
					zap.String("server", s.serverAddr.String()))
			}
		}
	}
}

// serverToClient 服务器到客户端数据转发
func (s *UDPSession) serverToClient() {
	defer s.wg.Done()

	buffer := make([]byte, s.options.BufferSize)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			// 从服务器读取数据
			s.serverConn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, err := s.serverConn.Read(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				s.logger.Error("从服务器读取UDP数据失败",
					zap.String("session_id", s.sessionID),
					zap.Error(err))
				s.stats.ReadErrors.Inc()
				continue
			}

			// 更新活动时间
			s.stats.LastActivity.Store(time.Now().Unix())

			// 发送到客户端
			_, err = s.clientConn.WriteToUDP(buffer[:n], s.clientAddr)
			if err != nil {
				s.logger.Error("发送UDP数据到客户端失败",
					zap.String("session_id", s.sessionID),
					zap.Error(err))
				s.stats.WriteErrors.Inc()
				continue
			}

			// 更新统计
			s.stats.PacketsOut.Inc()
			s.stats.BytesOut.Add(int64(n))

			if s.options.LogTraffic {
				s.logger.Debug("UDP数据: 服务器->客户端",
					zap.String("session_id", s.sessionID),
					zap.Int("bytes", n),
					zap.String("client", s.clientAddr.String()),
					zap.String("server", s.serverAddr.String()))
			}
		}
	}
}

// checkTimeout 检查会话超时
func (s *UDPSession) checkTimeout() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// 检查空闲超时
			if s.options.IdleTimeout > 0 {
				lastActivity := time.Unix(s.stats.LastActivity.Load(), 0)
				if time.Since(lastActivity) > s.options.IdleTimeout {
					s.logger.Debug("UDP会话空闲超时",
						zap.String("session_id", s.sessionID),
						zap.Duration("idle_time", time.Since(lastActivity)))
					s.stats.TimeoutErrors.Inc()
					s.Stop()
					return
				}
			}

			// 检查会话超时
			if s.options.SessionTimeout > 0 {
				if time.Since(s.stats.CreatedAt) > s.options.SessionTimeout {
					s.logger.Debug("UDP会话到期",
						zap.String("session_id", s.sessionID),
						zap.Duration("session_time", time.Since(s.stats.CreatedAt)))
					s.stats.TimeoutErrors.Inc()
					s.Stop()
					return
				}
			}
		}
	}
}

// IsActive 会话是否活跃
func (s *UDPSession) IsActive() bool {
	return !s.closed.Load()
}

// GetStats 获取会话统计
func (s *UDPSession) GetStats() map[string]interface{} {
	duration := time.Since(s.stats.CreatedAt)
	lastActivity := time.Unix(s.stats.LastActivity.Load(), 0)
	idleTime := time.Since(lastActivity)

	packetsIn := s.stats.PacketsIn.Load()
	packetsOut := s.stats.PacketsOut.Load()
	bytesIn := s.stats.BytesIn.Load()
	bytesOut := s.stats.BytesOut.Load()

	return map[string]interface{}{
		"session_id":     s.sessionID,
		"client":         s.clientAddr.String(),
		"server":         s.serverAddr.String(),
		"created_at":     s.stats.CreatedAt,
		"last_activity":  lastActivity,
		"duration_sec":   duration.Seconds(),
		"idle_time_sec":  idleTime.Seconds(),
		"packets_in":     packetsIn,
		"packets_out":    packetsOut,
		"total_packets":  packetsIn + packetsOut,
		"bytes_in":       bytesIn,
		"bytes_out":      bytesOut,
		"total_bytes":    bytesIn + bytesOut,
		"read_errors":    s.stats.ReadErrors.Load(),
		"write_errors":   s.stats.WriteErrors.Load(),
		"timeout_errors": s.stats.TimeoutErrors.Load(),
		"is_active":      s.IsActive(),
	}
}

// UDPSessionManager UDP会话管理器
type UDPSessionManager struct {
	// 会话映射
	sessions sync.Map // key=sessionID, value=*UDPSession

	// 全局选项
	options *UDPSessionOptions

	// 统计
	activeCount  atomic.Int32
	totalCount   atomic.Int64
	expiredCount atomic.Int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 日志
	logger *zap.Logger
}

// NewUDPSessionManager 创建UDP会话管理器
func NewUDPSessionManager(options *UDPSessionOptions) *UDPSessionManager {
	if options == nil {
		options = DefaultUDPSessionOptions()
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &UDPSessionManager{
		options: options,
		ctx:     ctx,
		cancel:  cancel,
		logger:  zap.L().Named("udp-session-manager"),
	}

	return manager
}

// Start 启动会话管理器
func (m *UDPSessionManager) Start() error {
	// 启动清理任务
	m.wg.Add(1)
	go m.cleanupTask()

	m.logger.Info("UDP会话管理器启动")

	return nil
}

// Stop 停止会话管理器
func (m *UDPSessionManager) Stop() error {
	m.logger.Info("停止UDP会话管理器")

	// 取消上下文
	m.cancel()

	// 等待任务结束
	m.wg.Wait()

	// 停止所有会话
	m.sessions.Range(func(key, value interface{}) bool {
		session := value.(*UDPSession)
		session.Stop()
		return true
	})

	return nil
}

// GetOrCreateSession 获取或创建UDP会话
func (m *UDPSessionManager) GetOrCreateSession(clientConn *net.UDPConn, clientAddr, serverAddr *net.UDPAddr) *UDPSession {
	sessionID := fmt.Sprintf("%s-%s", clientAddr.String(), serverAddr.String())

	// 尝试获取现有会话
	if existing, ok := m.sessions.Load(sessionID); ok {
		session := existing.(*UDPSession)
		if session.IsActive() {
			return session
		}
	}

	// 创建新会话
	session := NewUDPSession(clientConn, clientAddr, serverAddr, m.options)
	m.sessions.Store(sessionID, session)

	// 更新计数
	m.activeCount.Inc()
	m.totalCount.Inc()

	// 启动会话
	if err := session.Start(); err != nil {
		m.logger.Error("启动UDP会话失败",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	return session
}

// GetSession 获取UDP会话
func (m *UDPSessionManager) GetSession(clientAddr, serverAddr *net.UDPAddr) *UDPSession {
	sessionID := fmt.Sprintf("%s-%s", clientAddr.String(), serverAddr.String())

	if existing, ok := m.sessions.Load(sessionID); ok {
		return existing.(*UDPSession)
	}

	return nil
}

// RemoveSession 移除UDP会话
func (m *UDPSessionManager) RemoveSession(sessionID string) {
	if session, ok := m.sessions.LoadAndDelete(sessionID); ok {
		udpSession := session.(*UDPSession)
		if udpSession.IsActive() {
			udpSession.Stop()
		}
		m.activeCount.Dec()
		m.expiredCount.Inc()
	}
}

// cleanupTask 清理过期会话
func (m *UDPSessionManager) cleanupTask() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupExpiredSessions()
		}
	}
}

// cleanupExpiredSessions 清理过期会话
func (m *UDPSessionManager) cleanupExpiredSessions() {
	var expiredCount int

	m.sessions.Range(func(key, value interface{}) bool {
		sessionID := key.(string)
		session := value.(*UDPSession)

		// 检查会话是否活跃
		if !session.IsActive() {
			m.sessions.Delete(sessionID)
			expiredCount++
			return true
		}

		// 检查空闲超时
		lastActivity := time.Unix(session.stats.LastActivity.Load(), 0)
		if m.options.IdleTimeout > 0 && time.Since(lastActivity) > m.options.IdleTimeout {
			m.logger.Debug("移除空闲UDP会话",
				zap.String("session_id", sessionID),
				zap.Duration("idle_time", time.Since(lastActivity)))
			session.Stop()
			m.sessions.Delete(sessionID)
			m.activeCount.Dec()
			m.expiredCount.Inc()
			expiredCount++
		}

		// 检查会话超时
		if m.options.SessionTimeout > 0 && time.Since(session.stats.CreatedAt) > m.options.SessionTimeout {
			m.logger.Debug("移除过期UDP会话",
				zap.String("session_id", sessionID),
				zap.Duration("session_time", time.Since(session.stats.CreatedAt)))
			session.Stop()
			m.sessions.Delete(sessionID)
			m.activeCount.Dec()
			m.expiredCount.Inc()
			expiredCount++
		}

		return true
	})

	if expiredCount > 0 {
		m.logger.Debug("清理过期UDP会话",
			zap.Int("expired_count", expiredCount),
			zap.Int32("active_count", m.activeCount.Load()))
	}
}

// GetStats 获取管理器统计
func (m *UDPSessionManager) GetStats() map[string]interface{} {
	var activeSessions []*UDPSession

	m.sessions.Range(func(key, value interface{}) bool {
		session := value.(*UDPSession)
		if session.IsActive() {
			activeSessions = append(activeSessions, session)
		}
		return true
	})

	// 汇总活跃会话统计
	var totalBytesIn, totalBytesOut int64
	var totalPacketsIn, totalPacketsOut int64

	for _, session := range activeSessions {
		totalBytesIn += session.stats.BytesIn.Load()
		totalBytesOut += session.stats.BytesOut.Load()
		totalPacketsIn += session.stats.PacketsIn.Load()
		totalPacketsOut += session.stats.PacketsOut.Load()
	}

	return map[string]interface{}{
		"active_sessions":   m.activeCount.Load(),
		"total_sessions":    m.totalCount.Load(),
		"expired_sessions":  m.expiredCount.Load(),
		"total_bytes_in":    totalBytesIn,
		"total_bytes_out":   totalBytesOut,
		"total_packets_in":  totalPacketsIn,
		"total_packets_out": totalPacketsOut,
	}
}
