package protocol

import (
	"fmt"
	"net"
	"sync"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// ProtocolHandler 协议处理器接口
type ProtocolHandler interface {
	// Handle 处理连接
	Handle(conn net.Conn, info *ProtocolInfo) error

	// Protocol 返回处理的协议类型
	Protocol() Protocol
}

// ProtocolRouter 智能协议路由器
type ProtocolRouter struct {
	detector *ProtocolDetector
	handlers map[Protocol]ProtocolHandler
	mu       sync.RWMutex

	// 默认处理器（当没有匹配的处理器时使用）
	defaultHandler ProtocolHandler
}

// NewProtocolRouter 创建协议路由器
func NewProtocolRouter() *ProtocolRouter {
	return &ProtocolRouter{
		detector: NewProtocolDetector(),
		handlers: make(map[Protocol]ProtocolHandler),
	}
}

// RegisterHandler 注册协议处理器
func (pr *ProtocolRouter) RegisterHandler(handler ProtocolHandler) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.handlers[handler.Protocol()] = handler
	logger.Info("注册协议处理器",
		zap.String("protocol", handler.Protocol().String()))
}

// SetDefaultHandler 设置默认处理器
func (pr *ProtocolRouter) SetDefaultHandler(handler ProtocolHandler) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.defaultHandler = handler
	logger.Info("设置默认协议处理器")
}

// Route 路由连接到对应的处理器
func (pr *ProtocolRouter) Route(conn net.Conn) error {
	// 检测协议
	info, data, err := pr.detector.DetectWithInfo(conn)
	if err != nil {
		logger.Error("协议检测失败", zap.Error(err))
		return fmt.Errorf("协议检测失败: %w", err)
	}

	// 创建带预读数据的连接
	wrappedConn := NewPeekConn(conn, data)

	// 查找处理器
	pr.mu.RLock()
	handler, ok := pr.handlers[info.Protocol]
	if !ok {
		// 尝试使用默认处理器
		handler = pr.defaultHandler
	}
	pr.mu.RUnlock()

	if handler == nil {
		return fmt.Errorf("没有找到协议 %s 的处理器", info.Protocol)
	}

	logger.Info("路由连接",
		zap.String("protocol", info.Protocol.String()),
		zap.Float64("confidence", info.Confidence))

	// 调用处理器
	return handler.Handle(wrappedConn, info)
}

// GetDetectionStats 获取检测统计信息
func (pr *ProtocolRouter) GetDetectionStats() DetectionStatsSnapshot {
	return pr.detector.GetStats()
}

// SimpleProtocolHandler 简单协议处理器适配器
type SimpleProtocolHandler struct {
	protocol   Protocol
	handleFunc func(net.Conn, *ProtocolInfo) error
}

// NewSimpleHandler 创建简单协议处理器
func NewSimpleHandler(protocol Protocol, handleFunc func(net.Conn, *ProtocolInfo) error) *SimpleProtocolHandler {
	return &SimpleProtocolHandler{
		protocol:   protocol,
		handleFunc: handleFunc,
	}
}

// Handle 实现 ProtocolHandler 接口
func (h *SimpleProtocolHandler) Handle(conn net.Conn, info *ProtocolInfo) error {
	if h.handleFunc != nil {
		return h.handleFunc(conn, info)
	}
	return fmt.Errorf("处理器未实现")
}

// Protocol 返回处理的协议类型
func (h *SimpleProtocolHandler) Protocol() Protocol {
	return h.protocol
}
