package handlers

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"gkipass/client/internal/detector"

	"go.uber.org/zap"
)

// HTTPHandler HTTP协议处理器
type HTTPHandler struct {
	*BaseHandler
	upstreamAddr string
}

// NewHTTPHandler 创建HTTP处理器
func NewHTTPHandler(upstreamAddr string) *HTTPHandler {
	return &HTTPHandler{
		BaseHandler:  NewBaseHandler("http", detector.ProtocolHTTP),
		upstreamAddr: upstreamAddr,
	}
}

// Handle 处理HTTP连接
func (h *HTTPHandler) Handle(ctx context.Context, conn net.Conn, result *detector.DetectionResult) error {
	defer conn.Close()

	h.logger.Info("处理HTTP连接",
		zap.String("remote_addr", conn.RemoteAddr().String()),
		zap.String("protocol", string(result.Protocol)),
		zap.Float64("confidence", result.Confidence))

	// 读取HTTP请求
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		h.recordConnection(false, 0)
		return fmt.Errorf("读取HTTP请求失败: %w", err)
	}

	// 处理不同类型的HTTP请求
	if req.Method == "CONNECT" {
		return h.handleConnect(conn, req)
	}

	return h.handleHTTPProxy(conn, req)
}

// handleConnect 处理CONNECT方法（用于HTTPS代理）
func (h *HTTPHandler) handleConnect(conn net.Conn, req *http.Request) error {
	// 连接到目标服务器
	targetConn, err := net.DialTimeout("tcp", req.Host, 10*time.Second)
	if err != nil {
		// 发送错误响应
		response := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
		conn.Write([]byte(response))
		h.recordConnection(false, 0)
		return fmt.Errorf("连接目标服务器失败: %w", err)
	}
	defer targetConn.Close()

	// 发送成功响应
	response := "HTTP/1.1 200 Connection Established\r\n\r\n"
	if _, err := conn.Write([]byte(response)); err != nil {
		h.recordConnection(false, 0)
		return fmt.Errorf("发送CONNECT响应失败: %w", err)
	}

	// 开始双向中继
	relay := NewRelayConnection(conn, targetConn)
	if err := relay.Start(); err != nil {
		h.recordConnection(false, 0)
		return err
	}

	relay.Wait()
	stats := relay.GetStats()
	totalBytes := stats["total_bytes"].(int64)
	h.recordConnection(true, totalBytes)

	return nil
}

// handleHTTPProxy 处理普通HTTP代理
func (h *HTTPHandler) handleHTTPProxy(conn net.Conn, req *http.Request) error {
	// 解析目标URL
	var targetHost string
	if req.URL.IsAbs() {
		targetHost = req.URL.Host
	} else {
		targetHost = req.Host
	}

	if targetHost == "" {
		response := "HTTP/1.1 400 Bad Request\r\n\r\n"
		conn.Write([]byte(response))
		h.recordConnection(false, 0)
		return fmt.Errorf("无效的请求，缺少Host")
	}

	// 添加默认端口
	if !strings.Contains(targetHost, ":") {
		targetHost += ":80"
	}

	// 连接到目标服务器
	targetConn, err := net.DialTimeout("tcp", targetHost, 10*time.Second)
	if err != nil {
		response := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
		conn.Write([]byte(response))
		h.recordConnection(false, 0)
		return fmt.Errorf("连接目标服务器失败: %w", err)
	}
	defer targetConn.Close()

	// 修改请求URL为相对路径
	if req.URL.IsAbs() {
		req.URL.Scheme = ""
		req.URL.Host = ""
	}

	// 转发请求到目标服务器
	if err := req.Write(targetConn); err != nil {
		h.recordConnection(false, 0)
		return fmt.Errorf("转发请求失败: %w", err)
	}

	// 开始双向中继
	relay := NewRelayConnection(conn, targetConn)
	if err := relay.Start(); err != nil {
		h.recordConnection(false, 0)
		return err
	}

	relay.Wait()
	stats := relay.GetStats()
	totalBytes := stats["total_bytes"].(int64)
	h.recordConnection(true, totalBytes)

	return nil
}

// SOCKSHandler SOCKS协议处理器
type SOCKSHandler struct {
	*BaseHandler
}

// NewSOCKSHandler 创建SOCKS处理器
func NewSOCKSHandler() *SOCKSHandler {
	return &SOCKSHandler{
		BaseHandler: NewBaseHandler("socks", detector.ProtocolSOCKS5),
	}
}

// Handle 处理SOCKS连接
func (s *SOCKSHandler) Handle(ctx context.Context, conn net.Conn, result *detector.DetectionResult) error {
	defer conn.Close()

	s.logger.Info("处理SOCKS连接",
		zap.String("remote_addr", conn.RemoteAddr().String()),
		zap.String("protocol", string(result.Protocol)),
		zap.Float64("confidence", result.Confidence))

	// 根据检测结果选择处理方式
	switch result.Protocol {
	case detector.ProtocolSOCKS4:
		return s.handleSOCKS4(conn)
	case detector.ProtocolSOCKS5:
		return s.handleSOCKS5(conn)
	default:
		s.recordConnection(false, 0)
		return fmt.Errorf("不支持的SOCKS版本: %s", result.Protocol)
	}
}

// handleSOCKS4 处理SOCKS4协议
func (s *SOCKSHandler) handleSOCKS4(conn net.Conn) error {
	// 读取SOCKS4请求
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil || n < 8 {
		s.recordConnection(false, 0)
		return fmt.Errorf("读取SOCKS4请求失败")
	}

	// 解析请求
	version := buffer[0]
	command := buffer[1]

	if version != 4 {
		s.recordConnection(false, 0)
		return fmt.Errorf("无效的SOCKS版本: %d", version)
	}

	if command != 1 { // 只支持CONNECT
		// 发送失败响应
		response := []byte{0, 91, 0, 0, 0, 0, 0, 0} // 91 = 请求被拒绝
		conn.Write(response)
		s.recordConnection(false, 0)
		return fmt.Errorf("不支持的SOCKS4命令: %d", command)
	}

	// 提取目标地址和端口
	port := (uint16(buffer[2]) << 8) | uint16(buffer[3])
	ip := fmt.Sprintf("%d.%d.%d.%d", buffer[4], buffer[5], buffer[6], buffer[7])
	targetAddr := fmt.Sprintf("%s:%d", ip, port)

	return s.connectAndRelay(conn, targetAddr, []byte{0, 90, 0, 0, 0, 0, 0, 0}) // 90 = 请求成功
}

// handleSOCKS5 处理SOCKS5协议
func (s *SOCKSHandler) handleSOCKS5(conn net.Conn) error {
	// 阶段1：认证方法协商
	buffer := make([]byte, 256)
	n, err := conn.Read(buffer)
	if err != nil || n < 3 {
		s.recordConnection(false, 0)
		return fmt.Errorf("读取SOCKS5认证请求失败")
	}

	version := buffer[0]
	if version != 5 {
		s.recordConnection(false, 0)
		return fmt.Errorf("无效的SOCKS版本: %d", version)
	}

	// 回复：使用无认证方法
	response := []byte{5, 0} // 版本5，方法0（无认证）
	if _, err := conn.Write(response); err != nil {
		s.recordConnection(false, 0)
		return fmt.Errorf("发送认证响应失败: %w", err)
	}

	// 阶段2：连接请求
	n, err = conn.Read(buffer)
	if err != nil || n < 4 {
		s.recordConnection(false, 0)
		return fmt.Errorf("读取SOCKS5连接请求失败")
	}

	version = buffer[0]
	command := buffer[1]
	addrType := buffer[3]

	if version != 5 {
		s.recordConnection(false, 0)
		return fmt.Errorf("无效的SOCKS5版本: %d", version)
	}

	if command != 1 { // 只支持CONNECT
		response := []byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0} // 7 = 不支持的命令
		conn.Write(response)
		s.recordConnection(false, 0)
		return fmt.Errorf("不支持的SOCKS5命令: %d", command)
	}

	// 解析目标地址
	var targetAddr string
	pos := 4

	switch addrType {
	case 1: // IPv4
		if n < pos+6 {
			response := []byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0} // 1 = 一般性SOCKS服务器故障
			conn.Write(response)
			s.recordConnection(false, 0)
			return fmt.Errorf("IPv4地址数据不完整")
		}
		ip := fmt.Sprintf("%d.%d.%d.%d", buffer[pos], buffer[pos+1], buffer[pos+2], buffer[pos+3])
		port := (uint16(buffer[pos+4]) << 8) | uint16(buffer[pos+5])
		targetAddr = fmt.Sprintf("%s:%d", ip, port)

	case 3: // 域名
		if n < pos+1 {
			response := []byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0}
			conn.Write(response)
			s.recordConnection(false, 0)
			return fmt.Errorf("域名长度数据不完整")
		}
		domainLen := buffer[pos]
		pos++
		if n < pos+int(domainLen)+2 {
			response := []byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0}
			conn.Write(response)
			s.recordConnection(false, 0)
			return fmt.Errorf("域名数据不完整")
		}
		domain := string(buffer[pos : pos+int(domainLen)])
		pos += int(domainLen)
		port := (uint16(buffer[pos]) << 8) | uint16(buffer[pos+1])
		targetAddr = fmt.Sprintf("%s:%d", domain, port)

	default:
		response := []byte{5, 8, 0, 1, 0, 0, 0, 0, 0, 0} // 8 = 不支持的地址类型
		conn.Write(response)
		s.recordConnection(false, 0)
		return fmt.Errorf("不支持的地址类型: %d", addrType)
	}

	successResponse := []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0} // 连接成功
	return s.connectAndRelay(conn, targetAddr, successResponse)
}

// connectAndRelay 连接目标并中继数据
func (s *SOCKSHandler) connectAndRelay(conn net.Conn, targetAddr string, successResponse []byte) error {
	// 连接到目标服务器
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		s.recordConnection(false, 0)
		return fmt.Errorf("连接目标服务器失败: %w", err)
	}
	defer targetConn.Close()

	// 发送成功响应
	if _, err := conn.Write(successResponse); err != nil {
		s.recordConnection(false, 0)
		return fmt.Errorf("发送成功响应失败: %w", err)
	}

	// 开始双向中继
	relay := NewRelayConnection(conn, targetConn)
	if err := relay.Start(); err != nil {
		s.recordConnection(false, 0)
		return err
	}

	relay.Wait()
	stats := relay.GetStats()
	totalBytes := stats["total_bytes"].(int64)
	s.recordConnection(true, totalBytes)

	s.logger.Info("SOCKS连接处理完成",
		zap.String("target_addr", targetAddr),
		zap.Int64("bytes_transferred", totalBytes))

	return nil
}

// TCPHandler TCP协议处理器
type TCPHandler struct {
	*BaseHandler
	upstreamAddr string
}

// NewTCPHandler 创建TCP处理器
func NewTCPHandler(upstreamAddr string) *TCPHandler {
	return &TCPHandler{
		BaseHandler:  NewBaseHandler("tcp", detector.ProtocolTCP),
		upstreamAddr: upstreamAddr,
	}
}

// Handle 处理TCP连接
func (t *TCPHandler) Handle(ctx context.Context, conn net.Conn, result *detector.DetectionResult) error {
	defer conn.Close()

	t.logger.Info("处理TCP连接",
		zap.String("remote_addr", conn.RemoteAddr().String()),
		zap.String("upstream", t.upstreamAddr))

	// 连接到上游服务器
	upstreamConn, err := net.DialTimeout("tcp", t.upstreamAddr, 10*time.Second)
	if err != nil {
		t.recordConnection(false, 0)
		return fmt.Errorf("连接上游服务器失败: %w", err)
	}
	defer upstreamConn.Close()

	// 开始双向中继
	relay := NewRelayConnection(conn, upstreamConn)
	if err := relay.Start(); err != nil {
		t.recordConnection(false, 0)
		return err
	}

	relay.Wait()
	stats := relay.GetStats()
	totalBytes := stats["total_bytes"].(int64)
	t.recordConnection(true, totalBytes)

	t.logger.Info("TCP连接处理完成",
		zap.Int64("bytes_transferred", totalBytes))

	return nil
}

// UnknownHandler 未知协议处理器
type UnknownHandler struct {
	*BaseHandler
	defaultUpstream string
}

// NewUnknownHandler 创建未知协议处理器
func NewUnknownHandler(defaultUpstream string) *UnknownHandler {
	return &UnknownHandler{
		BaseHandler:     NewBaseHandler("unknown", detector.ProtocolUnknown),
		defaultUpstream: defaultUpstream,
	}
}

// Handle 处理未知协议连接
func (u *UnknownHandler) Handle(ctx context.Context, conn net.Conn, result *detector.DetectionResult) error {
	defer conn.Close()

	u.logger.Info("处理未知协议连接",
		zap.String("remote_addr", conn.RemoteAddr().String()),
		zap.String("detected_protocol", string(result.Protocol)),
		zap.Float64("confidence", result.Confidence),
		zap.String("default_upstream", u.defaultUpstream))

	if u.defaultUpstream == "" {
		u.recordConnection(false, 0)
		return fmt.Errorf("未配置默认上游地址")
	}

	// 连接到默认上游服务器
	upstreamConn, err := net.DialTimeout("tcp", u.defaultUpstream, 10*time.Second)
	if err != nil {
		u.recordConnection(false, 0)
		return fmt.Errorf("连接默认上游服务器失败: %w", err)
	}
	defer upstreamConn.Close()

	// 开始双向中继
	relay := NewRelayConnection(conn, upstreamConn)
	if err := relay.Start(); err != nil {
		u.recordConnection(false, 0)
		return err
	}

	relay.Wait()
	stats := relay.GetStats()
	totalBytes := stats["total_bytes"].(int64)
	u.recordConnection(true, totalBytes)

	u.logger.Info("未知协议连接处理完成",
		zap.Int64("bytes_transferred", totalBytes))

	return nil
}
