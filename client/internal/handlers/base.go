package handlers

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/internal/detector"

	"go.uber.org/zap"
)

// Handler 协议处理器接口
type Handler interface {
	// Name 获取处理器名称
	Name() string

	// Protocol 获取支持的协议
	Protocol() detector.Protocol

	// Handle 处理连接
	Handle(ctx context.Context, conn net.Conn, result *detector.DetectionResult) error

	// GetStats 获取统计信息
	GetStats() map[string]interface{}
}

// BaseHandler 基础处理器
type BaseHandler struct {
	name     string
	protocol detector.Protocol
	logger   *zap.Logger

	// 统计信息
	stats struct {
		connectionsHandled atomic.Int64
		bytesTransferred   atomic.Int64
		errorCount         atomic.Int64
		successCount       atomic.Int64
	}
}

// NewBaseHandler 创建基础处理器
func NewBaseHandler(name string, protocol detector.Protocol) *BaseHandler {
	return &BaseHandler{
		name:     name,
		protocol: protocol,
		logger:   zap.L().Named("handler-" + name),
	}
}

// Name 获取处理器名称
func (bh *BaseHandler) Name() string {
	return bh.name
}

// Protocol 获取支持的协议
func (bh *BaseHandler) Protocol() detector.Protocol {
	return bh.protocol
}

// GetStats 获取统计信息
func (bh *BaseHandler) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"name":                bh.name,
		"protocol":            string(bh.protocol),
		"connections_handled": bh.stats.connectionsHandled.Load(),
		"bytes_transferred":   bh.stats.bytesTransferred.Load(),
		"error_count":         bh.stats.errorCount.Load(),
		"success_count":       bh.stats.successCount.Load(),
		"success_rate":        bh.getSuccessRate(),
	}
}

// getSuccessRate 计算成功率
func (bh *BaseHandler) getSuccessRate() float64 {
	total := bh.stats.connectionsHandled.Load()
	if total == 0 {
		return 0.0
	}
	success := bh.stats.successCount.Load()
	return float64(success) / float64(total)
}

// recordConnection 记录连接处理
func (bh *BaseHandler) recordConnection(success bool, bytesTransferred int64) {
	bh.stats.connectionsHandled.Add(1)
	if success {
		bh.stats.successCount.Add(1)
	} else {
		bh.stats.errorCount.Add(1)
	}
	if bytesTransferred > 0 {
		bh.stats.bytesTransferred.Add(bytesTransferred)
	}
}

// RelayConnection 双向数据中继
type RelayConnection struct {
	client net.Conn
	server net.Conn
	logger *zap.Logger

	// 统计
	clientToServer atomic.Int64
	serverToClient atomic.Int64

	// 状态
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRelayConnection 创建中继连接
func NewRelayConnection(client, server net.Conn) *RelayConnection {
	ctx, cancel := context.WithCancel(context.Background())

	return &RelayConnection{
		client: client,
		server: server,
		logger: zap.L().Named("relay"),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start 开始双向中继
func (rc *RelayConnection) Start() error {
	rc.logger.Debug("开始双向数据中继",
		zap.String("client", rc.client.RemoteAddr().String()),
		zap.String("server", rc.server.RemoteAddr().String()))

	// 启动双向复制
	rc.wg.Add(2)
	go rc.copyData(rc.client, rc.server, &rc.clientToServer, "client->server")
	go rc.copyData(rc.server, rc.client, &rc.serverToClient, "server->client")

	return nil
}

// Wait 等待中继完成
func (rc *RelayConnection) Wait() {
	rc.wg.Wait()
}

// Stop 停止中继
func (rc *RelayConnection) Stop() {
	if rc.cancel != nil {
		rc.cancel()
	}

	// 关闭连接
	if rc.client != nil {
		rc.client.Close()
	}
	if rc.server != nil {
		rc.server.Close()
	}

	rc.wg.Wait()
}

// copyData 复制数据
func (rc *RelayConnection) copyData(src, dst net.Conn, counter *atomic.Int64, direction string) {
	defer rc.wg.Done()
	defer rc.logger.Debug("数据复制结束", zap.String("direction", direction))

	buffer := make([]byte, 32*1024) // 32KB缓冲区

	for {
		select {
		case <-rc.ctx.Done():
			return
		default:
		}

		// 设置读取超时
		src.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := src.Read(buffer)
		src.SetReadDeadline(time.Time{})

		if err != nil {
			if err != io.EOF {
				rc.logger.Debug("读取数据失败",
					zap.String("direction", direction),
					zap.Error(err))
			}
			return
		}

		if n > 0 {
			// 设置写入超时
			dst.SetWriteDeadline(time.Now().Add(30 * time.Second))
			_, err = dst.Write(buffer[:n])
			dst.SetWriteDeadline(time.Time{})

			if err != nil {
				rc.logger.Debug("写入数据失败",
					zap.String("direction", direction),
					zap.Error(err))
				return
			}

			counter.Add(int64(n))
		}
	}
}

// GetStats 获取中继统计
func (rc *RelayConnection) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"client_to_server": rc.clientToServer.Load(),
		"server_to_client": rc.serverToClient.Load(),
		"total_bytes":      rc.clientToServer.Load() + rc.serverToClient.Load(),
	}
}

// HandlerManager 处理器管理器
type HandlerManager struct {
	handlers map[detector.Protocol]Handler
	mutex    sync.RWMutex
	logger   *zap.Logger

	// 统计
	stats struct {
		totalHandlers      int
		totalConnections   atomic.Int64
		handledConnections atomic.Int64
		errorConnections   atomic.Int64
	}
}

// NewHandlerManager 创建处理器管理器
func NewHandlerManager() *HandlerManager {
	return &HandlerManager{
		handlers: make(map[detector.Protocol]Handler),
		logger:   zap.L().Named("handler-manager"),
	}
}

// RegisterHandler 注册处理器
func (hm *HandlerManager) RegisterHandler(handler Handler) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	protocol := handler.Protocol()
	hm.handlers[protocol] = handler
	hm.stats.totalHandlers = len(hm.handlers)

	hm.logger.Info("注册协议处理器",
		zap.String("name", handler.Name()),
		zap.String("protocol", string(protocol)))
}

// GetHandler 获取处理器
func (hm *HandlerManager) GetHandler(protocol detector.Protocol) (Handler, bool) {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	handler, exists := hm.handlers[protocol]
	return handler, exists
}

// HandleConnection 处理连接
func (hm *HandlerManager) HandleConnection(ctx context.Context, conn net.Conn, result *detector.DetectionResult) error {
	hm.stats.totalConnections.Add(1)

	handler, exists := hm.GetHandler(result.Protocol)
	if !exists {
		// 尝试使用未知协议处理器
		handler, exists = hm.GetHandler(detector.ProtocolUnknown)
		if !exists {
			hm.stats.errorConnections.Add(1)
			hm.logger.Error("没有找到协议处理器",
				zap.String("protocol", string(result.Protocol)),
				zap.String("remote_addr", conn.RemoteAddr().String()))
			return ErrNoHandler
		}
	}

	hm.logger.Info("处理连接",
		zap.String("handler", handler.Name()),
		zap.String("protocol", string(result.Protocol)),
		zap.Float64("confidence", result.Confidence),
		zap.String("remote_addr", conn.RemoteAddr().String()))

	// 执行处理器
	err := handler.Handle(ctx, conn, result)
	if err != nil {
		hm.stats.errorConnections.Add(1)
		hm.logger.Error("处理连接失败",
			zap.String("handler", handler.Name()),
			zap.String("remote_addr", conn.RemoteAddr().String()),
			zap.Error(err))
		return err
	}

	hm.stats.handledConnections.Add(1)
	return nil
}

// GetStats 获取管理器统计
func (hm *HandlerManager) GetStats() map[string]interface{} {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	handlerStats := make(map[string]interface{})
	for protocol, handler := range hm.handlers {
		handlerStats[string(protocol)] = handler.GetStats()
	}

	return map[string]interface{}{
		"total_handlers":      hm.stats.totalHandlers,
		"total_connections":   hm.stats.totalConnections.Load(),
		"handled_connections": hm.stats.handledConnections.Load(),
		"error_connections":   hm.stats.errorConnections.Load(),
		"success_rate":        hm.getSuccessRate(),
		"handlers":            handlerStats,
	}
}

// getSuccessRate 计算成功率
func (hm *HandlerManager) getSuccessRate() float64 {
	total := hm.stats.totalConnections.Load()
	if total == 0 {
		return 0.0
	}
	handled := hm.stats.handledConnections.Load()
	return float64(handled) / float64(total)
}

// ListHandlers 列出所有处理器
func (hm *HandlerManager) ListHandlers() []Handler {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	handlers := make([]Handler, 0, len(hm.handlers))
	for _, handler := range hm.handlers {
		handlers = append(handlers, handler)
	}
	return handlers
}

// RemoveHandler 移除处理器
func (hm *HandlerManager) RemoveHandler(protocol detector.Protocol) bool {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	if _, exists := hm.handlers[protocol]; exists {
		delete(hm.handlers, protocol)
		hm.stats.totalHandlers = len(hm.handlers)
		hm.logger.Info("移除协议处理器", zap.String("protocol", string(protocol)))
		return true
	}
	return false
}

// 错误定义
var (
	ErrNoHandler        = fmt.Errorf("没有找到协议处理器")
	ErrHandlerFailed    = fmt.Errorf("处理器执行失败")
	ErrInvalidProtocol  = fmt.Errorf("无效的协议")
	ErrConnectionClosed = fmt.Errorf("连接已关闭")
)





