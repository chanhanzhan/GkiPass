package pool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// FlowControlMode 流控模式
type FlowControlMode int

const (
	FlowControlOff      FlowControlMode = iota // 关闭流控
	FlowControlSoft                            // 软流控（仅记录）
	FlowControlModerate                        // 适度流控
	FlowControlStrict                          // 严格流控
)

// FlowControlStats 流控统计信息
type FlowControlStats struct {
	TotalRequests      atomic.Int64
	ThrottledRequests  atomic.Int64
	DroppedRequests    atomic.Int64
	TotalBytes         atomic.Int64
	ThrottledBytes     atomic.Int64
	BackpressureEvents atomic.Int64
	BurstDetections    atomic.Int64
	AvgWaitTime        atomic.Int64 // 纳秒
}

// FlowController 综合流量控制器
type FlowController struct {
	// 限流器
	rateLimiter *RateLimiter

	// 背压控制
	backpressure *BackpressureController

	// 配置
	mode            FlowControlMode
	maxConcurrent   int64
	burstThreshold  float64 // 突发检测阈值（倍数）
	adaptiveEnabled bool

	// 状态
	currentConcurrent atomic.Int64
	stats             FlowControlStats

	// 自适应参数
	lastRate       atomic.Int64
	lastAdjustTime atomic.Value // time.Time

	mu sync.RWMutex
}

// FlowControlConfig 流控配置
type FlowControlConfig struct {
	Mode            FlowControlMode
	RateLimit       int64   // 字节/秒
	BurstSize       int64   // 突发大小
	MaxConcurrent   int64   // 最大并发数
	BufferSize      int     // 背压缓冲区大小
	BurstThreshold  float64 // 突发检测阈值
	AdaptiveEnabled bool    // 启用自适应流控
}

// NewFlowController 创建流量控制器
func NewFlowController(config FlowControlConfig) *FlowController {
	fc := &FlowController{
		rateLimiter:     NewRateLimiter(config.RateLimit, config.BurstSize),
		backpressure:    NewBackpressureController(config.BufferSize),
		mode:            config.Mode,
		maxConcurrent:   config.MaxConcurrent,
		burstThreshold:  config.BurstThreshold,
		adaptiveEnabled: config.AdaptiveEnabled,
	}

	fc.lastRate.Store(config.RateLimit)
	fc.lastAdjustTime.Store(time.Now())

	// 启动自适应调整
	if config.AdaptiveEnabled {
		go fc.adaptiveAdjustLoop()
	}

	logger.Info("流量控制器已创建",
		zap.String("mode", fc.modeString()),
		zap.Int64("rate_limit", config.RateLimit),
		zap.Bool("adaptive", config.AdaptiveEnabled))

	return fc
}

// modeString 返回模式字符串
func (fc *FlowController) modeString() string {
	switch fc.mode {
	case FlowControlOff:
		return "off"
	case FlowControlSoft:
		return "soft"
	case FlowControlModerate:
		return "moderate"
	case FlowControlStrict:
		return "strict"
	default:
		return "unknown"
	}
}

// AllowRequest 检查是否允许请求
func (fc *FlowController) AllowRequest(ctx context.Context, size int64) error {
	fc.stats.TotalRequests.Add(1)
	fc.stats.TotalBytes.Add(size)

	// 检查流控模式
	if fc.mode == FlowControlOff {
		return nil
	}

	// 检查并发数
	current := fc.currentConcurrent.Add(1)
	defer fc.currentConcurrent.Add(-1)

	if current > fc.maxConcurrent {
		fc.stats.ThrottledRequests.Add(1)
		if fc.mode == FlowControlStrict {
			fc.stats.DroppedRequests.Add(1)
			return fmt.Errorf("超过最大并发数: %d/%d", current, fc.maxConcurrent)
		}
	}

	// 检查突发流量
	if fc.detectBurst(size) {
		fc.stats.BurstDetections.Add(1)
		logger.Warn("检测到流量突发",
			zap.Int64("size", size),
			zap.Int64("rate", fc.rateLimiter.GetRate()))

		if fc.mode == FlowControlStrict {
			return fmt.Errorf("检测到流量突发，拒绝请求")
		}
	}

	// 限流处理
	startTime := time.Now()
	err := fc.rateLimiter.WaitContext(ctx, size)
	if err != nil {
		fc.stats.ThrottledRequests.Add(1)
		fc.stats.ThrottledBytes.Add(size)
		return err
	}

	// 记录等待时间
	waitTime := time.Since(startTime)
	if waitTime > 10*time.Millisecond {
		fc.stats.AvgWaitTime.Store(waitTime.Nanoseconds())
	}

	return nil
}

// detectBurst 检测流量突发
func (fc *FlowController) detectBurst(size int64) bool {
	rate := fc.rateLimiter.GetRate()
	if rate == 0 {
		return false
	}

	// 如果单次传输大小超过速率的阈值倍数，认为是突发
	threshold := int64(float64(rate) * fc.burstThreshold)
	return size > threshold
}

// CheckBackpressure 检查背压
func (fc *FlowController) CheckBackpressure() error {
	if fc.mode == FlowControlOff {
		return nil
	}

	if fc.backpressure.IsWriteCongested() {
		fc.stats.BackpressureEvents.Add(1)
		logger.Warn("触发背压控制，写缓冲区拥塞")

		if fc.mode == FlowControlStrict {
			return fmt.Errorf("背压：写缓冲区拥塞")
		}
	}

	return nil
}

// adaptiveAdjustLoop 自适应调整循环
func (fc *FlowController) adaptiveAdjustLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		fc.adaptiveAdjust()
	}
}

// adaptiveAdjust 自适应调整限流参数
func (fc *FlowController) adaptiveAdjust() {
	readUsage, writeUsage := fc.backpressure.GetBufferUsage()
	throttleRate := float64(fc.stats.ThrottledRequests.Load()) / float64(fc.stats.TotalRequests.Load()+1)

	currentRate := fc.rateLimiter.GetRate()
	newRate := currentRate

	// 如果拥塞率低且节流率低，可以提高速率
	if readUsage < 0.3 && writeUsage < 0.3 && throttleRate < 0.1 {
		newRate = int64(float64(currentRate) * 1.2) // 提高20%
		logger.Info("自适应流控：提高速率",
			zap.Int64("old_rate", currentRate),
			zap.Int64("new_rate", newRate))
	}

	// 如果拥塞率高或节流率高，降低速率
	if readUsage > 0.7 || writeUsage > 0.7 || throttleRate > 0.3 {
		newRate = int64(float64(currentRate) * 0.8) // 降低20%
		logger.Warn("自适应流控：降低速率",
			zap.Int64("old_rate", currentRate),
			zap.Int64("new_rate", newRate),
			zap.Float64("read_usage", readUsage),
			zap.Float64("write_usage", writeUsage))
	}

	if newRate != currentRate && newRate > 0 {
		fc.rateLimiter.SetRate(newRate)
		fc.lastRate.Store(newRate)
		fc.lastAdjustTime.Store(time.Now())
	}
}

// GetStats 获取流控统计信息
func (fc *FlowController) GetStats() FlowControlStatsSnapshot {
	return FlowControlStatsSnapshot{
		Mode:               fc.mode,
		TotalRequests:      fc.stats.TotalRequests.Load(),
		ThrottledRequests:  fc.stats.ThrottledRequests.Load(),
		DroppedRequests:    fc.stats.DroppedRequests.Load(),
		TotalBytes:         fc.stats.TotalBytes.Load(),
		ThrottledBytes:     fc.stats.ThrottledBytes.Load(),
		BackpressureEvents: fc.stats.BackpressureEvents.Load(),
		BurstDetections:    fc.stats.BurstDetections.Load(),
		AvgWaitTime:        time.Duration(fc.stats.AvgWaitTime.Load()),
		CurrentConcurrent:  fc.currentConcurrent.Load(),
		MaxConcurrent:      fc.maxConcurrent,
		CurrentRate:        fc.rateLimiter.GetRate(),
	}
}

// FlowControlStatsSnapshot 流控统计快照
type FlowControlStatsSnapshot struct {
	Mode               FlowControlMode
	TotalRequests      int64
	ThrottledRequests  int64
	DroppedRequests    int64
	TotalBytes         int64
	ThrottledBytes     int64
	BackpressureEvents int64
	BurstDetections    int64
	AvgWaitTime        time.Duration
	CurrentConcurrent  int64
	MaxConcurrent      int64
	CurrentRate        int64
}

// String 格式化输出统计信息
func (s FlowControlStatsSnapshot) String() string {
	throttlePercent := 0.0
	if s.TotalRequests > 0 {
		throttlePercent = float64(s.ThrottledRequests) / float64(s.TotalRequests) * 100
	}

	return fmt.Sprintf("Mode=%d Total=%d Throttled=%d(%.1f%%) Dropped=%d Burst=%d Backpressure=%d Concurrent=%d/%d Rate=%dB/s",
		s.Mode, s.TotalRequests, s.ThrottledRequests, throttlePercent,
		s.DroppedRequests, s.BurstDetections, s.BackpressureEvents,
		s.CurrentConcurrent, s.MaxConcurrent, s.CurrentRate)
}

// SetMode 设置流控模式
func (fc *FlowController) SetMode(mode FlowControlMode) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	oldMode := fc.mode
	fc.mode = mode

	logger.Info("流控模式已更改",
		zap.String("old_mode", fmt.Sprintf("%d", oldMode)),
		zap.String("new_mode", fmt.Sprintf("%d", mode)))
}

// Close 关闭流量控制器
func (fc *FlowController) Close() error {
	fc.backpressure.Close()
	logger.Info("流量控制器已关闭")
	return nil
}
