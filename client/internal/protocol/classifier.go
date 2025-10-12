package protocol

import (
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"gkipass/client/internal/detector"
)

// TrafficType 流量类型
type TrafficType string

const (
	TrafficTypeTCP       TrafficType = "tcp"
	TrafficTypeUDP       TrafficType = "udp"
	TrafficTypeHTTP      TrafficType = "http"
	TrafficTypeHTTPS     TrafficType = "https"
	TrafficTypeWebSocket TrafficType = "websocket"
	TrafficTypeSSH       TrafficType = "ssh"
	TrafficTypeDNS       TrafficType = "dns"
	TrafficTypeUnknown   TrafficType = "unknown"
)

// ClassifierConfig 分类器配置
type ClassifierConfig struct {
	DetectionTimeout time.Duration // 检测超时时间
	BufferSize       int           // 缓冲区大小
	EnableCache      bool          // 启用缓存
	CacheSize        int           // 缓存大小
	Logger           *zap.Logger   // 日志记录器
}

// DefaultClassifierConfig 默认分类器配置
func DefaultClassifierConfig() *ClassifierConfig {
	return &ClassifierConfig{
		DetectionTimeout: 2 * time.Second,
		BufferSize:       4096,
		EnableCache:      true,
		CacheSize:        1000,
		Logger:           zap.L().Named("traffic-classifier"),
	}
}

// ClassificationResult 分类结果
type ClassificationResult struct {
	TrafficType TrafficType
	Protocol    detector.Protocol
	Confidence  float64
	Buffer      []byte
}

// Classifier 流量分类器
type Classifier struct {
	config   *ClassifierConfig
	detector *detector.Detector
	cache    *sync.Map // 缓存IP:Port -> TrafficType的映射
	logger   *zap.Logger
}

// NewClassifier 创建流量分类器
func NewClassifier(config *ClassifierConfig) *Classifier {
	if config == nil {
		config = DefaultClassifierConfig()
	}

	detectorConfig := &detector.DetectorConfig{
		BufferSize:  config.BufferSize,
		Timeout:     config.DetectionTimeout,
		MaxAttempts: 3,
	}

	return &Classifier{
		config:   config,
		detector: detector.NewDetector(detectorConfig),
		cache:    &sync.Map{},
		logger:   config.Logger,
	}
}

// Classify 对连接进行流量分类
func (c *Classifier) Classify(conn net.Conn) (*ClassificationResult, error) {
	// 生成连接标识符
	connID := conn.RemoteAddr().String() + "->" + conn.LocalAddr().String()

	// 检查缓存
	if c.config.EnableCache {
		if cachedType, ok := c.cache.Load(connID); ok {
			c.logger.Debug("从缓存获取流量类型",
				zap.String("conn_id", connID),
				zap.String("type", string(cachedType.(TrafficType))))

			return &ClassificationResult{
				TrafficType: cachedType.(TrafficType),
				Protocol:    detector.Protocol(cachedType.(TrafficType)),
				Confidence:  1.0, // 缓存的结果置信度为1.0
			}, nil
		}
	}

	// 使用协议检测器检测协议
	result, err := c.detector.DetectProtocol(conn)
	if err != nil {
		c.logger.Error("协议检测失败",
			zap.String("conn_id", connID),
			zap.Error(err))
		return nil, err
	}

	// 将检测到的协议转换为流量类型
	trafficType := c.mapProtocolToTrafficType(result.Protocol)

	// 更新缓存
	if c.config.EnableCache {
		c.cache.Store(connID, trafficType)
	}

	c.logger.Debug("流量分类完成",
		zap.String("conn_id", connID),
		zap.String("protocol", string(result.Protocol)),
		zap.String("traffic_type", string(trafficType)),
		zap.Float64("confidence", result.Confidence))

	return &ClassificationResult{
		TrafficType: trafficType,
		Protocol:    result.Protocol,
		Confidence:  result.Confidence,
		Buffer:      result.Buffer,
	}, nil
}

// mapProtocolToTrafficType 将协议映射为流量类型
func (c *Classifier) mapProtocolToTrafficType(protocol detector.Protocol) TrafficType {
	switch protocol {
	case detector.ProtocolTCP:
		return TrafficTypeTCP
	case detector.ProtocolUDP:
		return TrafficTypeUDP
	case detector.ProtocolHTTP:
		return TrafficTypeHTTP
	case detector.ProtocolTLS, detector.ProtocolHTTPS:
		return TrafficTypeHTTPS
	case detector.ProtocolWebsocket:
		return TrafficTypeWebSocket
	case detector.ProtocolSSH:
		return TrafficTypeSSH
	case detector.ProtocolDNS:
		return TrafficTypeDNS
	default:
		return TrafficTypeUnknown
	}
}

// ClearCache 清除缓存
func (c *Classifier) ClearCache() {
	c.cache = &sync.Map{}
	c.logger.Info("已清除流量分类缓存")
}

// GetStats 获取统计信息
func (c *Classifier) GetStats() map[string]interface{} {
	var cacheSize int
	c.cache.Range(func(_, _ interface{}) bool {
		cacheSize++
		return true
	})

	return map[string]interface{}{
		"cache_size": cacheSize,
		"enabled":    true,
	}
}
