package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// TransportType 传输类型
type TransportType string

const (
	TransportTCP  TransportType = "tcp"  // 普通TCP
	TransportTLS  TransportType = "tls"  // TLS加密
	TransportMTLS TransportType = "mtls" // 双向TLS
	TransportWS   TransportType = "ws"   // WebSocket
	TransportWSS  TransportType = "wss"  // WebSocket over TLS
	TransportNone TransportType = "none" // 无传输（用于测试）
)

// Transport 传输层接口
type Transport interface {
	// Type 获取传输类型
	Type() TransportType

	// Dial 拨号连接
	Dial(ctx context.Context, address string) (net.Conn, error)

	// Listen 开始监听
	Listen(ctx context.Context, address string) (net.Listener, error)

	// Close 关闭传输
	Close() error

	// GetStats 获取统计信息
	GetStats() map[string]interface{}
}

// Manager 传输管理器
type Manager struct {
	tlsConfig *tls.Config
	logger    *zap.Logger

	// 传输实例
	transports   map[TransportType]Transport
	transportsMu sync.RWMutex

	// 统计信息
	stats struct {
		connections   atomic.Int64
		bytesIn       atomic.Int64
		bytesOut      atomic.Int64
		dialAttempts  atomic.Int64
		dialSuccesses atomic.Int64
		dialFailures  atomic.Int64
	}

	// 状态
	ctx    context.Context
	cancel context.CancelFunc
}

// New 创建传输管理器
func New(tlsConfig *tls.Config) (*Manager, error) {
	manager := &Manager{
		tlsConfig:  tlsConfig,
		logger:     zap.L().Named("transport"),
		transports: make(map[TransportType]Transport),
	}

	// 初始化各种传输
	if err := manager.initTransports(); err != nil {
		return nil, fmt.Errorf("初始化传输层失败: %w", err)
	}

	return manager, nil
}

// initTransports 初始化传输层
func (m *Manager) initTransports() error {
	m.logger.Debug("🚛 初始化传输层...")

	// TCP传输
	tcpTransport := &TCPTransport{
		logger: m.logger.Named("tcp"),
	}
	m.transports[TransportTCP] = tcpTransport

	// TLS传输
	tlsTransport := &TLSTransport{
		tlsConfig: m.tlsConfig,
		logger:    m.logger.Named("tls"),
	}
	m.transports[TransportTLS] = tlsTransport

	// mTLS传输
	mtlsTransport := &MTLSTransport{
		tlsConfig: m.tlsConfig,
		logger:    m.logger.Named("mtls"),
	}
	m.transports[TransportMTLS] = mtlsTransport

	// WebSocket传输
	wsTransport := &WebSocketTransport{
		logger: m.logger.Named("ws"),
	}
	m.transports[TransportWS] = wsTransport

	// WebSocket over TLS传输
	wssTransport := &WebSocketTLSTransport{
		tlsConfig: m.tlsConfig,
		logger:    m.logger.Named("wss"),
	}
	m.transports[TransportWSS] = wssTransport

	m.logger.Info("✅ 传输层初始化完成", zap.Int("types", len(m.transports)))
	return nil
}

// Start 启动传输管理器
func (m *Manager) Start() error {
	m.ctx, m.cancel = context.WithCancel(context.Background())

	m.logger.Info("🚛 传输管理器启动")
	return nil
}

// Stop 停止传输管理器
func (m *Manager) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}

	// 关闭所有传输
	m.transportsMu.Lock()
	for _, transport := range m.transports {
		if err := transport.Close(); err != nil {
			m.logger.Error("关闭传输失败", zap.Error(err))
		}
	}
	m.transportsMu.Unlock()

	m.logger.Info("🚛 传输管理器停止")
	return nil
}

// GetTransport 获取指定类型的传输
func (m *Manager) GetTransport(transportType TransportType) (Transport, error) {
	m.transportsMu.RLock()
	defer m.transportsMu.RUnlock()

	transport, exists := m.transports[transportType]
	if !exists {
		return nil, fmt.Errorf("不支持的传输类型: %s", transportType)
	}

	return transport, nil
}

// Dial 使用指定传输类型拨号
func (m *Manager) Dial(transportType TransportType, address string) (net.Conn, error) {
	return m.DialContext(context.Background(), transportType, address)
}

// DialContext 使用指定传输类型和上下文拨号
func (m *Manager) DialContext(ctx context.Context, transportType TransportType, address string) (net.Conn, error) {
	m.stats.dialAttempts.Add(1)

	transport, err := m.GetTransport(transportType)
	if err != nil {
		m.stats.dialFailures.Add(1)
		return nil, err
	}

	conn, err := transport.Dial(ctx, address)
	if err != nil {
		m.stats.dialFailures.Add(1)
		return nil, fmt.Errorf("拨号失败 (%s): %w", transportType, err)
	}

	m.stats.dialSuccesses.Add(1)
	m.stats.connections.Add(1)

	m.logger.Debug("拨号成功",
		zap.String("transport", string(transportType)),
		zap.String("address", address))

	return conn, nil
}

// Listen 使用指定传输类型监听
func (m *Manager) Listen(transportType TransportType, address string) (net.Listener, error) {
	return m.ListenContext(context.Background(), transportType, address)
}

// ListenContext 使用指定传输类型和上下文监听
func (m *Manager) ListenContext(ctx context.Context, transportType TransportType, address string) (net.Listener, error) {
	transport, err := m.GetTransport(transportType)
	if err != nil {
		return nil, err
	}

	listener, err := transport.Listen(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("监听失败 (%s): %w", transportType, err)
	}

	m.logger.Info("监听启动",
		zap.String("transport", string(transportType)),
		zap.String("address", address))

	return listener, nil
}

// GetSupportedTransports 获取支持的传输类型
func (m *Manager) GetSupportedTransports() []TransportType {
	m.transportsMu.RLock()
	defer m.transportsMu.RUnlock()

	types := make([]TransportType, 0, len(m.transports))
	for transportType := range m.transports {
		types = append(types, transportType)
	}
	return types
}

// GetStats 获取传输统计
func (m *Manager) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"connections":     m.stats.connections.Load(),
		"bytes_in":        m.stats.bytesIn.Load(),
		"bytes_out":       m.stats.bytesOut.Load(),
		"dial_attempts":   m.stats.dialAttempts.Load(),
		"dial_successes":  m.stats.dialSuccesses.Load(),
		"dial_failures":   m.stats.dialFailures.Load(),
		"supported_types": len(m.transports),
	}

	// 添加各传输的统计
	m.transportsMu.RLock()
	transportStats := make(map[string]interface{})
	for transportType, transport := range m.transports {
		transportStats[string(transportType)] = transport.GetStats()
	}
	m.transportsMu.RUnlock()

	stats["transports"] = transportStats
	return stats
}

// TCPTransport TCP传输实现
type TCPTransport struct {
	logger *zap.Logger
}

func (t *TCPTransport) Type() TransportType {
	return TransportTCP
}

func (t *TCPTransport) Dial(ctx context.Context, address string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}
	return dialer.DialContext(ctx, "tcp", address)
}

func (t *TCPTransport) Listen(ctx context.Context, address string) (net.Listener, error) {
	return net.Listen("tcp", address)
}

func (t *TCPTransport) Close() error {
	return nil
}

func (t *TCPTransport) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"type": "tcp",
	}
}

// TLSTransport TLS传输实现
type TLSTransport struct {
	tlsConfig *tls.Config
	logger    *zap.Logger
}

func (t *TLSTransport) Type() TransportType {
	return TransportTLS
}

func (t *TLSTransport) Dial(ctx context.Context, address string) (net.Conn, error) {
	dialer := &tls.Dialer{
		Config: t.tlsConfig,
		NetDialer: &net.Dialer{
			Timeout: 10 * time.Second,
		},
	}
	return dialer.DialContext(ctx, "tcp", address)
}

func (t *TLSTransport) Listen(ctx context.Context, address string) (net.Listener, error) {
	return tls.Listen("tcp", address, t.tlsConfig)
}

func (t *TLSTransport) Close() error {
	return nil
}

func (t *TLSTransport) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"type": "tls",
	}
}

// MTLSTransport mTLS传输实现
type MTLSTransport struct {
	tlsConfig *tls.Config
	logger    *zap.Logger
}

func (t *MTLSTransport) Type() TransportType {
	return TransportMTLS
}

func (t *MTLSTransport) Dial(ctx context.Context, address string) (net.Conn, error) {
	// 为mTLS配置客户端证书
	config := t.tlsConfig.Clone()
	config.ClientAuth = tls.RequireAndVerifyClientCert

	dialer := &tls.Dialer{
		Config: config,
		NetDialer: &net.Dialer{
			Timeout: 10 * time.Second,
		},
	}
	return dialer.DialContext(ctx, "tcp", address)
}

func (t *MTLSTransport) Listen(ctx context.Context, address string) (net.Listener, error) {
	// 为mTLS配置服务器证书验证
	config := t.tlsConfig.Clone()
	config.ClientAuth = tls.RequireAndVerifyClientCert

	return tls.Listen("tcp", address, config)
}

func (t *MTLSTransport) Close() error {
	return nil
}

func (t *MTLSTransport) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"type": "mtls",
	}
}

// WebSocketTransport WebSocket传输实现
type WebSocketTransport struct {
	logger *zap.Logger
}

func (t *WebSocketTransport) Type() TransportType {
	return TransportWS
}

func (t *WebSocketTransport) Dial(ctx context.Context, address string) (net.Conn, error) {
	// 解析地址为WebSocket URL
	wsURL, err := t.parseWebSocketURL(address, false)
	if err != nil {
		return nil, err
	}

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}

	return &WebSocketConn{conn: conn}, nil
}

func (t *WebSocketTransport) Listen(ctx context.Context, address string) (net.Listener, error) {
	// WebSocket需要HTTP服务器，这里简化实现
	return nil, fmt.Errorf("WebSocket监听需要额外实现")
}

func (t *WebSocketTransport) Close() error {
	return nil
}

func (t *WebSocketTransport) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"type": "ws",
	}
}

func (t *WebSocketTransport) parseWebSocketURL(address string, tls bool) (string, error) {
	// 如果已经是完整URL，直接返回
	if u, err := url.Parse(address); err == nil && (u.Scheme == "ws" || u.Scheme == "wss") {
		return address, nil
	}

	// 构建WebSocket URL
	scheme := "ws"
	if tls {
		scheme = "wss"
	}

	return fmt.Sprintf("%s://%s/", scheme, address), nil
}

// WebSocketTLSTransport WebSocket over TLS传输实现
type WebSocketTLSTransport struct {
	tlsConfig *tls.Config
	logger    *zap.Logger
}

func (t *WebSocketTLSTransport) Type() TransportType {
	return TransportWSS
}

func (t *WebSocketTLSTransport) Dial(ctx context.Context, address string) (net.Conn, error) {
	// 解析地址为WSS URL
	wsURL, err := t.parseWebSocketURL(address, true)
	if err != nil {
		return nil, err
	}

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	dialer.TLSClientConfig = t.tlsConfig

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}

	return &WebSocketConn{conn: conn}, nil
}

func (t *WebSocketTLSTransport) Listen(ctx context.Context, address string) (net.Listener, error) {
	// WSS需要HTTPS服务器，这里简化实现
	return nil, fmt.Errorf("WebSocket TLS监听需要额外实现")
}

func (t *WebSocketTLSTransport) Close() error {
	return nil
}

func (t *WebSocketTLSTransport) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"type": "wss",
	}
}

func (t *WebSocketTLSTransport) parseWebSocketURL(address string, tls bool) (string, error) {
	// 如果已经是完整URL，直接返回
	if u, err := url.Parse(address); err == nil && (u.Scheme == "ws" || u.Scheme == "wss") {
		return address, nil
	}

	// 构建WebSocket URL
	scheme := "ws"
	if tls {
		scheme = "wss"
	}

	return fmt.Sprintf("%s://%s/", scheme, address), nil
}

// WebSocketConn WebSocket连接包装器
type WebSocketConn struct {
	conn        *websocket.Conn
	reader      io.Reader
	writeBuffer []byte
	mutex       sync.Mutex
}

func (w *WebSocketConn) Read(b []byte) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.reader == nil {
		_, reader, err := w.conn.NextReader()
		if err != nil {
			return 0, err
		}
		w.reader = reader
	}

	n, err := w.reader.Read(b)
	if err == io.EOF {
		// 消息读取完成，重置reader
		w.reader = nil
	}
	return n, err
}

func (w *WebSocketConn) Write(b []byte) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// 将数据添加到缓冲区
	w.writeBuffer = append(w.writeBuffer, b...)

	// 如果缓冲区达到一定大小或收到完整消息，发送数据
	if len(w.writeBuffer) > 0 {
		if err := w.conn.WriteMessage(websocket.BinaryMessage, w.writeBuffer); err != nil {
			return 0, err
		}
		w.writeBuffer = w.writeBuffer[:0] // 清空缓冲区
	}

	return len(b), nil
}

func (w *WebSocketConn) Close() error {
	// 发送任何剩余的缓冲数据
	w.mutex.Lock()
	if len(w.writeBuffer) > 0 {
		w.conn.WriteMessage(websocket.BinaryMessage, w.writeBuffer)
		w.writeBuffer = nil
	}
	w.mutex.Unlock()

	return w.conn.Close()
}

func (w *WebSocketConn) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

func (w *WebSocketConn) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

func (w *WebSocketConn) SetDeadline(t time.Time) error {
	if err := w.conn.SetReadDeadline(t); err != nil {
		return err
	}
	return w.conn.SetWriteDeadline(t)
}

func (w *WebSocketConn) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

func (w *WebSocketConn) SetWriteDeadline(t time.Time) error {
	return w.conn.SetWriteDeadline(t)
}
