package performance

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"go.uber.org/zap"
)

// PerformanceAlert 性能告警
type PerformanceAlert struct {
	Type        string                 `json:"type"`
	Level       string                 `json:"level"`        // INFO, WARN, ERROR, CRITICAL
	Message     string                 `json:"message"`
	MetricName  string                 `json:"metric_name"`
	Value       float64                `json:"value"`
	Threshold   float64                `json:"threshold"`
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata"`
	Suggestions []string               `json:"suggestions"`
}

// MetricThreshold 指标阈值配置
type MetricThreshold struct {
	Name         string  `json:"name"`
	WarnLevel    float64 `json:"warn_level"`
	ErrorLevel   float64 `json:"error_level"`
	CriticalLevel float64 `json:"critical_level"`
	Unit         string  `json:"unit"`
	Description  string  `json:"description"`
}

// PerformanceSnapshot 性能快照
type PerformanceSnapshot struct {
	Timestamp time.Time `json:"timestamp"`
	
	// CPU指标
	CPUUsagePercent  float64 `json:"cpu_usage_percent"`
	CPUCores         int     `json:"cpu_cores"`
	LoadAverage      float64 `json:"load_average"`
	
	// 内存指标
	MemoryUsedBytes  uint64  `json:"memory_used_bytes"`
	MemoryTotalBytes uint64  `json:"memory_total_bytes"`
	MemoryUsedPercent float64 `json:"memory_used_percent"`
	
	// Goroutine指标
	GoroutineCount   int     `json:"goroutine_count"`
	ThreadCount      int     `json:"thread_count"`
	
	// GC指标
	GCPauseNs        uint64  `json:"gc_pause_ns"`
	GCPausePercent   float64 `json:"gc_pause_percent"`
	GCCycles         uint32  `json:"gc_cycles"`
	GCForcedRuns     uint32  `json:"gc_forced_runs"`
	
	// 网络指标
	NetworkBytesRecv uint64  `json:"network_bytes_recv"`
	NetworkBytesSent uint64  `json:"network_bytes_sent"`
	NetworkPacketsRecv uint64 `json:"network_packets_recv"`
	NetworkPacketsSent uint64 `json:"network_packets_sent"`
	
	// 应用指标
	ConnectionCount  int64   `json:"connection_count"`
	RequestRate      float64 `json:"request_rate"`
	ErrorRate        float64 `json:"error_rate"`
	ResponseTimeMs   float64 `json:"response_time_ms"`
	
	// 自定义指标
	CustomMetrics    map[string]float64 `json:"custom_metrics"`
}

// TrendAnalysis 趋势分析
type TrendAnalysis struct {
	MetricName   string    `json:"metric_name"`
	Direction    string    `json:"direction"`    // INCREASING, DECREASING, STABLE
	ChangeRate   float64   `json:"change_rate"`  // 变化率
	Confidence   float64   `json:"confidence"`   // 置信度
	PredictedValue float64 `json:"predicted_value"` // 预测值
	TimeWindow   time.Duration `json:"time_window"`
}

// PerformanceAnalyzer 性能分析器
type PerformanceAnalyzer struct {
	config         *AnalyzerConfig
	thresholds     map[string]*MetricThreshold
	snapshots      []PerformanceSnapshot
	snapshotsMux   sync.RWMutex
	alerts         []PerformanceAlert
	alertsMux      sync.RWMutex
	
	// 统计计数器
	totalAlerts    atomic.Int64
	criticalAlerts atomic.Int64
	
	// 性能基线
	baseline       map[string]float64
	baselineMux    sync.RWMutex
	
	// 趋势分析
	trends         map[string]*TrendAnalysis
	trendsMux      sync.RWMutex
	
	// 控制
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	
	// 网络统计基线
	lastNetStats   *net.IOCountersStat
	
	logger         *zap.Logger
}

// AnalyzerConfig 分析器配置
type AnalyzerConfig struct {
	Enabled              bool          `json:"enabled"`
	SampleInterval       time.Duration `json:"sample_interval"`
	SnapshotRetention    int           `json:"snapshot_retention"`
	AlertRetention       int           `json:"alert_retention"`
	TrendAnalysisWindow  time.Duration `json:"trend_analysis_window"`
	BaselineWindow       time.Duration `json:"baseline_window"`
	EnablePrediction     bool          `json:"enable_prediction"`
	AlertCooldown        time.Duration `json:"alert_cooldown"`
}

// DefaultAnalyzerConfig 默认分析器配置
func DefaultAnalyzerConfig() *AnalyzerConfig {
	return &AnalyzerConfig{
		Enabled:              true,
		SampleInterval:       5 * time.Second,
		SnapshotRetention:    1440, // 保留24小时数据(5秒间隔)
		AlertRetention:       10000,
		TrendAnalysisWindow:  10 * time.Minute,
		BaselineWindow:       1 * time.Hour,
		EnablePrediction:     true,
		AlertCooldown:        30 * time.Second,
	}
}

// NewPerformanceAnalyzer 创建性能分析器
func NewPerformanceAnalyzer(config *AnalyzerConfig) *PerformanceAnalyzer {
	if config == nil {
		config = DefaultAnalyzerConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	analyzer := &PerformanceAnalyzer{
		config:     config,
		thresholds: make(map[string]*MetricThreshold),
		snapshots:  make([]PerformanceSnapshot, 0),
		alerts:     make([]PerformanceAlert, 0),
		baseline:   make(map[string]float64),
		trends:     make(map[string]*TrendAnalysis),
		ctx:        ctx,
		cancel:     cancel,
		logger:     zap.L().Named("performance-analyzer"),
	}
	
	// 设置默认阈值
	analyzer.setDefaultThresholds()
	
	return analyzer
}

// setDefaultThresholds 设置默认阈值
func (pa *PerformanceAnalyzer) setDefaultThresholds() {
	defaultThresholds := []*MetricThreshold{
		{
			Name:         "cpu_usage_percent",
			WarnLevel:    70.0,
			ErrorLevel:   85.0,
			CriticalLevel: 95.0,
			Unit:         "%",
			Description:  "CPU使用率",
		},
		{
			Name:         "memory_used_percent",
			WarnLevel:    70.0,
			ErrorLevel:   85.0,
			CriticalLevel: 95.0,
			Unit:         "%",
			Description:  "内存使用率",
		},
		{
			Name:         "goroutine_count",
			WarnLevel:    1000.0,
			ErrorLevel:   5000.0,
			CriticalLevel: 10000.0,
			Unit:         "个",
			Description:  "Goroutine数量",
		},
		{
			Name:         "gc_pause_percent",
			WarnLevel:    2.0,
			ErrorLevel:   5.0,
			CriticalLevel: 10.0,
			Unit:         "%",
			Description:  "GC暂停时间占比",
		},
		{
			Name:         "response_time_ms",
			WarnLevel:    100.0,
			ErrorLevel:   500.0,
			CriticalLevel: 1000.0,
			Unit:         "ms",
			Description:  "响应时间",
		},
		{
			Name:         "error_rate",
			WarnLevel:    1.0,
			ErrorLevel:   5.0,
			CriticalLevel: 10.0,
			Unit:         "%",
			Description:  "错误率",
		},
	}
	
	for _, threshold := range defaultThresholds {
		pa.thresholds[threshold.Name] = threshold
	}
}

// Start 启动性能分析器
func (pa *PerformanceAnalyzer) Start() error {
	if !pa.config.Enabled {
		return nil
	}
	
	pa.logger.Info("启动性能分析器",
		zap.Duration("sample_interval", pa.config.SampleInterval),
		zap.Int("snapshot_retention", pa.config.SnapshotRetention),
		zap.Duration("trend_window", pa.config.TrendAnalysisWindow))
	
	// 启动采样循环
	pa.wg.Add(1)
	go func() {
		defer pa.wg.Done()
		pa.samplingLoop()
	}()
	
	// 启动趋势分析循环
	if pa.config.EnablePrediction {
		pa.wg.Add(1)
		go func() {
			defer pa.wg.Done()
			pa.trendAnalysisLoop()
		}()
	}
	
	// 启动清理循环
	pa.wg.Add(1)
	go func() {
		defer pa.wg.Done()
		pa.cleanupLoop()
	}()
	
	return nil
}

// Stop 停止性能分析器
func (pa *PerformanceAnalyzer) Stop() error {
	if !pa.config.Enabled {
		return nil
	}
	
	pa.logger.Info("停止性能分析器")
	
	if pa.cancel != nil {
		pa.cancel()
	}
	
	pa.wg.Wait()
	
	pa.logger.Info("性能分析器已停止")
	return nil
}

// samplingLoop 采样循环
func (pa *PerformanceAnalyzer) samplingLoop() {
	ticker := time.NewTicker(pa.config.SampleInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-pa.ctx.Done():
			return
		case <-ticker.C:
			snapshot := pa.takeSnapshot()
			pa.addSnapshot(snapshot)
			pa.analyzeSnapshot(snapshot)
		}
	}
}

// takeSnapshot 获取性能快照
func (pa *PerformanceAnalyzer) takeSnapshot() PerformanceSnapshot {
	snapshot := PerformanceSnapshot{
		Timestamp:     time.Now(),
		CustomMetrics: make(map[string]float64),
	}
	
	// CPU指标
	if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
		snapshot.CPUUsagePercent = cpuPercent[0]
	}
	snapshot.CPUCores = runtime.NumCPU()
	
	// 内存指标
	if memInfo, err := mem.VirtualMemory(); err == nil {
		snapshot.MemoryUsedBytes = memInfo.Used
		snapshot.MemoryTotalBytes = memInfo.Total
		snapshot.MemoryUsedPercent = memInfo.UsedPercent
	}
	
	// Goroutine指标
	snapshot.GoroutineCount = runtime.NumGoroutine()
	
	// GC指标 - 使用MemStats获取GC信息
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	snapshot.GCCycles = memStats.NumGC
	snapshot.GCForcedRuns = memStats.NumForcedGC
	if memStats.PauseNs[(memStats.NumGC+255)%256] > 0 {
		snapshot.GCPauseNs = memStats.PauseNs[(memStats.NumGC+255)%256]
	}
	
	// 计算GC暂停时间占比
	if memStats.GCCPUFraction > 0 {
		snapshot.GCPausePercent = memStats.GCCPUFraction * 100
	}
	
	// 网络指标
	if netStats, err := net.IOCounters(false); err == nil && len(netStats) > 0 {
		current := &netStats[0]
		snapshot.NetworkBytesRecv = current.BytesRecv
		snapshot.NetworkBytesSent = current.BytesSent
		snapshot.NetworkPacketsRecv = current.PacketsRecv
		snapshot.NetworkPacketsSent = current.PacketsSent
		
		pa.lastNetStats = current
	}
	
	return snapshot
}

// addSnapshot 添加快照
func (pa *PerformanceAnalyzer) addSnapshot(snapshot PerformanceSnapshot) {
	pa.snapshotsMux.Lock()
	defer pa.snapshotsMux.Unlock()
	
	pa.snapshots = append(pa.snapshots, snapshot)
	
	// 保持快照数量在限制内
	if len(pa.snapshots) > pa.config.SnapshotRetention {
		pa.snapshots = pa.snapshots[len(pa.snapshots)-pa.config.SnapshotRetention:]
	}
}

// analyzeSnapshot 分析快照
func (pa *PerformanceAnalyzer) analyzeSnapshot(snapshot PerformanceSnapshot) {
	// 检查阈值
	pa.checkThresholds(snapshot)
	
	// 更新基线
	pa.updateBaseline(snapshot)
}

// checkThresholds 检查阈值
func (pa *PerformanceAnalyzer) checkThresholds(snapshot PerformanceSnapshot) {
	metrics := map[string]float64{
		"cpu_usage_percent":   snapshot.CPUUsagePercent,
		"memory_used_percent": snapshot.MemoryUsedPercent,
		"goroutine_count":     float64(snapshot.GoroutineCount),
		"gc_pause_percent":    snapshot.GCPausePercent,
		"response_time_ms":    snapshot.ResponseTimeMs,
		"error_rate":          snapshot.ErrorRate,
	}
	
	for metricName, value := range metrics {
		if threshold, exists := pa.thresholds[metricName]; exists {
			pa.evaluateThreshold(metricName, value, threshold, snapshot.Timestamp)
		}
	}
	
	// 检查自定义指标
	for metricName, value := range snapshot.CustomMetrics {
		if threshold, exists := pa.thresholds[metricName]; exists {
			pa.evaluateThreshold(metricName, value, threshold, snapshot.Timestamp)
		}
	}
}

// evaluateThreshold 评估阈值
func (pa *PerformanceAnalyzer) evaluateThreshold(metricName string, value float64, threshold *MetricThreshold, timestamp time.Time) {
	var level string
	var alertValue float64
	var suggestions []string
	
	if value >= threshold.CriticalLevel {
		level = "CRITICAL"
		alertValue = threshold.CriticalLevel
		suggestions = pa.getCriticalSuggestions(metricName)
	} else if value >= threshold.ErrorLevel {
		level = "ERROR"
		alertValue = threshold.ErrorLevel
		suggestions = pa.getErrorSuggestions(metricName)
	} else if value >= threshold.WarnLevel {
		level = "WARN"
		alertValue = threshold.WarnLevel
		suggestions = pa.getWarnSuggestions(metricName)
	} else {
		return // 没有超过阈值
	}
	
	alert := PerformanceAlert{
		Type:        "threshold",
		Level:       level,
		Message:     fmt.Sprintf("%s 超过%s阈值", threshold.Description, level),
		MetricName:  metricName,
		Value:       value,
		Threshold:   alertValue,
		Timestamp:   timestamp,
		Suggestions: suggestions,
		Metadata: map[string]interface{}{
			"unit":        threshold.Unit,
			"description": threshold.Description,
		},
	}
	
	pa.addAlert(alert)
	
	// 记录日志
	pa.logger.Warn("性能告警",
		zap.String("metric", metricName),
		zap.String("level", level),
		zap.Float64("value", value),
		zap.Float64("threshold", alertValue),
		zap.String("unit", threshold.Unit))
}

// getCriticalSuggestions 获取关键级别建议
func (pa *PerformanceAnalyzer) getCriticalSuggestions(metricName string) []string {
	switch metricName {
	case "cpu_usage_percent":
		return []string{
			"立即检查CPU密集型任务",
			"考虑扩容或负载均衡",
			"检查是否有死循环或阻塞操作",
			"优化算法和数据结构",
		}
	case "memory_used_percent":
		return []string{
			"立即检查内存泄漏",
			"释放不必要的缓存",
			"增加swap空间或内存",
			"检查大对象分配",
		}
	case "goroutine_count":
		return []string{
			"检查goroutine泄漏",
			"优化并发控制",
			"使用goroutine池",
			"检查阻塞的channel操作",
		}
	default:
		return []string{"立即检查相关系统组件"}
	}
}

// getErrorSuggestions 获取错误级别建议
func (pa *PerformanceAnalyzer) getErrorSuggestions(metricName string) []string {
	switch metricName {
	case "cpu_usage_percent":
		return []string{
			"检查CPU使用情况",
			"优化热点代码",
			"考虑异步处理",
		}
	case "memory_used_percent":
		return []string{
			"检查内存使用情况",
			"优化缓存策略",
			"考虑内存压缩",
		}
	case "goroutine_count":
		return []string{
			"检查goroutine使用",
			"优化并发设计",
			"实现超时机制",
		}
	default:
		return []string{"检查相关系统组件"}
	}
}

// getWarnSuggestions 获取警告级别建议
func (pa *PerformanceAnalyzer) getWarnSuggestions(metricName string) []string {
	switch metricName {
	case "cpu_usage_percent":
		return []string{
			"监控CPU使用趋势",
			"准备性能优化",
		}
	case "memory_used_percent":
		return []string{
			"监控内存使用趋势",
			"准备内存优化",
		}
	case "goroutine_count":
		return []string{
			"监控goroutine数量",
			"检查并发控制",
		}
	default:
		return []string{"关注相关指标变化"}
	}
}

// addAlert 添加告警
func (pa *PerformanceAnalyzer) addAlert(alert PerformanceAlert) {
	pa.alertsMux.Lock()
	defer pa.alertsMux.Unlock()
	
	pa.alerts = append(pa.alerts, alert)
	pa.totalAlerts.Add(1)
	
	if alert.Level == "CRITICAL" {
		pa.criticalAlerts.Add(1)
	}
	
	// 保持告警数量在限制内
	if len(pa.alerts) > pa.config.AlertRetention {
		pa.alerts = pa.alerts[len(pa.alerts)-pa.config.AlertRetention:]
	}
}

// updateBaseline 更新基线
func (pa *PerformanceAnalyzer) updateBaseline(snapshot PerformanceSnapshot) {
	pa.baselineMux.Lock()
	defer pa.baselineMux.Unlock()
	
	// 简化的基线更新逻辑，使用移动平均
	alpha := 0.1 // 平滑因子
	
	metrics := map[string]float64{
		"cpu_usage_percent":   snapshot.CPUUsagePercent,
		"memory_used_percent": snapshot.MemoryUsedPercent,
		"goroutine_count":     float64(snapshot.GoroutineCount),
		"gc_pause_percent":    snapshot.GCPausePercent,
	}
	
	for name, value := range metrics {
		if baseline, exists := pa.baseline[name]; exists {
			pa.baseline[name] = alpha*value + (1-alpha)*baseline
		} else {
			pa.baseline[name] = value
		}
	}
}

// trendAnalysisLoop 趋势分析循环
func (pa *PerformanceAnalyzer) trendAnalysisLoop() {
	ticker := time.NewTicker(pa.config.TrendAnalysisWindow)
	defer ticker.Stop()
	
	for {
		select {
		case <-pa.ctx.Done():
			return
		case <-ticker.C:
			pa.performTrendAnalysis()
		}
	}
}

// performTrendAnalysis 执行趋势分析
func (pa *PerformanceAnalyzer) performTrendAnalysis() {
	pa.snapshotsMux.RLock()
	snapshots := make([]PerformanceSnapshot, len(pa.snapshots))
	copy(snapshots, pa.snapshots)
	pa.snapshotsMux.RUnlock()
	
	if len(snapshots) < 10 {
		return // 数据不足
	}
	
	// 分析最近的数据点
	windowSize := int(pa.config.TrendAnalysisWindow / pa.config.SampleInterval)
	if windowSize > len(snapshots) {
		windowSize = len(snapshots)
	}
	
	recentSnapshots := snapshots[len(snapshots)-windowSize:]
	pa.analyzeTrends(recentSnapshots)
}

// analyzeTrends 分析趋势
func (pa *PerformanceAnalyzer) analyzeTrends(snapshots []PerformanceSnapshot) {
	if len(snapshots) < 2 {
		return
	}
	
	pa.trendsMux.Lock()
	defer pa.trendsMux.Unlock()
	
	// 分析CPU使用率趋势
	cpuValues := make([]float64, len(snapshots))
	for i, s := range snapshots {
		cpuValues[i] = s.CPUUsagePercent
	}
	pa.trends["cpu_usage_percent"] = pa.calculateTrend("cpu_usage_percent", cpuValues)
	
	// 分析内存使用率趋势
	memValues := make([]float64, len(snapshots))
	for i, s := range snapshots {
		memValues[i] = s.MemoryUsedPercent
	}
	pa.trends["memory_used_percent"] = pa.calculateTrend("memory_used_percent", memValues)
	
	// 分析Goroutine数量趋势
	goroutineValues := make([]float64, len(snapshots))
	for i, s := range snapshots {
		goroutineValues[i] = float64(s.GoroutineCount)
	}
	pa.trends["goroutine_count"] = pa.calculateTrend("goroutine_count", goroutineValues)
}

// calculateTrend 计算趋势
func (pa *PerformanceAnalyzer) calculateTrend(metricName string, values []float64) *TrendAnalysis {
	if len(values) < 2 {
		return nil
	}
	
	// 简单线性回归计算趋势
	n := float64(len(values))
	sumX, sumY, sumXY, sumXX := 0.0, 0.0, 0.0, 0.0
	
	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	
	slope := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
	intercept := (sumY - slope*sumX) / n
	
	// 预测下一个值
	nextX := float64(len(values))
	predictedValue := slope*nextX + intercept
	
	// 确定趋势方向
	direction := "STABLE"
	if slope > 0.1 {
		direction = "INCREASING"
	} else if slope < -0.1 {
		direction = "DECREASING"
	}
	
	// 计算置信度（简化版本）
	confidence := 0.8 // 固定置信度，实际应该基于统计计算
	
	return &TrendAnalysis{
		MetricName:     metricName,
		Direction:      direction,
		ChangeRate:     slope,
		Confidence:     confidence,
		PredictedValue: predictedValue,
		TimeWindow:     pa.config.TrendAnalysisWindow,
	}
}

// cleanupLoop 清理循环
func (pa *PerformanceAnalyzer) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-pa.ctx.Done():
			return
		case <-ticker.C:
			pa.performCleanup()
		}
	}
}

// performCleanup 执行清理
func (pa *PerformanceAnalyzer) performCleanup() {
	// 清理快照在addSnapshot中已经处理
	
	// 清理过期告警
	pa.alertsMux.Lock()
	if len(pa.alerts) > pa.config.AlertRetention {
		pa.alerts = pa.alerts[len(pa.alerts)-pa.config.AlertRetention:]
	}
	pa.alertsMux.Unlock()
	
	pa.logger.Debug("执行性能分析器清理")
}

// GetCurrentSnapshot 获取当前快照
func (pa *PerformanceAnalyzer) GetCurrentSnapshot() PerformanceSnapshot {
	return pa.takeSnapshot()
}

// GetSnapshots 获取快照历史
func (pa *PerformanceAnalyzer) GetSnapshots(count int) []PerformanceSnapshot {
	pa.snapshotsMux.RLock()
	defer pa.snapshotsMux.RUnlock()
	
	if count <= 0 || count > len(pa.snapshots) {
		count = len(pa.snapshots)
	}
	
	result := make([]PerformanceSnapshot, count)
	copy(result, pa.snapshots[len(pa.snapshots)-count:])
	return result
}

// GetAlerts 获取告警
func (pa *PerformanceAnalyzer) GetAlerts(count int) []PerformanceAlert {
	pa.alertsMux.RLock()
	defer pa.alertsMux.RUnlock()
	
	if count <= 0 || count > len(pa.alerts) {
		count = len(pa.alerts)
	}
	
	result := make([]PerformanceAlert, count)
	copy(result, pa.alerts[len(pa.alerts)-count:])
	return result
}

// GetTrends 获取趋势分析
func (pa *PerformanceAnalyzer) GetTrends() map[string]*TrendAnalysis {
	pa.trendsMux.RLock()
	defer pa.trendsMux.RUnlock()
	
	result := make(map[string]*TrendAnalysis)
	for k, v := range pa.trends {
		if v != nil {
			trend := *v // 复制
			result[k] = &trend
		}
	}
	return result
}

// GetBaseline 获取基线
func (pa *PerformanceAnalyzer) GetBaseline() map[string]float64 {
	pa.baselineMux.RLock()
	defer pa.baselineMux.RUnlock()
	
	result := make(map[string]float64)
	for k, v := range pa.baseline {
		result[k] = v
	}
	return result
}

// SetCustomMetric 设置自定义指标
func (pa *PerformanceAnalyzer) SetCustomMetric(name string, value float64) {
	// 这个方法可以被其他组件调用来上报自定义指标
	// 指标会在下次快照时被包含
}

// SetThreshold 设置阈值
func (pa *PerformanceAnalyzer) SetThreshold(threshold *MetricThreshold) {
	pa.thresholds[threshold.Name] = threshold
	pa.logger.Info("更新性能阈值",
		zap.String("metric", threshold.Name),
		zap.Float64("warn", threshold.WarnLevel),
		zap.Float64("error", threshold.ErrorLevel),
		zap.Float64("critical", threshold.CriticalLevel))
}

// GetStats 获取统计信息
func (pa *PerformanceAnalyzer) GetStats() map[string]interface{} {
	pa.snapshotsMux.RLock()
	snapshotCount := len(pa.snapshots)
	pa.snapshotsMux.RUnlock()
	
	pa.alertsMux.RLock()
	alertCount := len(pa.alerts)
	pa.alertsMux.RUnlock()
	
	return map[string]interface{}{
		"enabled":            pa.config.Enabled,
		"snapshot_count":     snapshotCount,
		"alert_count":        alertCount,
		"total_alerts":       pa.totalAlerts.Load(),
		"critical_alerts":    pa.criticalAlerts.Load(),
		"sample_interval":    pa.config.SampleInterval.String(),
		"trend_analysis":     pa.config.EnablePrediction,
		"thresholds_count":   len(pa.thresholds),
	}
}
