package traffic

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// FlowControlConfig 流量控制配置
type FlowControlConfig struct {
	// 令牌桶配置
	TokenBucketCapacity int64 `json:"token_bucket_capacity"`
	TokenBucketRate     int64 `json:"token_bucket_rate"`

	// 队列配置
	MaxQueueSize int           `json:"max_queue_size"`
	MaxWaitTime  time.Duration `json:"max_wait_time"`

	// 回压配置
	BackpressureThreshold float64       `json:"backpressure_threshold"` // 队列使用率阈值
	BackpressureWindow    time.Duration `json:"backpressure_window"`    // 回压检测窗口

	// 丢包配置
	DropThreshold    float64 `json:"drop_threshold"`    // 开始丢包的阈值
	AdaptiveDropping bool    `json:"adaptive_dropping"` // 自适应丢包
}

// DefaultFlowControlConfig 默认流量控制配置
func DefaultFlowControlConfig() *FlowControlConfig {
	return &FlowControlConfig{
		TokenBucketCapacity:   1000,
		TokenBucketRate:       100,
		MaxQueueSize:          10000,
		MaxWaitTime:           5 * time.Second,
		BackpressureThreshold: 0.8, // 80%
		BackpressureWindow:    1 * time.Second,
		DropThreshold:         0.95, // 95%
		AdaptiveDropping:      true,
	}
}

// BackpressureSignal 回压信号
type BackpressureSignal struct {
	Level     BackpressureLevel `json:"level"`
	Severity  float64           `json:"severity"` // 0.0-1.0严重程度
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
}

// BackpressureLevel 回压等级
type BackpressureLevel int

const (
	BackpressureLevelNone BackpressureLevel = iota
	BackpressureLevelLow
	BackpressureLevelMedium
	BackpressureLevelHigh
	BackpressureLevelCritical
)

// String 返回回压等级名称
func (b BackpressureLevel) String() string {
	switch b {
	case BackpressureLevelNone:
		return "NONE"
	case BackpressureLevelLow:
		return "LOW"
	case BackpressureLevelMedium:
		return "MEDIUM"
	case BackpressureLevelHigh:
		return "HIGH"
	case BackpressureLevelCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// FlowController 流量控制器
type FlowController struct {
	config        *FlowControlConfig
	rateLimiter   *RateLimiter
	priorityQueue *PriorityQueue
	multiQueue    *MultiQueue

	// 回压检测
	backpressureLevel  atomic.Int32 // BackpressureLevel
	backpressureSignal chan BackpressureSignal

	// 统计信息
	totalProcessed    atomic.Int64
	totalDropped      atomic.Int64
	totalBackpressure atomic.Int64
	avgProcessingTime atomic.Int64 // nanoseconds

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// NewFlowController 创建流量控制器
func NewFlowController(config *FlowControlConfig) *FlowController {
	if config == nil {
		config = DefaultFlowControlConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	fc := &FlowController{
		config:             config,
		rateLimiter:        NewRateLimiter(config.TokenBucketCapacity, config.TokenBucketRate),
		priorityQueue:      NewPriorityQueue(config.MaxQueueSize, config.MaxWaitTime),
		multiQueue:         NewMultiQueue(config.MaxQueueSize, config.MaxWaitTime),
		backpressureSignal: make(chan BackpressureSignal, 100),
		ctx:                ctx,
		cancel:             cancel,
		logger:             zap.L().Named("flow-controller"),
	}

	return fc
}

// Start 启动流量控制器
func (fc *FlowController) Start() error {
	fc.logger.Info("启动流量控制器",
		zap.Int64("token_capacity", fc.config.TokenBucketCapacity),
		zap.Int64("token_rate", fc.config.TokenBucketRate),
		zap.Int("queue_size", fc.config.MaxQueueSize))

	// 启动回压检测
	fc.wg.Add(1)
	go func() {
		defer fc.wg.Done()
		fc.backpressureMonitor()
	}()

	return nil
}

// Stop 停止流量控制器
func (fc *FlowController) Stop() error {
	fc.logger.Info("停止流量控制器")

	// 取消上下文
	if fc.cancel != nil {
		fc.cancel()
	}

	// 关闭队列
	fc.priorityQueue.Close()
	fc.multiQueue.CloseAll()

	// 等待协程结束
	fc.wg.Wait()

	// 关闭回压信号通道
	close(fc.backpressureSignal)

	fc.logger.Info("流量控制器已停止")
	return nil
}

// Submit 提交数据包处理
func (fc *FlowController) Submit(packet *Packet) error {
	return fc.SubmitToQueue("default", packet)
}

// SubmitToQueue 提交数据包到指定队列
func (fc *FlowController) SubmitToQueue(queueName string, packet *Packet) error {
	startTime := time.Now()
	defer func() {
		processingTime := time.Since(startTime).Nanoseconds()
		fc.updateAvgProcessingTime(processingTime)
	}()

	// 检查回压情况
	if fc.shouldDrop(packet) {
		fc.totalDropped.Add(1)
		fc.logger.Debug("回压丢包",
			zap.String("packet_id", packet.ID),
			zap.String("priority", packet.Priority.String()),
			zap.String("backpressure_level", fc.getBackpressureLevel().String()))
		return fmt.Errorf("回压丢包")
	}

	// 检查令牌桶限制
	tokenRequired := int64(packet.Size())
	if !fc.rateLimiter.TryConsume(queueName, tokenRequired) {
		// 如果是关键优先级，强制消费令牌
		if packet.Priority == PriorityCritical {
			ctx, cancel := context.WithTimeout(fc.ctx, time.Second)
			defer cancel()

			if err := fc.rateLimiter.Consume(ctx, queueName, tokenRequired); err != nil {
				fc.totalDropped.Add(1)
				return fmt.Errorf("令牌不足，丢弃关键数据包: %w", err)
			}
		} else {
			fc.totalDropped.Add(1)
			fc.logger.Debug("令牌不足，丢包",
				zap.String("packet_id", packet.ID),
				zap.Int64("required", tokenRequired),
				zap.String("queue", queueName))
			return fmt.Errorf("令牌不足")
		}
	}

	// 提交到队列
	queue := fc.multiQueue.GetQueue(queueName)
	if err := queue.Enqueue(packet); err != nil {
		fc.totalDropped.Add(1)
		return fmt.Errorf("入队失败: %w", err)
	}

	fc.totalProcessed.Add(1)

	fc.logger.Debug("数据包提交成功",
		zap.String("packet_id", packet.ID),
		zap.String("queue", queueName),
		zap.String("priority", packet.Priority.String()),
		zap.Int("size", packet.Size()))

	return nil
}

// Process 处理数据包
func (fc *FlowController) Process(ctx context.Context, queueName string, handler func(*Packet) error) error {
	queue := fc.multiQueue.GetQueue(queueName)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		packet, err := queue.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fc.logger.Error("出队失败", zap.Error(err))
			continue
		}

		// 处理数据包
		if err := handler(packet); err != nil {
			fc.logger.Error("处理数据包失败",
				zap.String("packet_id", packet.ID),
				zap.Error(err))

			// 根据错误类型决定是否重试
			if packet.Retries < 3 {
				packet.Retries++
				if queue.Enqueue(packet) != nil {
					fc.logger.Error("重新入队失败", zap.String("packet_id", packet.ID))
				}
			}
		}
	}
}

// shouldDrop 判断是否应该丢包
func (fc *FlowController) shouldDrop(packet *Packet) bool {
	if !fc.config.AdaptiveDropping {
		return false
	}

	// 关键优先级不丢包
	if packet.Priority == PriorityCritical {
		return false
	}

	// 获取当前回压等级
	level := fc.getBackpressureLevel()

	switch level {
	case BackpressureLevelNone, BackpressureLevelLow:
		return false
	case BackpressureLevelMedium:
		// 只丢弃低优先级包
		return packet.Priority == PriorityLow
	case BackpressureLevelHigh:
		// 丢弃低和普通优先级包
		return packet.Priority <= PriorityNormal
	case BackpressureLevelCritical:
		// 只保留关键优先级包
		return packet.Priority < PriorityCritical
	default:
		return false
	}
}

// backpressureMonitor 回压监控
func (fc *FlowController) backpressureMonitor() {
	ticker := time.NewTicker(fc.config.BackpressureWindow)
	defer ticker.Stop()

	fc.logger.Debug("启动回压监控")

	for {
		select {
		case <-fc.ctx.Done():
			fc.logger.Debug("停止回压监控")
			return
		case <-ticker.C:
			fc.checkBackpressure()
		}
	}
}

// checkBackpressure 检查回压情况
func (fc *FlowController) checkBackpressure() {
	// 计算整体队列使用率
	stats := fc.multiQueue.GetAllStats()
	queuesStats := stats["queues"].(map[string]interface{})

	var totalUtilization float64
	var queueCount int

	for _, queueStats := range queuesStats {
		if qStats, ok := queueStats.(map[string]interface{}); ok {
			if util, ok := qStats["utilization"].(float64); ok {
				totalUtilization += util
				queueCount++
			}
		}
	}

	if queueCount == 0 {
		return
	}

	avgUtilization := totalUtilization / float64(queueCount)

	// 计算回压等级
	var level BackpressureLevel
	var severity float64

	if avgUtilization < fc.config.BackpressureThreshold {
		level = BackpressureLevelNone
		severity = 0.0
	} else if avgUtilization < 0.85 {
		level = BackpressureLevelLow
		severity = (avgUtilization - fc.config.BackpressureThreshold) / (0.85 - fc.config.BackpressureThreshold)
	} else if avgUtilization < 0.90 {
		level = BackpressureLevelMedium
		severity = (avgUtilization - 0.85) / (0.90 - 0.85)
	} else if avgUtilization < fc.config.DropThreshold {
		level = BackpressureLevelHigh
		severity = (avgUtilization - 0.90) / (fc.config.DropThreshold - 0.90)
	} else {
		level = BackpressureLevelCritical
		severity = 1.0
	}

	// 更新回压等级
	oldLevel := BackpressureLevel(fc.backpressureLevel.Swap(int32(level)))

	// 如果等级变化，发送回压信号
	if level != oldLevel {
		signal := BackpressureSignal{
			Level:     level,
			Severity:  severity,
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("回压等级从 %s 变为 %s，队列平均使用率: %.2f%%", oldLevel.String(), level.String(), avgUtilization),
		}

		select {
		case fc.backpressureSignal <- signal:
			fc.totalBackpressure.Add(1)
			fc.logger.Info("回压等级变化",
				zap.String("old_level", oldLevel.String()),
				zap.String("new_level", level.String()),
				zap.Float64("utilization", avgUtilization),
				zap.Float64("severity", severity))
		default:
			fc.logger.Warn("回压信号通道已满")
		}
	}
}

// getBackpressureLevel 获取当前回压等级
func (fc *FlowController) getBackpressureLevel() BackpressureLevel {
	return BackpressureLevel(fc.backpressureLevel.Load())
}

// GetBackpressureSignal 获取回压信号通道
func (fc *FlowController) GetBackpressureSignal() <-chan BackpressureSignal {
	return fc.backpressureSignal
}

// updateAvgProcessingTime 更新平均处理时间
func (fc *FlowController) updateAvgProcessingTime(newTime int64) {
	for {
		oldAvg := fc.avgProcessingTime.Load()
		// 简单的指数移动平均
		newAvg := (oldAvg*9 + newTime) / 10
		if fc.avgProcessingTime.CompareAndSwap(oldAvg, newAvg) {
			break
		}
	}
}

// GetRateLimiter 获取速率限制器
func (fc *FlowController) GetRateLimiter() *RateLimiter {
	return fc.rateLimiter
}

// GetMultiQueue 获取多队列管理器
func (fc *FlowController) GetMultiQueue() *MultiQueue {
	return fc.multiQueue
}

// GetStats 获取流量控制器统计信息
func (fc *FlowController) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_processed":     fc.totalProcessed.Load(),
		"total_dropped":       fc.totalDropped.Load(),
		"total_backpressure":  fc.totalBackpressure.Load(),
		"avg_processing_time": time.Duration(fc.avgProcessingTime.Load()).String(),
		"backpressure_level":  fc.getBackpressureLevel().String(),
		"rate_limiter":        fc.rateLimiter.GetStats(),
		"queues":              fc.multiQueue.GetAllStats(),
		"config": map[string]interface{}{
			"token_bucket_capacity":  fc.config.TokenBucketCapacity,
			"token_bucket_rate":      fc.config.TokenBucketRate,
			"max_queue_size":         fc.config.MaxQueueSize,
			"backpressure_threshold": fc.config.BackpressureThreshold,
			"drop_threshold":         fc.config.DropThreshold,
			"adaptive_dropping":      fc.config.AdaptiveDropping,
		},
	}
}





