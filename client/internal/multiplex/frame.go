package multiplex

import (
	"encoding/binary"
	"fmt"
	"io"
)

// FrameType 帧类型
type FrameType uint8

const (
	FrameTypeData         FrameType = 0x00 // 数据帧
	FrameTypeWindowUpdate FrameType = 0x01 // 窗口更新帧
	FrameTypeStreamClose  FrameType = 0x02 // 流关闭帧
	FrameTypePing         FrameType = 0x03 // Ping帧
	FrameTypePong         FrameType = 0x04 // Pong帧
	FrameTypeGoAway       FrameType = 0x05 // 连接关闭帧
	FrameTypeStreamReset  FrameType = 0x06 // 流重置帧
	FrameTypeSettings     FrameType = 0x07 // 设置帧
	FrameTypeStreamOpen   FrameType = 0x08 // 流打开帧
)

// String 返回帧类型名称
func (ft FrameType) String() string {
	switch ft {
	case FrameTypeData:
		return "DATA"
	case FrameTypeWindowUpdate:
		return "WINDOW_UPDATE"
	case FrameTypeStreamClose:
		return "STREAM_CLOSE"
	case FrameTypePing:
		return "PING"
	case FrameTypePong:
		return "PONG"
	case FrameTypeGoAway:
		return "GO_AWAY"
	case FrameTypeStreamReset:
		return "STREAM_RESET"
	case FrameTypeSettings:
		return "SETTINGS"
	case FrameTypeStreamOpen:
		return "STREAM_OPEN"
	default:
		return fmt.Sprintf("UNKNOWN_%d", uint8(ft))
	}
}

// StreamID 流ID类型
type StreamID uint32

// Frame 帧结构
type Frame struct {
	Type     FrameType // 帧类型 (1 byte)
	Flags    uint8     // 标志位 (1 byte)
	StreamID StreamID  // 流ID (4 bytes)
	Length   uint32    // 负载长度 (4 bytes)
	Payload  []byte    // 负载数据
}

// 帧标志位
const (
	FlagEndStream  uint8 = 0x01 // 流结束标志
	FlagEndHeaders uint8 = 0x02 // 头部结束标志
	FlagPadded     uint8 = 0x04 // 填充标志
	FlagPriority   uint8 = 0x08 // 优先级标志
	FlagAck        uint8 = 0x10 // 确认标志
)

// 帧头长度 (类型1 + 标志1 + 流ID4 + 长度4 = 10字节)
const FrameHeaderLength = 10

// 最大帧长度 (1MB)
const MaxFrameLength = 1024 * 1024

// 最大流ID
const MaxStreamID = StreamID(0x7FFFFFFF)

// NewFrame 创建新帧
func NewFrame(frameType FrameType, streamID StreamID, payload []byte) *Frame {
	frame := &Frame{
		Type:     frameType,
		StreamID: streamID,
		Payload:  payload,
	}

	if payload != nil {
		frame.Length = uint32(len(payload))
	}

	return frame
}

// NewDataFrame 创建数据帧
func NewDataFrame(streamID StreamID, data []byte, endStream bool) *Frame {
	frame := NewFrame(FrameTypeData, streamID, data)
	if endStream {
		frame.Flags |= FlagEndStream
	}
	return frame
}

// NewWindowUpdateFrame 创建窗口更新帧
func NewWindowUpdateFrame(streamID StreamID, increment uint32) *Frame {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, increment)
	return NewFrame(FrameTypeWindowUpdate, streamID, payload)
}

// NewStreamCloseFrame 创建流关闭帧
func NewStreamCloseFrame(streamID StreamID) *Frame {
	frame := NewFrame(FrameTypeStreamClose, streamID, nil)
	frame.Flags |= FlagEndStream
	return frame
}

// NewStreamResetFrame 创建流重置帧
func NewStreamResetFrame(streamID StreamID, errorCode uint32) *Frame {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, errorCode)
	return NewFrame(FrameTypeStreamReset, streamID, payload)
}

// NewPingFrame 创建Ping帧
func NewPingFrame(data []byte, ack bool) *Frame {
	frame := NewFrame(FrameTypePing, 0, data)
	if ack {
		frame.Flags |= FlagAck
	}
	return frame
}

// NewPongFrame 创建Pong帧
func NewPongFrame(data []byte) *Frame {
	frame := NewFrame(FrameTypePong, 0, data)
	frame.Flags |= FlagAck
	return frame
}

// NewGoAwayFrame 创建连接关闭帧
func NewGoAwayFrame(lastStreamID StreamID, errorCode uint32, debugData []byte) *Frame {
	payload := make([]byte, 8+len(debugData))
	binary.BigEndian.PutUint32(payload[0:4], uint32(lastStreamID))
	binary.BigEndian.PutUint32(payload[4:8], errorCode)
	copy(payload[8:], debugData)
	return NewFrame(FrameTypeGoAway, 0, payload)
}

// NewStreamOpenFrame 创建流打开帧
func NewStreamOpenFrame(streamID StreamID, priority uint8) *Frame {
	payload := []byte{priority}
	return NewFrame(FrameTypeStreamOpen, streamID, payload)
}

// IsEndStream 检查是否为流结束帧
func (f *Frame) IsEndStream() bool {
	return f.Flags&FlagEndStream != 0
}

// IsAck 检查是否为确认帧
func (f *Frame) IsAck() bool {
	return f.Flags&FlagAck != 0
}

// GetWindowIncrement 获取窗口增量（仅适用于WINDOW_UPDATE帧）
func (f *Frame) GetWindowIncrement() uint32 {
	if f.Type != FrameTypeWindowUpdate || len(f.Payload) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(f.Payload)
}

// GetErrorCode 获取错误码（适用于STREAM_RESET和GO_AWAY帧）
func (f *Frame) GetErrorCode() uint32 {
	if (f.Type != FrameTypeStreamReset && f.Type != FrameTypeGoAway) || len(f.Payload) < 4 {
		return 0
	}
	if f.Type == FrameTypeGoAway {
		return binary.BigEndian.Uint32(f.Payload[4:8])
	}
	return binary.BigEndian.Uint32(f.Payload)
}

// GetLastStreamID 获取最后的流ID（仅适用于GO_AWAY帧）
func (f *Frame) GetLastStreamID() StreamID {
	if f.Type != FrameTypeGoAway || len(f.Payload) < 4 {
		return 0
	}
	return StreamID(binary.BigEndian.Uint32(f.Payload[0:4]))
}

// GetPriority 获取优先级（仅适用于STREAM_OPEN帧）
func (f *Frame) GetPriority() uint8 {
	if f.Type != FrameTypeStreamOpen || len(f.Payload) < 1 {
		return 0
	}
	return f.Payload[0]
}

// WriteTo 将帧写入到Writer
func (f *Frame) WriteTo(w io.Writer) (int64, error) {
	// 写入帧头
	header := make([]byte, FrameHeaderLength)
	header[0] = uint8(f.Type)
	header[1] = f.Flags
	binary.BigEndian.PutUint32(header[2:6], uint32(f.StreamID))
	binary.BigEndian.PutUint32(header[6:10], f.Length)

	n, err := w.Write(header)
	if err != nil {
		return int64(n), err
	}

	written := int64(n)

	// 写入负载
	if f.Length > 0 && f.Payload != nil {
		n, err = w.Write(f.Payload)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

// ReadFrom 从Reader读取帧
func (f *Frame) ReadFrom(r io.Reader) (int64, error) {
	// 读取帧头
	header := make([]byte, FrameHeaderLength)
	n, err := io.ReadFull(r, header)
	if err != nil {
		return int64(n), err
	}

	read := int64(n)

	// 解析帧头
	f.Type = FrameType(header[0])
	f.Flags = header[1]
	f.StreamID = StreamID(binary.BigEndian.Uint32(header[2:6]))
	f.Length = binary.BigEndian.Uint32(header[6:10])

	// 验证帧长度
	if f.Length > MaxFrameLength {
		return read, fmt.Errorf("帧长度过大: %d > %d", f.Length, MaxFrameLength)
	}

	// 读取负载
	if f.Length > 0 {
		f.Payload = make([]byte, f.Length)
		n, err = io.ReadFull(r, f.Payload)
		read += int64(n)
		if err != nil {
			return read, err
		}
	}

	return read, nil
}

// String 返回帧的字符串表示
func (f *Frame) String() string {
	return fmt.Sprintf("Frame{Type:%s, Flags:0x%02x, StreamID:%d, Length:%d}",
		f.Type.String(), f.Flags, f.StreamID, f.Length)
}

// Clone 复制帧
func (f *Frame) Clone() *Frame {
	clone := &Frame{
		Type:     f.Type,
		Flags:    f.Flags,
		StreamID: f.StreamID,
		Length:   f.Length,
	}

	if f.Payload != nil {
		clone.Payload = make([]byte, len(f.Payload))
		copy(clone.Payload, f.Payload)
	}

	return clone
}

// Validate 验证帧的有效性
func (f *Frame) Validate() error {
	// 检查流ID
	if f.StreamID > MaxStreamID {
		return fmt.Errorf("无效的流ID: %d", f.StreamID)
	}

	// 检查负载长度
	if f.Length != uint32(len(f.Payload)) {
		return fmt.Errorf("负载长度不匹配: 声明%d，实际%d", f.Length, len(f.Payload))
	}

	// 检查特定帧类型的要求
	switch f.Type {
	case FrameTypeWindowUpdate:
		if f.Length != 4 {
			return fmt.Errorf("WINDOW_UPDATE帧负载长度必须为4字节")
		}
		increment := f.GetWindowIncrement()
		if increment == 0 {
			return fmt.Errorf("WINDOW_UPDATE帧增量不能为0")
		}

	case FrameTypeStreamReset:
		if f.Length != 4 {
			return fmt.Errorf("STREAM_RESET帧负载长度必须为4字节")
		}

	case FrameTypeGoAway:
		if f.Length < 8 {
			return fmt.Errorf("GO_AWAY帧负载长度至少为8字节")
		}
		if f.StreamID != 0 {
			return fmt.Errorf("GO_AWAY帧的流ID必须为0")
		}

	case FrameTypePing, FrameTypePong:
		if f.StreamID != 0 {
			return fmt.Errorf("PING/PONG帧的流ID必须为0")
		}

	case FrameTypeSettings:
		if f.StreamID != 0 {
			return fmt.Errorf("SETTINGS帧的流ID必须为0")
		}
	}

	return nil
}

// ErrorCode 错误码
type ErrorCode uint32

const (
	ErrorCodeNoError            ErrorCode = 0x0
	ErrorCodeProtocolError      ErrorCode = 0x1
	ErrorCodeInternalError      ErrorCode = 0x2
	ErrorCodeFlowControlError   ErrorCode = 0x3
	ErrorCodeStreamClosed       ErrorCode = 0x5
	ErrorCodeFrameSizeError     ErrorCode = 0x6
	ErrorCodeRefusedStream      ErrorCode = 0x7
	ErrorCodeCancel             ErrorCode = 0x8
	ErrorCodeCompressionError   ErrorCode = 0x9
	ErrorCodeConnectError       ErrorCode = 0xa
	ErrorCodeEnhanceYourCalm    ErrorCode = 0xb
	ErrorCodeInadequateSecurity ErrorCode = 0xc
	ErrorCodeHTTP11Required     ErrorCode = 0xd
)

// String 返回错误码名称
func (ec ErrorCode) String() string {
	switch ec {
	case ErrorCodeNoError:
		return "NO_ERROR"
	case ErrorCodeProtocolError:
		return "PROTOCOL_ERROR"
	case ErrorCodeInternalError:
		return "INTERNAL_ERROR"
	case ErrorCodeFlowControlError:
		return "FLOW_CONTROL_ERROR"
	case ErrorCodeStreamClosed:
		return "STREAM_CLOSED"
	case ErrorCodeFrameSizeError:
		return "FRAME_SIZE_ERROR"
	case ErrorCodeRefusedStream:
		return "REFUSED_STREAM"
	case ErrorCodeCancel:
		return "CANCEL"
	case ErrorCodeCompressionError:
		return "COMPRESSION_ERROR"
	case ErrorCodeConnectError:
		return "CONNECT_ERROR"
	case ErrorCodeEnhanceYourCalm:
		return "ENHANCE_YOUR_CALM"
	case ErrorCodeInadequateSecurity:
		return "INADEQUATE_SECURITY"
	case ErrorCodeHTTP11Required:
		return "HTTP_1_1_REQUIRED"
	default:
		return fmt.Sprintf("UNKNOWN_ERROR_%d", uint32(ec))
	}
}





