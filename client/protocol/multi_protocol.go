package protocol

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"gkipass/client/logger"
	"gkipass/client/ws"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"go.uber.org/zap"
)

// MultiProtocolHandler 多协议处理器
type MultiProtocolHandler struct {
	nodeType      string
	tlsConfig     *tls.Config
	quicConfig    *quic.Config
	listeners     map[string]net.Listener   // protocol:port -> listener
	udpListeners  map[string]*net.UDPConn   // port -> udp listener
	quicListeners map[string]*quic.Listener // port -> quic listener
	httpServers   map[string]*http.Server   // port -> http server
	http3Servers  map[string]*http3.Server  // port -> http3 server
	wsUpgrader    websocket.Upgrader
	mu            sync.RWMutex
	stopChan      chan struct{}
}

// NewMultiProtocolHandler 创建多协议处理器
func NewMultiProtocolHandler(nodeType string, tlsConfig *tls.Config) *MultiProtocolHandler {
	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		EnableDatagrams: true,
	}

	wsUpgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			// 允许所有来源（生产环境应该限制）
			return true
		},
	}

	return &MultiProtocolHandler{
		nodeType:      nodeType,
		tlsConfig:     tlsConfig,
		quicConfig:    quicConfig,
		listeners:     make(map[string]net.Listener),
		udpListeners:  make(map[string]*net.UDPConn),
		quicListeners: make(map[string]*quic.Listener),
		httpServers:   make(map[string]*http.Server),
		http3Servers:  make(map[string]*http3.Server),
		wsUpgrader:    wsUpgrader,
		stopChan:      make(chan struct{}),
	}
}

// StartTunnelListener 启动隧道监听器
func (h *MultiProtocolHandler) StartTunnelListener(rule *ws.TunnelRule) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := fmt.Sprintf("%s:%d", rule.Protocol, rule.LocalPort)

	// 如果已存在，先停止
	if existing, exists := h.listeners[key]; exists {
		existing.Close()
		delete(h.listeners, key)
	}

	switch rule.Protocol {
	case "tcp":
		return h.startTCPListener(rule)
	case "udp":
		return h.startUDPListener(rule)
	case "http":
		return h.startHTTPListener(rule, false)
	case "https":
		return h.startHTTPListener(rule, true)
	case "ws":
		return h.startWebSocketListener(rule, false)
	case "wss":
		return h.startWebSocketListener(rule, true)
	case "quic":
		return h.startQUICListener(rule)
	case "tls":
		return h.startTLSListener(rule)
	default:
		return fmt.Errorf("不支持的协议: %s", rule.Protocol)
	}
}

// startTCPListener 启动TCP监听器
func (h *MultiProtocolHandler) startTCPListener(rule *ws.TunnelRule) error {
	addr := fmt.Sprintf(":%d", rule.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("TCP监听失败: %w", err)
	}

	key := fmt.Sprintf("%s:%d", rule.Protocol, rule.LocalPort)
	h.listeners[key] = listener

	go h.handleTCPConnections(listener, rule)

	logger.Info("TCP监听器已启动",
		zap.String("tunnel_id", rule.TunnelID),
		zap.Int("port", rule.LocalPort))

	return nil
}

// startUDPListener 启动UDP监听器
func (h *MultiProtocolHandler) startUDPListener(rule *ws.TunnelRule) error {
	addr := fmt.Sprintf(":%d", rule.LocalPort)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("解析UDP地址失败: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("UDP监听失败: %w", err)
	}

	key := fmt.Sprintf("%s:%d", rule.Protocol, rule.LocalPort)
	h.udpListeners[key] = udpConn

	go h.handleUDPConnections(udpConn, rule)

	logger.Info("UDP监听器已启动",
		zap.String("tunnel_id", rule.TunnelID),
		zap.Int("port", rule.LocalPort))

	return nil
}

// startHTTPListener 启动HTTP监听器
func (h *MultiProtocolHandler) startHTTPListener(rule *ws.TunnelRule, useTLS bool) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.createHTTPHandler(rule))

	addr := fmt.Sprintf(":%d", rule.LocalPort)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if useTLS && h.tlsConfig != nil {
		server.TLSConfig = h.tlsConfig
	}

	key := fmt.Sprintf("%s:%d", rule.Protocol, rule.LocalPort)
	h.httpServers[key] = server

	go func() {
		var err error
		if useTLS && h.tlsConfig != nil {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP服务器错误", zap.Error(err))
		}
	}()

	logger.Info("HTTP监听器已启动",
		zap.String("tunnel_id", rule.TunnelID),
		zap.Int("port", rule.LocalPort),
		zap.Bool("tls", useTLS))

	return nil
}

// startWebSocketListener 启动WebSocket监听器
func (h *MultiProtocolHandler) startWebSocketListener(rule *ws.TunnelRule, useTLS bool) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.createWebSocketHandler(rule))

	addr := fmt.Sprintf(":%d", rule.LocalPort)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	if useTLS && h.tlsConfig != nil {
		server.TLSConfig = h.tlsConfig
	}

	key := fmt.Sprintf("%s:%d", rule.Protocol, rule.LocalPort)
	h.httpServers[key] = server

	go func() {
		var err error
		if useTLS && h.tlsConfig != nil {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Error("WebSocket服务器错误", zap.Error(err))
		}
	}()

	logger.Info("WebSocket监听器已启动",
		zap.String("tunnel_id", rule.TunnelID),
		zap.Int("port", rule.LocalPort),
		zap.Bool("wss", useTLS))

	return nil
}

// startQUICListener 启动QUIC监听器
func (h *MultiProtocolHandler) startQUICListener(rule *ws.TunnelRule) error {
	if h.tlsConfig == nil {
		return fmt.Errorf("QUIC需要TLS配置")
	}

	addr := fmt.Sprintf(":%d", rule.LocalPort)
	listener, err := quic.ListenAddr(addr, h.tlsConfig, h.quicConfig)
	if err != nil {
		return fmt.Errorf("QUIC监听失败: %w", err)
	}

	key := fmt.Sprintf("%s:%d", rule.Protocol, rule.LocalPort)
	h.quicListeners[key] = listener

	go h.handleQUICConnections(listener, rule)

	logger.Info("QUIC监听器已启动",
		zap.String("tunnel_id", rule.TunnelID),
		zap.Int("port", rule.LocalPort))

	return nil
}

// startTLSListener 启动TLS监听器
func (h *MultiProtocolHandler) startTLSListener(rule *ws.TunnelRule) error {
	if h.tlsConfig == nil {
		return fmt.Errorf("TLS配置未提供")
	}

	addr := fmt.Sprintf(":%d", rule.LocalPort)
	listener, err := tls.Listen("tcp", addr, h.tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS监听失败: %w", err)
	}

	key := fmt.Sprintf("%s:%d", rule.Protocol, rule.LocalPort)
	h.listeners[key] = listener

	go h.handleTLSConnections(listener, rule)

	logger.Info("TLS监听器已启动",
		zap.String("tunnel_id", rule.TunnelID),
		zap.Int("port", rule.LocalPort))

	return nil
}

// handleTCPConnections 处理TCP连接
func (h *MultiProtocolHandler) handleTCPConnections(listener net.Listener, rule *ws.TunnelRule) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("TCP处理协程panic", zap.Any("panic", r))
		}
	}()

	for {
		select {
		case <-h.stopChan:
			return
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			if isClosedError(err) {
				return
			}
			logger.Error("接受TCP连接失败", zap.Error(err))
			continue
		}

		go h.handleConnection(conn, rule, "tcp")
	}
}

// handleUDPConnections 处理UDP连接
func (h *MultiProtocolHandler) handleUDPConnections(udpConn *net.UDPConn, rule *ws.TunnelRule) {
	defer func() {
		udpConn.Close()
		if r := recover(); r != nil {
			logger.Error("UDP处理协程panic", zap.Any("panic", r))
		}
	}()

	buffer := make([]byte, 64*1024) // 64KB UDP buffer
	clientMap := make(map[string]*net.UDPConn)
	var mu sync.RWMutex

	for {
		select {
		case <-h.stopChan:
			mu.Lock()
			for _, conn := range clientMap {
				conn.Close()
			}
			mu.Unlock()
			return
		default:
		}

		n, clientAddr, err := udpConn.ReadFromUDP(buffer)
		if err != nil {
			if isClosedError(err) {
				return
			}
			logger.Error("读取UDP数据失败", zap.Error(err))
			continue
		}

		clientKey := clientAddr.String()

		mu.RLock()
		targetConn, exists := clientMap[clientKey]
		mu.RUnlock()

		if !exists {
			// 创建到目标的UDP连接
			target := selectTarget(rule.Targets)
			targetAddr := fmt.Sprintf("%s:%d", target.Host, target.Port)

			targetUDPAddr, err := net.ResolveUDPAddr("udp", targetAddr)
			if err != nil {
				logger.Error("解析目标UDP地址失败", zap.Error(err))
				continue
			}

			targetConn, err = net.DialUDP("udp", nil, targetUDPAddr)
			if err != nil {
				logger.Error("连接UDP目标失败", zap.Error(err))
				continue
			}

			mu.Lock()
			clientMap[clientKey] = targetConn
			mu.Unlock()

			// 启动回程数据处理
			go func(client *net.UDPAddr, target *net.UDPConn) {
				defer target.Close()
				buffer := make([]byte, 64*1024)

				for {
					n, err := target.Read(buffer)
					if err != nil {
						break
					}

					udpConn.WriteToUDP(buffer[:n], client)
				}

				mu.Lock()
				delete(clientMap, clientKey)
				mu.Unlock()
			}(clientAddr, targetConn)
		}

		// 转发到目标
		targetConn.Write(buffer[:n])
	}
}

// handleQUICConnections 处理QUIC连接
func (h *MultiProtocolHandler) handleQUICConnections(listener *quic.Listener, rule *ws.TunnelRule) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("QUIC处理协程panic", zap.Any("panic", r))
		}
	}()

	for {
		select {
		case <-h.stopChan:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		conn, err := listener.Accept(ctx)
		cancel()
		if err != nil {
			if isClosedError(err) {
				return
			}
			logger.Error("接受QUIC连接失败", zap.Error(err))
			continue
		}

		go h.handleQUICConnection(conn, rule)
	}
}

// handleQUICConnection 处理QUIC连接
func (h *MultiProtocolHandler) handleQUICConnection(conn interface{}, rule *ws.TunnelRule) {
	// TODO: 实现QUIC连接处理
	logger.Info("QUIC连接处理暂未完全实现", zap.String("tunnel_id", rule.TunnelID))
}

// handleQUICStream 处理QUIC流（暂时简化实现）
func (h *MultiProtocolHandler) handleQUICStream(stream interface{}, rule *ws.TunnelRule) {
	// TODO: 完整实现QUIC流处理
	logger.Debug("QUIC流处理", zap.String("tunnel_id", rule.TunnelID))
}

// handleTLSConnections 处理TLS连接
func (h *MultiProtocolHandler) handleTLSConnections(listener net.Listener, rule *ws.TunnelRule) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("TLS处理协程panic", zap.Any("panic", r))
		}
	}()

	for {
		select {
		case <-h.stopChan:
			return
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			if isClosedError(err) {
				return
			}
			logger.Error("接受TLS连接失败", zap.Error(err))
			continue
		}

		go h.handleConnection(conn, rule, "tls")
	}
}

// createHTTPHandler 创建HTTP处理器
func (h *MultiProtocolHandler) createHTTPHandler(rule *ws.TunnelRule) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := selectTarget(rule.Targets)
		targetURL := fmt.Sprintf("http://%s:%d%s", target.Host, target.Port, r.URL.Path)

		// 创建反向代理请求
		proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
		if err != nil {
			http.Error(w, "创建代理请求失败", http.StatusInternalServerError)
			return
		}

		// 复制头部
		for key, values := range r.Header {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}

		// 发送请求
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(proxyReq)
		if err != nil {
			http.Error(w, "代理请求失败", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// 复制响应头
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		w.WriteHeader(resp.StatusCode)

		// 复制响应体
		buffer := make([]byte, 32*1024)
		for {
			n, err := resp.Body.Read(buffer)
			if n > 0 {
				w.Write(buffer[:n])
			}
			if err != nil {
				break
			}
		}
	}
}

// createWebSocketHandler 创建WebSocket处理器
func (h *MultiProtocolHandler) createWebSocketHandler(rule *ws.TunnelRule) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 升级到WebSocket
		clientConn, err := h.wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Error("WebSocket升级失败", zap.Error(err))
			return
		}
		defer clientConn.Close()

		// 连接到目标WebSocket服务器
		target := selectTarget(rule.Targets)
		targetURL := fmt.Sprintf("ws://%s:%d%s", target.Host, target.Port, r.URL.Path)

		targetConn, _, err := websocket.DefaultDialer.Dial(targetURL, nil)
		if err != nil {
			logger.Error("连接目标WebSocket失败", zap.Error(err))
			return
		}
		defer targetConn.Close()

		// 双向WebSocket代理
		h.proxyWebSocket(clientConn, targetConn, rule)
	}
}

// handleConnection 处理通用连接
func (h *MultiProtocolHandler) handleConnection(clientConn net.Conn, rule *ws.TunnelRule, protocol string) {
	defer func() {
		clientConn.Close()
		if r := recover(); r != nil {
			logger.Error("连接处理panic",
				zap.String("protocol", protocol),
				zap.Any("panic", r))
		}
	}()

	// 选择目标
	target := selectTarget(rule.Targets)
	targetAddr := fmt.Sprintf("%s:%d", target.Host, target.Port)

	// 建立目标连接
	var targetConn net.Conn
	var err error

	switch protocol {
	case "tls":
		// 对于TLS隧道，目标连接也使用TLS
		targetConn, err = tls.Dial("tcp", targetAddr, &tls.Config{
			ServerName:         target.Host,
			InsecureSkipVerify: true, // 生产环境应该验证证书
		})
	default:
		targetConn, err = net.Dial("tcp", targetAddr)
	}

	if err != nil {
		logger.Error("连接目标失败",
			zap.String("protocol", protocol),
			zap.String("target", targetAddr),
			zap.Error(err))
		return
	}
	defer targetConn.Close()

	logger.Debug("连接已建立",
		zap.String("protocol", protocol),
		zap.String("client", clientConn.RemoteAddr().String()),
		zap.String("target", targetAddr))

	// 双向转发
	h.bidirectionalCopy(clientConn, targetConn, rule)
}

// bidirectionalCopy 双向数据复制
func (h *MultiProtocolHandler) bidirectionalCopy(conn1, conn2 net.Conn, rule *ws.TunnelRule) {
	var wg sync.WaitGroup
	wg.Add(2)

	// conn1 -> conn2
	go func() {
		defer wg.Done()
		h.copyData(conn2, conn1, rule.TunnelID, "upstream")
	}()

	// conn2 -> conn1
	go func() {
		defer wg.Done()
		h.copyData(conn1, conn2, rule.TunnelID, "downstream")
	}()

	wg.Wait()
}

// proxyWebSocket 代理WebSocket连接
func (h *MultiProtocolHandler) proxyWebSocket(client, target *websocket.Conn, rule *ws.TunnelRule) {
	var wg sync.WaitGroup
	wg.Add(2)

	// client -> target
	go func() {
		defer wg.Done()
		for {
			messageType, data, err := client.ReadMessage()
			if err != nil {
				break
			}
			if err := target.WriteMessage(messageType, data); err != nil {
				break
			}
		}
	}()

	// target -> client
	go func() {
		defer wg.Done()
		for {
			messageType, data, err := target.ReadMessage()
			if err != nil {
				break
			}
			if err := client.WriteMessage(messageType, data); err != nil {
				break
			}
		}
	}()

	wg.Wait()
}

// copyData 复制数据
func (h *MultiProtocolHandler) copyData(dst, src net.Conn, tunnelID, direction string) {
	buffer := make([]byte, 32*1024)

	for {
		n, err := src.Read(buffer)
		if n > 0 {
			if _, writeErr := dst.Write(buffer[:n]); writeErr != nil {
				break
			}
			// TODO: 更新统计信息
		}
		if err != nil {
			break
		}
	}
}

// StopTunnelListener 停止隧道监听器
func (h *MultiProtocolHandler) StopTunnelListener(rule *ws.TunnelRule) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := fmt.Sprintf("%s:%d", rule.Protocol, rule.LocalPort)

	// 停止常规监听器
	if listener, exists := h.listeners[key]; exists {
		listener.Close()
		delete(h.listeners, key)
	}

	// 停止UDP监听器
	if udpConn, exists := h.udpListeners[key]; exists {
		udpConn.Close()
		delete(h.udpListeners, key)
	}

	// 停止QUIC监听器
	if quicListener, exists := h.quicListeners[key]; exists {
		quicListener.Close()
		delete(h.quicListeners, key)
	}

	// 停止HTTP服务器
	if server, exists := h.httpServers[key]; exists {
		server.Close()
		delete(h.httpServers, key)
	}

	// 停止HTTP/3服务器
	if h3Server, exists := h.http3Servers[key]; exists {
		h3Server.Close()
		delete(h.http3Servers, key)
	}

	logger.Info("监听器已停止",
		zap.String("tunnel_id", rule.TunnelID),
		zap.String("protocol", rule.Protocol),
		zap.Int("port", rule.LocalPort))

	return nil
}

// Stop 停止所有监听器
func (h *MultiProtocolHandler) Stop() {
	close(h.stopChan)

	h.mu.Lock()
	defer h.mu.Unlock()

	// 关闭所有常规监听器
	for key, listener := range h.listeners {
		listener.Close()
		logger.Debug("关闭监听器", zap.String("key", key))
	}
	h.listeners = make(map[string]net.Listener)

	// 关闭所有UDP监听器
	for key, udpConn := range h.udpListeners {
		udpConn.Close()
		logger.Debug("关闭UDP监听器", zap.String("key", key))
	}
	h.udpListeners = make(map[string]*net.UDPConn)

	// 关闭所有QUIC监听器
	for key, listener := range h.quicListeners {
		listener.Close()
		logger.Debug("关闭QUIC监听器", zap.String("key", key))
	}
	h.quicListeners = make(map[string]*quic.Listener)

	// 关闭所有HTTP服务器
	for key, server := range h.httpServers {
		server.Close()
		logger.Debug("关闭HTTP服务器", zap.String("key", key))
	}
	h.httpServers = make(map[string]*http.Server)

	// 关闭所有HTTP/3服务器
	for key, server := range h.http3Servers {
		server.Close()
		logger.Debug("关闭HTTP/3服务器", zap.String("key", key))
	}
	h.http3Servers = make(map[string]*http3.Server)

	logger.Info("多协议处理器已停止")
}

// selectTarget 选择目标（简单轮询）
func selectTarget(targets []ws.TunnelTarget) ws.TunnelTarget {
	if len(targets) == 0 {
		return ws.TunnelTarget{Host: "localhost", Port: 80, Weight: 1}
	}

	if len(targets) == 1 {
		return targets[0]
	}

	// 简单的权重随机选择
	totalWeight := 0
	for i := range targets {
		if targets[i].Weight <= 0 {
			targets[i].Weight = 1
		}
		totalWeight += targets[i].Weight
	}

	// 使用当前时间作为随机种子
	rand := int(time.Now().UnixNano() % int64(totalWeight))
	for _, target := range targets {
		rand -= target.Weight
		if rand < 0 {
			return target
		}
	}

	return targets[0]
}

// isClosedError 检查是否是连接关闭错误
func isClosedError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return errStr == "use of closed network connection" ||
		errStr == "connection was closed" ||
		errStr == "context canceled"
}
