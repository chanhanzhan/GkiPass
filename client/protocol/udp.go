package protocol

import (
	"fmt"
	"net"
	"sync"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// UDPSession UDP会话
type UDPSession struct {
	SrcAddr    *net.UDPAddr
	DstAddr    *net.UDPAddr
	SessionID  string
	LastActive time.Time
	Conn       *net.UDPConn
	mu         sync.Mutex
}

// Touch 更新最后活跃时间
func (s *UDPSession) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActive = time.Now()
}

// IsExpired 检查是否过期
func (s *UDPSession) IsExpired(timeout time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.LastActive) > timeout
}

// Close 关闭会话
func (s *UDPSession) Close() error {
	if s.Conn != nil {
		return s.Conn.Close()
	}
	return nil
}

// UDPSessionManager UDP会话管理器
type UDPSessionManager struct {
	sessions map[string]*UDPSession // 5-tuple hash → session
	mu       sync.RWMutex
	timeout  time.Duration
	stopChan chan struct{}
}

// NewUDPSessionManager 创建UDP会话管理器
func NewUDPSessionManager(timeout time.Duration) *UDPSessionManager {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	mgr := &UDPSessionManager{
		sessions: make(map[string]*UDPSession),
		timeout:  timeout,
		stopChan: make(chan struct{}),
	}

	// 启动清理协程
	go mgr.cleanupLoop()

	return mgr
}

// GetOrCreate 获取或创建会话
func (usm *UDPSessionManager) GetOrCreate(srcAddr, dstAddr *net.UDPAddr) (*UDPSession, error) {
	key := usm.makeKey(srcAddr, dstAddr)

	// 先尝试获取现有会话
	usm.mu.RLock()
	if session, ok := usm.sessions[key]; ok {
		usm.mu.RUnlock()
		session.Touch()
		return session, nil
	}
	usm.mu.RUnlock()

	// 创建新会话
	session := &UDPSession{
		SrcAddr:    srcAddr,
		DstAddr:    dstAddr,
		SessionID:  generateSessionID(),
		LastActive: time.Now(),
	}

	// 创建UDP连接
	conn, err := net.DialUDP("udp", nil, dstAddr)
	if err != nil {
		return nil, fmt.Errorf("创建UDP连接失败: %w", err)
	}
	session.Conn = conn

	// 保存会话
	usm.mu.Lock()
	usm.sessions[key] = session
	usm.mu.Unlock()

	logger.Debug("创建UDP会话",
		zap.String("session_id", session.SessionID),
		zap.String("src", srcAddr.String()),
		zap.String("dst", dstAddr.String()))

	return session, nil
}

// Remove 移除会话
func (usm *UDPSessionManager) Remove(srcAddr, dstAddr *net.UDPAddr) {
	key := usm.makeKey(srcAddr, dstAddr)

	usm.mu.Lock()
	defer usm.mu.Unlock()

	if session, ok := usm.sessions[key]; ok {
		session.Close()
		delete(usm.sessions, key)
		logger.Debug("移除UDP会话", zap.String("session_id", session.SessionID))
	}
}

// makeKey 生成五元组哈希键
func (usm *UDPSessionManager) makeKey(srcAddr, dstAddr *net.UDPAddr) string {
	return fmt.Sprintf("%s:%d-%s:%d-udp",
		srcAddr.IP.String(), srcAddr.Port,
		dstAddr.IP.String(), dstAddr.Port)
}

// cleanupLoop 清理过期会话循环
func (usm *UDPSessionManager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			usm.cleanup()
		case <-usm.stopChan:
			return
		}
	}
}

// cleanup 清理过期会话
func (usm *UDPSessionManager) cleanup() {
	usm.mu.Lock()
	defer usm.mu.Unlock()

	expiredCount := 0

	for key, session := range usm.sessions {
		if session.IsExpired(usm.timeout) {
			session.Close()
			delete(usm.sessions, key)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		logger.Debug("清理过期UDP会话", zap.Int("count", expiredCount))
	}
}

// Close 关闭管理器
func (usm *UDPSessionManager) Close() {
	close(usm.stopChan)

	usm.mu.Lock()
	defer usm.mu.Unlock()

	for _, session := range usm.sessions {
		session.Close()
	}
	usm.sessions = nil

	logger.Info("UDP会话管理器已关闭")
}

// GetSessionCount 获取会话数量
func (usm *UDPSessionManager) GetSessionCount() int {
	usm.mu.RLock()
	defer usm.mu.RUnlock()
	return len(usm.sessions)
}

// generateSessionID 生成会话ID
func generateSessionID() string {
	return fmt.Sprintf("udp-%d", time.Now().UnixNano())
}
