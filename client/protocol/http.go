package protocol

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"net"
	"net/http"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// HTTPProxyHandler HTTP代理处理器
type HTTPProxyHandler struct {
	enableCache       bool
	enableCompression bool
}

// NewHTTPProxyHandler 创建HTTP代理处理器
func NewHTTPProxyHandler(enableCache, enableCompression bool) *HTTPProxyHandler {
	return &HTTPProxyHandler{
		enableCache:       enableCache,
		enableCompression: enableCompression,
	}
}

// Handle 处理HTTP连接
func (h *HTTPProxyHandler) Handle(clientConn net.Conn, targetConn net.Conn) error {
	defer clientConn.Close()
	defer targetConn.Close()

	// 创建缓冲读取器
	clientReader := bufio.NewReader(clientConn)
	targetReader := bufio.NewReader(targetConn)

	// 读取HTTP请求
	req, err := http.ReadRequest(clientReader)
	if err != nil {
		return err
	}

	logger.Debug("HTTP请求",
		zap.String("method", req.Method),
		zap.String("host", req.Host),
		zap.String("uri", req.RequestURI))

	// 处理压缩
	if h.enableCompression && req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "gzip, deflate")
	}

	// 转发请求到目标
	if err := req.Write(targetConn); err != nil {
		return err
	}

	// 读取响应
	resp, err := http.ReadResponse(targetReader, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	logger.Debug("HTTP响应",
		zap.Int("status", resp.StatusCode),
		zap.String("content_type", resp.Header.Get("Content-Type")),
		zap.Int64("content_length", resp.ContentLength))

	// 处理gzip压缩
	if h.enableCompression && resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer gzReader.Close()
			resp.Body = gzReader
			resp.Header.Del("Content-Encoding")
			resp.ContentLength = -1
		}
	}

	// 转发响应到客户端
	return resp.Write(clientConn)
}

// TLSProxyHandler TLS代理处理器
type TLSProxyHandler struct {
	sniRouting map[string]string // SNI → target
}

// NewTLSProxyHandler 创建TLS代理处理器
func NewTLSProxyHandler() *TLSProxyHandler {
	return &TLSProxyHandler{
		sniRouting: make(map[string]string),
	}
}

// ExtractSNI 从ClientHello提取SNI
func ExtractSNI(data []byte) string {
	if len(data) < 43 {
		return ""
	}

	// TLS Record: Type(1) Version(2) Length(2) Handshake...
	if data[0] != 0x16 { // Handshake
		return ""
	}

	// Handshake: Type(1) Length(3) Version(2) Random(32) SessionIDLen(1)
	sessionIDLen := int(data[43])
	if len(data) < 44+sessionIDLen {
		return ""
	}

	pos := 44 + sessionIDLen

	// CipherSuites
	if len(data) < pos+2 {
		return ""
	}
	cipherSuitesLen := int(data[pos])<<8 | int(data[pos+1])
	pos += 2 + cipherSuitesLen

	// Compression Methods
	if len(data) < pos+1 {
		return ""
	}
	compressionLen := int(data[pos])
	pos += 1 + compressionLen

	// Extensions
	if len(data) < pos+2 {
		return ""
	}
	pos += 2

	// 遍历Extensions查找SNI
	for pos < len(data)-4 {
		extType := int(data[pos])<<8 | int(data[pos+1])
		extLen := int(data[pos+2])<<8 | int(data[pos+3])
		pos += 4

		if extType == 0 { // SNI extension
			if len(data) < pos+extLen {
				return ""
			}

			// SNI extension: ListLen(2) Type(1) NameLen(2) Name
			if extLen < 5 {
				return ""
			}

			nameLen := int(data[pos+3])<<8 | int(data[pos+4])
			if len(data) < pos+5+nameLen {
				return ""
			}

			sni := string(data[pos+5 : pos+5+nameLen])
			return sni
		}

		pos += extLen
	}

	return ""
}

// Handle 处理TLS连接
func (h *TLSProxyHandler) Handle(clientConn net.Conn, targetConn net.Conn) error {
	// 读取ClientHello提取SNI
	buf := make([]byte, 4096)
	n, err := clientConn.Read(buf)
	if err != nil {
		return err
	}

	sni := ExtractSNI(buf[:n])
	if sni != "" {
		logger.Debug("TLS SNI", zap.String("sni", sni))

		// SNI路由（如果配置了）
		if target, exists := h.sniRouting[sni]; exists {
			logger.Info("SNI路由",
				zap.String("sni", sni),
				zap.String("target", target))
			// TODO: 重新路由到指定目标
		}
	}

	// 写入已读取的数据到目标
	if _, err := targetConn.Write(buf[:n]); err != nil {
		return err
	}

	// 双向透传
	return bidirectionalCopy(clientConn, targetConn)
}

// AddSNIRoute 添加SNI路由规则
func (h *TLSProxyHandler) AddSNIRoute(sni, target string) {
	h.sniRouting[sni] = target
}

// SOCKS5ProxyHandler SOCKS5代理处理器
type SOCKS5ProxyHandler struct{}

// NewSOCKS5ProxyHandler 创建SOCKS5代理处理器
func NewSOCKS5ProxyHandler() *SOCKS5ProxyHandler {
	return &SOCKS5ProxyHandler{}
}

// Handle 处理SOCKS5连接
func (h *SOCKS5ProxyHandler) Handle(clientConn net.Conn, targetConn net.Conn) error {
	// SOCKS5握手
	// 客户端: VER(1) NMETHODS(1) METHODS(1-255)
	buf := make([]byte, 257)
	n, err := clientConn.Read(buf)
	if err != nil {
		return err
	}

	if n < 2 || buf[0] != 0x05 {
		return fmt.Errorf("invalid SOCKS5 version")
	}

	// 响应：VER(1) METHOD(1)
	// METHOD: 0x00 = 无认证
	response := []byte{0x05, 0x00}
	if _, err := clientConn.Write(response); err != nil {
		return err
	}

	// 读取连接请求
	// VER(1) CMD(1) RSV(1) ATYP(1) DST.ADDR(var) DST.PORT(2)
	n, err = clientConn.Read(buf)
	if err != nil {
		return err
	}

	if n < 4 || buf[0] != 0x05 {
		return fmt.Errorf("invalid SOCKS5 request")
	}

	cmd := buf[1]
	if cmd != 0x01 { // CONNECT
		return fmt.Errorf("unsupported SOCKS5 command: %d", cmd)
	}

	// 解析目标地址
	atyp := buf[3]
	var targetAddr string

	switch atyp {
	case 0x01: // IPv4
		if n < 10 {
			return fmt.Errorf("invalid IPv4 address")
		}
		ip := net.IPv4(buf[4], buf[5], buf[6], buf[7])
		port := int(buf[8])<<8 | int(buf[9])
		targetAddr = fmt.Sprintf("%s:%d", ip.String(), port)

	case 0x03: // 域名
		if n < 5 {
			return fmt.Errorf("invalid domain address")
		}
		domainLen := int(buf[4])
		if n < 5+domainLen+2 {
			return fmt.Errorf("incomplete domain address")
		}
		domain := string(buf[5 : 5+domainLen])
		port := int(buf[5+domainLen])<<8 | int(buf[5+domainLen+1])
		targetAddr = fmt.Sprintf("%s:%d", domain, port)

	default:
		return fmt.Errorf("unsupported address type: %d", atyp)
	}

	logger.Debug("SOCKS5请求", zap.String("target", targetAddr))

	// 响应成功
	// VER(1) REP(1) RSV(1) ATYP(1) BND.ADDR(var) BND.PORT(2)
	reply := []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := clientConn.Write(reply); err != nil {
		return err
	}

	// 开始代理数据
	return bidirectionalCopy(clientConn, targetConn)
}






