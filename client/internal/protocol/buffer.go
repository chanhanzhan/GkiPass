package protocol

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// BufferConfig 缓冲区配置
type BufferConfig struct {
	// 缓冲区池配置
	PoolSizes     []int         `json:"pool_sizes"`      // 不同大小的缓冲区池
	MaxPoolSize   int           `json:"max_pool_size"`   // 每个池的最大大小
	MinRetainTime time.Duration `json:"min_retain_time"` // 最小保留时间
	MaxIdleTime   time.Duration `json:"max_idle_time"`   // 最大空闲时间

	// 内存管理
	MaxTotalMemory   int64   `json:"max_total_memory"`  // 最大总内存
	GCThreshold      float64 `json:"gc_threshold"`      // GC阈值
	EnableMonitoring bool    `json:"enable_monitoring"` // 启用监控

	// 性能优化
	EnableWarmup    bool `json:"enable_warmup"`    // 启用预热
	WarmupCount     int  `json:"warmup_count"`     // 预热数量
	EnableMetrics   bool `json:"enable_metrics"`   // 启用指标收集
	EnableAlignment bool `json:"enable_alignment"` // 启用内存对齐
}

// DefaultBufferConfig 默认缓冲区配置
func DefaultBufferConfig() *BufferConfig {
	return &BufferConfig{
		PoolSizes:        []int{512, 1024, 2048, 4096, 8192, 16384, 32768, 65536},
		MaxPoolSize:      1000,
		MinRetainTime:    1 * time.Minute,
		MaxIdleTime:      10 * time.Minute,
		MaxTotalMemory:   512 * 1024 * 1024, // 512MB
		GCThreshold:      0.8,               // 80%
		EnableMonitoring: true,
		EnableWarmup:     true,
		WarmupCount:      10,
		EnableMetrics:    true,
		EnableAlignment:  true,
	}
}

// BufferManager 缓冲区管理器
type BufferManager struct {
	config *BufferConfig
	logger *zap.Logger

	// 缓冲区池
	pools      map[int]*BufferPool
	poolsMutex sync.RWMutex

	// 统计信息
	stats struct {
		totalAllocated atomic.Int64
		totalReleased  atomic.Int64
		totalMemory    atomic.Int64
		poolHits       atomic.Int64
		poolMisses     atomic.Int64
		gcTriggers     atomic.Int64
		peakMemory     atomic.Int64
	}

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// BufferPool 缓冲区池
type BufferPool struct {
	size    int
	pool    sync.Pool
	manager *BufferManager

	// 统计
	allocated atomic.Int64
	released  atomic.Int64
	inUse     atomic.Int64
	peak      atomic.Int64

	// 内存跟踪
	buffers    map[*[]byte]*BufferInfo
	buffersMux sync.RWMutex
}

// BufferInfo 缓冲区信息
type BufferInfo struct {
	Size        int
	AllocatedAt time.Time
	LastUsed    time.Time
	UseCount    int64
	Pool        *BufferPool
}

// SmartBuffer 智能缓冲区
type SmartBuffer struct {
	data     []byte
	size     int
	capacity int
	pool     *BufferPool
	info     *BufferInfo
	released bool
	refCount *atomic.Int64 // 使用指针共享引用计数
}

// NewBufferManager 创建缓冲区管理器
func NewBufferManager(config *BufferConfig) *BufferManager {
	if config == nil {
		config = DefaultBufferConfig()
	}

	manager := &BufferManager{
		config: config,
		logger: zap.L().Named("buffer-manager"),
		pools:  make(map[int]*BufferPool),
	}

	// 初始化缓冲区池
	for _, size := range config.PoolSizes {
		manager.pools[size] = &BufferPool{
			size:    size,
			manager: manager,
			buffers: make(map[*[]byte]*BufferInfo),
		}

		// 初始化sync.Pool
		pool := manager.pools[size]
		pool.pool = sync.Pool{
			New: func() interface{} {
				return manager.allocateBuffer(size)
			},
		}
	}

	return manager
}

// Start 启动缓冲区管理器
func (bm *BufferManager) Start(ctx context.Context) error {
	bm.ctx, bm.cancel = context.WithCancel(ctx)

	// 预热缓冲区池
	if bm.config.EnableWarmup {
		bm.warmupPools()
	}

	// 启动监控协程
	if bm.config.EnableMonitoring {
		bm.wg.Add(1)
		go bm.monitorLoop()
	}

	// 启动清理协程
	bm.wg.Add(1)
	go bm.cleanupLoop()

	bm.logger.Info("🗂️  缓冲区管理器启动",
		zap.Ints("pool_sizes", bm.config.PoolSizes),
		zap.Int("max_pool_size", bm.config.MaxPoolSize),
		zap.Int64("max_memory_mb", bm.config.MaxTotalMemory/1024/1024))

	return nil
}

// Stop 停止缓冲区管理器
func (bm *BufferManager) Stop() error {
	bm.logger.Info("🛑 正在停止缓冲区管理器...")

	if bm.cancel != nil {
		bm.cancel()
	}

	// 等待协程结束
	bm.wg.Wait()

	// 清理所有缓冲区
	bm.cleanup()

	bm.logger.Info("✅ 缓冲区管理器已停止")
	return nil
}

// GetBuffer 获取指定大小的缓冲区
func (bm *BufferManager) GetBuffer(size int) *SmartBuffer {
	// 查找合适的池
	poolSize := bm.findBestPoolSize(size)
	pool := bm.getPool(poolSize)

	if pool == nil {
		// 没有合适的池，直接分配
		bm.stats.poolMisses.Add(1)
		return bm.allocateDirectly(size)
	}

	// 从池中获取缓冲区
	bufferPtr := pool.pool.Get().(*[]byte)
	buffer := *bufferPtr

	// 更新统计
	pool.allocated.Add(1)
	pool.inUse.Add(1)
	bm.stats.poolHits.Add(1)
	bm.stats.totalAllocated.Add(1)

	// 更新峰值
	current := pool.inUse.Load()
	for {
		peak := pool.peak.Load()
		if current <= peak || pool.peak.CompareAndSwap(peak, current) {
			break
		}
	}

	// 创建智能缓冲区
	refCount := &atomic.Int64{}
	refCount.Store(1)

	smartBuffer := &SmartBuffer{
		data:     buffer[:size],
		size:     size,
		capacity: len(buffer),
		pool:     pool,
		released: false,
		refCount: refCount,
	}

	// 记录缓冲区信息
	if bm.config.EnableMetrics {
		pool.buffersMux.Lock()
		pool.buffers[bufferPtr] = &BufferInfo{
			Size:        size,
			AllocatedAt: time.Now(),
			LastUsed:    time.Now(),
			UseCount:    1,
			Pool:        pool,
		}
		smartBuffer.info = pool.buffers[bufferPtr]
		pool.buffersMux.Unlock()
	}

	return smartBuffer
}

// findBestPoolSize 查找最佳池大小
func (bm *BufferManager) findBestPoolSize(size int) int {
	bm.poolsMutex.RLock()
	defer bm.poolsMutex.RUnlock()

	// 找到最小的满足要求的池大小
	bestSize := 0
	for poolSize := range bm.pools {
		if poolSize >= size && (bestSize == 0 || poolSize < bestSize) {
			bestSize = poolSize
		}
	}

	return bestSize
}

// getPool 获取指定大小的池
func (bm *BufferManager) getPool(size int) *BufferPool {
	bm.poolsMutex.RLock()
	defer bm.poolsMutex.RUnlock()

	return bm.pools[size]
}

// allocateBuffer 分配缓冲区
func (bm *BufferManager) allocateBuffer(size int) *[]byte {
	// 检查内存限制
	if bm.stats.totalMemory.Load() > bm.config.MaxTotalMemory {
		bm.triggerGC()
	}

	// 分配内存
	var buffer []byte
	if bm.config.EnableAlignment {
		// 内存对齐分配（简化实现）
		alignedSize := (size + 63) &^ 63 // 64字节对齐
		buffer = make([]byte, alignedSize)[:size]
	} else {
		buffer = make([]byte, size)
	}

	// 更新内存统计
	bm.stats.totalMemory.Add(int64(cap(buffer)))

	return &buffer
}

// allocateDirectly 直接分配（不使用池）
func (bm *BufferManager) allocateDirectly(size int) *SmartBuffer {
	bufferPtr := bm.allocateBuffer(size)
	buffer := *bufferPtr

	refCount := &atomic.Int64{}
	refCount.Store(1)

	return &SmartBuffer{
		data:     buffer,
		size:     size,
		capacity: len(buffer),
		pool:     nil, // 不属于任何池
		released: false,
		refCount: refCount,
	}
}

// warmupPools 预热缓冲区池
func (bm *BufferManager) warmupPools() {
	bm.logger.Info("🔥 开始预热缓冲区池...")

	for size, pool := range bm.pools {
		for i := 0; i < bm.config.WarmupCount; i++ {
			buffer := pool.pool.Get()
			pool.pool.Put(buffer)
		}

		bm.logger.Debug("池预热完成",
			zap.Int("size", size),
			zap.Int("count", bm.config.WarmupCount))
	}

	bm.logger.Info("✅ 缓冲区池预热完成")
}

// triggerGC 触发垃圾回收
func (bm *BufferManager) triggerGC() {
	bm.stats.gcTriggers.Add(1)

	bm.logger.Debug("💾 触发垃圾回收",
		zap.Int64("total_memory", bm.stats.totalMemory.Load()),
		zap.Int64("max_memory", bm.config.MaxTotalMemory))

	runtime.GC()
	runtime.GC() // 调用两次确保彻底清理

	// 更新峰值内存
	current := bm.stats.totalMemory.Load()
	for {
		peak := bm.stats.peakMemory.Load()
		if current <= peak || bm.stats.peakMemory.CompareAndSwap(peak, current) {
			break
		}
	}
}

// monitorLoop 监控循环
func (bm *BufferManager) monitorLoop() {
	defer bm.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bm.logStatistics()
			bm.checkMemoryPressure()

		case <-bm.ctx.Done():
			return
		}
	}
}

// cleanupLoop 清理循环
func (bm *BufferManager) cleanupLoop() {
	defer bm.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bm.cleanupIdleBuffers()

		case <-bm.ctx.Done():
			return
		}
	}
}

// logStatistics 记录统计信息
func (bm *BufferManager) logStatistics() {
	stats := bm.GetStats()

	bm.logger.Info("📊 缓冲区管理器统计",
		zap.Int64("total_allocated", stats["total_allocated"].(int64)),
		zap.Int64("total_released", stats["total_released"].(int64)),
		zap.Int64("memory_mb", stats["memory_usage_mb"].(int64)),
		zap.Float64("hit_rate", stats["hit_rate"].(float64)),
		zap.Int64("gc_triggers", stats["gc_triggers"].(int64)))
}

// checkMemoryPressure 检查内存压力
func (bm *BufferManager) checkMemoryPressure() {
	currentMemory := bm.stats.totalMemory.Load()
	threshold := int64(float64(bm.config.MaxTotalMemory) * bm.config.GCThreshold)

	if currentMemory > threshold {
		bm.logger.Warn("⚠️  内存压力过高，触发清理",
			zap.Int64("current_mb", currentMemory/1024/1024),
			zap.Int64("threshold_mb", threshold/1024/1024))

		bm.triggerGC()
		bm.cleanupIdleBuffers()
	}
}

// cleanupIdleBuffers 清理空闲缓冲区
func (bm *BufferManager) cleanupIdleBuffers() {
	if !bm.config.EnableMetrics {
		return
	}

	now := time.Now()
	cleanupCount := 0

	bm.poolsMutex.RLock()
	pools := make([]*BufferPool, 0, len(bm.pools))
	for _, pool := range bm.pools {
		pools = append(pools, pool)
	}
	bm.poolsMutex.RUnlock()

	for _, pool := range pools {
		pool.buffersMux.Lock()
		for bufferPtr, info := range pool.buffers {
			if now.Sub(info.LastUsed) > bm.config.MaxIdleTime {
				delete(pool.buffers, bufferPtr)
				cleanupCount++
			}
		}
		pool.buffersMux.Unlock()
	}

	if cleanupCount > 0 {
		bm.logger.Debug("🧹 清理空闲缓冲区", zap.Int("count", cleanupCount))
	}
}

// cleanup 清理所有缓冲区
func (bm *BufferManager) cleanup() {
	bm.poolsMutex.Lock()
	defer bm.poolsMutex.Unlock()

	for _, pool := range bm.pools {
		pool.buffersMux.Lock()
		pool.buffers = make(map[*[]byte]*BufferInfo)
		pool.buffersMux.Unlock()
	}

	bm.logger.Info("🧹 缓冲区清理完成")
}

// GetStats 获取统计信息
func (bm *BufferManager) GetStats() map[string]interface{} {
	totalAllocated := bm.stats.totalAllocated.Load()
	totalReleased := bm.stats.totalReleased.Load()
	poolHits := bm.stats.poolHits.Load()
	poolMisses := bm.stats.poolMisses.Load()

	var hitRate float64
	if poolHits+poolMisses > 0 {
		hitRate = float64(poolHits) / float64(poolHits+poolMisses)
	}

	poolStats := make(map[string]interface{})
	bm.poolsMutex.RLock()
	for size, pool := range bm.pools {
		poolStats[fmt.Sprintf("pool_%d", size)] = map[string]interface{}{
			"allocated": pool.allocated.Load(),
			"released":  pool.released.Load(),
			"in_use":    pool.inUse.Load(),
			"peak":      pool.peak.Load(),
		}
	}
	bm.poolsMutex.RUnlock()

	return map[string]interface{}{
		"total_allocated": totalAllocated,
		"total_released":  totalReleased,
		"active_buffers":  totalAllocated - totalReleased,
		"memory_usage":    bm.stats.totalMemory.Load(),
		"memory_usage_mb": bm.stats.totalMemory.Load() / 1024 / 1024,
		"peak_memory_mb":  bm.stats.peakMemory.Load() / 1024 / 1024,
		"pool_hits":       poolHits,
		"pool_misses":     poolMisses,
		"hit_rate":        hitRate,
		"gc_triggers":     bm.stats.gcTriggers.Load(),
		"pools":           poolStats,
		"config":          bm.config,
	}
}

// SmartBuffer 方法实现

// Data 获取数据
func (sb *SmartBuffer) Data() []byte {
	if sb.released {
		panic("访问已释放的缓冲区")
	}
	return sb.data
}

// Size 获取大小
func (sb *SmartBuffer) Size() int {
	return sb.size
}

// Capacity 获取容量
func (sb *SmartBuffer) Capacity() int {
	return sb.capacity
}

// Resize 调整大小
func (sb *SmartBuffer) Resize(newSize int) error {
	if sb.released {
		return fmt.Errorf("无法调整已释放的缓冲区大小")
	}

	if newSize > sb.capacity {
		return fmt.Errorf("新大小超过容量限制: %d > %d", newSize, sb.capacity)
	}

	sb.data = sb.data[:newSize]
	sb.size = newSize

	// 更新使用统计
	if sb.info != nil {
		sb.info.LastUsed = time.Now()
		sb.info.UseCount++
	}

	return nil
}

// Clone 克隆缓冲区
func (sb *SmartBuffer) Clone() *SmartBuffer {
	if sb.released {
		panic("无法克隆已释放的缓冲区")
	}

	sb.refCount.Add(1)

	return &SmartBuffer{
		data:     sb.data,
		size:     sb.size,
		capacity: sb.capacity,
		pool:     sb.pool,
		info:     sb.info,
		released: false,
		refCount: sb.refCount, // 共享同一个引用计数器指针
	}
}

// Release 释放缓冲区
func (sb *SmartBuffer) Release() {
	if sb.released {
		return
	}

	if sb.refCount.Add(-1) > 0 {
		return // 还有其他引用
	}

	sb.released = true

	if sb.pool != nil {
		// 归还到池中
		// 重新构造原始缓冲区指针
		originalBuffer := make([]byte, sb.capacity)
		copy(originalBuffer, sb.data)
		bufferPtr := &originalBuffer
		sb.pool.pool.Put(bufferPtr)

		// 更新统计
		sb.pool.released.Add(1)
		sb.pool.inUse.Add(-1)
		sb.pool.manager.stats.totalReleased.Add(1)

		// 更新使用信息
		if sb.info != nil {
			sb.info.LastUsed = time.Now()
		}
	} else {
		// 直接释放内存
		sb.pool.manager.stats.totalMemory.Add(-int64(sb.capacity))
		sb.pool.manager.stats.totalReleased.Add(1)
	}

	// 清空数据引用
	sb.data = nil
}

// Write 实现io.Writer接口
func (sb *SmartBuffer) Write(p []byte) (int, error) {
	if sb.released {
		return 0, fmt.Errorf("缓冲区已释放")
	}

	needed := len(p)
	if sb.size+needed > sb.capacity {
		return 0, fmt.Errorf("缓冲区容量不足")
	}

	copy(sb.data[sb.size:], p)
	sb.size += needed

	// 更新使用统计
	if sb.info != nil {
		sb.info.LastUsed = time.Now()
		sb.info.UseCount++
	}

	return needed, nil
}

// Read 实现io.Reader接口
func (sb *SmartBuffer) Read(p []byte) (int, error) {
	if sb.released {
		return 0, fmt.Errorf("缓冲区已释放")
	}

	n := copy(p, sb.data)

	// 更新使用统计
	if sb.info != nil {
		sb.info.LastUsed = time.Now()
		sb.info.UseCount++
	}

	return n, nil
}

// String 实现Stringer接口
func (sb *SmartBuffer) String() string {
	if sb.released {
		return "<released buffer>"
	}
	return fmt.Sprintf("<SmartBuffer size=%d capacity=%d>", sb.size, sb.capacity)
}

// GetBufferManager 全局缓冲区管理器（单例）
var (
	globalBufferManager *BufferManager
	globalBufferOnce    sync.Once
)

// GetGlobalBufferManager 获取全局缓冲区管理器
func GetGlobalBufferManager() *BufferManager {
	globalBufferOnce.Do(func() {
		globalBufferManager = NewBufferManager(DefaultBufferConfig())
	})
	return globalBufferManager
}

// GetBuffer 从全局管理器获取缓冲区（便捷函数）
func GetBuffer(size int) *SmartBuffer {
	return GetGlobalBufferManager().GetBuffer(size)
}
