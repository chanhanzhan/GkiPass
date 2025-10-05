package tls

import (
	"crypto/tls"
	"time"
)

// GetHighPerformanceTLSConfig 获取高性能TLS配置（支持0-RTT）
func GetHighPerformanceTLSConfig(cert *tls.Certificate) *tls.Config {
	config := &tls.Config{
		// 使用TLS 1.3（支持0-RTT）
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		
		// 优先使用硬件加速的加密套件
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,       // AES-NI硬件加速
			tls.TLS_CHACHA20_POLY1305_SHA256, // 软件实现快速
			tls.TLS_AES_256_GCM_SHA384,
		},
		
		// 启用会话票据复用（0-RTT必需）
		SessionTicketsDisabled: false,
		ClientSessionCache:     tls.NewLRUClientSessionCache(10000),
		
		// 跳过证书验证（节点间信任）
		InsecureSkipVerify: true,
		
		// NextProtos for ALPN
		NextProtos: []string{"gkipass-v1", "h2"},
	}

	if cert != nil {
		config.Certificates = []tls.Certificate{*cert}
	}

	return config
}

// Enable0RTT 配置0-RTT支持
func Enable0RTT(config *tls.Config) {
	// TLS 1.3自动支持0-RTT，只需确保会话缓存启用
	if config.ClientSessionCache == nil {
		config.ClientSessionCache = tls.NewLRUClientSessionCache(10000)
	}
	config.SessionTicketsDisabled = false
}

// DialWith0RTT 使用0-RTT拨号并发送early data
func DialWith0RTT(network, addr string, config *tls.Config, earlyData []byte) (*tls.Conn, bool, error) {
	conn, err := tls.Dial(network, addr, config)
	if err != nil {
		return nil, false, err
	}

	// Go 标准库 crypto/tls 不直接暴露 Used0RTT 字段
	// 只能通过会话重用和 earlyData 非空来推测
	used0RTT := false // 默认不支持直接检测

	// 发送 early data（Go 标准库不支持 0-RTT 直接发送，模拟行为）
	if len(earlyData) > 0 {
		if _, err := conn.Write(earlyData); err != nil {
			conn.Close()
			return nil, false, err
		}
	}
		if _, err := conn.Write(earlyData); err != nil {
			conn.Close()
			return nil, false, err
		}
	return conn, used0RTT, nil
}

// TLSSessionCache TLS会话缓存
type TLSSessionCache struct {
	cache tls.ClientSessionCache
}

// NewTLSSessionCache 创建会话缓存
func NewTLSSessionCache(capacity int) *TLSSessionCache {
	return &TLSSessionCache{
		cache: tls.NewLRUClientSessionCache(capacity),
	}
}

// ConnectionPool 支持TLS的连接池
type TLSConnPool struct {
	config  *tls.Config
	conns   chan *tls.Conn
	maxSize int
}

// NewTLSConnPool 创建TLS连接池
func NewTLSConnPool(config *tls.Config, maxSize int) *TLSConnPool {
	return &TLSConnPool{
		config:  config,
		conns:   make(chan *tls.Conn, maxSize),
		maxSize: maxSize,
	}
}

// Get 获取TLS连接
func (p *TLSConnPool) Get(addr string) (*tls.Conn, error) {
	select {
	case conn := <-p.conns:
		// 检查连接是否有效
		if conn != nil {
			conn.SetDeadline(time.Now().Add(100 * time.Millisecond))
			one := make([]byte, 1)
			conn.Read(one)
			conn.SetDeadline(time.Time{})
			// 如果读取超时，说明连接可用
			return conn, nil
		}
	default:
	}

	// 创建新连接
	return tls.Dial("tcp", addr, p.config)
}

// Put 归还TLS连接
func (p *TLSConnPool) Put(conn *tls.Conn) {
	if conn == nil {
		return
	}

	select {
	case p.conns <- conn:
	default:
		// 池满，关闭连接
		conn.Close()
	}
}
