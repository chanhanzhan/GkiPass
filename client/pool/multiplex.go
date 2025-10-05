package pool

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// FrameType 帧类型
type FrameType uint8

const (
	FrameTypeData  FrameType = 0x00 // 数据帧
	FrameTypeInit  FrameType = 0x01 // 初始化帧
	FrameTypeClose FrameType = 0x02 // 关闭帧
	FrameTypePing  FrameType = 0x03 // 心跳
	FrameTypePong  FrameType = 0x04 // 心跳响应
)

const (
	FrameVersion = 0x01      // 协议版本
	MaxFrameSize = 64 * 1024 // 最大帧大小 64KB
)

// Frame 多路复用帧
type Frame struct {
	Version   uint8
	Type      FrameType
	SessionID string
	Data      []byte
}

// Stream 多路复用流
type Stream struct {
	sessionID string
	sendChan  chan []byte
	recvChan  chan []byte
	closeChan chan struct{}
	closed    bool
	mu        sync.Mutex
}

// NewStream 创建流
func NewStream(sessionID string) *Stream {
	return &Stream{
		sessionID: sessionID,
		sendChan:  make(chan []byte, 256),
		recvChan:  make(chan []byte, 256),
		closeChan: make(chan struct{}),
	}
}

// Read 从流读取数据
func (s *Stream) Read(p []byte) (int, error) {
	select {
	case data := <-s.recvChan:
		n := copy(p, data)
		return n, nil
	case <-s.closeChan:
		return 0, io.EOF
	case <-time.After(30 * time.Second):
		return 0, fmt.Errorf("读超时")
	}
}

// Write 写入数据到流
func (s *Stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	s.mu.Unlock()

	select {
	case s.sendChan <- p:
		return len(p), nil
	case <-s.closeChan:
		return 0, io.ErrClosedPipe
	case <-time.After(5 * time.Second):
		return 0, fmt.Errorf("写超时")
	}
}

// Close 关闭流
func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.closeChan)
	return nil
}

// MultiplexConn 多路复用连接
type MultiplexConn struct {
	rawConn  net.Conn
	streams  map[string]*Stream
	sendChan chan *Frame
	mu       sync.RWMutex
	stopChan chan struct{}
	closed   bool
}

// NewMultiplexConn 创建多路复用连接
func NewMultiplexConn(rawConn net.Conn) *MultiplexConn {
	mc := &MultiplexConn{
		rawConn:  rawConn,
		streams:  make(map[string]*Stream),
		sendChan: make(chan *Frame, 1024),
		stopChan: make(chan struct{}),
	}

	// 启动读写循环
	go mc.readLoop()
	go mc.writeLoop()

	return mc
}

// OpenStream 打开新流
func (mc *MultiplexConn) OpenStream(sessionID string) (*Stream, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return nil, fmt.Errorf("连接已关闭")
	}

	if _, exists := mc.streams[sessionID]; exists {
		return nil, fmt.Errorf("流已存在: %s", sessionID)
	}

	stream := NewStream(sessionID)
	mc.streams[sessionID] = stream

	logger.Debug("打开新流", zap.String("session_id", sessionID))
	return stream, nil
}

// CloseStream 关闭流
func (mc *MultiplexConn) CloseStream(sessionID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if stream, exists := mc.streams[sessionID]; exists {
		stream.Close()
		delete(mc.streams, sessionID)
		logger.Debug("关闭流", zap.String("session_id", sessionID))
	}
}

// GetStream 获取流
func (mc *MultiplexConn) GetStream(sessionID string) (*Stream, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	stream, exists := mc.streams[sessionID]
	return stream, exists
}

// SendFrame 发送帧
func (mc *MultiplexConn) SendFrame(frame *Frame) error {
	select {
	case mc.sendChan <- frame:
		return nil
	case <-mc.stopChan:
		return io.ErrClosedPipe
	case <-time.After(5 * time.Second):
		return fmt.Errorf("发送帧超时")
	}
}

// readLoop 读取循环
func (mc *MultiplexConn) readLoop() {
	defer mc.Close()

	for {
		frame, err := mc.readFrame()
		if err != nil {
			if err != io.EOF {
				logger.Error("读取帧失败", zap.Error(err))
			}
			return
		}

		// 处理帧
		switch frame.Type {
		case FrameTypeData:
			mc.handleDataFrame(frame)
		case FrameTypeClose:
			mc.CloseStream(frame.SessionID)
		case FrameTypePing:
			mc.SendFrame(&Frame{Version: FrameVersion, Type: FrameTypePong})
		case FrameTypePong:
			// 忽略
		}
	}
}

// writeLoop 写入循环
func (mc *MultiplexConn) writeLoop() {
	for {
		select {
		case frame := <-mc.sendChan:
			if err := mc.writeFrame(frame); err != nil {
				logger.Error("写入帧失败", zap.Error(err))
				return
			}
		case <-mc.stopChan:
			return
		}
	}
}

// readFrame 读取一个帧
func (mc *MultiplexConn) readFrame() (*Frame, error) {
	// 读取帧头 (8字节)
	header := make([]byte, 8)
	if _, err := io.ReadFull(mc.rawConn, header); err != nil {
		return nil, err
	}

	version := header[0]
	frameType := FrameType(header[1])
	sessionIDLen := binary.BigEndian.Uint16(header[2:4])
	dataLen := binary.BigEndian.Uint32(header[4:8])

	// 检查帧大小
	if dataLen > MaxFrameSize {
		return nil, fmt.Errorf("帧太大: %d", dataLen)
	}

	// 读取SessionID
	sessionID := make([]byte, sessionIDLen)
	if sessionIDLen > 0 {
		if _, err := io.ReadFull(mc.rawConn, sessionID); err != nil {
			return nil, err
		}
	}

	// 读取数据
	data := make([]byte, dataLen)
	if dataLen > 0 {
		if _, err := io.ReadFull(mc.rawConn, data); err != nil {
			return nil, err
		}
	}

	return &Frame{
		Version:   version,
		Type:      frameType,
		SessionID: string(sessionID),
		Data:      data,
	}, nil
}

// writeFrame 写入一个帧
func (mc *MultiplexConn) writeFrame(frame *Frame) error {
	sessionIDBytes := []byte(frame.SessionID)

	// 构建帧头
	header := make([]byte, 8)
	header[0] = frame.Version
	header[1] = uint8(frame.Type)
	binary.BigEndian.PutUint16(header[2:4], uint16(len(sessionIDBytes)))
	binary.BigEndian.PutUint32(header[4:8], uint32(len(frame.Data)))

	// 写入帧头
	if _, err := mc.rawConn.Write(header); err != nil {
		return err
	}

	// 写入SessionID
	if len(sessionIDBytes) > 0 {
		if _, err := mc.rawConn.Write(sessionIDBytes); err != nil {
			return err
		}
	}

	// 写入数据
	if len(frame.Data) > 0 {
		if _, err := mc.rawConn.Write(frame.Data); err != nil {
			return err
		}
	}

	return nil
}

// handleDataFrame 处理数据帧
func (mc *MultiplexConn) handleDataFrame(frame *Frame) {
	stream, exists := mc.GetStream(frame.SessionID)
	if !exists {
		logger.Warn("收到未知流的数据", zap.String("session_id", frame.SessionID))
		return
	}

	select {
	case stream.recvChan <- frame.Data:
	case <-stream.closeChan:
	case <-time.After(1 * time.Second):
		logger.Warn("流接收队列满", zap.String("session_id", frame.SessionID))
	}
}

// Close 关闭多路复用连接
func (mc *MultiplexConn) Close() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return nil
	}

	mc.closed = true
	close(mc.stopChan)

	// 关闭所有流
	for _, stream := range mc.streams {
		stream.Close()
	}
	mc.streams = nil

	// 关闭底层连接
	return mc.rawConn.Close()
}

// StreamCount 获取流数量
func (mc *MultiplexConn) StreamCount() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return len(mc.streams)
}






