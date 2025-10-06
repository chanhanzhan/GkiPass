package protocol

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// Protocol 协议类型
type Protocol string

const (
	ProtocolTCP       Protocol = "tcp"
	ProtocolHTTP      Protocol = "http"
	ProtocolHTTP2     Protocol = "http2"
	ProtocolHTTPS     Protocol = "https"
	ProtocolTLS       Protocol = "tls"
	ProtocolTLS13     Protocol = "tls1.3"
	ProtocolSOCKS4    Protocol = "socks4"
	ProtocolSOCKS5    Protocol = "socks5"
	ProtocolSSH       Protocol = "ssh"
	ProtocolSSH2      Protocol = "ssh2"
	ProtocolQUIC      Protocol = "quic"
	ProtocolWebSocket Protocol = "websocket"
	ProtocolUnknown   Protocol = "unknown"
)

// String 返回协议名称
func (p Protocol) String() string {
	return string(p)
}

// ProtocolInfo 协议详细信息
type ProtocolInfo struct {
	Protocol   Protocol
	Version    string
	Confidence float64 // 检测置信度 0-1
	ExtraInfo  map[string]string
}

// DetectionStats 检测统计信息
type DetectionStats struct {
	TotalDetections atomic.Int64
	HTTPCount       atomic.Int64
	HTTPSCount      atomic.Int64
	TLSCount        atomic.Int64
	SOCKS5Count     atomic.Int64
	SSHCount        atomic.Int64
	UnknownCount    atomic.Int64
	AvgDetectTime   atomic.Int64 // 纳秒
}

// ProtocolDetector 增强的协议检测器
type ProtocolDetector struct {
	stats         DetectionStats
	peekSize      int
	detectTimeout time.Duration
}

// NewProtocolDetector 创建协议检测器
func NewProtocolDetector() *ProtocolDetector {
	return &ProtocolDetector{
		peekSize:      256, // 读取更多数据以提高检测准确性
		detectTimeout: 3 * time.Second,
	}
}

// DetectProtocol 检测协议类型（兼容旧接口）
func DetectProtocol(conn net.Conn) (Protocol, []byte, error) {
	detector := NewProtocolDetector()
	info, data, err := detector.DetectWithInfo(conn)
	if err != nil {
		return ProtocolUnknown, nil, err
	}
	return info.Protocol, data, nil
}

// DetectWithInfo 检测协议并返回详细信息
func (pd *ProtocolDetector) DetectWithInfo(conn net.Conn) (*ProtocolInfo, []byte, error) {
	startTime := time.Now()
	pd.stats.TotalDetections.Add(1)

	// 设置读超时
	conn.SetReadDeadline(time.Now().Add(pd.detectTimeout))
	defer conn.SetReadDeadline(time.Time{})

	// 读取数据用于检测
	buf := make([]byte, pd.peekSize)
	n, err := io.ReadAtLeast(conn, buf, 1)
	if err != nil {
		pd.stats.UnknownCount.Add(1)
		return &ProtocolInfo{
			Protocol:   ProtocolUnknown,
			Confidence: 0.0,
		}, nil, err
	}

	data := buf[:n]
	info := pd.detectEnhanced(data)

	// 更新统计
	switch info.Protocol {
	case ProtocolHTTP, ProtocolHTTP2:
		pd.stats.HTTPCount.Add(1)
	case ProtocolHTTPS, ProtocolTLS, ProtocolTLS13:
		pd.stats.HTTPSCount.Add(1)
		pd.stats.TLSCount.Add(1)
	case ProtocolSOCKS5, ProtocolSOCKS4:
		pd.stats.SOCKS5Count.Add(1)
	case ProtocolSSH, ProtocolSSH2:
		pd.stats.SSHCount.Add(1)
	default:
		pd.stats.UnknownCount.Add(1)
	}

	// 更新平均检测时间
	detectTime := time.Since(startTime).Nanoseconds()
	pd.stats.AvgDetectTime.Store(detectTime)

	logger.Debug("协议检测完成",
		zap.String("protocol", info.Protocol.String()),
		zap.Float64("confidence", info.Confidence),
		zap.Duration("detect_time", time.Since(startTime)))

	return info, data, nil
}

// GetStats 获取检测统计信息
func (pd *ProtocolDetector) GetStats() DetectionStatsSnapshot {
	return DetectionStatsSnapshot{
		TotalDetections: pd.stats.TotalDetections.Load(),
		HTTPCount:       pd.stats.HTTPCount.Load(),
		HTTPSCount:      pd.stats.HTTPSCount.Load(),
		TLSCount:        pd.stats.TLSCount.Load(),
		SOCKS5Count:     pd.stats.SOCKS5Count.Load(),
		SSHCount:        pd.stats.SSHCount.Load(),
		UnknownCount:    pd.stats.UnknownCount.Load(),
		AvgDetectTime:   time.Duration(pd.stats.AvgDetectTime.Load()),
	}
}

// DetectionStatsSnapshot 检测统计快照
type DetectionStatsSnapshot struct {
	TotalDetections int64
	HTTPCount       int64
	HTTPSCount      int64
	TLSCount        int64
	SOCKS5Count     int64
	SSHCount        int64
	UnknownCount    int64
	AvgDetectTime   time.Duration
}

// detectEnhanced 增强的协议检测（带置信度和详细信息）
func (pd *ProtocolDetector) detectEnhanced(data []byte) *ProtocolInfo {
	if len(data) == 0 {
		return &ProtocolInfo{
			Protocol:   ProtocolUnknown,
			Confidence: 0.0,
		}
	}

	// HTTP 方法检测（高置信度）
	httpMethods := [][]byte{
		[]byte("GET "), []byte("POST "), []byte("PUT "), []byte("DELETE "),
		[]byte("HEAD "), []byte("OPTIONS "), []byte("PATCH "), []byte("CONNECT "),
		[]byte("TRACE "),
	}
	for _, method := range httpMethods {
		if bytes.HasPrefix(data, method) {
			// 检查是否有HTTP版本标识
			if bytes.Contains(data, []byte("HTTP/")) {
				if bytes.Contains(data, []byte("HTTP/2")) {
					return &ProtocolInfo{
						Protocol:   ProtocolHTTP2,
						Version:    "2.0",
						Confidence: 1.0,
						ExtraInfo:  map[string]string{"method": string(bytes.TrimSpace(method))},
					}
				}
				return &ProtocolInfo{
					Protocol:   ProtocolHTTP,
					Version:    "1.1",
					Confidence: 1.0,
					ExtraInfo:  map[string]string{"method": string(bytes.TrimSpace(method))},
				}
			}
			return &ProtocolInfo{
				Protocol:   ProtocolHTTP,
				Confidence: 0.9,
				ExtraInfo:  map[string]string{"method": string(bytes.TrimSpace(method))},
			}
		}
	}

	// TLS/SSL ClientHello
	if len(data) >= 5 && data[0] == 0x16 && data[1] == 0x03 {
		var version string
		var protocol Protocol = ProtocolTLS
		confidence := 1.0

		switch data[2] {
		case 0x01:
			version = "1.0"
		case 0x02:
			version = "1.1"
		case 0x03:
			version = "1.2"
		case 0x04:
			version = "1.3"
			protocol = ProtocolTLS13
		default:
			confidence = 0.7
		}

		extraInfo := map[string]string{"version": version}

		// 尝试解析SNI（Server Name Indication）
		if len(data) >= 43 {
			// TLS ClientHello 通常在第43字节之后包含扩展
			// 这是简化实现，完整SNI解析需要更复杂的逻辑
			extraInfo["has_extensions"] = "true"
		}

		return &ProtocolInfo{
			Protocol:   protocol,
			Version:    version,
			Confidence: confidence,
			ExtraInfo:  extraInfo,
		}
	}

	// QUIC 协议检测
	// QUIC 初始包有特定的标记位
	if len(data) >= 1 && (data[0]&0xC0) == 0xC0 {
		// Long header bit pattern
		return &ProtocolInfo{
			Protocol:   ProtocolQUIC,
			Confidence: 0.8,
			ExtraInfo:  map[string]string{"header_type": "long"},
		}
	}

	// SOCKS5 握手
	if len(data) >= 2 && data[0] == 0x05 {
		nmethods := int(data[1])
		if len(data) >= 2+nmethods {
			return &ProtocolInfo{
				Protocol:   ProtocolSOCKS5,
				Version:    "5",
				Confidence: 1.0,
				ExtraInfo:  map[string]string{"nmethods": fmt.Sprintf("%d", nmethods)},
			}
		}
		return &ProtocolInfo{
			Protocol:   ProtocolSOCKS5,
			Version:    "5",
			Confidence: 0.9,
		}
	}

	// SOCKS4 握手
	if len(data) >= 2 && data[0] == 0x04 {
		return &ProtocolInfo{
			Protocol:   ProtocolSOCKS4,
			Version:    "4",
			Confidence: 0.8,
		}
	}

	// SSH 协议
	if bytes.HasPrefix(data, []byte("SSH-")) {
		if bytes.HasPrefix(data, []byte("SSH-2.0-")) {
			return &ProtocolInfo{
				Protocol:   ProtocolSSH2,
				Version:    "2.0",
				Confidence: 1.0,
			}
		}
		return &ProtocolInfo{
			Protocol:   ProtocolSSH,
			Confidence: 1.0,
		}
	}

	// WebSocket Upgrade
	if bytes.Contains(data, []byte("Upgrade: websocket")) ||
		bytes.Contains(data, []byte("Sec-WebSocket-Key:")) {
		return &ProtocolInfo{
			Protocol:   ProtocolWebSocket,
			Confidence: 1.0,
		}
	}

	// 默认当作普通TCP
	return &ProtocolInfo{
		Protocol:   ProtocolTCP,
		Confidence: 0.5,
	}
}

// detect 根据数据特征检测协议（向后兼容）
func detect(data []byte) Protocol {
	detector := NewProtocolDetector()
	info := detector.detectEnhanced(data)
	return info.Protocol
}

// PeekConn 可以预读数据的连接包装器
type PeekConn struct {
	net.Conn
	peeked []byte
	offset int
}

// NewPeekConn 创建可预读的连接
func NewPeekConn(conn net.Conn, peeked []byte) *PeekConn {
	return &PeekConn{
		Conn:   conn,
		peeked: peeked,
		offset: 0,
	}
}

// Read 实现io.Reader，先读预读的数据
func (pc *PeekConn) Read(p []byte) (int, error) {
	if pc.offset < len(pc.peeked) {
		n := copy(p, pc.peeked[pc.offset:])
		pc.offset += n
		return n, nil
	}
	return pc.Conn.Read(p)
}
