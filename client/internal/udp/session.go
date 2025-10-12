package udp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// FiveTuple UDP五元组标识
type FiveTuple struct {
	SrcIP   net.IP `json:"src_ip"`
	SrcPort uint16 `json:"src_port"`
	DstIP   net.IP `json:"dst_ip"`
	DstPort uint16 `json:"dst_port"`
	Proto   uint8  `json:"protocol"` // 通常是17 (UDP)
}

// String 返回五元组字符串表示
func (ft FiveTuple) String() string {
	return fmt.Sprintf("%s:%d->%s:%d/%d",
		ft.SrcIP.String(), ft.SrcPort,
		ft.DstIP.String(), ft.DstPort, ft.Proto)
}

// Hash 计算五元组哈希值
func (ft FiveTuple) Hash() string {
	return fmt.Sprintf("%s:%d-%s:%d-%d",
		ft.SrcIP.String(), ft.SrcPort,
		ft.DstIP.String(), ft.DstPort, ft.Proto)
}

// Reverse 返回反向五元组
func (ft FiveTuple) Reverse() FiveTuple {
	return FiveTuple{
		SrcIP:   ft.DstIP,
		SrcPort: ft.DstPort,
		DstIP:   ft.SrcIP,
		DstPort: ft.SrcPort,
		Proto:   ft.Proto,
	}
}

// UDPSession UDP会话
type UDPSession struct {
	id           string
	tuple        FiveTuple
	reverseTuple FiveTuple

	// 连接信息
	clientConn *net.UDPConn
	serverConn *net.UDPConn
	clientAddr *net.UDPAddr
	serverAddr *net.UDPAddr

	// 状态信息
	state        SessionState
	createdAt    time.Time
	lastActivity atomic.Int64 // Unix timestamp

	// 统计信息
	packetsIn  atomic.Int64
	packetsOut atomic.Int64
	bytesIn    atomic.Int64
	bytesOut   atomic.Int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mutex  sync.RWMutex

	logger *zap.Logger
}

// SessionState 会话状态
type SessionState int32

const (
	SessionStateNew SessionState = iota
	SessionStateActive
	SessionStateClosing
	SessionStateClosed
)

// String 返回会话状态名称
func (s SessionState) String() string {
	switch s {
	case SessionStateNew:
		return "NEW"
	case SessionStateActive:
		return "ACTIVE"
	case SessionStateClosing:
		return "CLOSING"
	case SessionStateClosed:
		return "CLOSED"
	default:
		return "UNKNOWN"
	}
}

// NewUDPSession 创建UDP会话
func NewUDPSession(id string, tuple FiveTuple, clientAddr, serverAddr *net.UDPAddr) *UDPSession {
	ctx, cancel := context.WithCancel(context.Background())

	session := &UDPSession{
		id:           id,
		tuple:        tuple,
		reverseTuple: tuple.Reverse(),
		clientAddr:   clientAddr,
		serverAddr:   serverAddr,
		state:        SessionStateNew,
		createdAt:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
		logger:       zap.L().Named(fmt.Sprintf("udp-session-%s", id)),
	}

	session.updateActivity()
	return session
}

// Start 启动UDP会话
func (s *UDPSession) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.state != SessionStateNew {
		return fmt.Errorf("会话状态无效: %s", s.state.String())
	}

	s.logger.Info("启动UDP会话",
		zap.String("tuple", s.tuple.String()),
		zap.String("client_addr", s.clientAddr.String()),
		zap.String("server_addr", s.serverAddr.String()))

	// 创建到服务端的连接
	var err error
	s.serverConn, err = net.DialUDP("udp", nil, s.serverAddr)
	if err != nil {
		return fmt.Errorf("连接服务端失败: %w", err)
	}

	// 启动数据转发协程
	s.state = SessionStateActive
	s.wg.Add(2)
	go s.forwardToServer()
	go s.forwardToClient()

	s.logger.Debug("UDP会话启动完成")
	return nil
}

// Stop 停止UDP会话
func (s *UDPSession) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.state == SessionStateClosed {
		return nil
	}

	s.logger.Debug("停止UDP会话")
	s.state = SessionStateClosing

	// 取消上下文
	if s.cancel != nil {
		s.cancel()
	}

	// 关闭连接
	if s.clientConn != nil {
		s.clientConn.Close()
	}
	if s.serverConn != nil {
		s.serverConn.Close()
	}

	// 等待协程结束
	s.wg.Wait()

	s.state = SessionStateClosed
	s.logger.Info("UDP会话已停止",
		zap.Int64("packets_in", s.packetsIn.Load()),
		zap.Int64("packets_out", s.packetsOut.Load()),
		zap.Int64("bytes_in", s.bytesIn.Load()),
		zap.Int64("bytes_out", s.bytesOut.Load()))

	return nil
}

// forwardToServer 转发数据到服务端
func (s *UDPSession) forwardToServer() {
	defer s.wg.Done()
	defer s.logger.Debug("停止转发到服务端")

	buffer := make([]byte, 65536) // 64KB缓冲区

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		if s.clientConn == nil {
			continue
		}

		// 设置读取超时
		s.clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := s.clientConn.Read(buffer)
		s.clientConn.SetReadDeadline(time.Time{})

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // 超时继续尝试
			}
			s.logger.Debug("从客户端读取数据失败", zap.Error(err))
			return
		}

		if n > 0 {
			// 转发到服务端
			_, err := s.serverConn.Write(buffer[:n])
			if err != nil {
				s.logger.Error("转发到服务端失败", zap.Error(err))
				return
			}

			s.packetsOut.Add(1)
			s.bytesOut.Add(int64(n))
			s.updateActivity()

			s.logger.Debug("转发数据到服务端", zap.Int("bytes", n))
		}
	}
}

// forwardToClient 转发数据到客户端
func (s *UDPSession) forwardToClient() {
	defer s.wg.Done()
	defer s.logger.Debug("停止转发到客户端")

	buffer := make([]byte, 65536) // 64KB缓冲区

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		if s.serverConn == nil {
			continue
		}

		// 设置读取超时
		s.serverConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := s.serverConn.Read(buffer)
		s.serverConn.SetReadDeadline(time.Time{})

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // 超时继续尝试
			}
			s.logger.Debug("从服务端读取数据失败", zap.Error(err))
			return
		}

		if n > 0 {
			// 转发到客户端
			_, err := s.clientConn.WriteToUDP(buffer[:n], s.clientAddr)
			if err != nil {
				s.logger.Error("转发到客户端失败", zap.Error(err))
				return
			}

			s.packetsIn.Add(1)
			s.bytesIn.Add(int64(n))
			s.updateActivity()

			s.logger.Debug("转发数据到客户端", zap.Int("bytes", n))
		}
	}
}

// SetClientConn 设置客户端连接
func (s *UDPSession) SetClientConn(conn *net.UDPConn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.clientConn = conn
}

// GetID 获取会话ID
func (s *UDPSession) GetID() string {
	return s.id
}

// GetTuple 获取五元组
func (s *UDPSession) GetTuple() FiveTuple {
	return s.tuple
}

// GetState 获取会话状态
func (s *UDPSession) GetState() SessionState {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.state
}

// IsActive 检查会话是否活跃
func (s *UDPSession) IsActive() bool {
	return s.GetState() == SessionStateActive
}

// GetLastActivity 获取最后活动时间
func (s *UDPSession) GetLastActivity() time.Time {
	return time.Unix(s.lastActivity.Load(), 0)
}

// updateActivity 更新活动时间
func (s *UDPSession) updateActivity() {
	s.lastActivity.Store(time.Now().Unix())
}

// GetStats 获取会话统计信息
func (s *UDPSession) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"id":            s.id,
		"tuple":         s.tuple.String(),
		"state":         s.GetState().String(),
		"created_at":    s.createdAt,
		"last_activity": s.GetLastActivity(),
		"packets_in":    s.packetsIn.Load(),
		"packets_out":   s.packetsOut.Load(),
		"bytes_in":      s.bytesIn.Load(),
		"bytes_out":     s.bytesOut.Load(),
		"duration":      time.Since(s.createdAt).Seconds(),
	}
}

// NATPair NAT映射对
type NATPair struct {
	Original FiveTuple `json:"original"`
	Mapped   FiveTuple `json:"mapped"`
}

// NATTable NAT映射表
type NATTable struct {
	// 原始 -> 映射
	originalToMapped map[string]FiveTuple
	// 映射 -> 原始
	mappedToOriginal map[string]FiveTuple

	mutex  sync.RWMutex
	logger *zap.Logger
}

// NewNATTable 创建NAT表
func NewNATTable() *NATTable {
	return &NATTable{
		originalToMapped: make(map[string]FiveTuple),
		mappedToOriginal: make(map[string]FiveTuple),
		logger:           zap.L().Named("nat-table"),
	}
}

// AddMapping 添加NAT映射
func (nt *NATTable) AddMapping(original, mapped FiveTuple) {
	nt.mutex.Lock()
	defer nt.mutex.Unlock()

	origKey := original.Hash()
	mappedKey := mapped.Hash()

	nt.originalToMapped[origKey] = mapped
	nt.mappedToOriginal[mappedKey] = original

	nt.logger.Debug("添加NAT映射",
		zap.String("original", original.String()),
		zap.String("mapped", mapped.String()))
}

// RemoveMapping 移除NAT映射
func (nt *NATTable) RemoveMapping(original FiveTuple) {
	nt.mutex.Lock()
	defer nt.mutex.Unlock()

	origKey := original.Hash()
	if mapped, exists := nt.originalToMapped[origKey]; exists {
		mappedKey := mapped.Hash()
		delete(nt.originalToMapped, origKey)
		delete(nt.mappedToOriginal, mappedKey)

		nt.logger.Debug("移除NAT映射",
			zap.String("original", original.String()),
			zap.String("mapped", mapped.String()))
	}
}

// LookupByOriginal 通过原始五元组查找映射
func (nt *NATTable) LookupByOriginal(original FiveTuple) (FiveTuple, bool) {
	nt.mutex.RLock()
	defer nt.mutex.RUnlock()

	mapped, exists := nt.originalToMapped[original.Hash()]
	return mapped, exists
}

// LookupByMapped 通过映射五元组查找原始
func (nt *NATTable) LookupByMapped(mapped FiveTuple) (FiveTuple, bool) {
	nt.mutex.RLock()
	defer nt.mutex.RUnlock()

	original, exists := nt.mappedToOriginal[mapped.Hash()]
	return original, exists
}

// GetMappings 获取所有映射
func (nt *NATTable) GetMappings() []NATPair {
	nt.mutex.RLock()
	defer nt.mutex.RUnlock()

	pairs := make([]NATPair, 0, len(nt.originalToMapped))
	for _, mapped := range nt.originalToMapped {
		// 通过映射的五元组查找原始五元组
		if original, exists := nt.mappedToOriginal[mapped.Hash()]; exists {
			pairs = append(pairs, NATPair{
				Original: original,
				Mapped:   mapped,
			})
		}
	}

	return pairs
}

// Clear 清空NAT表
func (nt *NATTable) Clear() {
	nt.mutex.Lock()
	defer nt.mutex.Unlock()

	count := len(nt.originalToMapped)
	nt.originalToMapped = make(map[string]FiveTuple)
	nt.mappedToOriginal = make(map[string]FiveTuple)

	nt.logger.Info("清空NAT表", zap.Int("removed_count", count))
}

// GetStats 获取NAT表统计
func (nt *NATTable) GetStats() map[string]interface{} {
	nt.mutex.RLock()
	defer nt.mutex.RUnlock()

	return map[string]interface{}{
		"total_mappings": len(nt.originalToMapped),
		"table_size":     len(nt.originalToMapped) + len(nt.mappedToOriginal),
	}
}
