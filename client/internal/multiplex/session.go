package multiplex

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

// SessionConfig 会话配置
type SessionConfig struct {
	InitialWindowSize    int64         // 初始窗口大小
	MaxFrameSize         uint32        // 最大帧大小
	MaxConcurrentStreams uint32        // 最大并发流数
	StreamIdleTimeout    time.Duration // 流空闲超时
	PingInterval         time.Duration // Ping间隔
	PingTimeout          time.Duration // Ping超时
	EnableFlowControl    bool          // 启用流控制
}

// DefaultSessionConfig 默认会话配置
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		InitialWindowSize:    65536,       // 64KB
		MaxFrameSize:         1024 * 1024, // 1MB
		MaxConcurrentStreams: 1000,
		StreamIdleTimeout:    5 * time.Minute,
		PingInterval:         30 * time.Second,
		PingTimeout:          10 * time.Second,
		EnableFlowControl:    true,
	}
}

// Session 多路复用会话
type Session struct {
	conn   net.Conn
	config *SessionConfig

	// 流管理
	streams      map[StreamID]*Stream
	streamsMutex sync.RWMutex
	nextStreamID atomic.Uint32
	isClient     bool

	// 帧处理
	frameReader *FrameReader
	frameWriter *FrameWriter
	writeMutex  sync.Mutex

	// 流控制
	connectionSendWindow atomic.Int64
	connectionRecvWindow atomic.Int64

	// 状态管理
	closed      atomic.Bool
	closeReason atomic.Value // ErrorCode
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	// 通知通道
	acceptCh chan *Stream
	pingCh   chan []byte
	pongCh   chan []byte

	// 统计信息
	stats struct {
		streamsCreated atomic.Int64
		streamsClosed  atomic.Int64
		framesSent     atomic.Int64
		framesReceived atomic.Int64
		bytesSent      atomic.Int64
		bytesReceived  atomic.Int64
		pingsSent      atomic.Int64
		pongsReceived  atomic.Int64
	}

	logger *zap.Logger
}

// NewSession 创建新会话
func NewSession(conn net.Conn, config *SessionConfig, isClient bool) *Session {
	if config == nil {
		config = DefaultSessionConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	session := &Session{
		conn:     conn,
		config:   config,
		isClient: isClient,
		streams:  make(map[StreamID]*Stream),
		acceptCh: make(chan *Stream, 32),
		pingCh:   make(chan []byte, 8),
		pongCh:   make(chan []byte, 8),
		ctx:      ctx,
		cancel:   cancel,
		logger:   zap.L().Named("multiplex-session"),
	}

	// 设置初始流ID（客户端使用奇数，服务端使用偶数）
	if isClient {
		session.nextStreamID.Store(1)
	} else {
		session.nextStreamID.Store(2)
	}

	// 设置连接级别的流控窗口
	session.connectionSendWindow.Store(config.InitialWindowSize)
	session.connectionRecvWindow.Store(config.InitialWindowSize)

	// 创建帧读写器
	session.frameReader = NewFrameReader(conn)
	session.frameWriter = NewFrameWriter(conn)

	return session
}

// Start 启动会话
func (s *Session) Start() error {
	s.logger.Info("启动多路复用会话",
		zap.Bool("is_client", s.isClient),
		zap.Int64("initial_window", s.config.InitialWindowSize))

	// 启动后台协程
	s.wg.Add(3)
	go s.readLoop()
	go s.pingLoop()
	go s.cleanupLoop()

	return nil
}

// Close 关闭会话
func (s *Session) Close() error {
	return s.CloseWithError(ErrorCodeNoError)
}

// CloseWithError 以指定错误关闭会话
func (s *Session) CloseWithError(errorCode ErrorCode) error {
	if s.closed.CompareAndSwap(false, true) {
		s.logger.Info("关闭多路复用会话", zap.String("error", errorCode.String()))

		s.closeReason.Store(errorCode)

		// 发送GO_AWAY帧
		lastStreamID := s.getLastStreamID()
		frame := NewGoAwayFrame(lastStreamID, uint32(errorCode), nil)
		s.writeFrame(frame)

		// 取消上下文
		if s.cancel != nil {
			s.cancel()
		}

		// 关闭所有流
		s.closeAllStreams()

		// 关闭底层连接
		s.conn.Close()

		// 等待协程结束
		s.wg.Wait()

		s.logger.Info("多路复用会话已关闭")
	}

	return nil
}

// IsClosed 检查会话是否已关闭
func (s *Session) IsClosed() bool {
	return s.closed.Load()
}

// OpenStream 打开新流
func (s *Session) OpenStream() (*Stream, error) {
	return s.OpenStreamWithPriority(0)
}

// OpenStreamWithPriority 以指定优先级打开新流
func (s *Session) OpenStreamWithPriority(priority uint8) (*Stream, error) {
	if s.IsClosed() {
		return nil, fmt.Errorf("会话已关闭")
	}

	// 检查并发流限制
	s.streamsMutex.RLock()
	activeStreams := len(s.streams)
	s.streamsMutex.RUnlock()

	if activeStreams >= int(s.config.MaxConcurrentStreams) {
		return nil, fmt.Errorf("超过最大并发流数限制")
	}

	// 生成新的流ID
	streamID := s.generateStreamID()

	// 创建流
	stream := NewStream(streamID, s, s.config.InitialWindowSize)
	stream.priority = priority
	stream.setState(StreamStateOpen)

	// 注册流
	s.streamsMutex.Lock()
	s.streams[streamID] = stream
	s.streamsMutex.Unlock()

	// 发送流打开帧
	frame := NewStreamOpenFrame(streamID, priority)
	if err := s.writeFrame(frame); err != nil {
		s.removeStream(streamID)
		return nil, err
	}

	s.stats.streamsCreated.Add(1)
	s.logger.Debug("打开新流", zap.Uint32("stream_id", uint32(streamID)))

	return stream, nil
}

// AcceptStream 接受新流
func (s *Session) AcceptStream() (*Stream, error) {
	select {
	case stream := <-s.acceptCh:
		return stream, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

// AcceptStreamWithTimeout 在超时时间内接受新流
func (s *Session) AcceptStreamWithTimeout(timeout time.Duration) (*Stream, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case stream := <-s.acceptCh:
		return stream, nil
	case <-timer.C:
		return nil, fmt.Errorf("接受流超时")
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

// GetStream 获取指定ID的流
func (s *Session) GetStream(streamID StreamID) (*Stream, bool) {
	s.streamsMutex.RLock()
	defer s.streamsMutex.RUnlock()

	stream, exists := s.streams[streamID]
	return stream, exists
}

// Ping 发送Ping
func (s *Session) Ping(data []byte) error {
	frame := NewPingFrame(data, false)
	s.stats.pingsSent.Add(1)
	return s.writeFrame(frame)
}

// writeFrame 写入帧
func (s *Session) writeFrame(frame *Frame) error {
	if s.IsClosed() {
		return fmt.Errorf("会话已关闭")
	}

	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	if err := s.frameWriter.WriteFrame(frame); err != nil {
		return err
	}

	s.stats.framesSent.Add(1)
	s.stats.bytesSent.Add(int64(frame.Length + FrameHeaderLength))

	return nil
}

// readLoop 读取循环
func (s *Session) readLoop() {
	defer s.wg.Done()
	defer s.logger.Debug("读取循环退出")

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		frame, err := s.frameReader.ReadFrame()
		if err != nil {
			if err != io.EOF && !s.IsClosed() {
				s.logger.Error("读取帧失败", zap.Error(err))
				s.CloseWithError(ErrorCodeProtocolError)
			}
			return
		}

		s.stats.framesReceived.Add(1)
		s.stats.bytesReceived.Add(int64(frame.Length + FrameHeaderLength))

		if err := s.handleFrame(frame); err != nil {
			s.logger.Error("处理帧失败", zap.Error(err))
			s.CloseWithError(ErrorCodeProtocolError)
			return
		}
	}
}

// handleFrame 处理帧
func (s *Session) handleFrame(frame *Frame) error {
	s.logger.Debug("收到帧", zap.String("frame", frame.String()))

	// 验证帧
	if err := frame.Validate(); err != nil {
		return err
	}

	switch frame.Type {
	case FrameTypeData, FrameTypeWindowUpdate, FrameTypeStreamClose, FrameTypeStreamReset:
		return s.handleStreamFrame(frame)
	case FrameTypeStreamOpen:
		return s.handleStreamOpenFrame(frame)
	case FrameTypePing:
		return s.handlePingFrame(frame)
	case FrameTypePong:
		return s.handlePongFrame(frame)
	case FrameTypeGoAway:
		return s.handleGoAwayFrame(frame)
	case FrameTypeSettings:
		return s.handleSettingsFrame(frame)
	default:
		s.logger.Warn("未知帧类型", zap.String("type", frame.Type.String()))
		return nil
	}
}

// handleStreamFrame 处理流相关帧
func (s *Session) handleStreamFrame(frame *Frame) error {
	stream, exists := s.GetStream(frame.StreamID)
	if !exists {
		// 流不存在，可能已经关闭
		s.logger.Warn("流不存在", zap.Uint32("stream_id", uint32(frame.StreamID)))
		return nil
	}

	return stream.handleFrame(frame)
}

// handleStreamOpenFrame 处理流打开帧
func (s *Session) handleStreamOpenFrame(frame *Frame) error {
	streamID := frame.StreamID

	// 检查流ID有效性
	if !s.isValidStreamID(streamID) {
		resetFrame := NewStreamResetFrame(streamID, uint32(ErrorCodeProtocolError))
		return s.writeFrame(resetFrame)
	}

	// 检查并发流限制
	s.streamsMutex.RLock()
	activeStreams := len(s.streams)
	s.streamsMutex.RUnlock()

	if activeStreams >= int(s.config.MaxConcurrentStreams) {
		resetFrame := NewStreamResetFrame(streamID, uint32(ErrorCodeRefusedStream))
		return s.writeFrame(resetFrame)
	}

	// 创建新流
	stream := NewStream(streamID, s, s.config.InitialWindowSize)
	stream.priority = frame.GetPriority()
	stream.setState(StreamStateOpen)

	// 注册流
	s.streamsMutex.Lock()
	s.streams[streamID] = stream
	s.streamsMutex.Unlock()

	s.stats.streamsCreated.Add(1)
	s.logger.Debug("接受新流", zap.Uint32("stream_id", uint32(streamID)))

	// 通知应用层
	select {
	case s.acceptCh <- stream:
	case <-s.ctx.Done():
		return s.ctx.Err()
	}

	return nil
}

// handlePingFrame 处理Ping帧
func (s *Session) handlePingFrame(frame *Frame) error {
	if frame.IsAck() {
		// 这是Pong响应
		select {
		case s.pongCh <- frame.Payload:
		default:
		}
	} else {
		// 发送Pong响应
		pongFrame := NewPongFrame(frame.Payload)
		return s.writeFrame(pongFrame)
	}
	return nil
}

// handlePongFrame 处理Pong帧
func (s *Session) handlePongFrame(frame *Frame) error {
	s.stats.pongsReceived.Add(1)
	select {
	case s.pongCh <- frame.Payload:
	default:
	}
	return nil
}

// handleGoAwayFrame 处理连接关闭帧
func (s *Session) handleGoAwayFrame(frame *Frame) error {
	lastStreamID := frame.GetLastStreamID()
	errorCode := ErrorCode(frame.GetErrorCode())

	s.logger.Info("收到GO_AWAY帧",
		zap.Uint32("last_stream_id", uint32(lastStreamID)),
		zap.String("error", errorCode.String()))

	// 关闭会话
	return s.CloseWithError(errorCode)
}

// handleSettingsFrame 处理设置帧
func (s *Session) handleSettingsFrame(frame *Frame) error {
	// 简化实现，暂不处理设置
	s.logger.Debug("收到SETTINGS帧")
	return nil
}

// pingLoop Ping循环
func (s *Session) pingLoop() {
	defer s.wg.Done()
	defer s.logger.Debug("Ping循环退出")

	ticker := time.NewTicker(s.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.IsClosed() {
				return
			}

			// 发送Ping
			pingData := []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
			if err := s.Ping(pingData); err != nil {
				s.logger.Error("发送Ping失败", zap.Error(err))
				return
			}

		case <-s.ctx.Done():
			return
		}
	}
}

// cleanupLoop 清理循环
func (s *Session) cleanupLoop() {
	defer s.wg.Done()
	defer s.logger.Debug("清理循环退出")

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupIdleStreams()
		case <-s.ctx.Done():
			return
		}
	}
}

// cleanupIdleStreams 清理空闲流
func (s *Session) cleanupIdleStreams() {
	now := time.Now().Unix()
	timeout := int64(s.config.StreamIdleTimeout.Seconds())

	s.streamsMutex.Lock()
	defer s.streamsMutex.Unlock()

	var toRemove []StreamID
	for id, stream := range s.streams {
		if now-stream.lastActivity.Load() > timeout {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		stream := s.streams[id]
		stream.Reset(ErrorCodeCancel)
		delete(s.streams, id)
		s.stats.streamsClosed.Add(1)
		s.logger.Debug("清理空闲流", zap.Uint32("stream_id", uint32(id)))
	}
}

// removeStream 移除流
func (s *Session) removeStream(streamID StreamID) {
	s.streamsMutex.Lock()
	defer s.streamsMutex.Unlock()

	if stream, exists := s.streams[streamID]; exists {
		stream.forceClose()
		delete(s.streams, streamID)
		s.stats.streamsClosed.Add(1)
	}
}

// closeAllStreams 关闭所有流
func (s *Session) closeAllStreams() {
	s.streamsMutex.Lock()
	defer s.streamsMutex.Unlock()

	for id, stream := range s.streams {
		stream.forceClose()
		delete(s.streams, id)
		s.stats.streamsClosed.Add(1)
	}
}

// generateStreamID 生成新的流ID
func (s *Session) generateStreamID() StreamID {
	for {
		id := StreamID(s.nextStreamID.Add(2)) // 客户端奇数，服务端偶数
		if id <= MaxStreamID {
			return id
		}
		// 流ID耗尽，重置（简化处理）
		if s.isClient {
			s.nextStreamID.Store(1)
		} else {
			s.nextStreamID.Store(2)
		}
	}
}

// isValidStreamID 检查流ID是否有效
func (s *Session) isValidStreamID(streamID StreamID) bool {
	if streamID == 0 || streamID > MaxStreamID {
		return false
	}

	// 检查奇偶性
	isOdd := uint32(streamID)%2 == 1
	return s.isClient != isOdd // 客户端发起的流是奇数，服务端是偶数
}

// getLastStreamID 获取最后的流ID
func (s *Session) getLastStreamID() StreamID {
	s.streamsMutex.RLock()
	defer s.streamsMutex.RUnlock()

	var lastID StreamID
	for id := range s.streams {
		if id > lastID {
			lastID = id
		}
	}
	return lastID
}

// GetStats 获取会话统计信息
func (s *Session) GetStats() map[string]interface{} {
	s.streamsMutex.RLock()
	activeStreams := len(s.streams)
	s.streamsMutex.RUnlock()

	return map[string]interface{}{
		"is_client":              s.isClient,
		"active_streams":         activeStreams,
		"streams_created":        s.stats.streamsCreated.Load(),
		"streams_closed":         s.stats.streamsClosed.Load(),
		"frames_sent":            s.stats.framesSent.Load(),
		"frames_received":        s.stats.framesReceived.Load(),
		"bytes_sent":             s.stats.bytesSent.Load(),
		"bytes_received":         s.stats.bytesReceived.Load(),
		"pings_sent":             s.stats.pingsSent.Load(),
		"pongs_received":         s.stats.pongsReceived.Load(),
		"connection_send_window": s.connectionSendWindow.Load(),
		"connection_recv_window": s.connectionRecvWindow.Load(),
		"closed":                 s.IsClosed(),
	}
}

// FrameReader 帧读取器
type FrameReader struct {
	reader io.Reader
}

// NewFrameReader 创建帧读取器
func NewFrameReader(reader io.Reader) *FrameReader {
	return &FrameReader{reader: reader}
}

// ReadFrame 读取帧
func (fr *FrameReader) ReadFrame() (*Frame, error) {
	frame := &Frame{}
	_, err := frame.ReadFrom(fr.reader)
	return frame, err
}

// FrameWriter 帧写入器
type FrameWriter struct {
	writer io.Writer
}

// NewFrameWriter 创建帧写入器
func NewFrameWriter(writer io.Writer) *FrameWriter {
	return &FrameWriter{writer: writer}
}

// WriteFrame 写入帧
func (fw *FrameWriter) WriteFrame(frame *Frame) error {
	_, err := frame.WriteTo(fw.writer)
	return err
}





