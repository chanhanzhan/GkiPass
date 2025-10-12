package recovery

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ErrorType 错误类型
type ErrorType int

const (
	ErrorTypeConnection ErrorType = iota
	ErrorTypeRuleSync
	ErrorTypeAuthentication
	ErrorTypeCertificate
	ErrorTypeNetwork
	ErrorTypeTimeout
	ErrorTypeUnknown
)

// String 返回错误类型名称
func (et ErrorType) String() string {
	switch et {
	case ErrorTypeConnection:
		return "CONNECTION"
	case ErrorTypeRuleSync:
		return "RULE_SYNC"
	case ErrorTypeAuthentication:
		return "AUTHENTICATION"
	case ErrorTypeCertificate:
		return "CERTIFICATE"
	case ErrorTypeNetwork:
		return "NETWORK"
	case ErrorTypeTimeout:
		return "TIMEOUT"
	case ErrorTypeUnknown:
		return "UNKNOWN"
	default:
		return "UNDEFINED"
	}
}

// RecoveryAction 恢复动作
type RecoveryAction int

const (
	RecoveryActionRetry RecoveryAction = iota
	RecoveryActionReconnect
	RecoveryActionRestart
	RecoveryActionSkip
	RecoveryActionEscalate
)

// String 返回恢复动作名称
func (ra RecoveryAction) String() string {
	switch ra {
	case RecoveryActionRetry:
		return "RETRY"
	case RecoveryActionReconnect:
		return "RECONNECT"
	case RecoveryActionRestart:
		return "RESTART"
	case RecoveryActionSkip:
		return "SKIP"
	case RecoveryActionEscalate:
		return "ESCALATE"
	default:
		return "UNKNOWN"
	}
}

// ErrorEvent 错误事件
type ErrorEvent struct {
	ID        string                 `json:"id"`
	Type      ErrorType              `json:"type"`
	Component string                 `json:"component"` // 组件名称
	Error     error                  `json:"error"`
	Context   map[string]interface{} `json:"context"` // 错误上下文
	Timestamp time.Time              `json:"timestamp"`
	Severity  int                    `json:"severity"` // 严重程度 1-10
}

// NewErrorEvent 创建错误事件
func NewErrorEvent(errorType ErrorType, component string, err error, severity int) *ErrorEvent {
	return &ErrorEvent{
		ID:        generateEventID(),
		Type:      errorType,
		Component: component,
		Error:     err,
		Context:   make(map[string]interface{}),
		Timestamp: time.Now(),
		Severity:  severity,
	}
}

// RecoveryStrategy 恢复策略
type RecoveryStrategy struct {
	MaxAttempts     int           `json:"max_attempts"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	BackoffFactor   float64       `json:"backoff_factor"`
	Jitter          bool          `json:"jitter"`          // 是否添加随机抖动
	CircuitBreaker  bool          `json:"circuit_breaker"` // 是否启用熔断器
	TimeoutDuration time.Duration `json:"timeout_duration"`
}

// DefaultRecoveryStrategy 默认恢复策略
func DefaultRecoveryStrategy() *RecoveryStrategy {
	return &RecoveryStrategy{
		MaxAttempts:     5,
		InitialDelay:    1 * time.Second,
		MaxDelay:        60 * time.Second,
		BackoffFactor:   2.0,
		Jitter:          true,
		CircuitBreaker:  true,
		TimeoutDuration: 30 * time.Second,
	}
}

// RecoveryAttempt 恢复尝试记录
type RecoveryAttempt struct {
	EventID   string         `json:"event_id"`
	Attempt   int            `json:"attempt"`
	Action    RecoveryAction `json:"action"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time"`
	Success   bool           `json:"success"`
	Error     error          `json:"error,omitempty"`
	Duration  time.Duration  `json:"duration"`
}

// RecoveryHandler 恢复处理器接口
type RecoveryHandler interface {
	CanHandle(event *ErrorEvent) bool
	Handle(ctx context.Context, event *ErrorEvent, strategy *RecoveryStrategy) (*RecoveryAttempt, error)
	GetName() string
}

// ManagerConfig 恢复管理器配置
type ManagerConfig struct {
	// 基础配置
	Enabled            bool          `json:"enabled"`
	MaxConcurrentTasks int           `json:"max_concurrent_tasks"`
	TaskTimeout        time.Duration `json:"task_timeout"`

	// 策略配置
	DefaultStrategy     *RecoveryStrategy               `json:"default_strategy"`
	TypeStrategies      map[ErrorType]*RecoveryStrategy `json:"type_strategies"`
	ComponentStrategies map[string]*RecoveryStrategy    `json:"component_strategies"`

	// 监控配置
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	MetricsEnabled      bool          `json:"metrics_enabled"`

	// 熔断器配置
	CircuitBreakerConfig *CircuitBreakerConfig `json:"circuit_breaker_config"`
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	Enabled          bool          `json:"enabled"`
	FailureThreshold int           `json:"failure_threshold"`
	SuccessThreshold int           `json:"success_threshold"`
	Timeout          time.Duration `json:"timeout"`
}

// DefaultManagerConfig 默认管理器配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		Enabled:             true,
		MaxConcurrentTasks:  10,
		TaskTimeout:         5 * time.Minute,
		DefaultStrategy:     DefaultRecoveryStrategy(),
		TypeStrategies:      make(map[ErrorType]*RecoveryStrategy),
		ComponentStrategies: make(map[string]*RecoveryStrategy),
		HealthCheckInterval: 30 * time.Second,
		MetricsEnabled:      true,
		CircuitBreakerConfig: &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 3,
			Timeout:          60 * time.Second,
		},
	}
}

// Manager 错误恢复管理器
type Manager struct {
	config   *ManagerConfig
	handlers []RecoveryHandler
	events   chan *ErrorEvent

	// 状态跟踪
	activeRecoveries map[string]*RecoveryTask
	recoveryHistory  []*RecoveryAttempt
	historyMutex     sync.RWMutex

	// 熔断器
	circuitBreakers map[string]*CircuitBreaker
	cbMutex         sync.RWMutex

	// 统计信息
	totalEvents          atomic.Int64
	successfulRecoveries atomic.Int64
	failedRecoveries     atomic.Int64

	// 控制
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	semaphore chan struct{} // 限制并发任务数

	logger *zap.Logger
}

// RecoveryTask 恢复任务
type RecoveryTask struct {
	Event     *ErrorEvent
	Handler   RecoveryHandler
	Strategy  *RecoveryStrategy
	Attempts  []*RecoveryAttempt
	StartTime time.Time
	Status    TaskStatus
	mutex     sync.RWMutex
}

// TaskStatus 任务状态
type TaskStatus int

const (
	TaskStatusPending TaskStatus = iota
	TaskStatusRunning
	TaskStatusCompleted
	TaskStatusFailed
	TaskStatusTimeout
)

// String 返回任务状态名称
func (ts TaskStatus) String() string {
	switch ts {
	case TaskStatusPending:
		return "PENDING"
	case TaskStatusRunning:
		return "RUNNING"
	case TaskStatusCompleted:
		return "COMPLETED"
	case TaskStatusFailed:
		return "FAILED"
	case TaskStatusTimeout:
		return "TIMEOUT"
	default:
		return "UNKNOWN"
	}
}

// NewManager 创建恢复管理器
func NewManager(config *ManagerConfig) *Manager {
	if config == nil {
		config = DefaultManagerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		config:           config,
		handlers:         make([]RecoveryHandler, 0),
		events:           make(chan *ErrorEvent, 1000),
		activeRecoveries: make(map[string]*RecoveryTask),
		recoveryHistory:  make([]*RecoveryAttempt, 0),
		circuitBreakers:  make(map[string]*CircuitBreaker),
		ctx:              ctx,
		cancel:           cancel,
		semaphore:        make(chan struct{}, config.MaxConcurrentTasks),
		logger:           zap.L().Named("recovery-manager"),
	}
}

// Start 启动恢复管理器
func (m *Manager) Start() error {
	if !m.config.Enabled {
		return fmt.Errorf("恢复管理器未启用")
	}

	m.logger.Info("启动错误恢复管理器",
		zap.Int("max_concurrent_tasks", m.config.MaxConcurrentTasks),
		zap.Duration("task_timeout", m.config.TaskTimeout))

	// 启动事件处理协程
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.eventLoop()
	}()

	// 启动健康检查协程
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.healthCheckLoop()
	}()

	return nil
}

// Stop 停止恢复管理器
func (m *Manager) Stop() error {
	m.logger.Info("停止错误恢复管理器")

	// 取消上下文
	if m.cancel != nil {
		m.cancel()
	}

	// 关闭事件通道
	close(m.events)

	// 等待协程结束
	m.wg.Wait()

	m.logger.Info("错误恢复管理器已停止")
	return nil
}

// RegisterHandler 注册恢复处理器
func (m *Manager) RegisterHandler(handler RecoveryHandler) {
	m.handlers = append(m.handlers, handler)
	m.logger.Info("注册恢复处理器", zap.String("handler", handler.GetName()))
}

// ReportError 报告错误
func (m *Manager) ReportError(errorType ErrorType, component string, err error, severity int) {
	event := NewErrorEvent(errorType, component, err, severity)

	select {
	case m.events <- event:
		m.totalEvents.Add(1)
		m.logger.Debug("错误事件已提交",
			zap.String("event_id", event.ID),
			zap.String("type", event.Type.String()),
			zap.String("component", component))
	default:
		m.logger.Warn("错误事件队列已满，丢弃事件",
			zap.String("type", event.Type.String()),
			zap.String("component", component))
	}
}

// eventLoop 事件处理循环
func (m *Manager) eventLoop() {
	m.logger.Debug("启动错误事件处理循环")

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Debug("停止错误事件处理循环")
			return
		case event, ok := <-m.events:
			if !ok {
				return
			}
			m.processEvent(event)
		}
	}
}

// processEvent 处理错误事件
func (m *Manager) processEvent(event *ErrorEvent) {
	// 检查熔断器状态
	if m.isCircuitBreakerOpen(event.Component, event.Type) {
		m.logger.Debug("熔断器开启，跳过恢复",
			zap.String("event_id", event.ID),
			zap.String("component", event.Component))
		return
	}

	// 查找合适的处理器
	handler := m.findHandler(event)
	if handler == nil {
		m.logger.Warn("未找到合适的恢复处理器",
			zap.String("event_id", event.ID),
			zap.String("type", event.Type.String()))
		return
	}

	// 获取恢复策略
	strategy := m.getStrategy(event)

	// 创建恢复任务
	task := &RecoveryTask{
		Event:     event,
		Handler:   handler,
		Strategy:  strategy,
		Attempts:  make([]*RecoveryAttempt, 0),
		StartTime: time.Now(),
		Status:    TaskStatusPending,
	}

	// 添加到活跃任务
	m.activeRecoveries[event.ID] = task

	// 异步执行恢复任务
	go m.executeRecoveryTask(task)
}

// executeRecoveryTask 执行恢复任务
func (m *Manager) executeRecoveryTask(task *RecoveryTask) {
	// 获取信号量
	select {
	case m.semaphore <- struct{}{}:
		defer func() { <-m.semaphore }()
	case <-m.ctx.Done():
		return
	}

	task.mutex.Lock()
	task.Status = TaskStatusRunning
	task.mutex.Unlock()

	// 创建任务上下文
	taskCtx, taskCancel := context.WithTimeout(m.ctx, m.config.TaskTimeout)
	defer taskCancel()

	m.logger.Info("开始执行恢复任务",
		zap.String("event_id", task.Event.ID),
		zap.String("handler", task.Handler.GetName()),
		zap.Int("max_attempts", task.Strategy.MaxAttempts))

	success := false
	for attempt := 1; attempt <= task.Strategy.MaxAttempts; attempt++ {
		// 检查上下文是否已取消
		select {
		case <-taskCtx.Done():
			task.mutex.Lock()
			task.Status = TaskStatusTimeout
			task.mutex.Unlock()

			delete(m.activeRecoveries, task.Event.ID)
			m.logger.Warn("恢复任务超时", zap.String("event_id", task.Event.ID))
			return
		default:
		}

		// 计算延迟时间
		delay := m.calculateDelay(task.Strategy, attempt)
		if attempt > 1 && delay > 0 {
			m.logger.Debug("等待重试",
				zap.String("event_id", task.Event.ID),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))

			select {
			case <-time.After(delay):
			case <-taskCtx.Done():
				return
			}
		}

		// 执行恢复尝试
		recoveryAttempt, err := task.Handler.Handle(taskCtx, task.Event, task.Strategy)
		if recoveryAttempt == nil {
			recoveryAttempt = &RecoveryAttempt{
				EventID:   task.Event.ID,
				Attempt:   attempt,
				Action:    RecoveryActionRetry,
				StartTime: time.Now(),
				EndTime:   time.Now(),
				Success:   false,
				Error:     err,
				Duration:  0,
			}
		}

		// 记录尝试结果
		task.mutex.Lock()
		task.Attempts = append(task.Attempts, recoveryAttempt)
		task.mutex.Unlock()

		m.addToHistory(recoveryAttempt)

		if err == nil && recoveryAttempt.Success {
			success = true
			m.successfulRecoveries.Add(1)
			m.updateCircuitBreaker(task.Event.Component, task.Event.Type, true)

			m.logger.Info("恢复任务成功",
				zap.String("event_id", task.Event.ID),
				zap.Int("attempt", attempt),
				zap.Duration("duration", recoveryAttempt.Duration))
			break
		} else {
			m.updateCircuitBreaker(task.Event.Component, task.Event.Type, false)

			m.logger.Warn("恢复尝试失败",
				zap.String("event_id", task.Event.ID),
				zap.Int("attempt", attempt),
				zap.Error(err))
		}
	}

	// 更新任务状态
	task.mutex.Lock()
	if success {
		task.Status = TaskStatusCompleted
	} else {
		task.Status = TaskStatusFailed
		m.failedRecoveries.Add(1)
	}
	task.mutex.Unlock()

	// 从活跃任务中移除
	delete(m.activeRecoveries, task.Event.ID)

	if !success {
		m.logger.Error("恢复任务最终失败",
			zap.String("event_id", task.Event.ID),
			zap.Int("total_attempts", len(task.Attempts)))
	}
}

// findHandler 查找处理器
func (m *Manager) findHandler(event *ErrorEvent) RecoveryHandler {
	for _, handler := range m.handlers {
		if handler.CanHandle(event) {
			return handler
		}
	}
	return nil
}

// getStrategy 获取恢复策略
func (m *Manager) getStrategy(event *ErrorEvent) *RecoveryStrategy {
	// 优先使用组件特定策略
	if strategy, exists := m.config.ComponentStrategies[event.Component]; exists {
		return strategy
	}

	// 其次使用类型特定策略
	if strategy, exists := m.config.TypeStrategies[event.Type]; exists {
		return strategy
	}

	// 最后使用默认策略
	return m.config.DefaultStrategy
}

// calculateDelay 计算延迟时间
func (m *Manager) calculateDelay(strategy *RecoveryStrategy, attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}

	// 指数退避
	delay := time.Duration(float64(strategy.InitialDelay) *
		pow(strategy.BackoffFactor, float64(attempt-2)))

	// 限制最大延迟
	if delay > strategy.MaxDelay {
		delay = strategy.MaxDelay
	}

	// 添加随机抖动
	if strategy.Jitter {
		jitter := time.Duration(float64(delay) * 0.1 * (randomFloat() - 0.5))
		delay += jitter
	}

	return delay
}

// healthCheckLoop 健康检查循环
func (m *Manager) healthCheckLoop() {
	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	m.logger.Debug("启动健康检查循环")

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Debug("停止健康检查循环")
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// performHealthCheck 执行健康检查
func (m *Manager) performHealthCheck() {
	// 清理历史记录
	m.cleanupHistory()

	// 重置过期的熔断器
	m.resetExpiredCircuitBreakers()

	// 检查长时间运行的任务
	m.checkLongRunningTasks()
}

// cleanupHistory 清理历史记录
func (m *Manager) cleanupHistory() {
	m.historyMutex.Lock()
	defer m.historyMutex.Unlock()

	// 保留最近1000条记录
	if len(m.recoveryHistory) > 1000 {
		m.recoveryHistory = m.recoveryHistory[len(m.recoveryHistory)-1000:]
	}
}

// checkLongRunningTasks 检查长时间运行的任务
func (m *Manager) checkLongRunningTasks() {
	now := time.Now()
	timeout := m.config.TaskTimeout

	for eventID, task := range m.activeRecoveries {
		if now.Sub(task.StartTime) > timeout {
			m.logger.Warn("检测到长时间运行的恢复任务",
				zap.String("event_id", eventID),
				zap.Duration("running_time", now.Sub(task.StartTime)))
		}
	}
}

// addToHistory 添加到历史记录
func (m *Manager) addToHistory(attempt *RecoveryAttempt) {
	m.historyMutex.Lock()
	defer m.historyMutex.Unlock()

	m.recoveryHistory = append(m.recoveryHistory, attempt)
}

// GetActiveRecoveries 获取活跃恢复任务
func (m *Manager) GetActiveRecoveries() map[string]*RecoveryTask {
	result := make(map[string]*RecoveryTask)
	for id, task := range m.activeRecoveries {
		result[id] = task
	}
	return result
}

// GetRecoveryHistory 获取恢复历史
func (m *Manager) GetRecoveryHistory(limit int) []*RecoveryAttempt {
	m.historyMutex.RLock()
	defer m.historyMutex.RUnlock()

	if limit <= 0 || limit > len(m.recoveryHistory) {
		limit = len(m.recoveryHistory)
	}

	result := make([]*RecoveryAttempt, limit)
	startIdx := len(m.recoveryHistory) - limit
	copy(result, m.recoveryHistory[startIdx:])

	return result
}

// GetStats 获取管理器统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.historyMutex.RLock()
	historyCount := len(m.recoveryHistory)
	m.historyMutex.RUnlock()

	m.cbMutex.RLock()
	cbCount := len(m.circuitBreakers)
	m.cbMutex.RUnlock()

	return map[string]interface{}{
		"enabled":               m.config.Enabled,
		"total_events":          m.totalEvents.Load(),
		"successful_recoveries": m.successfulRecoveries.Load(),
		"failed_recoveries":     m.failedRecoveries.Load(),
		"active_recoveries":     len(m.activeRecoveries),
		"recovery_history":      historyCount,
		"circuit_breakers":      cbCount,
		"registered_handlers":   len(m.handlers),
		"config": map[string]interface{}{
			"max_concurrent_tasks":  m.config.MaxConcurrentTasks,
			"task_timeout":          m.config.TaskTimeout.String(),
			"health_check_interval": m.config.HealthCheckInterval.String(),
			"metrics_enabled":       m.config.MetricsEnabled,
		},
	}
}

// 辅助函数

// generateEventID 生成事件ID
func generateEventID() string {
	return fmt.Sprintf("evt_%d_%d", time.Now().UnixNano(), randomInt(1000, 9999))
}

// pow 计算幂
func pow(base, exp float64) float64 {
	if exp == 0 {
		return 1
	}
	result := base
	for i := 1; i < int(exp); i++ {
		result *= base
	}
	return result
}

// randomFloat 生成随机浮点数 [0,1)
func randomFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

// randomInt 生成指定范围的随机整数
func randomInt(min, max int) int {
	return min + int(time.Now().UnixNano()%(int64(max-min+1)))
}
