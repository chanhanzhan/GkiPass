package common

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// BaseComponent 基础组件
type BaseComponent struct {
	name    string
	version string
	logger  *zap.Logger
	config  interface{}

	// 状态管理
	state     atomic.Int32 // 0: stopped, 1: starting, 2: running, 3: stopping
	startTime time.Time

	// 统计信息
	stats struct {
		startCount    atomic.Int64
		stopCount     atomic.Int64
		errorCount    atomic.Int64
		lastError     atomic.Value // error
		lastErrorTime atomic.Int64 // unix timestamp
	}

	// 生命周期钩子
	hooks struct {
		beforeStart []func() error
		afterStart  []func() error
		beforeStop  []func() error
		afterStop   []func() error
	}

	// 并发控制
	mutex sync.RWMutex

	// 健康检查
	healthChecker   HealthChecker
	lastHealthCheck atomic.Int64
	healthScore     atomic.Value // float64
}

// ComponentState 组件状态
type ComponentState int32

const (
	StateStopped ComponentState = iota
	StateStarting
	StateRunning
	StateStopping
)

func (s ComponentState) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// NewBaseComponent 创建基础组件
func NewBaseComponent(name, version string, logger *zap.Logger) *BaseComponent {
	if logger == nil {
		logger = zap.NewNop()
	}

	bc := &BaseComponent{
		name:    name,
		version: version,
		logger:  logger.Named(name),
	}

	bc.healthScore.Store(1.0)
	return bc
}

// GetName 获取组件名称
func (bc *BaseComponent) GetName() string {
	return bc.name
}

// GetVersion 获取组件版本
func (bc *BaseComponent) GetVersion() string {
	return bc.version
}

// GetState 获取组件状态
func (bc *BaseComponent) GetState() ComponentState {
	return ComponentState(bc.state.Load())
}

// IsRunning 检查组件是否正在运行
func (bc *BaseComponent) IsRunning() bool {
	return bc.GetState() == StateRunning
}

// IsHealthy 检查组件是否健康
func (bc *BaseComponent) IsHealthy() bool {
	if bc.healthChecker != nil {
		return bc.healthChecker.IsHealthy()
	}

	// 默认健康检查：组件正在运行且无最近错误
	if !bc.IsRunning() {
		return false
	}

	lastErrorTime := bc.stats.lastErrorTime.Load()
	if lastErrorTime > 0 {
		// 如果最近5分钟内有错误，认为不健康
		if time.Since(time.Unix(lastErrorTime, 0)) < 5*time.Minute {
			return false
		}
	}

	return true
}

// GetHealthScore 获取健康分数
func (bc *BaseComponent) GetHealthScore() float64 {
	if score := bc.healthScore.Load(); score != nil {
		return score.(float64)
	}
	return 0.0
}

// SetHealthChecker 设置健康检查器
func (bc *BaseComponent) SetHealthChecker(checker HealthChecker) {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()
	bc.healthChecker = checker
}

// SetConfig 设置配置
func (bc *BaseComponent) SetConfig(config interface{}) {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()
	bc.config = config
}

// GetConfig 获取配置
func (bc *BaseComponent) GetConfig() interface{} {
	bc.mutex.RLock()
	defer bc.mutex.RUnlock()
	return bc.config
}

// AddStartHook 添加启动前钩子
func (bc *BaseComponent) AddBeforeStartHook(hook func() error) {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()
	bc.hooks.beforeStart = append(bc.hooks.beforeStart, hook)
}

// AddAfterStartHook 添加启动后钩子
func (bc *BaseComponent) AddAfterStartHook(hook func() error) {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()
	bc.hooks.afterStart = append(bc.hooks.afterStart, hook)
}

// AddBeforeStopHook 添加停止前钩子
func (bc *BaseComponent) AddBeforeStopHook(hook func() error) {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()
	bc.hooks.beforeStop = append(bc.hooks.beforeStop, hook)
}

// AddAfterStopHook 添加停止后钩子
func (bc *BaseComponent) AddAfterStopHook(hook func() error) {
	bc.mutex.Lock()
	defer bc.mutex.Unlock()
	bc.hooks.afterStop = append(bc.hooks.afterStop, hook)
}

// Start 启动组件
func (bc *BaseComponent) Start() error {
	// 检查状态
	if !bc.state.CompareAndSwap(int32(StateStopped), int32(StateStarting)) {
		return fmt.Errorf("组件 %s 已经启动或正在启动", bc.name)
	}

	bc.logger.Info("启动组件", zap.String("version", bc.version))

	// 执行启动前钩子
	if err := bc.executeHooks(bc.hooks.beforeStart); err != nil {
		bc.state.Store(int32(StateStopped))
		bc.recordError(err)
		return fmt.Errorf("执行启动前钩子失败: %w", err)
	}

	// 记录启动时间
	bc.startTime = time.Now()

	// 设置为运行状态
	bc.state.Store(int32(StateRunning))
	bc.stats.startCount.Add(1)

	// 执行启动后钩子
	if err := bc.executeHooks(bc.hooks.afterStart); err != nil {
		bc.logger.Warn("执行启动后钩子失败", zap.Error(err))
		// 不因为后置钩子失败而停止组件
	}

	bc.logger.Info("组件启动完成",
		zap.Duration("startup_time", time.Since(bc.startTime)))

	return nil
}

// Stop 停止组件
func (bc *BaseComponent) Stop() error {
	// 检查状态
	currentState := bc.GetState()
	if currentState == StateStopped {
		return nil
	}

	if !bc.state.CompareAndSwap(int32(currentState), int32(StateStopping)) {
		return fmt.Errorf("组件 %s 状态异常，无法停止", bc.name)
	}

	bc.logger.Info("停止组件")

	// 执行停止前钩子
	if err := bc.executeHooks(bc.hooks.beforeStop); err != nil {
		bc.logger.Warn("执行停止前钩子失败", zap.Error(err))
		// 继续停止流程
	}

	// 设置为停止状态
	bc.state.Store(int32(StateStopped))
	bc.stats.stopCount.Add(1)

	// 执行停止后钩子
	if err := bc.executeHooks(bc.hooks.afterStop); err != nil {
		bc.logger.Warn("执行停止后钩子失败", zap.Error(err))
	}

	uptime := time.Since(bc.startTime)
	bc.logger.Info("组件停止完成",
		zap.Duration("uptime", uptime))

	return nil
}

// executeHooks 执行钩子函数
func (bc *BaseComponent) executeHooks(hooks []func() error) error {
	for i, hook := range hooks {
		if err := hook(); err != nil {
			return fmt.Errorf("钩子函数 %d 执行失败: %w", i, err)
		}
	}
	return nil
}

// recordError 记录错误
func (bc *BaseComponent) recordError(err error) {
	if err != nil {
		bc.stats.errorCount.Add(1)
		bc.stats.lastError.Store(err)
		bc.stats.lastErrorTime.Store(time.Now().Unix())

		// 更新健康分数
		currentScore := bc.GetHealthScore()
		newScore := currentScore * 0.9 // 每次错误降低10%
		if newScore < 0.1 {
			newScore = 0.1
		}
		bc.healthScore.Store(newScore)

		bc.logger.Error("组件错误", zap.Error(err))
	}
}

// RecoverHealth 恢复健康状态
func (bc *BaseComponent) RecoverHealth() {
	currentScore := bc.GetHealthScore()
	newScore := currentScore + 0.1 // 每次恢复增加10%
	if newScore > 1.0 {
		newScore = 1.0
	}
	bc.healthScore.Store(newScore)
}

// GetStats 获取统计信息
func (bc *BaseComponent) GetStats() map[string]interface{} {
	var lastError string
	if err := bc.stats.lastError.Load(); err != nil {
		lastError = err.(error).Error()
	}

	var uptime time.Duration
	if bc.IsRunning() {
		uptime = time.Since(bc.startTime)
	}

	return map[string]interface{}{
		"name":            bc.name,
		"version":         bc.version,
		"state":           bc.GetState().String(),
		"is_running":      bc.IsRunning(),
		"is_healthy":      bc.IsHealthy(),
		"health_score":    bc.GetHealthScore(),
		"start_count":     bc.stats.startCount.Load(),
		"stop_count":      bc.stats.stopCount.Load(),
		"error_count":     bc.stats.errorCount.Load(),
		"last_error":      lastError,
		"last_error_time": time.Unix(bc.stats.lastErrorTime.Load(), 0),
		"start_time":      bc.startTime,
		"uptime":          uptime,
		"uptime_seconds":  uptime.Seconds(),
	}
}

// BaseManager 基础管理器
type BaseManager struct {
	*BaseComponent

	// 子组件管理
	components map[string]Component
	compMutex  sync.RWMutex

	// 上下文管理
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 监控
	monitorInterval time.Duration
	enableMonitor   bool
}

// NewBaseManager 创建基础管理器
func NewBaseManager(name, version string, logger *zap.Logger) *BaseManager {
	bm := &BaseManager{
		BaseComponent:   NewBaseComponent(name, version, logger),
		components:      make(map[string]Component),
		monitorInterval: 30 * time.Second,
		enableMonitor:   true,
	}

	// 添加默认的启动停止钩子
	bm.AddAfterStartHook(bm.startMonitoring)
	bm.AddBeforeStopHook(bm.stopMonitoring)

	return bm
}

// RegisterComponent 注册子组件
func (bm *BaseManager) RegisterComponent(name string, component Component) error {
	bm.compMutex.Lock()
	defer bm.compMutex.Unlock()

	if _, exists := bm.components[name]; exists {
		return fmt.Errorf("组件 %s 已存在", name)
	}

	bm.components[name] = component
	bm.logger.Debug("注册组件", zap.String("component", name))
	return nil
}

// UnregisterComponent 注销子组件
func (bm *BaseManager) UnregisterComponent(name string) error {
	bm.compMutex.Lock()
	defer bm.compMutex.Unlock()

	component, exists := bm.components[name]
	if !exists {
		return fmt.Errorf("组件 %s 不存在", name)
	}

	// 停止组件
	if err := component.Stop(); err != nil {
		bm.logger.Warn("停止组件失败", zap.String("component", name), zap.Error(err))
	}

	delete(bm.components, name)
	bm.logger.Debug("注销组件", zap.String("component", name))
	return nil
}

// GetComponent 获取子组件
func (bm *BaseManager) GetComponent(name string) (Component, bool) {
	bm.compMutex.RLock()
	defer bm.compMutex.RUnlock()

	component, exists := bm.components[name]
	return component, exists
}

// ListComponents 列出所有子组件
func (bm *BaseManager) ListComponents() []string {
	bm.compMutex.RLock()
	defer bm.compMutex.RUnlock()

	names := make([]string, 0, len(bm.components))
	for name := range bm.components {
		names = append(names, name)
	}
	return names
}

// StartAll 启动所有子组件
func (bm *BaseManager) StartAll() error {
	bm.compMutex.RLock()
	components := make([]Component, 0, len(bm.components))
	for _, comp := range bm.components {
		components = append(components, comp)
	}
	bm.compMutex.RUnlock()

	for _, comp := range components {
		if err := comp.Start(); err != nil {
			bm.logger.Error("启动子组件失败",
				zap.String("component", comp.GetName()),
				zap.Error(err))
			return err
		}
	}

	return nil
}

// StopAll 停止所有子组件
func (bm *BaseManager) StopAll() error {
	bm.compMutex.RLock()
	components := make([]Component, 0, len(bm.components))
	for _, comp := range bm.components {
		components = append(components, comp)
	}
	bm.compMutex.RUnlock()

	// 反向停止
	for i := len(components) - 1; i >= 0; i-- {
		comp := components[i]
		if err := comp.Stop(); err != nil {
			bm.logger.Error("停止子组件失败",
				zap.String("component", comp.GetName()),
				zap.Error(err))
		}
	}

	return nil
}

// SetMonitorInterval 设置监控间隔
func (bm *BaseManager) SetMonitorInterval(interval time.Duration) {
	bm.monitorInterval = interval
}

// EnableMonitoring 启用监控
func (bm *BaseManager) EnableMonitoring(enabled bool) {
	bm.enableMonitor = enabled
}

// startMonitoring 启动监控
func (bm *BaseManager) startMonitoring() error {
	if !bm.enableMonitor {
		return nil
	}

	bm.ctx, bm.cancel = context.WithCancel(context.Background())

	bm.wg.Add(1)
	go bm.monitorLoop()

	bm.logger.Debug("启动监控", zap.Duration("interval", bm.monitorInterval))
	return nil
}

// stopMonitoring 停止监控
func (bm *BaseManager) stopMonitoring() error {
	if bm.cancel != nil {
		bm.cancel()
	}

	bm.wg.Wait()
	bm.logger.Debug("停止监控")
	return nil
}

// monitorLoop 监控循环
func (bm *BaseManager) monitorLoop() {
	defer bm.wg.Done()

	ticker := time.NewTicker(bm.monitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-bm.ctx.Done():
			return
		case <-ticker.C:
			bm.performHealthCheck()
		}
	}
}

// performHealthCheck 执行健康检查
func (bm *BaseManager) performHealthCheck() {
	bm.compMutex.RLock()
	components := make([]Component, 0, len(bm.components))
	for _, comp := range bm.components {
		components = append(components, comp)
	}
	bm.compMutex.RUnlock()

	healthyCount := 0
	totalCount := len(components)

	for _, comp := range components {
		if comp.IsHealthy() {
			healthyCount++
		} else {
			bm.logger.Warn("组件不健康",
				zap.String("component", comp.GetName()),
				zap.Float64("health_score", comp.GetHealthScore()))
		}
	}

	// 更新管理器健康分数
	if totalCount > 0 {
		managerScore := float64(healthyCount) / float64(totalCount)
		bm.healthScore.Store(managerScore)
	}

	bm.lastHealthCheck.Store(time.Now().Unix())
}

// GetStats 获取管理器统计信息
func (bm *BaseManager) GetStats() map[string]interface{} {
	stats := bm.BaseComponent.GetStats()

	// 添加子组件统计
	bm.compMutex.RLock()
	componentStats := make(map[string]interface{})
	for name, comp := range bm.components {
		componentStats[name] = comp.GetStats()
	}
	bm.compMutex.RUnlock()

	stats["components"] = componentStats
	stats["component_count"] = len(bm.components)
	stats["monitor_interval"] = bm.monitorInterval
	stats["monitor_enabled"] = bm.enableMonitor
	stats["last_health_check"] = time.Unix(bm.lastHealthCheck.Load(), 0)

	return stats
}

// EventDrivenComponent 事件驱动组件
type EventDrivenComponent struct {
	*BaseComponent

	eventBus EventBus
	handlers map[string][]EventHandler
	mutex    sync.RWMutex
}

// NewEventDrivenComponent 创建事件驱动组件
func NewEventDrivenComponent(name, version string, logger *zap.Logger, eventBus EventBus) *EventDrivenComponent {
	return &EventDrivenComponent{
		BaseComponent: NewBaseComponent(name, version, logger),
		eventBus:      eventBus,
		handlers:      make(map[string][]EventHandler),
	}
}

// Subscribe 订阅事件
func (edc *EventDrivenComponent) Subscribe(event string, handler EventHandler) error {
	edc.mutex.Lock()
	defer edc.mutex.Unlock()

	edc.handlers[event] = append(edc.handlers[event], handler)

	if edc.eventBus != nil {
		return edc.eventBus.Subscribe(event, handler)
	}

	return nil
}

// Publish 发布事件
func (edc *EventDrivenComponent) Publish(event string, data interface{}) error {
	if edc.eventBus != nil {
		return edc.eventBus.Publish(event, data)
	}

	// 本地处理
	edc.mutex.RLock()
	handlers := edc.handlers[event]
	edc.mutex.RUnlock()

	for _, handler := range handlers {
		if err := handler.Handle(data); err != nil {
			edc.recordError(err)
			return err
		}
	}

	return nil
}

// ConfigurableComponent 可配置组件
type ConfigurableComponent struct {
	*BaseComponent

	configValidator Validator
	configMutex     sync.RWMutex
}

// NewConfigurableComponent 创建可配置组件
func NewConfigurableComponent(name, version string, logger *zap.Logger) *ConfigurableComponent {
	return &ConfigurableComponent{
		BaseComponent: NewBaseComponent(name, version, logger),
	}
}

// SetConfigValidator 设置配置验证器
func (cc *ConfigurableComponent) SetConfigValidator(validator Validator) {
	cc.configMutex.Lock()
	defer cc.configMutex.Unlock()
	cc.configValidator = validator
}

// LoadConfig 加载配置
func (cc *ConfigurableComponent) LoadConfig(config interface{}) error {
	cc.configMutex.Lock()
	defer cc.configMutex.Unlock()

	// 验证配置
	if cc.configValidator != nil {
		if err := cc.configValidator.Validate(config); err != nil {
			return fmt.Errorf("配置验证失败: %w", err)
		}
	}

	cc.config = config
	cc.logger.Info("配置加载完成")
	return nil
}

// ValidateConfig 验证配置
func (cc *ConfigurableComponent) ValidateConfig() error {
	cc.configMutex.RLock()
	defer cc.configMutex.RUnlock()

	if cc.configValidator != nil && cc.config != nil {
		return cc.configValidator.Validate(cc.config)
	}

	return nil
}
