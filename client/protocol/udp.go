package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// FiveTuple UDP 五元组（源IP、源端口、目标IP、目标端口、协议）
type FiveTuple struct {
	SrcIP    net.IP
	SrcPort  int
	DstIP    net.IP
	DstPort  int
	Protocol string // "udp" or "udp4" or "udp6"
}

// String 返回五元组的字符串表示
func (ft *FiveTuple) String() string {
	return fmt.Sprintf("%s:%d->%s:%d/%s",
		ft.SrcIP.String(), ft.SrcPort,
		ft.DstIP.String(), ft.DstPort,
		ft.Protocol)
}

// Hash 生成五元组的哈希键
func (ft *FiveTuple) Hash() string {
	return fmt.Sprintf("%s:%d-%s:%d-%s",
		ft.SrcIP.String(), ft.SrcPort,
		ft.DstIP.String(), ft.DstPort,
		ft.Protocol)
}

// UDPSessionStats UDP会话统计信息
type UDPSessionStats struct {
	BytesIn      atomic.Int64
	BytesOut     atomic.Int64
	PacketsIn    atomic.Int64
	PacketsOut   atomic.Int64
	ErrorCount   atomic.Int64
	LastError    atomic.Value // string
	CreatedAt    time.Time
	LastActiveAt atomic.Value // time.Time
}

// UDPSession UDP会话（增强版）
type UDPSession struct {
	Tuple      FiveTuple
	SessionID  string
	Conn       *net.UDPConn
	Stats      UDPSessionStats
	NATMapping *NATMapping // NAT映射信息
	mu         sync.RWMutex
	closed     atomic.Bool
}

// Touch 更新最后活跃时间
func (s *UDPSession) Touch() {
	s.Stats.LastActiveAt.Store(time.Now())
}

// IsExpired 检查是否过期
func (s *UDPSession) IsExpired(timeout time.Duration) bool {
	lastActive := s.Stats.LastActiveAt.Load().(time.Time)
	return time.Since(lastActive) > timeout
}

// Close 关闭会话
func (s *UDPSession) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil // 已经关闭
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Conn != nil {
		return s.Conn.Close()
	}
	return nil
}

// IsClosed 检查会话是否已关闭
func (s *UDPSession) IsClosed() bool {
	return s.closed.Load()
}

// RecordPacketIn 记录接收的数据包
func (s *UDPSession) RecordPacketIn(bytes int64) {
	s.Stats.BytesIn.Add(bytes)
	s.Stats.PacketsIn.Add(1)
	s.Touch()
}

// RecordPacketOut 记录发送的数据包
func (s *UDPSession) RecordPacketOut(bytes int64) {
	s.Stats.BytesOut.Add(bytes)
	s.Stats.PacketsOut.Add(1)
	s.Touch()
}

// RecordError 记录错误
func (s *UDPSession) RecordError(err error) {
	s.Stats.ErrorCount.Add(1)
	s.Stats.LastError.Store(err.Error())
}

// GetStats 获取会话统计信息快照
func (s *UDPSession) GetStats() UDPSessionStatsSnapshot {
	lastActive := s.Stats.LastActiveAt.Load().(time.Time)
	lastError := ""
	if err := s.Stats.LastError.Load(); err != nil {
		lastError = err.(string)
	}

	return UDPSessionStatsSnapshot{
		SessionID:    s.SessionID,
		Tuple:        s.Tuple.String(),
		BytesIn:      s.Stats.BytesIn.Load(),
		BytesOut:     s.Stats.BytesOut.Load(),
		PacketsIn:    s.Stats.PacketsIn.Load(),
		PacketsOut:   s.Stats.PacketsOut.Load(),
		ErrorCount:   s.Stats.ErrorCount.Load(),
		LastError:    lastError,
		CreatedAt:    s.Stats.CreatedAt,
		LastActiveAt: lastActive,
		Age:          time.Since(s.Stats.CreatedAt),
		IdleTime:     time.Since(lastActive),
	}
}

// UDPSessionStatsSnapshot UDP会话统计快照
type UDPSessionStatsSnapshot struct {
	SessionID    string
	Tuple        string
	BytesIn      int64
	BytesOut     int64
	PacketsIn    int64
	PacketsOut   int64
	ErrorCount   int64
	LastError    string
	CreatedAt    time.Time
	LastActiveAt time.Time
	Age          time.Duration
	IdleTime     time.Duration
}

// NATType NAT类型
type NATType int

const (
	NATTypeUnknown NATType = iota
	NATTypeNone            // 无NAT
	NATTypeFullCone
	NATTypeRestrictedCone
	NATTypePortRestrictedCone
	NATTypeSymmetric
)

func (nt NATType) String() string {
	switch nt {
	case NATTypeFullCone:
		return "Full Cone"
	case NATTypeRestrictedCone:
		return "Restricted Cone"
	case NATTypePortRestrictedCone:
		return "Port Restricted Cone"
	case NATTypeSymmetric:
		return "Symmetric"
	default:
		return "Unknown"
	}
}

// NATMapping NAT映射信息
type NATMapping struct {
	LocalAddr  *net.UDPAddr
	PublicAddr *net.UDPAddr
	NATType    NATType
	DetectedAt time.Time
	STUNServer string
}

// UDPSessionManager UDP会话管理器（增强版）
type UDPSessionManager struct {
	sessions       map[string]*UDPSession // 5-tuple hash → session
	mu             sync.RWMutex
	timeout        time.Duration
	stopChan       chan struct{}
	natDetector    *NATDetector
	sessionCounter atomic.Int64
}

// NewUDPSessionManager 创建UDP会话管理器
func NewUDPSessionManager(timeout time.Duration) *UDPSessionManager {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	mgr := &UDPSessionManager{
		sessions:    make(map[string]*UDPSession),
		timeout:     timeout,
		stopChan:    make(chan struct{}),
		natDetector: NewNATDetector(),
	}

	// 启动清理协程
	go mgr.cleanupLoop()

	return mgr
}

// GetOrCreate 获取或创建会话
func (usm *UDPSessionManager) GetOrCreate(srcAddr, dstAddr *net.UDPAddr) (*UDPSession, error) {
	tuple := FiveTuple{
		SrcIP:    srcAddr.IP,
		SrcPort:  srcAddr.Port,
		DstIP:    dstAddr.IP,
		DstPort:  dstAddr.Port,
		Protocol: "udp",
	}
	key := tuple.Hash()

	// 先尝试获取现有会话
	usm.mu.RLock()
	if session, ok := usm.sessions[key]; ok {
		usm.mu.RUnlock()
		if !session.IsClosed() {
			session.Touch()
			return session, nil
		}
	} else {
		usm.mu.RUnlock()
	}

	// 创建新会话
	session := &UDPSession{
		Tuple:     tuple,
		SessionID: generateSessionID(),
		Stats: UDPSessionStats{
			CreatedAt: time.Now(),
		},
	}
	session.Stats.LastActiveAt.Store(time.Now())

	// 创建UDP连接
	conn, err := net.DialUDP("udp", nil, dstAddr)
	if err != nil {
		return nil, fmt.Errorf("创建UDP连接失败: %w", err)
	}
	session.Conn = conn

	// 尝试检测NAT映射（非阻塞）
	go usm.detectNATForSession(session)

	// 保存会话
	usm.mu.Lock()
	usm.sessions[key] = session
	count := usm.sessionCounter.Add(1)
	usm.mu.Unlock()

	logger.Debug("创建UDP会话",
		zap.String("session_id", session.SessionID),
		zap.String("tuple", tuple.String()),
		zap.Int64("total_sessions", count))

	return session, nil
}

// detectNATForSession 为会话检测NAT映射
func (usm *UDPSessionManager) detectNATForSession(session *UDPSession) {
	if usm.natDetector == nil {
		return
	}

	natMapping, err := usm.natDetector.DetectNAT(session.Conn)
	if err != nil {
		logger.Debug("NAT检测失败",
			zap.String("session_id", session.SessionID),
			zap.Error(err))
		return
	}

	session.mu.Lock()
	session.NATMapping = natMapping
	session.mu.Unlock()

	logger.Info("NAT检测成功",
		zap.String("session_id", session.SessionID),
		zap.String("nat_type", natMapping.NATType.String()),
		zap.String("public_addr", natMapping.PublicAddr.String()))
}

// Remove 移除会话
func (usm *UDPSessionManager) Remove(tuple FiveTuple) {
	key := tuple.Hash()

	usm.mu.Lock()
	defer usm.mu.Unlock()

	if session, ok := usm.sessions[key]; ok {
		session.Close()
		delete(usm.sessions, key)
		logger.Debug("移除UDP会话",
			zap.String("session_id", session.SessionID),
			zap.String("tuple", tuple.String()))
	}
}

// RemoveByKey 通过键移除会话（兼容旧接口）
func (usm *UDPSessionManager) RemoveByKey(srcAddr, dstAddr *net.UDPAddr) {
	tuple := FiveTuple{
		SrcIP:    srcAddr.IP,
		SrcPort:  srcAddr.Port,
		DstIP:    dstAddr.IP,
		DstPort:  dstAddr.Port,
		Protocol: "udp",
	}
	usm.Remove(tuple)
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
	var totalBytesIn, totalBytesOut int64

	for key, session := range usm.sessions {
		if session.IsExpired(usm.timeout) || session.IsClosed() {
			// 收集统计信息
			stats := session.GetStats()
			totalBytesIn += stats.BytesIn
			totalBytesOut += stats.BytesOut

			session.Close()
			delete(usm.sessions, key)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		logger.Debug("清理过期UDP会话",
			zap.Int("count", expiredCount),
			zap.Int64("bytes_in", totalBytesIn),
			zap.Int64("bytes_out", totalBytesOut))
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

// GetAllStats 获取所有会话的统计信息
func (usm *UDPSessionManager) GetAllStats() []UDPSessionStatsSnapshot {
	usm.mu.RLock()
	defer usm.mu.RUnlock()

	stats := make([]UDPSessionStatsSnapshot, 0, len(usm.sessions))
	for _, session := range usm.sessions {
		stats = append(stats, session.GetStats())
	}
	return stats
}

// GetAggregatedStats 获取聚合统计信息
func (usm *UDPSessionManager) GetAggregatedStats() UDPAggregatedStats {
	usm.mu.RLock()
	defer usm.mu.RUnlock()

	var stats UDPAggregatedStats
	stats.TotalSessions = int64(len(usm.sessions))
	stats.CounterValue = usm.sessionCounter.Load()

	for _, session := range usm.sessions {
		snapshot := session.GetStats()
		stats.TotalBytesIn += snapshot.BytesIn
		stats.TotalBytesOut += snapshot.BytesOut
		stats.TotalPacketsIn += snapshot.PacketsIn
		stats.TotalPacketsOut += snapshot.PacketsOut
		stats.TotalErrors += snapshot.ErrorCount
	}

	return stats
}

// UDPAggregatedStats UDP聚合统计信息
type UDPAggregatedStats struct {
	TotalSessions   int64
	CounterValue    int64
	TotalBytesIn    int64
	TotalBytesOut   int64
	TotalPacketsIn  int64
	TotalPacketsOut int64
	TotalErrors     int64
}

// generateSessionID 生成会话ID（使用安全随机数）
func generateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// 回退到时间戳
		return fmt.Sprintf("udp-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("udp-%s", hex.EncodeToString(b))
}

// NATDetector NAT检测器（使用STUN协议）
type NATDetector struct {
	stunServers []string
	timeout     time.Duration
	mu          sync.RWMutex
}

// NewNATDetector 创建NAT检测器
func NewNATDetector() *NATDetector {
	return &NATDetector{
		stunServers: []string{
			"stun.l.google.com:19302",
			"stun1.l.google.com:19302",
			"stun2.l.google.com:19302",
		},
		timeout: 5 * time.Second,
	}
}

// DetectNAT 检测NAT类型（简化版STUN实现）
func (nd *NATDetector) DetectNAT(conn *net.UDPConn) (*NATMapping, error) {
	nd.mu.RLock()
	defer nd.mu.RUnlock()

	if len(nd.stunServers) == 0 {
		return nil, fmt.Errorf("没有可用的STUN服务器")
	}

	// 尝试第一个STUN服务器
	stunServer := nd.stunServers[0]

	// 获取本地地址
	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// 简化实现：仅返回基本映射信息
	// 完整的STUN实现需要发送STUN绑定请求并解析响应
	// 尝试STUN检测
	publicAddr, natType, err := nd.performSTUNDetection(conn, stunServer)
	if err != nil {
		// STUN失败时使用本地地址
		return &NATMapping{
			LocalAddr:  localAddr,
			PublicAddr: localAddr,
			NATType:    NATTypeUnknown,
			DetectedAt: time.Now(),
			STUNServer: stunServer,
		}, nil
	}

	// 解析公网地址
	parsedPublicAddr, err := net.ResolveUDPAddr("udp", publicAddr)
	if err != nil {
		parsedPublicAddr = localAddr
	}

	return &NATMapping{
		LocalAddr:  localAddr,
		PublicAddr: parsedPublicAddr,
		NATType:    natType,
		DetectedAt: time.Now(),
		STUNServer: stunServer,
	}, nil
}

// performSTUNDetection 执行STUN协议检测
func (nd *NATDetector) performSTUNDetection(conn *net.UDPConn, stunServer string) (string, NATType, error) {
	// 解析STUN服务器地址
	serverAddr, err := net.ResolveUDPAddr("udp", stunServer)
	if err != nil {
		return "", NATTypeUnknown, err
	}

	// 创建STUN Binding Request
	// Magic Cookie: 0x2112A442
	// Transaction ID: 12 bytes random
	transactionID := make([]byte, 12)
	_, _ = rand.Read(transactionID)

	request := make([]byte, 20)
	// Message Type: Binding Request (0x0001)
	request[0] = 0x00
	request[1] = 0x01
	// Message Length: 0
	request[2] = 0x00
	request[3] = 0x00
	// Magic Cookie
	request[4] = 0x21
	request[5] = 0x12
	request[6] = 0xA4
	request[7] = 0x42
	// Transaction ID
	copy(request[8:], transactionID)

	// 发送请求
	conn.SetDeadline(time.Now().Add(nd.timeout))
	_, err = conn.WriteToUDP(request, serverAddr)
	if err != nil {
		return "", NATTypeUnknown, err
	}

	// 接收响应
	buffer := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buffer)
	if err != nil {
		return "", NATTypeUnknown, err
	}

	// 验证响应
	if n < 20 {
		return "", NATTypeUnknown, fmt.Errorf("invalid STUN response")
	}

	// 检查Magic Cookie
	if buffer[4] != 0x21 || buffer[5] != 0x12 || buffer[6] != 0xA4 || buffer[7] != 0x42 {
		return "", NATTypeUnknown, fmt.Errorf("invalid magic cookie")
	}

	// 解析XOR-MAPPED-ADDRESS属性
	publicAddr, err := parseSTUNResponse(buffer[:n], transactionID)
	if err != nil {
		return "", NATTypeUnknown, err
	}

	// 简单的NAT类型判定
	localAddr := conn.LocalAddr().String()
	if publicAddr == localAddr {
		return publicAddr, NATTypeNone, nil
	}

	return publicAddr, NATTypeFullCone, nil
}

// parseSTUNResponse 解析STUN响应获取公网地址
func parseSTUNResponse(data []byte, transactionID []byte) (string, error) {
	// 跳过STUN头部(20字节)
	offset := 20
	messageLength := int(data[2])<<8 | int(data[3])

	for offset < 20+messageLength {
		if offset+4 > len(data) {
			break
		}

		attrType := int(data[offset])<<8 | int(data[offset+1])
		attrLength := int(data[offset+2])<<8 | int(data[offset+3])
		offset += 4

		if offset+attrLength > len(data) {
			break
		}

		// XOR-MAPPED-ADDRESS (0x0020)
		if attrType == 0x0020 && attrLength >= 8 {
			family := data[offset+1]
			port := (int(data[offset+2]) << 8) | int(data[offset+3])

			// XOR with magic cookie
			port ^= 0x2112

			if family == 0x01 { // IPv4
				ip := make([]byte, 4)
				for i := 0; i < 4; i++ {
					ip[i] = data[offset+4+i] ^ data[4+i]
				}
				return fmt.Sprintf("%d.%d.%d.%d:%d", ip[0], ip[1], ip[2], ip[3], port), nil
			}
		}

		offset += attrLength
	}

	return "", fmt.Errorf("no XOR-MAPPED-ADDRESS found")
}
