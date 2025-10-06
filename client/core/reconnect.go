package core

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// ReconnectStrategy 重连策略
type ReconnectStrategy struct {
	minDelay     time.Duration // 最小延迟
	maxDelay     time.Duration // 最大延迟
	multiplier   float64       // 延迟倍增因子
	jitter       float64       // 抖动因子 (0-1)
	attemptCount int           // 当前尝试次数
	maxAttempts  int           // 最大尝试次数 (0=无限)
	resetAfter   time.Duration // 成功后重置计数器的时间
	lastAttempt  time.Time     // 最后一次尝试时间
	lastSuccess  time.Time     // 最后一次成功时间
	mu           sync.RWMutex
}

// NewReconnectStrategy 创建重连策略
// 默认配置：最小1秒，最大5分钟，指数退避
func NewReconnectStrategy() *ReconnectStrategy {
	return &ReconnectStrategy{
		minDelay:    1 * time.Second,
		maxDelay:    5 * time.Minute,
		multiplier:  2.0,
		jitter:      0.1,
		maxAttempts: 0, // 0 表示无限重试
		resetAfter:  30 * time.Second,
	}
}

// WithMinDelay 设置最小延迟
func (rs *ReconnectStrategy) WithMinDelay(d time.Duration) *ReconnectStrategy {
	rs.minDelay = d
	return rs
}

// WithMaxDelay 设置最大延迟
func (rs *ReconnectStrategy) WithMaxDelay(d time.Duration) *ReconnectStrategy {
	rs.maxDelay = d
	return rs
}

// WithMaxAttempts 设置最大尝试次数
func (rs *ReconnectStrategy) WithMaxAttempts(n int) *ReconnectStrategy {
	rs.maxAttempts = n
	return rs
}

// NextDelay 计算下一次重连的延迟时间（指数退避 + 抖动）
func (rs *ReconnectStrategy) NextDelay() (time.Duration, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	// 检查是否超过最大尝试次数
	if rs.maxAttempts > 0 && rs.attemptCount >= rs.maxAttempts {
		return 0, false
	}

	// 检查是否需要重置计数器（距离上次成功超过resetAfter时间）
	if !rs.lastSuccess.IsZero() && time.Since(rs.lastSuccess) > rs.resetAfter {
		rs.attemptCount = 0
	}

	// 计算基础延迟（指数退避）
	delay := float64(rs.minDelay) * math.Pow(rs.multiplier, float64(rs.attemptCount))

	// 限制在最大延迟范围内
	if delay > float64(rs.maxDelay) {
		delay = float64(rs.maxDelay)
	}

	// 添加抖动以避免雪崩效应
	// jitter = delay * (1 ± jitter)
	jitterAmount := delay * rs.jitter * (2*rand.Float64() - 1)
	delay += jitterAmount

	// 确保不小于最小延迟
	if delay < float64(rs.minDelay) {
		delay = float64(rs.minDelay)
	}

	rs.attemptCount++
	rs.lastAttempt = time.Now()

	logger.Debug("计算重连延迟",
		zap.Int("attempt", rs.attemptCount),
		zap.Duration("delay", time.Duration(delay)),
		zap.Int("max_attempts", rs.maxAttempts))

	return time.Duration(delay), true
}

// RecordSuccess 记录连接成功
func (rs *ReconnectStrategy) RecordSuccess() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.lastSuccess = time.Now()

	logger.Info("连接成功，重置重连计数器",
		zap.Int("previous_attempts", rs.attemptCount))

	// 延迟重置计数器，避免短暂连接后立即断开时计数器被错误重置
	go func() {
		time.Sleep(rs.resetAfter)
		rs.mu.Lock()
		defer rs.mu.Unlock()
		if time.Since(rs.lastSuccess) >= rs.resetAfter {
			rs.attemptCount = 0
		}
	}()
}

// Reset 重置重连策略
func (rs *ReconnectStrategy) Reset() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.attemptCount = 0
	rs.lastAttempt = time.Time{}
	rs.lastSuccess = time.Time{}

	logger.Info("重连策略已重置")
}

// GetAttemptCount 获取当前尝试次数
func (rs *ReconnectStrategy) GetAttemptCount() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.attemptCount
}

// ShouldRetry 检查是否应该继续重试
func (rs *ReconnectStrategy) ShouldRetry() bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if rs.maxAttempts == 0 {
		return true // 无限重试
	}

	return rs.attemptCount < rs.maxAttempts
}

// ConnectionState 连接状态（用于持久化）
type ConnectionState struct {
	LastAttempt   time.Time     `json:"last_attempt"`
	LastSuccess   time.Time     `json:"last_success"`
	AttemptCount  int           `json:"attempt_count"`
	LastError     string        `json:"last_error,omitempty"`
	TotalAttempts int64         `json:"total_attempts"`
	TotalFailures int64         `json:"total_failures"`
	Uptime        time.Duration `json:"uptime"`
}

// SaveState 保存当前状态
func (rs *ReconnectStrategy) SaveState() *ConnectionState {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return &ConnectionState{
		LastAttempt:  rs.lastAttempt,
		LastSuccess:  rs.lastSuccess,
		AttemptCount: rs.attemptCount,
	}
}

// LoadState 加载状态
func (rs *ReconnectStrategy) LoadState(state *ConnectionState) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.lastAttempt = state.LastAttempt
	rs.lastSuccess = state.LastSuccess
	rs.attemptCount = state.AttemptCount

	logger.Info("已加载连接状态",
		zap.Time("last_success", state.LastSuccess),
		zap.Int("attempt_count", state.AttemptCount))
}

