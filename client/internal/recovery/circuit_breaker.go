package recovery

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// CircuitBreakerState 熔断器状态
type CircuitBreakerState int

const (
	StateClosed CircuitBreakerState = iota
	StateOpen
	StateHalfOpen
)

// String 返回熔断器状态名称
func (cbs CircuitBreakerState) String() string {
	switch cbs {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	name   string
	config *CircuitBreakerConfig

	// 状态
	state           atomic.Int32 // CircuitBreakerState
	failureCount    atomic.Int32
	successCount    atomic.Int32
	lastFailureTime atomic.Int64 // Unix timestamp

	// 统计
	totalRequests  atomic.Int64
	totalFailures  atomic.Int64
	totalSuccesses atomic.Int64

	mutex  sync.RWMutex
	logger *zap.Logger
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(name string, config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = &CircuitBreakerConfig{
			FailureThreshold: 5,
			SuccessThreshold: 3,
			Timeout:          60 * time.Second,
		}
	}

	return &CircuitBreaker{
		name:   name,
		config: config,
		logger: zap.L().Named("circuit-breaker").With(zap.String("name", name)),
	}
}

// Call 调用熔断器保护的操作
func (cb *CircuitBreaker) Call(fn func() error) error {
	if !cb.AllowRequest() {
		return fmt.Errorf("熔断器开启，拒绝请求")
	}

	cb.totalRequests.Add(1)

	err := fn()
	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// AllowRequest 检查是否允许请求
func (cb *CircuitBreaker) AllowRequest() bool {
	state := CircuitBreakerState(cb.state.Load())

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// 检查是否应该进入半开状态
		if cb.shouldAttemptReset() {
			cb.setState(StateHalfOpen)
			cb.logger.Info("熔断器进入半开状态")
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.totalSuccesses.Add(1)

	state := CircuitBreakerState(cb.state.Load())

	switch state {
	case StateClosed:
		cb.failureCount.Store(0)
	case StateHalfOpen:
		successCount := cb.successCount.Add(1)
		if int(successCount) >= cb.config.SuccessThreshold {
			cb.setState(StateClosed)
			cb.failureCount.Store(0)
			cb.successCount.Store(0)
			cb.logger.Info("熔断器恢复到关闭状态")
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.totalFailures.Add(1)
	cb.lastFailureTime.Store(time.Now().Unix())

	state := CircuitBreakerState(cb.state.Load())
	failureCount := cb.failureCount.Add(1)

	switch state {
	case StateClosed:
		if int(failureCount) >= cb.config.FailureThreshold {
			cb.setState(StateOpen)
			cb.logger.Warn("熔断器开启",
				zap.Int32("failure_count", failureCount),
				zap.Int("threshold", cb.config.FailureThreshold))
		}
	case StateHalfOpen:
		cb.setState(StateOpen)
		cb.successCount.Store(0)
		cb.logger.Warn("熔断器从半开状态回到开启状态")
	}
}

// shouldAttemptReset 检查是否应该尝试重置
func (cb *CircuitBreaker) shouldAttemptReset() bool {
	lastFailure := time.Unix(cb.lastFailureTime.Load(), 0)
	return time.Since(lastFailure) >= cb.config.Timeout
}

// setState 设置状态
func (cb *CircuitBreaker) setState(newState CircuitBreakerState) {
	oldState := CircuitBreakerState(cb.state.Swap(int32(newState)))
	if oldState != newState {
		cb.logger.Debug("熔断器状态变更",
			zap.String("old_state", oldState.String()),
			zap.String("new_state", newState.String()))
	}
}

// GetState 获取当前状态
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	return CircuitBreakerState(cb.state.Load())
}

// IsOpen 检查熔断器是否开启
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.GetState() == StateOpen
}

// Reset 重置熔断器
func (cb *CircuitBreaker) Reset() {
	cb.setState(StateClosed)
	cb.failureCount.Store(0)
	cb.successCount.Store(0)
	cb.logger.Info("熔断器已重置")
}

// GetStats 获取熔断器统计信息
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"name":            cb.name,
		"state":           cb.GetState().String(),
		"failure_count":   cb.failureCount.Load(),
		"success_count":   cb.successCount.Load(),
		"total_requests":  cb.totalRequests.Load(),
		"total_failures":  cb.totalFailures.Load(),
		"total_successes": cb.totalSuccesses.Load(),
		"last_failure":    time.Unix(cb.lastFailureTime.Load(), 0),
		"config": map[string]interface{}{
			"failure_threshold": cb.config.FailureThreshold,
			"success_threshold": cb.config.SuccessThreshold,
			"timeout":           cb.config.Timeout.String(),
		},
	}
}

// 熔断器管理相关方法（在manager.go中使用）

// isCircuitBreakerOpen 检查熔断器是否开启
func (m *Manager) isCircuitBreakerOpen(component string, errorType ErrorType) bool {
	if !m.config.CircuitBreakerConfig.Enabled {
		return false
	}

	cbKey := fmt.Sprintf("%s:%s", component, errorType.String())

	m.cbMutex.RLock()
	cb, exists := m.circuitBreakers[cbKey]
	m.cbMutex.RUnlock()

	if !exists {
		return false
	}

	return cb.IsOpen()
}

// updateCircuitBreaker 更新熔断器状态
func (m *Manager) updateCircuitBreaker(component string, errorType ErrorType, success bool) {
	if !m.config.CircuitBreakerConfig.Enabled {
		return
	}

	cbKey := fmt.Sprintf("%s:%s", component, errorType.String())

	m.cbMutex.Lock()
	cb, exists := m.circuitBreakers[cbKey]
	if !exists {
		cb = NewCircuitBreaker(cbKey, m.config.CircuitBreakerConfig)
		m.circuitBreakers[cbKey] = cb
	}
	m.cbMutex.Unlock()

	if success {
		cb.RecordSuccess()
	} else {
		cb.RecordFailure()
	}
}

// resetExpiredCircuitBreakers 重置过期的熔断器
func (m *Manager) resetExpiredCircuitBreakers() {
	m.cbMutex.RLock()
	cbs := make([]*CircuitBreaker, 0, len(m.circuitBreakers))
	for _, cb := range m.circuitBreakers {
		cbs = append(cbs, cb)
	}
	m.cbMutex.RUnlock()

	for _, cb := range cbs {
		if cb.GetState() == StateOpen && cb.shouldAttemptReset() {
			cb.setState(StateHalfOpen)
		}
	}
}
