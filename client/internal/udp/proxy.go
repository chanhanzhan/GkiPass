package udp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ProxyConfig UDP代理配置
type ProxyConfig struct {
	ListenAddr   string        `json:"listen_addr"`   // 监听地址
	TargetAddr   string        `json:"target_addr"`   // 目标地址
	BufferSize   int           `json:"buffer_size"`   // 缓冲区大小
	ReadTimeout  time.Duration `json:"read_timeout"`  // 读取超时
	WriteTimeout time.Duration `json:"write_timeout"` // 写入超时
	MaxClients   int           `json:"max_clients"`   // 最大客户端数
}

// DefaultProxyConfig 默认代理配置
func DefaultProxyConfig() *ProxyConfig {
	return &ProxyConfig{
		ListenAddr:   ":0",
		TargetAddr:   "",
		BufferSize:   65536,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		MaxClients:   1000,
	}
}

// Proxy UDP代理
type Proxy struct {
	config     *ProxyConfig
	manager    *Manager
	listener   *net.UDPConn
	targetAddr *net.UDPAddr

	// 统计信息
	totalConnections atomic.Int64
	activeClients    atomic.Int64
	bytesReceived    atomic.Int64
	bytesSent        atomic.Int64
	packetsReceived  atomic.Int64
	packetsSent      atomic.Int64

	// 控制
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running atomic.Bool

	logger *zap.Logger
}

// NewProxy 创建UDP代理
func NewProxy(config *ProxyConfig, manager *Manager) (*Proxy, error) {
	if config == nil {
		config = DefaultProxyConfig()
	}

	if manager == nil {
		return nil, fmt.Errorf("会话管理器不能为空")
	}

	ctx, cancel := context.WithCancel(context.Background())

	proxy := &Proxy{
		config:  config,
		manager: manager,
		ctx:     ctx,
		cancel:  cancel,
		logger:  zap.L().Named("udp-proxy"),
	}

	// 解析目标地址
	if config.TargetAddr != "" {
		targetAddr, err := net.ResolveUDPAddr("udp", config.TargetAddr)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("解析目标地址失败: %w", err)
		}
		proxy.targetAddr = targetAddr
	}

	return proxy, nil
}

// Start 启动UDP代理
func (p *Proxy) Start() error {
	if p.running.Load() {
		return fmt.Errorf("代理已在运行")
	}

	// 创建监听socket
	listenAddr, err := net.ResolveUDPAddr("udp", p.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("解析监听地址失败: %w", err)
	}

	listener, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return fmt.Errorf("创建UDP监听器失败: %w", err)
	}

	p.listener = listener
	p.running.Store(true)

	p.logger.Info("启动UDP代理",
		zap.String("listen_addr", listener.LocalAddr().String()),
		zap.String("target_addr", p.config.TargetAddr))

	// 启动接收协程
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.receiveLoop()
	}()

	return nil
}

// Stop 停止UDP代理
func (p *Proxy) Stop() error {
	if !p.running.Load() {
		return nil
	}

	p.logger.Info("停止UDP代理")
	p.running.Store(false)

	// 取消上下文
	if p.cancel != nil {
		p.cancel()
	}

	// 关闭监听器
	if p.listener != nil {
		p.listener.Close()
	}

	// 等待协程结束
	p.wg.Wait()

	p.logger.Info("UDP代理已停止",
		zap.Int64("total_connections", p.totalConnections.Load()),
		zap.Int64("bytes_received", p.bytesReceived.Load()),
		zap.Int64("bytes_sent", p.bytesSent.Load()))

	return nil
}

// receiveLoop 接收循环
func (p *Proxy) receiveLoop() {
	defer p.logger.Debug("接收循环结束")

	buffer := make([]byte, p.config.BufferSize)

	for p.running.Load() {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		// 设置读取超时
		if p.config.ReadTimeout > 0 {
			p.listener.SetReadDeadline(time.Now().Add(p.config.ReadTimeout))
		}

		// 接收数据
		n, clientAddr, err := p.listener.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // 超时继续
			}
			if !p.running.Load() {
				return // 正常关闭
			}
			p.logger.Error("接收UDP数据失败", zap.Error(err))
			continue
		}

		if n > 0 {
			p.packetsReceived.Add(1)
			p.bytesReceived.Add(int64(n))

			// 处理数据包
			go p.handlePacket(buffer[:n], clientAddr)
		}
	}
}

// handlePacket 处理数据包
func (p *Proxy) handlePacket(data []byte, clientAddr *net.UDPAddr) {
	if p.targetAddr == nil {
		p.logger.Error("目标地址未设置")
		return
	}

	// 创建五元组
	tuple, err := ParseFiveTuple(clientAddr, p.targetAddr, "udp")
	if err != nil {
		p.logger.Error("解析五元组失败", zap.Error(err))
		return
	}

	// 获取或创建会话
	session, err := p.manager.GetSession(tuple, clientAddr, p.targetAddr)
	if err != nil {
		p.logger.Error("获取UDP会话失败", zap.Error(err))
		return
	}

	// 设置客户端连接（这里使用代理的监听器）
	session.SetClientConn(p.listener)

	p.logger.Debug("处理UDP数据包",
		zap.String("client_addr", clientAddr.String()),
		zap.String("target_addr", p.targetAddr.String()),
		zap.Int("size", len(data)),
		zap.String("session_id", session.GetID()))
}

// SetTargetAddr 设置目标地址
func (p *Proxy) SetTargetAddr(addr string) error {
	targetAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("解析目标地址失败: %w", err)
	}

	p.targetAddr = targetAddr
	p.config.TargetAddr = addr

	p.logger.Info("更新目标地址", zap.String("target_addr", addr))
	return nil
}

// GetListenAddr 获取监听地址
func (p *Proxy) GetListenAddr() net.Addr {
	if p.listener != nil {
		return p.listener.LocalAddr()
	}
	return nil
}

// IsRunning 检查是否运行中
func (p *Proxy) IsRunning() bool {
	return p.running.Load()
}

// GetStats 获取代理统计信息
func (p *Proxy) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"running":           p.running.Load(),
		"total_connections": p.totalConnections.Load(),
		"active_clients":    p.activeClients.Load(),
		"bytes_received":    p.bytesReceived.Load(),
		"bytes_sent":        p.bytesSent.Load(),
		"packets_received":  p.packetsReceived.Load(),
		"packets_sent":      p.packetsSent.Load(),
	}

	if p.listener != nil {
		stats["listen_addr"] = p.listener.LocalAddr().String()
	}
	if p.targetAddr != nil {
		stats["target_addr"] = p.targetAddr.String()
	}

	return stats
}

// Forwarder UDP转发器
type Forwarder struct {
	localAddr  *net.UDPAddr
	remoteAddr *net.UDPAddr
	manager    *Manager

	// 统计
	bytesForwarded   atomic.Int64
	packetsForwarded atomic.Int64

	logger *zap.Logger
}

// NewForwarder 创建UDP转发器
func NewForwarder(localAddr, remoteAddr string, manager *Manager) (*Forwarder, error) {
	local, err := net.ResolveUDPAddr("udp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("解析本地地址失败: %w", err)
	}

	remote, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("解析远程地址失败: %w", err)
	}

	return &Forwarder{
		localAddr:  local,
		remoteAddr: remote,
		manager:    manager,
		logger:     zap.L().Named("udp-forwarder"),
	}, nil
}

// Forward 转发UDP数据包
func (f *Forwarder) Forward(data []byte, sourceAddr *net.UDPAddr) error {
	// 创建连接到远程地址
	conn, err := net.DialUDP("udp", f.localAddr, f.remoteAddr)
	if err != nil {
		return fmt.Errorf("连接远程地址失败: %w", err)
	}
	defer conn.Close()

	// 发送数据
	n, err := conn.Write(data)
	if err != nil {
		return fmt.Errorf("发送数据失败: %w", err)
	}

	f.packetsForwarded.Add(1)
	f.bytesForwarded.Add(int64(n))

	f.logger.Debug("转发UDP数据包",
		zap.String("source", sourceAddr.String()),
		zap.String("remote", f.remoteAddr.String()),
		zap.Int("size", n))

	return nil
}

// GetStats 获取转发器统计信息
func (f *Forwarder) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"local_addr":        f.localAddr.String(),
		"remote_addr":       f.remoteAddr.String(),
		"bytes_forwarded":   f.bytesForwarded.Load(),
		"packets_forwarded": f.packetsForwarded.Load(),
	}
}





