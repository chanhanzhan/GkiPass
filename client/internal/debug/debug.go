package debug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	_ "net/http/pprof" // 导入pprof

	"go.uber.org/zap"

	"gkipass/client/internal/config"
)

// Mode 调试模式
type Mode string

const (
	ModeServer Mode = "server"
	ModeClient Mode = "client"
)

// TrafficTestResult 流量测试结果
type TrafficTestResult struct {
	StartTime       time.Time     `json:"start_time"`
	EndTime         time.Time     `json:"end_time"`
	Duration        time.Duration `json:"duration"`
	BytesSent       int64         `json:"bytes_sent"`
	BytesReceived   int64         `json:"bytes_received"`
	PacketsSent     int64         `json:"packets_sent"`
	PacketsReceived int64         `json:"packets_received"`
	PacketLoss      float64       `json:"packet_loss"`
	Throughput      float64       `json:"throughput"` // Mbps
	RTT             time.Duration `json:"rtt"`
	Jitter          time.Duration `json:"jitter"`
	Errors          []string      `json:"errors"`
}

// Manager 调试管理器
type Manager struct {
	config *config.DebugConfig
	mode   Mode

	// 服务器
	serverListener net.Listener
	serverPort     int

	// 客户端
	clientConn net.Conn
	clientPort int

	// 统计
	totalTests      atomic.Int64
	successfulTests atomic.Int64
	failedTests     atomic.Int64

	// 测试结果
	testResults  []TrafficTestResult
	resultsMutex sync.RWMutex

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// NewManager 创建调试管理器
func NewManager(cfg *config.DebugConfig) *Manager {
	if cfg == nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		config:      cfg,
		mode:        Mode(cfg.Mode),
		testResults: make([]TrafficTestResult, 0),
		ctx:         ctx,
		cancel:      cancel,
		logger:      zap.L().Named("debug-manager"),
	}
}

// Start 启动调试管理器
func (m *Manager) Start() error {
	if !m.config.Enabled {
		return nil
	}

	m.logger.Info("启动调试管理器",
		zap.String("mode", string(m.mode)),
		zap.Int("listen_port", m.config.ListenPort),
		zap.String("target_addr", m.config.TargetAddr),
		zap.String("protocol", m.config.Protocol))

	// 启动pprof服务器
	if m.config.EnablePprof {
		go m.startPprofServer()
	}

	// 根据模式启动相应服务
	switch m.mode {
	case ModeServer:
		if err := m.startServer(); err != nil {
			return fmt.Errorf("启动调试服务器失败: %w", err)
		}
	case ModeClient:
		if err := m.startClient(); err != nil {
			return fmt.Errorf("启动调试客户端失败: %w", err)
		}
	default:
		return fmt.Errorf("不支持的调试模式: %s", m.mode)
	}

	// 启动流量测试
	if m.config.TrafficTest {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.trafficTestLoop()
		}()
	}

	return nil
}

// Stop 停止调试管理器
func (m *Manager) Stop() error {
	if !m.config.Enabled {
		return nil
	}

	m.logger.Info("停止调试管理器")

	// 取消上下文
	if m.cancel != nil {
		m.cancel()
	}

	// 关闭服务器监听器
	if m.serverListener != nil {
		m.serverListener.Close()
	}

	// 关闭客户端连接
	if m.clientConn != nil {
		m.clientConn.Close()
	}

	// 等待协程结束
	m.wg.Wait()

	m.logger.Info("调试管理器已停止")
	return nil
}

// startPprofServer 启动pprof服务器
func (m *Manager) startPprofServer() {
	addr := fmt.Sprintf(":%d", m.config.PprofPort)
	m.logger.Info("启动pprof服务器", zap.String("addr", addr))

	server := &http.Server{
		Addr:    addr,
		Handler: http.DefaultServeMux,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		m.logger.Error("pprof服务器启动失败", zap.Error(err))
	}
}

// startServer 启动调试服务器
func (m *Manager) startServer() error {
	addr := fmt.Sprintf(":%d", m.config.ListenPort)

	switch m.config.Protocol {
	case "tcp":
		return m.startTCPServer(addr)
	case "udp":
		return m.startUDPServer(addr)
	case "http":
		return m.startHTTPServer(addr)
	case "ws":
		return m.startWSServer(addr)
	case "wss":
		return m.startWSSServer(addr)
	case "tls":
		return m.startTLSServer(addr)
	case "tls-mux":
		return m.startTLSMuxServer(addr)
	case "kcp":
		return m.startKCPServer(addr)
	case "quic":
		return m.startQUICServer(addr)
	default:
		return fmt.Errorf("不支持的协议: %s", m.config.Protocol)
	}
}

// startTCPServer 启动TCP服务器
func (m *Manager) startTCPServer(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("TCP监听失败: %w", err)
	}

	m.serverListener = listener
	m.serverPort = listener.Addr().(*net.TCPAddr).Port

	m.logger.Info("TCP调试服务器启动",
		zap.String("addr", listener.Addr().String()),
		zap.Int("port", m.serverPort))

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.acceptTCPConnections(listener)
	}()

	return nil
}

// acceptTCPConnections 接受TCP连接
func (m *Manager) acceptTCPConnections(listener net.Listener) {
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			if m.ctx.Err() != nil {
				return
			}
			m.logger.Error("接受连接失败", zap.Error(err))
			continue
		}

		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.handleTCPConnection(conn)
		}()
	}
}

// handleTCPConnection 处理TCP连接
func (m *Manager) handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	m.logger.Debug("处理TCP连接", zap.String("remote", conn.RemoteAddr().String()))

	buffer := make([]byte, m.config.TestDataSize)

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		// 读取数据
		n, err := conn.Read(buffer)
		if err != nil {
			if err.Error() != "EOF" {
				m.logger.Debug("读取数据失败", zap.Error(err))
			}
			return
		}

		// 回发数据
		if _, err := conn.Write(buffer[:n]); err != nil {
			m.logger.Debug("写入数据失败", zap.Error(err))
			return
		}
	}
}

// startUDPServer 启动UDP服务器
func (m *Manager) startUDPServer(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("解析UDP地址失败: %w", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("UDP监听失败: %w", err)
	}

	m.serverPort = conn.LocalAddr().(*net.UDPAddr).Port

	m.logger.Info("UDP调试服务器启动",
		zap.String("addr", conn.LocalAddr().String()),
		zap.Int("port", m.serverPort))

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer conn.Close()
		m.handleUDPConnection(conn)
	}()

	return nil
}

// handleUDPConnection 处理UDP连接
func (m *Manager) handleUDPConnection(conn *net.UDPConn) {
	buffer := make([]byte, m.config.TestDataSize)

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		// 读取数据
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if m.ctx.Err() != nil {
				return
			}
			m.logger.Debug("读取UDP数据失败", zap.Error(err))
			continue
		}

		// 回发数据
		if _, err := conn.WriteToUDP(buffer[:n], addr); err != nil {
			m.logger.Debug("写入UDP数据失败", zap.Error(err))
		}
	}
}

// startHTTPServer 启动HTTP服务器
func (m *Manager) startHTTPServer(addr string) error {
	mux := http.NewServeMux()

	// 添加测试端点
	mux.HandleFunc("/test", m.handleHTTPTest)
	mux.HandleFunc("/stats", m.handleHTTPStats)
	mux.HandleFunc("/health", m.handleHTTPHealth)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("HTTP监听失败: %w", err)
	}

	m.serverListener = listener
	m.serverPort = listener.Addr().(*net.TCPAddr).Port

	m.logger.Info("HTTP调试服务器启动",
		zap.String("addr", listener.Addr().String()),
		zap.Int("port", m.serverPort))

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			m.logger.Error("HTTP服务器运行失败", zap.Error(err))
		}
	}()

	return nil
}

// handleHTTPTest HTTP测试端点
func (m *Manager) handleHTTPTest(w http.ResponseWriter, r *http.Request) {
	// 读取请求体
	buffer := make([]byte, m.config.TestDataSize)
	n, err := r.Body.Read(buffer)
	if err != nil && err.Error() != "EOF" {
		http.Error(w, "读取请求失败", http.StatusBadRequest)
		return
	}

	// 回发数据
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(buffer[:n])
}

// handleHTTPStats HTTP统计端点
func (m *Manager) handleHTTPStats(w http.ResponseWriter, r *http.Request) {
	stats := m.GetStats()

	w.Header().Set("Content-Type", "application/json")

	// 简单的JSON序列化
	fmt.Fprintf(w, `{
		"total_tests": %d,
		"successful_tests": %d,
		"failed_tests": %d,
		"server_port": %d,
		"client_port": %d
	}`, stats["total_tests"], stats["successful_tests"], stats["failed_tests"],
		stats["server_port"], stats["client_port"])
}

// handleHTTPHealth HTTP健康检查端点
func (m *Manager) handleHTTPHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "ok", "timestamp": "%s"}`, time.Now().Format(time.RFC3339))
}

// startClient 启动调试客户端
func (m *Manager) startClient() error {
	// 客户端模式下，连接到服务器
	serverAddr := m.config.TargetAddr

	switch m.config.Protocol {
	case "tcp":
		return m.connectTCP(serverAddr)
	case "udp":
		return m.connectUDP(serverAddr)
	case "http":
		// HTTP客户端不需要持久连接
		return nil
	default:
		return fmt.Errorf("不支持的协议: %s", m.config.Protocol)
	}
}

// connectTCP 连接TCP服务器
func (m *Manager) connectTCP(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("连接TCP服务器失败: %w", err)
	}

	m.clientConn = conn
	m.clientPort = conn.LocalAddr().(*net.TCPAddr).Port

	m.logger.Info("TCP客户端连接成功",
		zap.String("server", addr),
		zap.Int("local_port", m.clientPort))

	return nil
}

// connectUDP 连接UDP服务器
func (m *Manager) connectUDP(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("解析UDP地址失败: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("连接UDP服务器失败: %w", err)
	}

	m.clientConn = conn
	m.clientPort = conn.LocalAddr().(*net.UDPAddr).Port

	m.logger.Info("UDP客户端连接成功",
		zap.String("server", addr),
		zap.Int("local_port", m.clientPort))

	return nil
}

// trafficTestLoop 流量测试循环
func (m *Manager) trafficTestLoop() {
	m.logger.Info("启动流量测试循环",
		zap.Duration("interval", m.config.TestInterval),
		zap.Duration("duration", m.config.TestDuration))

	ticker := time.NewTicker(m.config.TestInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Info("停止流量测试循环")
			return
		case <-ticker.C:
			if err := m.runTrafficTest(); err != nil {
				m.logger.Error("流量测试失败", zap.Error(err))
				m.failedTests.Add(1)
			} else {
				m.successfulTests.Add(1)
			}
			m.totalTests.Add(1)
		}
	}
}

// runTrafficTest 运行流量测试
func (m *Manager) runTrafficTest() error {
	result := TrafficTestResult{
		StartTime: time.Now(),
		Errors:    make([]string, 0),
	}

	switch m.config.Protocol {
	case "tcp":
		return m.runTCPTest(&result)
	case "udp":
		return m.runUDPTest(&result)
	case "http":
		return m.runHTTPTest(&result)
	default:
		return fmt.Errorf("不支持的协议: %s", m.config.Protocol)
	}
}

// runTCPTest 运行TCP测试
func (m *Manager) runTCPTest(result *TrafficTestResult) error {
	if m.clientConn == nil {
		return fmt.Errorf("TCP客户端未连接")
	}

	// 创建测试数据
	testData := make([]byte, m.config.TestDataSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	startTime := time.Now()

	// 发送数据
	n, err := m.clientConn.Write(testData)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("发送失败: %v", err))
		return err
	}
	result.BytesSent = int64(n)
	result.PacketsSent = 1

	// 接收响应
	response := make([]byte, len(testData))
	n, err = m.clientConn.Read(response)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("接收失败: %v", err))
		return err
	}
	result.BytesReceived = int64(n)
	result.PacketsReceived = 1

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	result.RTT = result.Duration

	// 计算吞吐量 (Mbps)
	if result.Duration > 0 {
		result.Throughput = float64(result.BytesSent+result.BytesReceived) * 8 / result.Duration.Seconds() / 1e6
	}

	// 保存结果
	m.saveTestResult(result)

	m.logger.Debug("TCP测试完成",
		zap.Duration("rtt", result.RTT),
		zap.Float64("throughput", result.Throughput),
		zap.Int64("bytes_sent", result.BytesSent))

	return nil
}

// runUDPTest 运行UDP测试
func (m *Manager) runUDPTest(result *TrafficTestResult) error {
	if m.clientConn == nil {
		return fmt.Errorf("UDP客户端未连接")
	}

	// 创建测试数据
	testData := make([]byte, m.config.TestDataSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	startTime := time.Now()

	// 发送数据
	n, err := m.clientConn.Write(testData)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("发送失败: %v", err))
		return err
	}
	result.BytesSent = int64(n)
	result.PacketsSent = 1

	// 设置读取超时
	m.clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// 接收响应
	response := make([]byte, len(testData))
	n, err = m.clientConn.Read(response)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("接收失败: %v", err))
		return err
	}
	result.BytesReceived = int64(n)
	result.PacketsReceived = 1

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	result.RTT = result.Duration

	// 计算吞吐量 (Mbps)
	if result.Duration > 0 {
		result.Throughput = float64(result.BytesSent+result.BytesReceived) * 8 / result.Duration.Seconds() / 1e6
	}

	// 保存结果
	m.saveTestResult(result)

	m.logger.Debug("UDP测试完成",
		zap.Duration("rtt", result.RTT),
		zap.Float64("throughput", result.Throughput),
		zap.Int64("bytes_sent", result.BytesSent))

	return nil
}

// runHTTPTest 运行HTTP测试
func (m *Manager) runHTTPTest(result *TrafficTestResult) error {
	// HTTP测试实现
	// 这里可以实现HTTP客户端测试逻辑
	return fmt.Errorf("HTTP测试暂未实现")
}

// saveTestResult 保存测试结果
func (m *Manager) saveTestResult(result *TrafficTestResult) {
	m.resultsMutex.Lock()
	defer m.resultsMutex.Unlock()

	m.testResults = append(m.testResults, *result)

	// 保留最近的100个结果
	if len(m.testResults) > 100 {
		m.testResults = m.testResults[len(m.testResults)-100:]
	}
}

// GetTestResults 获取测试结果
func (m *Manager) GetTestResults() []TrafficTestResult {
	m.resultsMutex.RLock()
	defer m.resultsMutex.RUnlock()

	results := make([]TrafficTestResult, len(m.testResults))
	copy(results, m.testResults)
	return results
}

// GetStats 获取统计信息
func (m *Manager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":          m.config.Enabled,
		"mode":             string(m.mode),
		"protocol":         m.config.Protocol,
		"listen_port":      m.config.ListenPort,
		"target_addr":      m.config.TargetAddr,
		"total_tests":      m.totalTests.Load(),
		"successful_tests": m.successfulTests.Load(),
		"failed_tests":     m.failedTests.Load(),
		"traffic_test":     m.config.TrafficTest,
		"test_interval":    m.config.TestInterval.String(),
		"test_duration":    m.config.TestDuration.String(),
		"test_data_size":   m.config.TestDataSize,
	}
}

// 新协议服务端占位符函数

// startWSServer 启动WebSocket服务器
func (m *Manager) startWSServer(addr string) error {
	m.logger.Info("WebSocket服务器启动", zap.String("addr", addr))
	// TODO: 实现WebSocket服务器
	return fmt.Errorf("WebSocket服务器暂未实现")
}

// startWSSServer 启动WebSocket Secure服务器
func (m *Manager) startWSSServer(addr string) error {
	m.logger.Info("WebSocket Secure服务器启动", zap.String("addr", addr))
	// TODO: 实现WebSocket Secure服务器
	return fmt.Errorf("WebSocket Secure服务器暂未实现")
}

// startTLSServer 启动TLS服务器
func (m *Manager) startTLSServer(addr string) error {
	m.logger.Info("TLS服务器启动", zap.String("addr", addr))
	// TODO: 实现TLS服务器
	return fmt.Errorf("TLS服务器暂未实现")
}

// startTLSMuxServer 启动TLS多路复用服务器
func (m *Manager) startTLSMuxServer(addr string) error {
	m.logger.Info("TLS多路复用服务器启动", zap.String("addr", addr))
	// TODO: 实现TLS多路复用服务器
	return fmt.Errorf("TLS多路复用服务器暂未实现")
}

// startKCPServer 启动KCP服务器
func (m *Manager) startKCPServer(addr string) error {
	m.logger.Info("KCP服务器启动", zap.String("addr", addr))
	// TODO: 实现KCP服务器
	return fmt.Errorf("KCP服务器暂未实现")
}

// startQUICServer 启动QUIC服务器
func (m *Manager) startQUICServer(addr string) error {
	m.logger.Info("QUIC服务器启动", zap.String("addr", addr))
	// TODO: 实现QUIC服务器
	return fmt.Errorf("QUIC服务器暂未实现")
}
