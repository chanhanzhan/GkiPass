package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// DiagnosticLevel 诊断级别
type DiagnosticLevel int

const (
	LevelInfo DiagnosticLevel = iota
	LevelWarn
	LevelError
	LevelCritical
)

// String 返回诊断级别名称
func (l DiagnosticLevel) String() string {
	switch l {
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// DiagnosticEvent 诊断事件
type DiagnosticEvent struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Level       DiagnosticLevel        `json:"level"`
	Category    string                 `json:"category"`    // 类别: network, performance, security, etc.
	Component   string                 `json:"component"`   // 组件名称
	Message     string                 `json:"message"`     // 事件消息
	Details     map[string]interface{} `json:"details"`     // 详细信息
	Metrics     map[string]float64     `json:"metrics"`     // 相关指标
	Tags        []string               `json:"tags"`        // 标签
	Correlation string                 `json:"correlation"` // 关联ID
}

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	// 系统指标
	CPUUsage       float64 `json:"cpu_usage"`       // CPU使用率
	MemoryUsage    float64 `json:"memory_usage"`    // 内存使用率
	GoroutineCount int     `json:"goroutine_count"` // 协程数量

	// 网络指标
	ConnectionCount int64   `json:"connection_count"` // 连接数
	NetworkLatency  float64 `json:"network_latency"`  // 网络延迟(ms)
	ThroughputIn    float64 `json:"throughput_in"`    // 入站吞吐量(Mbps)
	ThroughputOut   float64 `json:"throughput_out"`   // 出站吞吐量(Mbps)
	PacketLoss      float64 `json:"packet_loss"`      // 丢包率

	// 应用指标
	RequestsPerSecond float64 `json:"requests_per_second"` // 每秒请求数
	ErrorRate         float64 `json:"error_rate"`          // 错误率
	ResponseTime      float64 `json:"response_time"`       // 响应时间(ms)

	// 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// HealthStatus 健康状态
type HealthStatus struct {
	Status     string                     `json:"status"` // OK, DEGRADED, DOWN
	LastCheck  time.Time                  `json:"last_check"`
	Uptime     time.Duration              `json:"uptime"`
	Version    string                     `json:"version"`
	Components map[string]ComponentHealth `json:"components"`
	Metrics    PerformanceMetrics         `json:"metrics"`
	Issues     []DiagnosticEvent          `json:"issues"`
}

// ComponentHealth 组件健康状态
type ComponentHealth struct {
	Status    string                 `json:"status"`
	LastCheck time.Time              `json:"last_check"`
	Message   string                 `json:"message"`
	Metrics   map[string]interface{} `json:"metrics"`
}

// DiagnosticsConfig 诊断配置
type DiagnosticsConfig struct {
	Enabled         bool               `json:"enabled"`
	CollectInterval time.Duration      `json:"collect_interval"`
	RetentionPeriod time.Duration      `json:"retention_period"`
	MaxEvents       int                `json:"max_events"`
	HTTPPort        int                `json:"http_port"`
	EnableProfiling bool               `json:"enable_profiling"`
	EnableTracing   bool               `json:"enable_tracing"`
	AlertThresholds map[string]float64 `json:"alert_thresholds"`
}

// DefaultDiagnosticsConfig 默认诊断配置
func DefaultDiagnosticsConfig() *DiagnosticsConfig {
	return &DiagnosticsConfig{
		Enabled:         true,
		CollectInterval: 10 * time.Second,
		RetentionPeriod: 24 * time.Hour,
		MaxEvents:       10000,
		HTTPPort:        8088,
		EnableProfiling: true,
		EnableTracing:   false,
		AlertThresholds: map[string]float64{
			"cpu_usage":     80.0,
			"memory_usage":  85.0,
			"error_rate":    5.0,
			"response_time": 1000.0,
			"packet_loss":   1.0,
		},
	}
}

// Manager 诊断管理器
type Manager struct {
	config    *DiagnosticsConfig
	startTime time.Time

	// 事件存储
	events    []DiagnosticEvent
	eventsMux sync.RWMutex

	// 指标收集
	metrics     PerformanceMetrics
	metricsMux  sync.RWMutex
	lastCollect time.Time

	// 健康检查
	healthCheckers map[string]HealthChecker

	// 统计
	totalEvents     atomic.Int64
	alertsTriggered atomic.Int64

	// HTTP服务器
	httpServer *http.Server

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// HealthChecker 健康检查器接口
type HealthChecker interface {
	CheckHealth(ctx context.Context) ComponentHealth
	GetName() string
}

// NewManager 创建诊断管理器
func NewManager(cfg *DiagnosticsConfig) *Manager {
	if cfg == nil {
		cfg = DefaultDiagnosticsConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		config:         cfg,
		startTime:      time.Now(),
		events:         make([]DiagnosticEvent, 0),
		healthCheckers: make(map[string]HealthChecker),
		ctx:            ctx,
		cancel:         cancel,
		logger:         zap.L().Named("diagnostics"),
	}
}

// Start 启动诊断管理器
func (m *Manager) Start() error {
	if !m.config.Enabled {
		return nil
	}

	m.logger.Info("启动诊断管理器",
		zap.Duration("collect_interval", m.config.CollectInterval),
		zap.Duration("retention_period", m.config.RetentionPeriod),
		zap.Int("http_port", m.config.HTTPPort))

	// 启动HTTP服务器
	if err := m.startHTTPServer(); err != nil {
		return fmt.Errorf("启动HTTP服务器失败: %w", err)
	}

	// 启动指标收集
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.metricsCollectionLoop()
	}()

	// 启动健康检查
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.healthCheckLoop()
	}()

	// 启动清理任务
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.cleanupLoop()
	}()

	return nil
}

// Stop 停止诊断管理器
func (m *Manager) Stop() error {
	if !m.config.Enabled {
		return nil
	}

	m.logger.Info("停止诊断管理器")

	// 停止HTTP服务器
	if m.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.httpServer.Shutdown(shutdownCtx)
	}

	// 取消上下文
	if m.cancel != nil {
		m.cancel()
	}

	// 等待协程结束
	m.wg.Wait()

	m.logger.Info("诊断管理器已停止")
	return nil
}

// startHTTPServer 启动HTTP服务器
func (m *Manager) startHTTPServer() error {
	mux := http.NewServeMux()

	// 健康检查端点
	mux.HandleFunc("/health", m.handleHealth)
	mux.HandleFunc("/metrics", m.handleMetrics)
	mux.HandleFunc("/events", m.handleEvents)
	mux.HandleFunc("/diagnostics", m.handleDiagnostics)

	// 如果启用了性能分析
	if m.config.EnableProfiling {
		mux.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, r *http.Request) {
			http.DefaultServeMux.ServeHTTP(w, r)
		})
	}

	addr := fmt.Sprintf(":%d", m.config.HTTPPort)
	m.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("HTTP服务器启动失败", zap.Error(err))
		}
	}()

	m.logger.Info("诊断HTTP服务器已启动", zap.String("addr", addr))
	return nil
}

// metricsCollectionLoop 指标收集循环
func (m *Manager) metricsCollectionLoop() {
	ticker := time.NewTicker(m.config.CollectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.collectMetrics()
		}
	}
}

// collectMetrics 收集性能指标
func (m *Manager) collectMetrics() {
	m.metricsMux.Lock()
	defer m.metricsMux.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 更新指标
	m.metrics = PerformanceMetrics{
		CPUUsage:       m.getCPUUsage(),
		MemoryUsage:    float64(memStats.Sys) / (1024 * 1024 * 1024), // GB
		GoroutineCount: runtime.NumGoroutine(),
		Timestamp:      time.Now(),
	}

	m.lastCollect = time.Now()

	// 检查阈值并触发告警
	m.checkThresholds()
}

// getCPUUsage 获取CPU使用率（简化版本）
func (m *Manager) getCPUUsage() float64 {
	// 这里应该实现实际的CPU使用率计算
	// 为了简化，返回一个示例值
	return 10.0 + float64(runtime.NumGoroutine())/100.0
}

// checkThresholds 检查阈值
func (m *Manager) checkThresholds() {
	for metric, threshold := range m.config.AlertThresholds {
		var value float64
		var ok bool

		switch metric {
		case "cpu_usage":
			value, ok = m.metrics.CPUUsage, true
		case "memory_usage":
			value, ok = m.metrics.MemoryUsage, true
		case "error_rate":
			value, ok = m.metrics.ErrorRate, true
		case "response_time":
			value, ok = m.metrics.ResponseTime, true
		case "packet_loss":
			value, ok = m.metrics.PacketLoss, true
		}

		if ok && value > threshold {
			m.triggerAlert(metric, value, threshold)
		}
	}
}

// triggerAlert 触发告警
func (m *Manager) triggerAlert(metric string, value, threshold float64) {
	m.alertsTriggered.Add(1)

	event := DiagnosticEvent{
		ID:        fmt.Sprintf("alert_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		Level:     LevelWarn,
		Category:  "performance",
		Component: "diagnostics",
		Message:   fmt.Sprintf("指标 %s 超过阈值", metric),
		Details: map[string]interface{}{
			"metric":    metric,
			"value":     value,
			"threshold": threshold,
		},
		Tags: []string{"alert", "threshold"},
	}

	m.RecordEvent(event)
	m.logger.Warn("性能告警触发",
		zap.String("metric", metric),
		zap.Float64("value", value),
		zap.Float64("threshold", threshold))
}

// healthCheckLoop 健康检查循环
func (m *Manager) healthCheckLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.performHealthChecks()
		}
	}
}

// performHealthChecks 执行健康检查
func (m *Manager) performHealthChecks() {
	for name, checker := range m.healthCheckers {
		health := checker.CheckHealth(m.ctx)
		if health.Status != "OK" {
			m.logger.Warn("组件健康检查失败",
				zap.String("component", name),
				zap.String("status", health.Status),
				zap.String("message", health.Message))
		}
	}
}

// cleanupLoop 清理循环
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupOldEvents()
		}
	}
}

// cleanupOldEvents 清理旧事件
func (m *Manager) cleanupOldEvents() {
	m.eventsMux.Lock()
	defer m.eventsMux.Unlock()

	cutoff := time.Now().Add(-m.config.RetentionPeriod)
	newEvents := make([]DiagnosticEvent, 0)

	for _, event := range m.events {
		if event.Timestamp.After(cutoff) {
			newEvents = append(newEvents, event)
		}
	}

	// 限制最大事件数
	if len(newEvents) > m.config.MaxEvents {
		newEvents = newEvents[len(newEvents)-m.config.MaxEvents:]
	}

	m.events = newEvents
	m.logger.Debug("清理旧诊断事件", zap.Int("remaining", len(m.events)))
}

// RecordEvent 记录诊断事件
func (m *Manager) RecordEvent(event DiagnosticEvent) {
	m.eventsMux.Lock()
	defer m.eventsMux.Unlock()

	m.events = append(m.events, event)
	m.totalEvents.Add(1)

	// 如果事件过多，移除最旧的
	if len(m.events) > m.config.MaxEvents {
		m.events = m.events[1:]
	}

	m.logger.Debug("记录诊断事件",
		zap.String("id", event.ID),
		zap.String("level", event.Level.String()),
		zap.String("category", event.Category),
		zap.String("message", event.Message))
}

// RegisterHealthChecker 注册健康检查器
func (m *Manager) RegisterHealthChecker(checker HealthChecker) {
	m.healthCheckers[checker.GetName()] = checker
	m.logger.Info("注册健康检查器", zap.String("name", checker.GetName()))
}

// GetHealthStatus 获取健康状态
func (m *Manager) GetHealthStatus() HealthStatus {
	m.metricsMux.RLock()
	metrics := m.metrics
	m.metricsMux.RUnlock()

	m.eventsMux.RLock()
	recentIssues := make([]DiagnosticEvent, 0)
	cutoff := time.Now().Add(-1 * time.Hour)
	for _, event := range m.events {
		if event.Timestamp.After(cutoff) && event.Level >= LevelWarn {
			recentIssues = append(recentIssues, event)
		}
	}
	m.eventsMux.RUnlock()

	// 确定整体状态
	status := "OK"
	if len(recentIssues) > 0 {
		status = "DEGRADED"
		for _, issue := range recentIssues {
			if issue.Level >= LevelCritical {
				status = "DOWN"
				break
			}
		}
	}

	// 收集组件健康状态
	components := make(map[string]ComponentHealth)
	for name, checker := range m.healthCheckers {
		components[name] = checker.CheckHealth(m.ctx)
	}

	return HealthStatus{
		Status:     status,
		LastCheck:  time.Now(),
		Uptime:     time.Since(m.startTime),
		Version:    "1.0.0", // TODO: 从配置获取
		Components: components,
		Metrics:    metrics,
		Issues:     recentIssues,
	}
}

// HTTP处理器

// handleHealth 健康检查端点
func (m *Manager) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := m.GetHealthStatus()

	w.Header().Set("Content-Type", "application/json")
	if health.Status != "OK" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(health)
}

// handleMetrics 指标端点
func (m *Manager) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m.metricsMux.RLock()
	metrics := m.metrics
	m.metricsMux.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// handleEvents 事件端点
func (m *Manager) handleEvents(w http.ResponseWriter, r *http.Request) {
	m.eventsMux.RLock()
	events := make([]DiagnosticEvent, len(m.events))
	copy(events, m.events)
	m.eventsMux.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// handleDiagnostics 综合诊断端点
func (m *Manager) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"health": m.GetHealthStatus(),
		"stats": map[string]interface{}{
			"total_events":     m.totalEvents.Load(),
			"alerts_triggered": m.alertsTriggered.Load(),
			"uptime":           time.Since(m.startTime).Seconds(),
		},
		"config": m.config,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// GetStats 获取统计信息
func (m *Manager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":          m.config.Enabled,
		"total_events":     m.totalEvents.Load(),
		"alerts_triggered": m.alertsTriggered.Load(),
		"uptime":           time.Since(m.startTime).Seconds(),
		"last_collect":     m.lastCollect,
		"health_checkers":  len(m.healthCheckers),
		"config": map[string]interface{}{
			"collect_interval": m.config.CollectInterval.String(),
			"retention_period": m.config.RetentionPeriod.String(),
			"max_events":       m.config.MaxEvents,
			"http_port":        m.config.HTTPPort,
		},
	}
}
