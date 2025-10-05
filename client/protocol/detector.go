package protocol

import (
	"bytes"
	"io"
	"net"
	"time"
)

// Protocol 协议类型
type Protocol string

const (
	ProtocolTCP     Protocol = "tcp"
	ProtocolHTTP    Protocol = "http"
	ProtocolHTTPS   Protocol = "https"
	ProtocolTLS     Protocol = "tls"
	ProtocolSOCKS5  Protocol = "socks5"
	ProtocolSSH     Protocol = "ssh"
	ProtocolUnknown Protocol = "unknown"
)

// DetectProtocol 检测协议类型
func DetectProtocol(conn net.Conn) (Protocol, []byte, error) {
	// 设置读超时
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	// 读取前16个字节用于检测
	buf := make([]byte, 16)
	n, err := io.ReadAtLeast(conn, buf, 1)
	if err != nil {
		return ProtocolUnknown, nil, err
	}

	data := buf[:n]
	protocol := detect(data)

	return protocol, data, nil
}

// detect 根据数据特征检测协议
func detect(data []byte) Protocol {
	if len(data) == 0 {
		return ProtocolUnknown
	}

	// HTTP 方法检测
	httpMethods := [][]byte{
		[]byte("GET "),
		[]byte("POST"),
		[]byte("PUT "),
		[]byte("DELE"),
		[]byte("HEAD"),
		[]byte("OPTI"),
		[]byte("PATC"),
		[]byte("CONN"),
	}

	for _, method := range httpMethods {
		if bytes.HasPrefix(data, method) {
			return ProtocolHTTP
		}
	}

	// TLS/SSL ClientHello
	// 格式: 0x16 (Handshake) 0x03 (Version) 0xXX (Minor version)
	if len(data) >= 3 && data[0] == 0x16 && data[1] == 0x03 {
		// 可能是 TLS 1.0 (0x0301), 1.1 (0x0302), 1.2 (0x0303), 1.3 (0x0304)
		if data[2] >= 0x01 && data[2] <= 0x04 {
			return ProtocolTLS
		}
	}

	// SOCKS5 握手
	// 格式: 0x05 (version) 0xXX (nmethods) ...
	if len(data) >= 2 && data[0] == 0x05 {
		return ProtocolSOCKS5
	}

	// SSH 协议
	// SSH-2.0-xxx 或 SSH-1.x-xxx
	if bytes.HasPrefix(data, []byte("SSH-")) {
		return ProtocolSSH
	}

	// 默认当作普通TCP
	return ProtocolTCP
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






