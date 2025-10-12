package multiplex

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// StreamState 流状态
type StreamState int32

const (
	StreamStateIdle           StreamState = iota // 空闲状态
	StreamStateOpen                              // 打开状态
	StreamStateHalfClosed                        // 半关闭状态
	StreamStateClosed                            // 关闭状态
	StreamStateReservedLocal                     // 本地保留
	StreamStateReservedRemote                    // 远程保留
)

// String 返回流状态名称
func (s StreamState) String() string {
	switch s {
	case StreamStateIdle:
		return "IDLE"
	case StreamStateOpen:
		return "OPEN"
	case StreamStateHalfClosed:
		return "HALF_CLOSED"
	case StreamStateClosed:
		return "CLOSED"
	case StreamStateReservedLocal:
		return "RESERVED_LOCAL"
	case StreamStateReservedRemote:
		return "RESERVED_REMOTE"
	default:
		return fmt.Sprintf("UNKNOWN_%d", int32(s))
	}
}

// Stream 多路复用流
type Stream struct {
	id       StreamID
	session  *Session
	state    atomic.Int32 // StreamState
	priority uint8

	// 读写控制
	readBuf     *RingBuffer
	writeBuf    *RingBuffer
	readClosed  atomic.Bool
	writeClosed atomic.Bool

	// 流控制
	sendWindow    atomic.Int64 // 发送窗口
	recvWindow    atomic.Int64 // 接收窗口
	maxRecvWindow int64        // 最大接收窗口

	// 通知通道
	readNotify  chan struct{}
	writeNotify chan struct{}
	closeNotify chan struct{}

	// 统计信息
	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
	createdAt    time.Time
	lastActivity atomic.Int64 // Unix timestamp

	// 上下文
	ctx    context.Context
	cancel context.CancelFunc

	// 锁
	readMutex  sync.Mutex
	writeMutex sync.Mutex
	stateMutex sync.RWMutex

	logger *zap.Logger
}

// NewStream 创建新流
func NewStream(id StreamID, session *Session, initialWindow int64) *Stream {
	ctx, cancel := context.WithCancel(session.ctx)

	stream := &Stream{
		id:            id,
		session:       session,
		priority:      0,
		readBuf:       NewRingBuffer(32 * 1024), // 32KB读缓冲
		writeBuf:      NewRingBuffer(32 * 1024), // 32KB写缓冲
		maxRecvWindow: initialWindow,
		readNotify:    make(chan struct{}, 1),
		writeNotify:   make(chan struct{}, 1),
		closeNotify:   make(chan struct{}),
		createdAt:     time.Now(),
		ctx:           ctx,
		cancel:        cancel,
		logger:        session.logger.Named(fmt.Sprintf("stream-%d", id)),
	}

	// 设置初始窗口大小
	stream.sendWindow.Store(initialWindow)
	stream.recvWindow.Store(initialWindow)
	stream.lastActivity.Store(time.Now().Unix())

	// 设置初始状态
	stream.setState(StreamStateIdle)

	return stream
}

// ID 获取流ID
func (s *Stream) ID() StreamID {
	return s.id
}

// State 获取流状态
func (s *Stream) State() StreamState {
	return StreamState(s.state.Load())
}

// setState 设置流状态
func (s *Stream) setState(state StreamState) {
	old := StreamState(s.state.Swap(int32(state)))
	if old != state {
		s.logger.Debug("流状态变更",
			zap.String("old_state", old.String()),
			zap.String("new_state", state.String()))
	}
}

// Read 从流中读取数据
func (s *Stream) Read(p []byte) (n int, err error) {
	s.readMutex.Lock()
	defer s.readMutex.Unlock()

	// 检查流状态
	if s.readClosed.Load() {
		return 0, io.EOF
	}

	// 从缓冲区读取数据
	for {
		n, err = s.readBuf.Read(p)
		if n > 0 {
			s.bytesRead.Add(int64(n))
			s.updateActivity()

			// 更新接收窗口
			s.updateRecvWindow(int64(n))
			return n, nil
		}

		if err != nil {
			return 0, err
		}

		// 缓冲区为空，等待数据
		select {
		case <-s.readNotify:
			// 有新数据到达，继续读取
			continue
		case <-s.closeNotify:
			return 0, io.EOF
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		}
	}
}

// Write 向流中写入数据
func (s *Stream) Write(p []byte) (n int, err error) {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	// 检查流状态
	if s.writeClosed.Load() {
		return 0, fmt.Errorf("流已关闭写入")
	}

	remaining := len(p)
	written := 0

	for remaining > 0 {
		// 检查发送窗口
		window := s.sendWindow.Load()
		if window <= 0 {
			// 等待窗口更新
			select {
			case <-s.writeNotify:
				continue
			case <-s.ctx.Done():
				return written, s.ctx.Err()
			}
		}

		// 计算本次可发送的数据量
		chunkSize := remaining
		if int64(chunkSize) > window {
			chunkSize = int(window)
		}

		// 准备数据帧
		chunk := p[written : written+chunkSize]
		frame := NewDataFrame(s.id, chunk, false)

		// 发送帧
		if err := s.session.writeFrame(frame); err != nil {
			return written, err
		}

		// 更新状态
		written += chunkSize
		remaining -= chunkSize
		s.sendWindow.Add(-int64(chunkSize))
		s.bytesWritten.Add(int64(chunkSize))
		s.updateActivity()
	}

	return written, nil
}

// Close 关闭流
func (s *Stream) Close() error {
	return s.CloseWrite()
}

// CloseRead 关闭读取端
func (s *Stream) CloseRead() error {
	if s.readClosed.CompareAndSwap(false, true) {
		s.logger.Debug("关闭流读取端")
		close(s.closeNotify)
		s.notifyRead()
	}
	return nil
}

// CloseWrite 关闭写入端
func (s *Stream) CloseWrite() error {
	if s.writeClosed.CompareAndSwap(false, true) {
		s.logger.Debug("关闭流写入端")

		// 发送流关闭帧
		frame := NewStreamCloseFrame(s.id)
		if err := s.session.writeFrame(frame); err != nil {
			return err
		}

		s.notifyWrite()
	}
	return nil
}

// Reset 重置流
func (s *Stream) Reset(errorCode ErrorCode) error {
	s.logger.Info("重置流", zap.String("error", errorCode.String()))

	// 发送重置帧
	frame := NewStreamResetFrame(s.id, uint32(errorCode))
	if err := s.session.writeFrame(frame); err != nil {
		return err
	}

	// 关闭流
	s.forceClose()
	return nil
}

// forceClose 强制关闭流
func (s *Stream) forceClose() {
	s.readClosed.Store(true)
	s.writeClosed.Store(true)
	s.setState(StreamStateClosed)

	if s.cancel != nil {
		s.cancel()
	}

	// 通知所有等待的操作
	s.notifyRead()
	s.notifyWrite()
	close(s.closeNotify)
}

// IsClosed 检查流是否已关闭
func (s *Stream) IsClosed() bool {
	return s.State() == StreamStateClosed
}

// IsReadClosed 检查读取端是否已关闭
func (s *Stream) IsReadClosed() bool {
	return s.readClosed.Load()
}

// IsWriteClosed 检查写入端是否已关闭
func (s *Stream) IsWriteClosed() bool {
	return s.writeClosed.Load()
}

// handleFrame 处理接收到的帧
func (s *Stream) handleFrame(frame *Frame) error {
	s.updateActivity()

	switch frame.Type {
	case FrameTypeData:
		return s.handleDataFrame(frame)
	case FrameTypeWindowUpdate:
		return s.handleWindowUpdateFrame(frame)
	case FrameTypeStreamClose:
		return s.handleStreamCloseFrame(frame)
	case FrameTypeStreamReset:
		return s.handleStreamResetFrame(frame)
	default:
		s.logger.Warn("未知帧类型", zap.String("type", frame.Type.String()))
		return nil
	}
}

// handleDataFrame 处理数据帧
func (s *Stream) handleDataFrame(frame *Frame) error {
	// 检查接收窗口
	if s.recvWindow.Load() < int64(frame.Length) {
		return fmt.Errorf("接收窗口不足")
	}

	// 写入数据到缓冲区
	if frame.Length > 0 {
		if err := s.readBuf.Write(frame.Payload); err != nil {
			return err
		}

		// 更新接收窗口
		s.recvWindow.Add(-int64(frame.Length))
	}

	// 如果是流结束帧，关闭读取端
	if frame.IsEndStream() {
		s.CloseRead()
	}

	// 通知读取操作
	s.notifyRead()

	return nil
}

// handleWindowUpdateFrame 处理窗口更新帧
func (s *Stream) handleWindowUpdateFrame(frame *Frame) error {
	increment := frame.GetWindowIncrement()
	if increment == 0 {
		return fmt.Errorf("窗口更新增量不能为0")
	}

	// 更新发送窗口
	newWindow := s.sendWindow.Add(int64(increment))
	s.logger.Debug("更新发送窗口",
		zap.Uint32("increment", increment),
		zap.Int64("new_window", newWindow))

	// 通知写入操作
	s.notifyWrite()

	return nil
}

// handleStreamCloseFrame 处理流关闭帧
func (s *Stream) handleStreamCloseFrame(frame *Frame) error {
	s.logger.Debug("收到流关闭帧")
	s.CloseRead()
	return nil
}

// handleStreamResetFrame 处理流重置帧
func (s *Stream) handleStreamResetFrame(frame *Frame) error {
	errorCode := ErrorCode(frame.GetErrorCode())
	s.logger.Info("收到流重置帧", zap.String("error", errorCode.String()))
	s.forceClose()
	return nil
}

// updateRecvWindow 更新接收窗口
func (s *Stream) updateRecvWindow(consumed int64) {
	// 如果消费了超过窗口一半的数据，发送窗口更新
	if consumed > s.maxRecvWindow/2 {
		increment := uint32(consumed)
		frame := NewWindowUpdateFrame(s.id, increment)

		if err := s.session.writeFrame(frame); err != nil {
			s.logger.Error("发送窗口更新失败", zap.Error(err))
		} else {
			s.recvWindow.Add(consumed)
			s.logger.Debug("发送窗口更新",
				zap.Uint32("increment", increment),
				zap.Int64("new_window", s.recvWindow.Load()))
		}
	}
}

// updateActivity 更新活动时间
func (s *Stream) updateActivity() {
	s.lastActivity.Store(time.Now().Unix())
}

// notifyRead 通知读取操作
func (s *Stream) notifyRead() {
	select {
	case s.readNotify <- struct{}{}:
	default:
	}
}

// notifyWrite 通知写入操作
func (s *Stream) notifyWrite() {
	select {
	case s.writeNotify <- struct{}{}:
	default:
	}
}

// GetStats 获取流统计信息
func (s *Stream) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"id":                s.id,
		"state":             s.State().String(),
		"priority":          s.priority,
		"bytes_read":        s.bytesRead.Load(),
		"bytes_written":     s.bytesWritten.Load(),
		"send_window":       s.sendWindow.Load(),
		"recv_window":       s.recvWindow.Load(),
		"read_closed":       s.readClosed.Load(),
		"write_closed":      s.writeClosed.Load(),
		"created_at":        s.createdAt,
		"last_activity":     time.Unix(s.lastActivity.Load(), 0),
		"read_buffer_size":  s.readBuf.Len(),
		"write_buffer_size": s.writeBuf.Len(),
	}
}

// RingBuffer 环形缓冲区
type RingBuffer struct {
	buf   []byte
	start int
	end   int
	size  int
	mutex sync.RWMutex
}

// NewRingBuffer 创建环形缓冲区
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([]byte, capacity),
	}
}

// Write 写入数据
func (rb *RingBuffer) Write(data []byte) error {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	dataLen := len(data)
	if dataLen > rb.available() {
		return fmt.Errorf("缓冲区空间不足")
	}

	for i, b := range data {
		rb.buf[rb.end] = b
		rb.end = (rb.end + 1) % len(rb.buf)
		if i == dataLen-1 {
			rb.size += dataLen
		}
	}

	return nil
}

// Read 读取数据
func (rb *RingBuffer) Read(p []byte) (n int, err error) {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	if rb.size == 0 {
		return 0, fmt.Errorf("缓冲区为空")
	}

	n = len(p)
	if n > rb.size {
		n = rb.size
	}

	for i := 0; i < n; i++ {
		p[i] = rb.buf[rb.start]
		rb.start = (rb.start + 1) % len(rb.buf)
	}

	rb.size -= n
	return n, nil
}

// Len 获取缓冲区中的数据长度
func (rb *RingBuffer) Len() int {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()
	return rb.size
}

// available 获取可用空间
func (rb *RingBuffer) available() int {
	return len(rb.buf) - rb.size
}





