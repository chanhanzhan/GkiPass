package recovery

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// RetryPolicy 重试策略接口
type RetryPolicy interface {
	ShouldRetry(attempt int, err error, duration time.Duration) bool
	GetDelay(attempt int) time.Duration
	GetName() string
}

// ExponentialBackoffRetryPolicy 指数退避重试策略
type ExponentialBackoffRetryPolicy struct {
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	MaxAttempts   int
	BackoffFactor float64
	Jitter        bool
}

func NewExponentialBackoffRetryPolicy(initialDelay, maxDelay time.Duration, maxAttempts int) *ExponentialBackoffRetryPolicy {
	return &ExponentialBackoffRetryPolicy{
		InitialDelay:  initialDelay,
		MaxDelay:      maxDelay,
		MaxAttempts:   maxAttempts,
		BackoffFactor: 2.0,
		Jitter:        true,
	}
}

func (p *ExponentialBackoffRetryPolicy) ShouldRetry(attempt int, err error, duration time.Duration) bool {
	if attempt >= p.MaxAttempts {
		return false
	}

	// 根据错误类型决定是否重试
	if !isRetryableError(err) {
		return false
	}

	return true
}

func (p *ExponentialBackoffRetryPolicy) GetDelay(attempt int) time.Duration {
	delay := time.Duration(float64(p.InitialDelay) * math.Pow(p.BackoffFactor, float64(attempt-1)))

	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}

	// 添加随机抖动
	if p.Jitter {
		jitter := time.Duration(float64(delay) * 0.1 * (2*rand.Float64() - 1))
		delay += jitter
	}

	return delay
}

func (p *ExponentialBackoffRetryPolicy) GetName() string {
	return "exponential_backoff"
}

// LinearBackoffRetryPolicy 线性退避重试策略
type LinearBackoffRetryPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	MaxAttempts  int
	Increment    time.Duration
}

func NewLinearBackoffRetryPolicy(initialDelay, maxDelay time.Duration, maxAttempts int) *LinearBackoffRetryPolicy {
	return &LinearBackoffRetryPolicy{
		InitialDelay: initialDelay,
		MaxDelay:     maxDelay,
		MaxAttempts:  maxAttempts,
		Increment:    initialDelay,
	}
}

func (p *LinearBackoffRetryPolicy) ShouldRetry(attempt int, err error, duration time.Duration) bool {
	if attempt >= p.MaxAttempts {
		return false
	}
	return isRetryableError(err)
}

func (p *LinearBackoffRetryPolicy) GetDelay(attempt int) time.Duration {
	delay := p.InitialDelay + time.Duration(attempt-1)*p.Increment
	if delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	return delay
}

func (p *LinearBackoffRetryPolicy) GetName() string {
	return "linear_backoff"
}

// FixedDelayRetryPolicy 固定延迟重试策略
type FixedDelayRetryPolicy struct {
	Delay       time.Duration
	MaxAttempts int
}

func NewFixedDelayRetryPolicy(delay time.Duration, maxAttempts int) *FixedDelayRetryPolicy {
	return &FixedDelayRetryPolicy{
		Delay:       delay,
		MaxAttempts: maxAttempts,
	}
}

func (p *FixedDelayRetryPolicy) ShouldRetry(attempt int, err error, duration time.Duration) bool {
	if attempt >= p.MaxAttempts {
		return false
	}
	return isRetryableError(err)
}

func (p *FixedDelayRetryPolicy) GetDelay(attempt int) time.Duration {
	return p.Delay
}

func (p *FixedDelayRetryPolicy) GetName() string {
	return "fixed_delay"
}

// AdaptiveRetryPolicy 自适应重试策略
type AdaptiveRetryPolicy struct {
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	MaxAttempts    int
	SuccessRate    atomic.Value // float64
	RecentAttempts []bool       // 最近的尝试结果
	mutex          sync.RWMutex
	windowSize     int
}

func NewAdaptiveRetryPolicy(initialDelay, maxDelay time.Duration, maxAttempts int) *AdaptiveRetryPolicy {
	policy := &AdaptiveRetryPolicy{
		InitialDelay:   initialDelay,
		MaxDelay:       maxDelay,
		MaxAttempts:    maxAttempts,
		RecentAttempts: make([]bool, 0, 100),
		windowSize:     100,
	}
	policy.SuccessRate.Store(0.5) // 初始成功率50%
	return policy
}

func (p *AdaptiveRetryPolicy) ShouldRetry(attempt int, err error, duration time.Duration) bool {
	if attempt >= p.MaxAttempts {
		return false
	}

	if !isRetryableError(err) {
		return false
	}

	// 根据成功率调整重试意愿
	successRate := p.SuccessRate.Load().(float64)
	if successRate < 0.1 && attempt > 2 {
		// 成功率太低，减少重试
		return false
	}

	return true
}

func (p *AdaptiveRetryPolicy) GetDelay(attempt int) time.Duration {
	successRate := p.SuccessRate.Load().(float64)

	// 根据成功率调整延迟
	baseDelay := time.Duration(float64(p.InitialDelay) * math.Pow(2.0, float64(attempt-1)))

	if successRate < 0.3 {
		// 成功率低，增加延迟
		baseDelay = time.Duration(float64(baseDelay) * 1.5)
	} else if successRate > 0.8 {
		// 成功率高，减少延迟
		baseDelay = time.Duration(float64(baseDelay) * 0.8)
	}

	if baseDelay > p.MaxDelay {
		baseDelay = p.MaxDelay
	}

	return baseDelay
}

func (p *AdaptiveRetryPolicy) UpdateSuccessRate(success bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.RecentAttempts = append(p.RecentAttempts, success)
	if len(p.RecentAttempts) > p.windowSize {
		p.RecentAttempts = p.RecentAttempts[1:]
	}

	// 计算成功率
	successCount := 0
	for _, s := range p.RecentAttempts {
		if s {
			successCount++
		}
	}

	newSuccessRate := float64(successCount) / float64(len(p.RecentAttempts))
	p.SuccessRate.Store(newSuccessRate)
}

func (p *AdaptiveRetryPolicy) GetName() string {
	return "adaptive"
}

// Retrier 重试器
type Retrier struct {
	policy  RetryPolicy
	logger  *zap.Logger
	metrics *RetryMetrics
}

// RetryMetrics 重试指标
type RetryMetrics struct {
	TotalAttempts     atomic.Int64
	SuccessfulRetries atomic.Int64
	FailedRetries     atomic.Int64
	TotalDelay        atomic.Int64 // 总延迟时间(毫秒)
}

func NewRetrier(policy RetryPolicy) *Retrier {
	return &Retrier{
		policy:  policy,
		logger:  zap.L().Named("retrier").With(zap.String("policy", policy.GetName())),
		metrics: &RetryMetrics{},
	}
}

// Execute 执行重试操作
func (r *Retrier) Execute(ctx context.Context, operation func() error) error {
	return r.ExecuteWithCallback(ctx, operation, nil)
}

// ExecuteWithCallback 执行重试操作（带回调）
func (r *Retrier) ExecuteWithCallback(ctx context.Context, operation func() error, callback func(attempt int, err error)) error {
	var lastErr error

	for attempt := 1; ; attempt++ {
		r.metrics.TotalAttempts.Add(1)

		// 执行操作
		startTime := time.Now()
		err := operation()
		duration := time.Since(startTime)

		if err == nil {
			// 成功
			r.metrics.SuccessfulRetries.Add(1)
			if adaptivePolicy, ok := r.policy.(*AdaptiveRetryPolicy); ok {
				adaptivePolicy.UpdateSuccessRate(true)
			}

			r.logger.Debug("操作成功",
				zap.Int("attempt", attempt),
				zap.Duration("duration", duration))

			if callback != nil {
				callback(attempt, nil)
			}

			return nil
		}

		lastErr = err

		// 检查是否应该重试
		if !r.policy.ShouldRetry(attempt, err, duration) {
			r.metrics.FailedRetries.Add(1)
			if adaptivePolicy, ok := r.policy.(*AdaptiveRetryPolicy); ok {
				adaptivePolicy.UpdateSuccessRate(false)
			}

			r.logger.Error("重试策略决定停止重试",
				zap.Int("attempt", attempt),
				zap.Error(err))

			if callback != nil {
				callback(attempt, err)
			}

			break
		}

		// 计算延迟时间
		delay := r.policy.GetDelay(attempt)
		r.metrics.TotalDelay.Add(delay.Milliseconds())

		r.logger.Warn("操作失败，准备重试",
			zap.Int("attempt", attempt),
			zap.Error(err),
			zap.Duration("delay", delay))

		if callback != nil {
			callback(attempt, err)
		}

		// 等待延迟时间
		select {
		case <-ctx.Done():
			return fmt.Errorf("重试被取消: %w", ctx.Err())
		case <-time.After(delay):
			// 继续下一次重试
		}
	}

	return fmt.Errorf("重试失败，最后错误: %w", lastErr)
}

// GetMetrics 获取重试指标
func (r *Retrier) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"policy":             r.policy.GetName(),
		"total_attempts":     r.metrics.TotalAttempts.Load(),
		"successful_retries": r.metrics.SuccessfulRetries.Load(),
		"failed_retries":     r.metrics.FailedRetries.Load(),
		"total_delay_ms":     r.metrics.TotalDelay.Load(),
	}
}

// CreateRetryPolicy 创建重试策略
func CreateRetryPolicy(strategyType string, config *RecoveryStrategy) RetryPolicy {
	switch strategyType {
	case "exponential":
		return NewExponentialBackoffRetryPolicy(
			config.InitialDelay,
			config.MaxDelay,
			config.MaxAttempts,
		)
	case "linear":
		return NewLinearBackoffRetryPolicy(
			config.InitialDelay,
			config.MaxDelay,
			config.MaxAttempts,
		)
	case "fixed":
		return NewFixedDelayRetryPolicy(
			config.InitialDelay,
			config.MaxAttempts,
		)
	case "adaptive":
		return NewAdaptiveRetryPolicy(
			config.InitialDelay,
			config.MaxDelay,
			config.MaxAttempts,
		)
	default:
		return NewExponentialBackoffRetryPolicy(
			config.InitialDelay,
			config.MaxDelay,
			config.MaxAttempts,
		)
	}
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// 网络相关错误通常可以重试
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"network unreachable",
		"temporary failure",
		"timeout",
		"i/o timeout",
		"broken pipe",
		"no route to host",
	}

	for _, retryableErr := range retryableErrors {
		if contains(errStr, retryableErr) {
			return true
		}
	}

	// 某些错误不应该重试
	nonRetryableErrors := []string{
		"authentication failed",
		"permission denied",
		"not found",
		"bad request",
		"invalid argument",
		"certificate",
	}

	for _, nonRetryableErr := range nonRetryableErrors {
		if contains(errStr, nonRetryableErr) {
			return false
		}
	}

	// 默认可以重试
	return true
}

// contains 检查字符串是否包含子字符串（忽略大小写）
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					findSubstring(s, substr))))
}

// findSubstring 查找子字符串
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// RetryBudget 重试预算管理
type RetryBudget struct {
	maxRetries     int
	currentRetries atomic.Int32
	windowSize     time.Duration
	resetTime      atomic.Int64
	mutex          sync.RWMutex
}

func NewRetryBudget(maxRetries int, windowSize time.Duration) *RetryBudget {
	budget := &RetryBudget{
		maxRetries: maxRetries,
		windowSize: windowSize,
	}
	budget.resetTime.Store(time.Now().Add(windowSize).Unix())
	return budget
}

// CanRetry 检查是否可以重试
func (rb *RetryBudget) CanRetry() bool {
	now := time.Now()
	resetTime := time.Unix(rb.resetTime.Load(), 0)

	// 检查是否需要重置预算
	if now.After(resetTime) {
		rb.mutex.Lock()
		if now.After(time.Unix(rb.resetTime.Load(), 0)) {
			rb.currentRetries.Store(0)
			rb.resetTime.Store(now.Add(rb.windowSize).Unix())
		}
		rb.mutex.Unlock()
	}

	current := rb.currentRetries.Load()
	return int(current) < rb.maxRetries
}

// ConsumeRetry 消费一次重试机会
func (rb *RetryBudget) ConsumeRetry() bool {
	if !rb.CanRetry() {
		return false
	}

	rb.currentRetries.Add(1)
	return true
}

// GetStats 获取预算统计
func (rb *RetryBudget) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"max_retries":     rb.maxRetries,
		"current_retries": rb.currentRetries.Load(),
		"window_size":     rb.windowSize.String(),
		"reset_time":      time.Unix(rb.resetTime.Load(), 0),
	}
}
