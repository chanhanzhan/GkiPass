package protocol

import (
	"io"
	"net"
)

// Handler 协议处理器接口
type Handler interface {
	Handle(clientConn net.Conn, targetConn net.Conn) error
}

// TCPHandler TCP透明转发
type TCPHandler struct{}

func (h *TCPHandler) Handle(clientConn net.Conn, targetConn net.Conn) error {
	return bidirectionalCopy(clientConn, targetConn)
}

// HTTPHandler HTTP协议处理
type HTTPHandler struct{}

func (h *HTTPHandler) Handle(clientConn net.Conn, targetConn net.Conn) error {
	// 目前直接透传，后续可以添加HTTP特性（缓存、压缩等）
	return bidirectionalCopy(clientConn, targetConn)
}

// TLSHandler TLS协议处理
type TLSHandler struct{}

func (h *TLSHandler) Handle(clientConn net.Conn, targetConn net.Conn) error {
	// 目前直接透传，后续可以添加TLS特性（SNI路由等）
	return bidirectionalCopy(clientConn, targetConn)
}

// SOCKS5Handler SOCKS5协议处理
type SOCKS5Handler struct{}

func (h *SOCKS5Handler) Handle(clientConn net.Conn, targetConn net.Conn) error {
	// 目前直接透传，后续可以实现SOCKS5协议
	return bidirectionalCopy(clientConn, targetConn)
}

// bidirectionalCopy 双向拷贝数据
func bidirectionalCopy(conn1, conn2 net.Conn) error {
	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(conn1, conn2)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(conn2, conn1)
		errChan <- err
	}()

	// 等待任意方向结束
	err1 := <-errChan
	err2 := <-errChan

	if err1 != nil {
		return err1
	}
	return err2
}

// GetHandler 根据协议类型获取处理器
func GetHandler(protocol Protocol) Handler {
	switch protocol {
	case ProtocolHTTP, ProtocolHTTPS:
		return &HTTPHandler{}
	case ProtocolTLS:
		return &TLSHandler{}
	case ProtocolSOCKS5:
		return &SOCKS5Handler{}
	default:
		return &TCPHandler{}
	}
}






