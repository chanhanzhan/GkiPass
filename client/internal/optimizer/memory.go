package optimizer

import (
	"context"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// MemoryConfig 内存优化配置
type MemoryConfig struct {
	Enabled                 bool          `json:"enabled"`
	GCPercentTarget         int           `json:"gc_percent_target"`         // GC目标百分比
	MaxMemoryUsageMB        int64         `json:"max_memory_usage_mb"`       // 最大内存使用(MB)
	GCForceInterval         time.Duration `json:"gc_force_interval"`         // 强制GC间隔
	MemoryCheckInterval     time.Duration `json:"memory_check_interval"`     // 内存检查间隔
	MemoryPressureThreshold float64       `json:"memory_pressure_threshold"` // 内存压力阈值
	EnableMemoryProfiling   bool          `json:"enable_memory_profiling"`   // 启用内存分析
}

// GoroutineConfig Goroutine优化配置
type GoroutineConfig struct {
	Enabled                bool          `json:"enabled"`
	MaxGoroutines          int           `json:"max_goroutines"`           // 最大Goroutine数量
	MonitorInterval        time.Duration `json:"monitor_interval"`         // 监控间隔
	GoroutineLeakThreshold int           `json:"goroutine_leak_threshold"` // 泄漏阈值
	EnableStackTrace       bool          `json:"enable_stack_trace"`       // 启用堆栈跟踪
	WorkerPoolSize         int           `json:"worker_pool_size"`         // 工作池大小
}

// DefaultMemoryConfig 默认内存配置
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		Enabled:                 true,
		GCPercentTarget:         100, // Go默认值
		MaxMemoryUsageMB:        512, // 512MB限制
		GCForceInterval:         5 * time.Minute,
		MemoryCheckInterval:     30 * time.Second,
		MemoryPressureThreshold: 0.8, // 80%内存压力阈值
		EnableMemoryProfiling:   false,
	}
}

// DefaultGoroutineConfig 默认Goroutine配置
func DefaultGoroutineConfig() *GoroutineConfig {
	return &GoroutineConfig{
		Enabled:                true,
		MaxGoroutines:          1000,
		MonitorInterval:        10 * time.Second,
		GoroutineLeakThreshold: 500,
		EnableStackTrace:       false,
		WorkerPoolSize:         runtime.NumCPU() * 2,
	}
}

// MemoryStats 内存统计
type MemoryStats struct {
	AllocatedMB    float64   `json:"allocated_mb"`
	SystemMB       float64   `json:"system_mb"`
	GCCycles       uint32    `json:"gc_cycles"`
	GCPauseMs      float64   `json:"gc_pause_ms"`
	GCCPUPercent   float64   `json:"gc_cpu_percent"`
	HeapObjects    uint64    `json:"heap_objects"`
	HeapInUseMB    float64   `json:"heap_in_use_mb"`
	StackInUseMB   float64   `json:"stack_in_use_mb"`
	NumGoroutines  int       `json:"num_goroutines"`
	MemoryPressure float64   `json:"memory_pressure"`
	LastGCTime     time.Time `json:"last_gc_time"`
	NextGCTargetMB float64   `json:"next_gc_target_mb"`
}

// GoroutineStats Goroutine统计
type GoroutineStats struct {
	Total           int                  `json:"total"`
	Running         int                  `json:"running"`
	Waiting         int                  `json:"waiting"`
	LeakSuspected   bool                 `json:"leak_suspected"`
	WorkerPoolUsage float64              `json:"worker_pool_usage"`
	TopStacks       []GoroutineStackInfo `json:"top_stacks,omitempty"`
}

// GoroutineStackInfo Goroutine堆栈信息
type GoroutineStackInfo struct {
	Count      int    `json:"count"`
	Function   string `json:"function"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Stacktrace string `json:"stacktrace,omitempty"`
}

// MemoryOptimizer 内存优化器
type MemoryOptimizer struct {
	config   *MemoryConfig
	stats    MemoryStats
	statsMux sync.RWMutex

	// 优化统计
	forcedGCs    atomic.Int64
	memoryAlerts atomic.Int64
	lastGCForced time.Time

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// GoroutineOptimizer Goroutine优化器
type GoroutineOptimizer struct {
	config   *GoroutineConfig
	stats    GoroutineStats
	statsMux sync.RWMutex

	// 工作池
	workerPool chan func()
	workerBusy atomic.Int64

	// 监控统计
	goroutineAlerts atomic.Int64
	peakGoroutines  atomic.Int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *zap.Logger
}

// NewMemoryOptimizer 创建内存优化器
func NewMemoryOptimizer(config *MemoryConfig) *MemoryOptimizer {
	if config == nil {
		config = DefaultMemoryConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &MemoryOptimizer{
		config: config,
		ctx:    ctx,
		cancel: cancel,
		logger: zap.L().Named("memory-optimizer"),
	}
}

// NewGoroutineOptimizer 创建Goroutine优化器
func NewGoroutineOptimizer(config *GoroutineConfig) *GoroutineOptimizer {
	if config == nil {
		config = DefaultGoroutineConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	optimizer := &GoroutineOptimizer{
		config:     config,
		workerPool: make(chan func(), config.WorkerPoolSize*2), // 缓冲队列
		ctx:        ctx,
		cancel:     cancel,
		logger:     zap.L().Named("goroutine-optimizer"),
	}

	return optimizer
}

// Start 启动内存优化器
func (mo *MemoryOptimizer) Start() error {
	if !mo.config.Enabled {
		return nil
	}

	mo.logger.Info("启动内存优化器",
		zap.Int("gc_percent", mo.config.GCPercentTarget),
		zap.Int64("max_memory_mb", mo.config.MaxMemoryUsageMB),
		zap.Duration("check_interval", mo.config.MemoryCheckInterval))

	// 设置GC目标
	debug.SetGCPercent(mo.config.GCPercentTarget)

	// 启动监控循环
	mo.wg.Add(1)
	go func() {
		defer mo.wg.Done()
		mo.monitorLoop()
	}()

	// 启动GC循环
	mo.wg.Add(1)
	go func() {
		defer mo.wg.Done()
		mo.gcLoop()
	}()

	return nil
}

// Start 启动Goroutine优化器
func (g *GoroutineOptimizer) Start() error {
	if !g.config.Enabled {
		return nil
	}

	g.logger.Info("启动Goroutine优化器",
		zap.Int("max_goroutines", g.config.MaxGoroutines),
		zap.Int("worker_pool_size", g.config.WorkerPoolSize),
		zap.Duration("monitor_interval", g.config.MonitorInterval))

	// 启动工作池
	for i := 0; i < g.config.WorkerPoolSize; i++ {
		g.wg.Add(1)
		go func(id int) {
			defer g.wg.Done()
			g.worker(id)
		}(i)
	}

	// 启动监控循环
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		g.monitorLoop()
	}()

	return nil
}

// Stop 停止内存优化器
func (mo *MemoryOptimizer) Stop() error {
	if !mo.config.Enabled {
		return nil
	}

	mo.logger.Info("停止内存优化器")

	if mo.cancel != nil {
		mo.cancel()
	}

	mo.wg.Wait()

	mo.logger.Info("内存优化器已停止")
	return nil
}

// Stop 停止Goroutine优化器
func (g *GoroutineOptimizer) Stop() error {
	if !g.config.Enabled {
		return nil
	}

	g.logger.Info("停止Goroutine优化器")

	if g.cancel != nil {
		g.cancel()
	}

	// 关闭工作池
	close(g.workerPool)

	g.wg.Wait()

	g.logger.Info("Goroutine优化器已停止")
	return nil
}

// monitorLoop 内存监控循环
func (mo *MemoryOptimizer) monitorLoop() {
	ticker := time.NewTicker(mo.config.MemoryCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mo.ctx.Done():
			return
		case <-ticker.C:
			mo.updateStats()
			mo.checkMemoryPressure()
		}
	}
}

// gcLoop GC循环
func (mo *MemoryOptimizer) gcLoop() {
	ticker := time.NewTicker(mo.config.GCForceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mo.ctx.Done():
			return
		case <-ticker.C:
			mo.performGC()
		}
	}
}

// updateStats 更新内存统计
func (mo *MemoryOptimizer) updateStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	mo.statsMux.Lock()
	defer mo.statsMux.Unlock()

	mo.stats = MemoryStats{
		AllocatedMB:    float64(m.Alloc) / 1024 / 1024,
		SystemMB:       float64(m.Sys) / 1024 / 1024,
		GCCycles:       m.NumGC,
		GCPauseMs:      float64(m.PauseNs[(m.NumGC+255)%256]) / 1e6,
		GCCPUPercent:   m.GCCPUFraction * 100,
		HeapObjects:    m.HeapObjects,
		HeapInUseMB:    float64(m.HeapInuse) / 1024 / 1024,
		StackInUseMB:   float64(m.StackInuse) / 1024 / 1024,
		NumGoroutines:  runtime.NumGoroutine(),
		NextGCTargetMB: float64(m.NextGC) / 1024 / 1024,
	}

	if m.LastGC > 0 {
		mo.stats.LastGCTime = time.Unix(0, int64(m.LastGC))
	}

	// 计算内存压力
	if mo.config.MaxMemoryUsageMB > 0 {
		mo.stats.MemoryPressure = mo.stats.SystemMB / float64(mo.config.MaxMemoryUsageMB)
	}
}

// checkMemoryPressure 检查内存压力
func (mo *MemoryOptimizer) checkMemoryPressure() {
	mo.statsMux.RLock()
	pressure := mo.stats.MemoryPressure
	systemMB := mo.stats.SystemMB
	mo.statsMux.RUnlock()

	if pressure > mo.config.MemoryPressureThreshold {
		mo.memoryAlerts.Add(1)

		mo.logger.Warn("内存压力过高",
			zap.Float64("pressure", pressure),
			zap.Float64("system_mb", systemMB),
			zap.Float64("threshold", mo.config.MemoryPressureThreshold))

		// 触发GC
		mo.performGC()

		// 如果仍然过高，可以考虑其他措施
		if pressure > 0.9 {
			mo.logger.Error("内存使用接近极限，考虑增加内存或优化代码",
				zap.Float64("pressure", pressure))
		}
	}
}

// performGC 执行GC
func (mo *MemoryOptimizer) performGC() {
	if time.Since(mo.lastGCForced) < time.Minute {
		return // 避免频繁强制GC
	}

	before := mo.getAllocatedMB()
	runtime.GC()
	after := mo.getAllocatedMB()

	mo.forcedGCs.Add(1)
	mo.lastGCForced = time.Now()

	freed := before - after
	mo.logger.Debug("执行强制GC",
		zap.Float64("before_mb", before),
		zap.Float64("after_mb", after),
		zap.Float64("freed_mb", freed))
}

// getAllocatedMB 获取已分配内存(MB)
func (mo *MemoryOptimizer) getAllocatedMB() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.Alloc) / 1024 / 1024
}

// monitorLoop Goroutine监控循环
func (g *GoroutineOptimizer) monitorLoop() {
	ticker := time.NewTicker(g.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			g.updateStats()
			g.checkGoroutineHealth()
		}
	}
}

// updateStats 更新Goroutine统计
func (g *GoroutineOptimizer) updateStats() {
	numGoroutines := runtime.NumGoroutine()

	g.statsMux.Lock()
	defer g.statsMux.Unlock()

	g.stats.Total = numGoroutines
	g.stats.WorkerPoolUsage = float64(g.workerBusy.Load()) / float64(g.config.WorkerPoolSize)

	// 更新峰值
	if int64(numGoroutines) > g.peakGoroutines.Load() {
		g.peakGoroutines.Store(int64(numGoroutines))
	}

	// 检查是否疑似泄漏
	g.stats.LeakSuspected = numGoroutines > g.config.GoroutineLeakThreshold

	// 如果启用了堆栈跟踪，收集堆栈信息
	if g.config.EnableStackTrace && g.stats.LeakSuspected {
		g.stats.TopStacks = g.collectStackInfo()
	}
}

// checkGoroutineHealth 检查Goroutine健康状态
func (g *GoroutineOptimizer) checkGoroutineHealth() {
	g.statsMux.RLock()
	total := g.stats.Total
	leaked := g.stats.LeakSuspected
	g.statsMux.RUnlock()

	if total > g.config.MaxGoroutines {
		g.goroutineAlerts.Add(1)
		g.logger.Warn("Goroutine数量超过限制",
			zap.Int("current", total),
			zap.Int("max", g.config.MaxGoroutines))
	}

	if leaked {
		g.logger.Warn("疑似Goroutine泄漏",
			zap.Int("current", total),
			zap.Int("threshold", g.config.GoroutineLeakThreshold))
	}
}

// collectStackInfo 收集堆栈信息
func (g *GoroutineOptimizer) collectStackInfo() []GoroutineStackInfo {
	// 简化版本 - 实际实现需要解析runtime.Stack()的输出
	buf := make([]byte, 1024*1024) // 1MB buffer
	n := runtime.Stack(buf, true)
	stackTrace := string(buf[:n])

	// 这里应该解析堆栈跟踪，提取关键信息
	// 为了简化，我们返回一个基本的统计
	return []GoroutineStackInfo{
		{
			Count:      g.stats.Total,
			Function:   "总计",
			Stacktrace: stackTrace[:min(len(stackTrace), 1000)], // 限制长度
		},
	}
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// worker 工作池工作者
func (g *GoroutineOptimizer) worker(id int) {
	g.logger.Debug("启动工作者", zap.Int("id", id))

	for task := range g.workerPool {
		if task == nil {
			break
		}

		g.workerBusy.Add(1)
		func() {
			defer func() {
				g.workerBusy.Add(-1)
				if r := recover(); r != nil {
					g.logger.Error("工作者任务panic",
						zap.Int("worker_id", id),
						zap.Any("panic", r))
				}
			}()
			task()
		}()
	}

	g.logger.Debug("工作者停止", zap.Int("id", id))
}

// SubmitTask 提交任务到工作池
func (g *GoroutineOptimizer) SubmitTask(task func()) bool {
	select {
	case g.workerPool <- task:
		return true
	default:
		// 工作池满，拒绝任务
		g.logger.Warn("工作池已满，拒绝任务")
		return false
	}
}

// GetMemoryStats 获取内存统计
func (mo *MemoryOptimizer) GetMemoryStats() MemoryStats {
	mo.statsMux.RLock()
	defer mo.statsMux.RUnlock()
	return mo.stats
}

// GetGoroutineStats 获取Goroutine统计
func (g *GoroutineOptimizer) GetGoroutineStats() GoroutineStats {
	g.statsMux.RLock()
	defer g.statsMux.RUnlock()
	return g.stats
}

// GetStats 获取内存优化器统计
func (mo *MemoryOptimizer) GetStats() map[string]interface{} {
	stats := mo.GetMemoryStats()

	return map[string]interface{}{
		"enabled":            mo.config.Enabled,
		"memory_stats":       stats,
		"forced_gcs":         mo.forcedGCs.Load(),
		"memory_alerts":      mo.memoryAlerts.Load(),
		"gc_percent_target":  mo.config.GCPercentTarget,
		"max_memory_mb":      mo.config.MaxMemoryUsageMB,
		"pressure_threshold": mo.config.MemoryPressureThreshold,
	}
}

// GetStats 获取Goroutine优化器统计
func (g *GoroutineOptimizer) GetStats() map[string]interface{} {
	stats := g.GetGoroutineStats()

	return map[string]interface{}{
		"enabled":          g.config.Enabled,
		"goroutine_stats":  stats,
		"goroutine_alerts": g.goroutineAlerts.Load(),
		"peak_goroutines":  g.peakGoroutines.Load(),
		"max_goroutines":   g.config.MaxGoroutines,
		"worker_pool_size": g.config.WorkerPoolSize,
		"worker_pool_busy": g.workerBusy.Load(),
	}
}
