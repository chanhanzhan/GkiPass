package udp

import (
	"context"
	"crypto/md5"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// SessionConfig UDP会话配置
type SessionConfig struct {
	// 超时配置
	IdleTimeout     time.Duration `json:"idle_timeout"`     // 空闲超时
	SessionTimeout  time.Duration `json:"session_timeout"`  // 会话最大生存时间
	CleanupInterval time.Duration `json:"cleanup_interval"` // 清理间隔

	// 缓冲区配置
	BufferSize  int `json:"buffer_size"`  // 缓冲区大小
	MaxSessions int `json:"max_sessions"` // 最大会话数

	// NAT配置
	NATPortRange [2]int `json:"nat_port_range"` // NAT端口范围 [开始, 结束]
	EnableNAT    bool   `json:"enable_nat"`     // 是否启用NAT
}

// DefaultSessionConfig 默认配置
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		IdleTimeout:     30 * time.Second,
		SessionTimeout:  10 * time.Minute,
		CleanupInterval: 10 * time.Second,
		BufferSize:      65536,
		MaxSessions:     10000,
		NATPortRange:    [2]int{20000, 30000},
		EnableNAT:       true,
	}
}

// Manager UDP会话管理器
type Manager struct {
	config   *SessionConfig
	sessions map[string]*UDPSession // sessionID -> session
	tupleMap map[string]string      // tuple hash -> sessionID
	natTable *NATTable

	// 统计信息
	totalSessions   atomic.Int64
	activeSessions  atomic.Int64
	expiredSessions atomic.Int64
	createdSessions atomic.Int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mutex  sync.RWMutex

	// NAT端口分配
	natPortCounter atomic.Int32
	natPortInUse   map[int]bool
	natMutex       sync.RWMutex

	logger *zap.Logger
}

// NewManager 创建UDP会话管理器
func NewManager(config *SessionConfig) *Manager {
	if config == nil {
		config = DefaultSessionConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		config:       config,
		sessions:     make(map[string]*UDPSession),
		tupleMap:     make(map[string]string),
		natTable:     NewNATTable(),
		ctx:          ctx,
		cancel:       cancel,
		natPortInUse: make(map[int]bool),
		logger:       zap.L().Named("udp-manager"),
	}

	// 初始化NAT端口计数器
	m.natPortCounter.Store(int32(config.NATPortRange[0]))

	return m
}

// Start 启动UDP会话管理器
func (m *Manager) Start() error {
	m.logger.Info("启动UDP会话管理器",
		zap.Duration("idle_timeout", m.config.IdleTimeout),
		zap.Duration("session_timeout", m.config.SessionTimeout),
		zap.Int("max_sessions", m.config.MaxSessions))

	// 启动清理协程
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.cleanupLoop()
	}()

	return nil
}

// Stop 停止UDP会话管理器
func (m *Manager) Stop() error {
	m.logger.Info("停止UDP会话管理器")

	// 取消上下文
	if m.cancel != nil {
		m.cancel()
	}

	// 停止所有会话
	m.mutex.Lock()
	sessions := make([]*UDPSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mutex.Unlock()

	for _, session := range sessions {
		session.Stop()
	}

	// 等待清理协程结束
	m.wg.Wait()

	// 清理资源
	m.mutex.Lock()
	m.sessions = make(map[string]*UDPSession)
	m.tupleMap = make(map[string]string)
	m.natTable.Clear()
	m.mutex.Unlock()

	m.logger.Info("UDP会话管理器已停止")
	return nil
}

// GetSession 获取或创建UDP会话
func (m *Manager) GetSession(tuple FiveTuple, clientAddr, serverAddr *net.UDPAddr) (*UDPSession, error) {
	tupleHash := tuple.Hash()

	// 先尝试获取已存在的会话
	m.mutex.RLock()
	if sessionID, exists := m.tupleMap[tupleHash]; exists {
		if session, ok := m.sessions[sessionID]; ok && session.IsActive() {
			m.mutex.RUnlock()
			m.logger.Debug("复用已存在的UDP会话", zap.String("session_id", sessionID))
			return session, nil
		}
	}
	m.mutex.RUnlock()

	// 检查会话数限制
	if int(m.activeSessions.Load()) >= m.config.MaxSessions {
		return nil, fmt.Errorf("会话数已达上限: %d", m.config.MaxSessions)
	}

	// 创建新会话
	sessionID := m.generateSessionID(tuple)
	session := NewUDPSession(sessionID, tuple, clientAddr, serverAddr)

	// 添加到管理器
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.sessions[sessionID] = session
	m.tupleMap[tupleHash] = sessionID

	// 启动会话
	if err := session.Start(); err != nil {
		delete(m.sessions, sessionID)
		delete(m.tupleMap, tupleHash)
		return nil, fmt.Errorf("启动会话失败: %w", err)
	}

	// 更新统计
	m.activeSessions.Add(1)
	m.createdSessions.Add(1)
	m.totalSessions.Add(1)

	m.logger.Info("创建新UDP会话",
		zap.String("session_id", sessionID),
		zap.String("tuple", tuple.String()),
		zap.String("client_addr", clientAddr.String()),
		zap.String("server_addr", serverAddr.String()))

	return session, nil
}

// RemoveSession 移除UDP会话
func (m *Manager) RemoveSession(sessionID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	// 停止会话
	session.Stop()

	// 从映射中移除
	tupleHash := session.GetTuple().Hash()
	delete(m.sessions, sessionID)
	delete(m.tupleMap, tupleHash)

	// 移除NAT映射
	m.natTable.RemoveMapping(session.GetTuple())

	// 更新统计
	if session.IsActive() {
		m.activeSessions.Add(-1)
	}

	m.logger.Debug("移除UDP会话", zap.String("session_id", sessionID))
	return nil
}

// GetSessionByID 根据ID获取会话
func (m *Manager) GetSessionByID(sessionID string) (*UDPSession, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	session, exists := m.sessions[sessionID]
	return session, exists
}

// GetSessionByTuple 根据五元组获取会话
func (m *Manager) GetSessionByTuple(tuple FiveTuple) (*UDPSession, bool) {
	tupleHash := tuple.Hash()

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if sessionID, exists := m.tupleMap[tupleHash]; exists {
		if session, ok := m.sessions[sessionID]; ok {
			return session, true
		}
	}

	return nil, false
}

// AllocateNATPort 分配NAT端口
func (m *Manager) AllocateNATPort() (int, error) {
	if !m.config.EnableNAT {
		return 0, fmt.Errorf("NAT未启用")
	}

	m.natMutex.Lock()
	defer m.natMutex.Unlock()

	start := m.config.NATPortRange[0]
	end := m.config.NATPortRange[1]

	// 从当前计数器开始查找可用端口
	current := int(m.natPortCounter.Load())
	for i := 0; i < (end - start + 1); i++ {
		port := start + (current-start+i)%(end-start+1)

		if !m.natPortInUse[port] {
			m.natPortInUse[port] = true
			m.natPortCounter.Store(int32(port + 1))

			m.logger.Debug("分配NAT端口", zap.Int("port", port))
			return port, nil
		}
	}

	return 0, fmt.Errorf("无可用NAT端口")
}

// ReleaseNATPort 释放NAT端口
func (m *Manager) ReleaseNATPort(port int) {
	if !m.config.EnableNAT {
		return
	}

	m.natMutex.Lock()
	defer m.natMutex.Unlock()

	delete(m.natPortInUse, port)
	m.logger.Debug("释放NAT端口", zap.Int("port", port))
}

// CreateNATMapping 创建NAT映射
func (m *Manager) CreateNATMapping(original FiveTuple, targetIP net.IP) (FiveTuple, error) {
	if !m.config.EnableNAT {
		return original, nil
	}

	// 分配NAT端口
	natPort, err := m.AllocateNATPort()
	if err != nil {
		return FiveTuple{}, fmt.Errorf("分配NAT端口失败: %w", err)
	}

	// 创建映射的五元组
	mapped := FiveTuple{
		SrcIP:   targetIP,
		SrcPort: uint16(natPort),
		DstIP:   original.DstIP,
		DstPort: original.DstPort,
		Proto:   original.Proto,
	}

	// 添加到NAT表
	m.natTable.AddMapping(original, mapped)

	m.logger.Debug("创建NAT映射",
		zap.String("original", original.String()),
		zap.String("mapped", mapped.String()))

	return mapped, nil
}

// cleanupLoop 清理循环
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	m.logger.Debug("启动UDP会话清理循环")

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Debug("停止UDP会话清理循环")
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

// cleanup 清理过期会话
func (m *Manager) cleanup() {
	now := time.Now()
	var expiredSessions []string

	m.mutex.RLock()
	for sessionID, session := range m.sessions {
		lastActivity := session.GetLastActivity()
		sessionAge := now.Sub(session.createdAt)

		// 检查空闲超时
		if now.Sub(lastActivity) > m.config.IdleTimeout {
			expiredSessions = append(expiredSessions, sessionID)
			continue
		}

		// 检查会话超时
		if sessionAge > m.config.SessionTimeout {
			expiredSessions = append(expiredSessions, sessionID)
			continue
		}

		// 检查会话状态
		if !session.IsActive() {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}
	m.mutex.RUnlock()

	// 移除过期会话
	for _, sessionID := range expiredSessions {
		if err := m.RemoveSession(sessionID); err != nil {
			m.logger.Error("移除过期会话失败",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			m.expiredSessions.Add(1)
		}
	}

	if len(expiredSessions) > 0 {
		m.logger.Debug("清理过期会话", zap.Int("count", len(expiredSessions)))
	}
}

// generateSessionID 生成会话ID
func (m *Manager) generateSessionID(tuple FiveTuple) string {
	data := fmt.Sprintf("%s:%d-%s:%d-%d-%d",
		tuple.SrcIP.String(), tuple.SrcPort,
		tuple.DstIP.String(), tuple.DstPort,
		tuple.Proto, time.Now().UnixNano())

	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)[:16]
}

// GetAllSessions 获取所有会话
func (m *Manager) GetAllSessions() map[string]*UDPSession {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	sessions := make(map[string]*UDPSession, len(m.sessions))
	for id, session := range m.sessions {
		sessions[id] = session
	}

	return sessions
}

// GetStats 获取管理器统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	activeCount := len(m.sessions)
	m.mutex.RUnlock()

	m.natMutex.RLock()
	natPortsInUse := len(m.natPortInUse)
	m.natMutex.RUnlock()

	return map[string]interface{}{
		"total_sessions":   m.totalSessions.Load(),
		"active_sessions":  activeCount,
		"expired_sessions": m.expiredSessions.Load(),
		"created_sessions": m.createdSessions.Load(),
		"nat_ports_in_use": natPortsInUse,
		"nat_stats":        m.natTable.GetStats(),
		"config": map[string]interface{}{
			"idle_timeout":     m.config.IdleTimeout.String(),
			"session_timeout":  m.config.SessionTimeout.String(),
			"cleanup_interval": m.config.CleanupInterval.String(),
			"max_sessions":     m.config.MaxSessions,
			"enable_nat":       m.config.EnableNAT,
			"nat_port_range":   m.config.NATPortRange,
		},
	}
}

// ParseFiveTuple 从网络地址解析五元组
func ParseFiveTuple(localAddr, remoteAddr net.Addr, protocol string) (FiveTuple, error) {
	var tuple FiveTuple

	// 解析协议
	switch strings.ToLower(protocol) {
	case "udp":
		tuple.Proto = 17
	case "tcp":
		tuple.Proto = 6
	default:
		return tuple, fmt.Errorf("不支持的协议: %s", protocol)
	}

	// 解析本地地址
	if udpAddr, ok := localAddr.(*net.UDPAddr); ok {
		tuple.SrcIP = udpAddr.IP
		tuple.SrcPort = uint16(udpAddr.Port)
	} else if tcpAddr, ok := localAddr.(*net.TCPAddr); ok {
		tuple.SrcIP = tcpAddr.IP
		tuple.SrcPort = uint16(tcpAddr.Port)
	} else {
		// 尝试解析字符串形式的地址
		addr := localAddr.String()
		if host, portStr, err := net.SplitHostPort(addr); err == nil {
			if ip := net.ParseIP(host); ip != nil {
				tuple.SrcIP = ip
				if port, err := strconv.Atoi(portStr); err == nil {
					tuple.SrcPort = uint16(port)
				}
			}
		}
		if tuple.SrcIP == nil {
			return tuple, fmt.Errorf("无法解析本地地址: %s", localAddr.String())
		}
	}

	// 解析远程地址
	if udpAddr, ok := remoteAddr.(*net.UDPAddr); ok {
		tuple.DstIP = udpAddr.IP
		tuple.DstPort = uint16(udpAddr.Port)
	} else if tcpAddr, ok := remoteAddr.(*net.TCPAddr); ok {
		tuple.DstIP = tcpAddr.IP
		tuple.DstPort = uint16(tcpAddr.Port)
	} else {
		// 尝试解析字符串形式的地址
		addr := remoteAddr.String()
		if host, portStr, err := net.SplitHostPort(addr); err == nil {
			if ip := net.ParseIP(host); ip != nil {
				tuple.DstIP = ip
				if port, err := strconv.Atoi(portStr); err == nil {
					tuple.DstPort = uint16(port)
				}
			}
		}
		if tuple.DstIP == nil {
			return tuple, fmt.Errorf("无法解析远程地址: %s", remoteAddr.String())
		}
	}

	return tuple, nil
}





