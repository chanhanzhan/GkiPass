package healthcheck

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"gkipass/client/internal/common"
	"gkipass/client/internal/protocol"
)

// HealthStatus 健康状态
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"   // 健康
	HealthStatusDegraded  HealthStatus = "degraded"  // 降级
	HealthStatusUnhealthy HealthStatus = "unhealthy" // 不健康
	HealthStatusUnknown   HealthStatus = "unknown"   // 未知
)

// HealthComponent 组件健康状态
type HealthComponent struct {
	Name        string                 `json:"name"`
	Status      HealthStatus           `json:"status"`
	Description string                 `json:"description,omitempty"`
	LastChecked time.Time              `json:"last_checked"`
	Latency     time.Duration          `json:"latency,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// HealthReport 健康报告
type HealthReport struct {
	Status       HealthStatus               `json:"status"`
	Components   map[string]HealthComponent `json:"components"`
	StartupTime  time.Time                  `json:"startup_time"`
	Uptime       time.Duration              `json:"uptime"`
	LastReported time.Time                  `json:"last_reported"`
	Details      map[string]interface{}     `json:"details,omitempty"`
}

// HealthChecker 健康检查配置
type HealthCheckerConfig struct {
	CheckInterval      time.Duration   // 检查间隔
	ReportInterval     time.Duration   // 上报间隔
	ConnectionTimeout  time.Duration   // 连接超时
	UnhealthyThreshold int             // 不健康阈值
	RecoveryThreshold  int             // 恢复阈值
	Components         []string        // 要检查的组件
	DetailedReport     bool            // 是否生成详细报告
	EnabledChecks      map[string]bool // 启用的检查项
	Logger             *zap.Logger     // 日志记录器
}

// DefaultHealthCheckerConfig 默认健康检查配置
func DefaultHealthCheckerConfig() *HealthCheckerConfig {
	return &HealthCheckerConfig{
		CheckInterval:      30 * time.Second,
		ReportInterval:     5 * time.Minute,
		ConnectionTimeout:  5 * time.Second,
		UnhealthyThreshold: 3,
		RecoveryThreshold:  2,
		Components:         []string{"network", "api", "websocket", "system"},
		DetailedReport:     true,
		EnabledChecks:      map[string]bool{"all": true},
		Logger:             zap.L().Named("health-check"),
	}
}

// HealthChecker 健康检查器
type HealthChecker struct {
	config        *HealthCheckerConfig
	status        HealthStatus
	components    map[string]HealthComponent
	startupTime   time.Time
	lastReported  time.Time
	checkResults  map[string][]bool // 用于跟踪连续成功或失败的检查
	failureCount  map[string]int
	successCount  map[string]int
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	reportHandler func(*HealthReport)
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(config *HealthCheckerConfig) *HealthChecker {
	if config == nil {
		config = DefaultHealthCheckerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	checker := &HealthChecker{
		config:       config,
		status:       HealthStatusUnknown,
		components:   make(map[string]HealthComponent),
		startupTime:  time.Now(),
		lastReported: time.Time{},
		checkResults: make(map[string][]bool),
		failureCount: make(map[string]int),
		successCount: make(map[string]int),
		ctx:          ctx,
		cancel:       cancel,
	}

	// 初始化组件状态
	for _, component := range config.Components {
		checker.components[component] = HealthComponent{
			Name:        component,
			Status:      HealthStatusUnknown,
			LastChecked: time.Time{},
			Details:     make(map[string]interface{}),
		}
	}

	return checker
}

// Start 启动健康检查
func (hc *HealthChecker) Start() {
	// 健康检查循环
	go func() {
		checkTicker := time.NewTicker(hc.config.CheckInterval)
		reportTicker := time.NewTicker(hc.config.ReportInterval)
		defer checkTicker.Stop()
		defer reportTicker.Stop()

		for {
			select {
			case <-checkTicker.C:
				hc.performHealthCheck()

			case <-reportTicker.C:
				if hc.reportHandler != nil {
					report := hc.GetHealthReport()
					hc.reportHandler(report)
					hc.lastReported = time.Now()
				}

			case <-hc.ctx.Done():
				return
			}
		}
	}()
}

// Stop 停止健康检查
func (hc *HealthChecker) Stop() {
	hc.cancel()
}

// SetReportHandler 设置上报处理器
func (hc *HealthChecker) SetReportHandler(handler func(*HealthReport)) {
	hc.reportHandler = handler
}

// GetHealthReport 获取健康报告
func (hc *HealthChecker) GetHealthReport() *HealthReport {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	// 复制组件状态
	components := make(map[string]HealthComponent)
	for name, component := range hc.components {
		components[name] = component
	}

	// 计算系统运行时间
	uptime := time.Since(hc.startupTime)

	// 创建报告
	report := &HealthReport{
		Status:       hc.status,
		Components:   components,
		StartupTime:  hc.startupTime,
		Uptime:       uptime,
		LastReported: hc.lastReported,
		Details:      make(map[string]interface{}),
	}

	// 添加详细信息
	if hc.config.DetailedReport {
		report.Details["version"] = "1.0.0" // 替换为实际版本
		report.Details["go_version"] = "1.18"
		report.Details["os"] = "linux"
		report.Details["arch"] = "amd64"
	}

	return report
}

// performHealthCheck 执行健康检查
func (hc *HealthChecker) performHealthCheck() {
	// 检查各个组件
	for component := range hc.components {
		if !hc.isCheckEnabled(component) {
			continue
		}

		var (
			status      HealthStatus
			description string
			details     map[string]interface{}
			latency     time.Duration
			err         error
		)

		// 根据组件类型执行不同的检查
		switch component {
		case "network":
			status, description, details, latency, err = hc.checkNetwork()
		case "api":
			status, description, details, latency, err = hc.checkAPI()
		case "websocket":
			status, description, details, latency, err = hc.checkWebSocket()
		case "system":
			status, description, details, latency, err = hc.checkSystem()
		default:
			status = HealthStatusUnknown
			description = fmt.Sprintf("未知的组件: %s", component)
		}

		if err != nil {
			hc.config.Logger.Warn("健康检查失败",
				zap.String("component", component),
				zap.Error(err))
		}

		// 更新组件状态
		hc.updateComponentStatus(component, status, description, details, latency)
	}

	// 更新整体状态
	hc.updateOverallStatus()
}

// updateComponentStatus 更新组件状态
func (hc *HealthChecker) updateComponentStatus(component string, status HealthStatus, description string, details map[string]interface{}, latency time.Duration) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	// 更新检查结果
	isHealthy := status == HealthStatusHealthy

	// 初始化检查结果数组
	if _, exists := hc.checkResults[component]; !exists {
		hc.checkResults[component] = make([]bool, 0, hc.config.UnhealthyThreshold)
	}

	// 添加最新的检查结果
	hc.checkResults[component] = append(hc.checkResults[component], isHealthy)

	// 保持结果数组的长度
	maxHistoryLength := max(hc.config.UnhealthyThreshold, hc.config.RecoveryThreshold)
	if len(hc.checkResults[component]) > maxHistoryLength {
		hc.checkResults[component] = hc.checkResults[component][len(hc.checkResults[component])-maxHistoryLength:]
	}

	// 统计连续的失败和成功次数
	failureCount := 0
	successCount := 0
	for i := len(hc.checkResults[component]) - 1; i >= 0; i-- {
		result := hc.checkResults[component][i]
		if result {
			successCount++
			failureCount = 0
		} else {
			failureCount++
			successCount = 0
		}
	}

	hc.failureCount[component] = failureCount
	hc.successCount[component] = successCount

	// 根据阈值更新组件状态
	currentComp := hc.components[component]

	// 如果当前是未知状态，直接使用当前检查结果
	if currentComp.Status == HealthStatusUnknown {
		currentComp.Status = status
	} else if failureCount >= hc.config.UnhealthyThreshold {
		// 连续失败达到阈值时标记为不健康
		currentComp.Status = HealthStatusUnhealthy
	} else if successCount >= hc.config.RecoveryThreshold && currentComp.Status == HealthStatusUnhealthy {
		// 从不健康状态恢复
		currentComp.Status = HealthStatusHealthy
	} else if isHealthy && currentComp.Status != HealthStatusUnhealthy {
		// 正常情况下保持健康状态
		currentComp.Status = HealthStatusHealthy
	} else if !isHealthy && currentComp.Status == HealthStatusHealthy {
		// 单次故障降级，但不立即标记为不健康
		currentComp.Status = HealthStatusDegraded
	}

	// 更新组件信息
	currentComp.Description = description
	currentComp.LastChecked = time.Now()
	currentComp.Latency = latency

	// 更新详情
	if details != nil {
		for k, v := range details {
			currentComp.Details[k] = v
		}
	}

	hc.components[component] = currentComp
}

// updateOverallStatus 更新整体状态
func (hc *HealthChecker) updateOverallStatus() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	unhealthyCount := 0
	degradedCount := 0
	unknownCount := 0
	totalCount := 0

	for _, component := range hc.components {
		totalCount++
		switch component.Status {
		case HealthStatusUnhealthy:
			unhealthyCount++
		case HealthStatusDegraded:
			degradedCount++
		case HealthStatusUnknown:
			unknownCount++
		}
	}

	// 确定整体状态
	if unhealthyCount > 0 {
		hc.status = HealthStatusUnhealthy
	} else if degradedCount > 0 {
		hc.status = HealthStatusDegraded
	} else if unknownCount == totalCount {
		hc.status = HealthStatusUnknown
	} else {
		hc.status = HealthStatusHealthy
	}
}

// checkNetwork 检查网络
func (hc *HealthChecker) checkNetwork() (HealthStatus, string, map[string]interface{}, time.Duration, error) {
	details := make(map[string]interface{})
	start := time.Now()

	// 检查面板服务器连接
	serverAddr := "api.example.com" // 替换为实际的服务器地址
	serverPort := 443

	// 测量连接延迟
	isOpen := common.Network.IsPortOpen(serverAddr, serverPort, hc.config.ConnectionTimeout)
	latency := time.Since(start)

	// 记录结果
	details["server_reachable"] = isOpen
	details["latency_ms"] = latency.Milliseconds()

	var status HealthStatus
	var description string

	if isOpen {
		status = HealthStatusHealthy
		description = fmt.Sprintf("网络连接正常，延迟: %s", latency)
	} else {
		status = HealthStatusUnhealthy
		description = fmt.Sprintf("无法连接到服务器 %s:%d", serverAddr, serverPort)
		return status, description, details, latency, fmt.Errorf("服务器连接失败")
	}

	return status, description, details, latency, nil
}

// checkAPI 检查API连接
func (hc *HealthChecker) checkAPI() (HealthStatus, string, map[string]interface{}, time.Duration, error) {
	details := make(map[string]interface{})
	start := time.Now()

	// 模拟API健康检查
	// 实际应用中，应该调用API端点
	isHealthy := true
	latency := time.Since(start)

	details["api_version"] = "v1"
	details["latency_ms"] = latency.Milliseconds()

	var status HealthStatus
	var description string

	if isHealthy {
		status = HealthStatusHealthy
		description = fmt.Sprintf("API连接正常，延迟: %s", latency)
	} else {
		status = HealthStatusUnhealthy
		description = "API连接异常"
		return status, description, details, latency, fmt.Errorf("API连接失败")
	}

	return status, description, details, latency, nil
}

// checkWebSocket 检查WebSocket连接
func (hc *HealthChecker) checkWebSocket() (HealthStatus, string, map[string]interface{}, time.Duration, error) {
	details := make(map[string]interface{})
	start := time.Now()

	// 模拟WebSocket健康检查
	// 实际应用中，应该检查WebSocket连接状态
	isConnected := true
	latency := time.Since(start)

	details["connected"] = isConnected
	details["latency_ms"] = latency.Milliseconds()

	var status HealthStatus
	var description string

	if isConnected {
		status = HealthStatusHealthy
		description = fmt.Sprintf("WebSocket连接正常，延迟: %s", latency)
	} else {
		status = HealthStatusUnhealthy
		description = "WebSocket连接断开"
		return status, description, details, latency, fmt.Errorf("WebSocket连接断开")
	}

	return status, description, details, latency, nil
}

// checkSystem 检查系统状态
func (hc *HealthChecker) checkSystem() (HealthStatus, string, map[string]interface{}, time.Duration, error) {
	details := make(map[string]interface{})
	start := time.Now()

	// 获取系统状态
	memStats := common.Runtime.GetMemStats()

	// 收集系统指标
	details["heap_alloc_mb"] = float64(memStats.HeapAlloc) / 1024 / 1024
	details["heap_sys_mb"] = float64(memStats.HeapSys) / 1024 / 1024
	details["heap_objects"] = memStats.HeapObjects
	details["num_gc"] = memStats.NumGC

	// 简单的阈值判断
	isHealthy := memStats.HeapAlloc < memStats.HeapSys*80/100 // 堆使用率低于80%
	latency := time.Since(start)

	var status HealthStatus
	var description string

	if isHealthy {
		status = HealthStatusHealthy
		description = "系统运行正常"
	} else {
		status = HealthStatusDegraded
		description = "系统资源使用率较高"
	}

	return status, description, details, latency, nil
}

// isCheckEnabled 检查是否启用了指定的检查项
func (hc *HealthChecker) isCheckEnabled(component string) bool {
	if enabled, exists := hc.config.EnabledChecks[component]; exists {
		return enabled
	}
	return hc.config.EnabledChecks["all"]
}

// Helper function for min/max until Go 1.21
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// HealthReporter 健康报告上传接口
type HealthReporter interface {
	ReportHealth(report *HealthReport) error
}

// WebSocketHealthReporter 基于WebSocket的健康报告上传器
type WebSocketHealthReporter struct {
	wsClient    interface{} // WebSocket客户端，应该实现发送消息的能力
	sendMessage func(interface{}, interface{}) error
}

// NewWebSocketHealthReporter 创建WebSocket健康报告上传器
func NewWebSocketHealthReporter(wsClient interface{}, sendFunc func(interface{}, interface{}) error) *WebSocketHealthReporter {
	return &WebSocketHealthReporter{
		wsClient:    wsClient,
		sendMessage: sendFunc,
	}
}

// ReportHealth 上报健康状态
func (r *WebSocketHealthReporter) ReportHealth(report *HealthReport) error {
	// 转换为适合WebSocket传输的格式
	data := map[string]interface{}{
		"status":        string(report.Status),
		"uptime":        report.Uptime.Seconds(),
		"startup_time":  report.StartupTime.Unix(),
		"last_reported": report.LastReported.Unix(),
		"components":    report.Components,
	}

	// 发送健康报告
	return r.sendMessage(protocol.MessageTypeStatusReport, data)
}
