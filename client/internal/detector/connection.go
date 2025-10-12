package detector

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DetectingConn 协议检测连接包装器
type DetectingConn struct {
	net.Conn
	detector        *Detector
	detected        bool
	detectionResult *DetectionResult
	peekBuffer      []byte
	peekSize        int
	mutex           sync.Mutex
	logger          *zap.Logger
}

// NewDetectingConn 创建协议检测连接
func NewDetectingConn(conn net.Conn, detector *Detector, peekSize int) *DetectingConn {
	if peekSize <= 0 {
		peekSize = 1024 // 默认预读1KB
	}

	return &DetectingConn{
		Conn:       conn,
		detector:   detector,
		peekSize:   peekSize,
		peekBuffer: make([]byte, 0, peekSize),
		logger:     zap.L().Named("detecting-conn"),
	}
}

// Read 读取数据并进行协议检测
func (dc *DetectingConn) Read(b []byte) (int, error) {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()

	// 如果还没有检测协议且缓冲区未满，先预读数据进行检测
	if !dc.detected && len(dc.peekBuffer) < dc.peekSize {
		needed := dc.peekSize - len(dc.peekBuffer)
		if needed > len(b) {
			needed = len(b)
		}

		tempBuf := make([]byte, needed)
		n, err := dc.Conn.Read(tempBuf)
		if n > 0 {
			dc.peekBuffer = append(dc.peekBuffer, tempBuf[:n]...)

			// 尝试检测协议
			if len(dc.peekBuffer) >= 16 || err != nil { // 至少16字节或读取结束
				dc.detectProtocol()
			}
		}

		if err != nil && err != io.EOF {
			return 0, err
		}
	}

	// 首先从预读缓冲区返回数据
	if len(dc.peekBuffer) > 0 {
		n := copy(b, dc.peekBuffer)
		dc.peekBuffer = dc.peekBuffer[n:]
		return n, nil
	}

	// 缓冲区为空，直接从连接读取
	return dc.Conn.Read(b)
}

// detectProtocol 检测协议
func (dc *DetectingConn) detectProtocol() {
	if dc.detected || len(dc.peekBuffer) == 0 {
		return
	}

	// 使用缓冲区数据创建一个reader
	bufReader := &bufferReader{buffer: dc.peekBuffer}

	// 使用自定义reader进行检测
	result, err := dc.detector.DetectProtocolFromReader(bufReader)
	if err != nil {
		dc.logger.Warn("协议检测失败", zap.Error(err))
		result = &DetectionResult{
			Protocol:   ProtocolUnknown,
			Confidence: 0.0,
			Buffer:     dc.peekBuffer,
		}
	}

	dc.detectionResult = result
	dc.detected = true

	dc.logger.Info("协议检测完成",
		zap.String("protocol", string(dc.detectionResult.Protocol)),
		zap.Float64("confidence", dc.detectionResult.Confidence),
		zap.String("remote_addr", dc.RemoteAddr().String()))
}

// GetDetectionResult 获取检测结果
func (dc *DetectingConn) GetDetectionResult() *DetectionResult {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()

	if !dc.detected && len(dc.peekBuffer) > 0 {
		dc.detectProtocol()
	}

	return dc.detectionResult
}

// IsDetected 检查是否已完成检测
func (dc *DetectingConn) IsDetected() bool {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	return dc.detected
}

// PeekProtocol 预读数据进行协议检测（不消费数据）
func (dc *DetectingConn) PeekProtocol(timeout time.Duration) (*DetectionResult, error) {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()

	if dc.detected {
		return dc.detectionResult, nil
	}

	// 设置读取超时
	if timeout > 0 {
		dc.Conn.SetReadDeadline(time.Now().Add(timeout))
		defer dc.Conn.SetReadDeadline(time.Time{})
	}

	// 预读数据
	for len(dc.peekBuffer) < dc.peekSize {
		tempBuf := make([]byte, dc.peekSize-len(dc.peekBuffer))
		n, err := dc.Conn.Read(tempBuf)
		if n > 0 {
			dc.peekBuffer = append(dc.peekBuffer, tempBuf[:n]...)
		}

		if err != nil {
			if err == io.EOF || len(dc.peekBuffer) > 0 {
				break // 有数据就尝试检测
			}
			return nil, err
		}

		// 如果已经有足够数据，可以提前检测
		if len(dc.peekBuffer) >= 64 {
			break
		}
	}

	// 执行检测
	if len(dc.peekBuffer) > 0 {
		dc.detectProtocol()
		return dc.detectionResult, nil
	}

	return &DetectionResult{
		Protocol:   ProtocolUnknown,
		Confidence: 0.0,
		Buffer:     make([]byte, 0),
	}, nil
}

// ConnectionManager 连接管理器
type ConnectionManager struct {
	detector *Detector
	handlers map[Protocol]ConnectionHandler
	mutex    sync.RWMutex
	logger   *zap.Logger

	// 统计
	stats struct {
		totalConnections    int64
		detectedConnections int64
		handledConnections  int64
	}
}

// ConnectionHandler 连接处理器
type ConnectionHandler func(conn net.Conn, result *DetectionResult) error

// NewConnectionManager 创建连接管理器
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		detector: NewDetector(nil),
		handlers: make(map[Protocol]ConnectionHandler),
		logger:   zap.L().Named("connection-manager"),
	}
}

// RegisterHandler 注册协议处理器
func (cm *ConnectionManager) RegisterHandler(protocol Protocol, handler ConnectionHandler) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.handlers[protocol] = handler
	cm.logger.Info("注册协议处理器", zap.String("protocol", string(protocol)))
}

// HandleConnection 处理连接
func (cm *ConnectionManager) HandleConnection(conn net.Conn) error {
	cm.stats.totalConnections++

	// 创建检测连接
	detectingConn := NewDetectingConn(conn, cm.detector, 1024)

	// 预读并检测协议
	result, err := detectingConn.PeekProtocol(5 * time.Second)
	if err != nil {
		cm.logger.Error("协议检测失败",
			zap.String("remote_addr", conn.RemoteAddr().String()),
			zap.Error(err))
		return err
	}

	cm.stats.detectedConnections++

	cm.logger.Info("检测到连接协议",
		zap.String("protocol", string(result.Protocol)),
		zap.Float64("confidence", result.Confidence),
		zap.String("remote_addr", conn.RemoteAddr().String()))

	// 查找处理器
	cm.mutex.RLock()
	handler, exists := cm.handlers[result.Protocol]
	cm.mutex.RUnlock()

	if !exists {
		// 没有找到特定处理器，尝试通用处理器
		cm.mutex.RLock()
		handler, exists = cm.handlers[ProtocolUnknown]
		cm.mutex.RUnlock()

		if !exists {
			return fmt.Errorf("没有找到协议处理器: %s", result.Protocol)
		}
	}

	// 执行处理器
	cm.stats.handledConnections++
	return handler(detectingConn, result)
}

// GetStats 获取统计信息
func (cm *ConnectionManager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_connections":    cm.stats.totalConnections,
		"detected_connections": cm.stats.detectedConnections,
		"handled_connections":  cm.stats.handledConnections,
		"registered_handlers":  len(cm.handlers),
	}
}

// ProtocolListener 协议监听器
type ProtocolListener struct {
	listener net.Listener
	manager  *ConnectionManager
	logger   *zap.Logger
}

// NewProtocolListener 创建协议监听器
func NewProtocolListener(listener net.Listener, manager *ConnectionManager) *ProtocolListener {
	return &ProtocolListener{
		listener: listener,
		manager:  manager,
		logger:   zap.L().Named("protocol-listener"),
	}
}

// Accept 接受连接并处理
func (pl *ProtocolListener) Accept() (net.Conn, error) {
	conn, err := pl.listener.Accept()
	if err != nil {
		return nil, err
	}

	// 在后台处理连接
	go func() {
		defer func() {
			if r := recover(); r != nil {
				pl.logger.Error("连接处理panic",
					zap.Any("panic", r),
					zap.String("remote_addr", conn.RemoteAddr().String()))
			}
		}()

		if err := pl.manager.HandleConnection(conn); err != nil {
			pl.logger.Error("处理连接失败",
				zap.String("remote_addr", conn.RemoteAddr().String()),
				zap.Error(err))
			conn.Close()
		}
	}()

	return conn, nil
}

// Close 关闭监听器
func (pl *ProtocolListener) Close() error {
	return pl.listener.Close()
}

// Addr 获取监听地址
func (pl *ProtocolListener) Addr() net.Addr {
	return pl.listener.Addr()
}

// ProtocolMatcher 协议匹配器
type ProtocolMatcher struct {
	matchers []ProtocolMatchRule
	mutex    sync.RWMutex
}

// ProtocolMatchRule 协议匹配规则
type ProtocolMatchRule struct {
	Protocol   Protocol
	Confidence float64
	Action     string // "accept", "reject", "forward"
	Target     string // 转发目标（如果action是forward）
}

// NewProtocolMatcher 创建协议匹配器
func NewProtocolMatcher() *ProtocolMatcher {
	return &ProtocolMatcher{
		matchers: make([]ProtocolMatchRule, 0),
	}
}

// AddRule 添加匹配规则
func (pm *ProtocolMatcher) AddRule(rule ProtocolMatchRule) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.matchers = append(pm.matchers, rule)
}

// Match 匹配协议规则
func (pm *ProtocolMatcher) Match(result *DetectionResult) *ProtocolMatchRule {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	for _, rule := range pm.matchers {
		if rule.Protocol == result.Protocol && result.Confidence >= rule.Confidence {
			return &rule
		}
	}

	return nil
}

// ClearRules 清空规则
func (pm *ProtocolMatcher) ClearRules() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.matchers = pm.matchers[:0]
}

// GetRules 获取所有规则
func (pm *ProtocolMatcher) GetRules() []ProtocolMatchRule {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	rules := make([]ProtocolMatchRule, len(pm.matchers))
	copy(rules, pm.matchers)
	return rules
}

// bufferReader 缓冲区读取器
type bufferReader struct {
	buffer []byte
	pos    int
}

// Read 实现io.Reader接口
func (r *bufferReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.buffer) {
		return 0, io.EOF
	}
	n = copy(p, r.buffer[r.pos:])
	r.pos += n
	return n, nil
}

// DetectProtocolFromReader 从Reader中检测协议
func (d *Detector) DetectProtocolFromReader(reader io.Reader) (*DetectionResult, error) {
	// 读取数据到缓冲区
	buffer := make([]byte, d.config.BufferSize)
	n, err := reader.Read(buffer)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("读取数据失败: %w", err)
	}

	// 分析缓冲区数据
	result := d.analyzeBuffer(buffer[:n])
	result.Buffer = buffer[:n]

	return result, nil
}
