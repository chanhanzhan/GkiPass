package protocol

import (
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TrafficManager 流量管理器
type TrafficManager struct {
	classifier *Classifier
	forwarder  *Forwarder
	mutex      sync.RWMutex
	logger     *zap.Logger
}

// NewTrafficManager 创建流量管理器
func NewTrafficManager() *TrafficManager {
	logger := zap.L().Named("traffic-manager")

	classifierConfig := DefaultClassifierConfig()
	classifier := NewClassifier(classifierConfig)

	forwarderConfig := DefaultForwarderConfig()
	forwarder := NewForwarder(forwarderConfig)

	return &TrafficManager{
		classifier: classifier,
		forwarder:  forwarder,
		logger:     logger,
	}
}

// Start 启动流量管理器
func (m *TrafficManager) Start() error {
	m.logger.Info("启动流量管理器")

	// 启动转发器
	if err := m.forwarder.Start(); err != nil {
		return err
	}

	return nil
}

// Stop 停止流量管理器
func (m *TrafficManager) Stop() error {
	m.logger.Info("停止流量管理器")

	// 停止转发器
	if err := m.forwarder.Stop(); err != nil {
		m.logger.Error("停止转发器失败", zap.Error(err))
	}

	return nil
}

// SetOptimizationMode 设置优化模式
func (m *TrafficManager) SetOptimizationMode(mode string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.logger.Info("设置优化模式", zap.String("mode", mode))

	switch mode {
	case "latency":
		// 优化延迟
		m.forwarder.config.BufferSize = 4096
		m.forwarder.config.IdleTimeout = 30 * time.Second
	case "throughput":
		// 优化吞吐量
		m.forwarder.config.BufferSize = 32768
		m.forwarder.config.IdleTimeout = 5 * time.Minute
	case "memory":
		// 优化内存使用
		m.forwarder.config.BufferSize = 4096
		m.forwarder.config.IdleTimeout = 1 * time.Minute
	default:
		m.logger.Warn("未知的优化模式", zap.String("mode", mode))
	}
}

// Forward 转发连接
func (m *TrafficManager) Forward(clientConn net.Conn, targetAddr string) error {
	return m.forwarder.Forward(clientConn, targetAddr)
}

// GetStats 获取统计信息
func (m *TrafficManager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]interface{}{
		"classifier": m.classifier.GetStats(),
		"forwarder":  m.forwarder.GetStats(),
	}
}

// ClassifyConnection 分类连接
func (m *TrafficManager) ClassifyConnection(conn net.Conn) (*ClassificationResult, error) {
	return m.classifier.Classify(conn)
}
