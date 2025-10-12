package protocol

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// BatchConfig 批处理配置
type BatchConfig struct {
	// 批处理参数
	MaxBatchSize   int           `json:"max_batch_size"`   // 最大批次大小
	MaxBatchDelay  time.Duration `json:"max_batch_delay"`  // 最大批次延迟
	MaxMemoryUsage int           `json:"max_memory_usage"` // 最大内存使用
	FlushThreshold float64       `json:"flush_threshold"`  // 刷新阈值（内存使用率）

	// 并发控制
	MaxWorkers     int           `json:"max_workers"`      // 最大工作协程数
	QueueSize      int           `json:"queue_size"`       // 队列大小
	WorkerIdleTime time.Duration `json:"worker_idle_time"` // 工作协程空闲时间

	// 性能调优
	EnableCompression bool `json:"enable_compression"` // 启用压缩
	CompressionLevel  int  `json:"compression_level"`  // 压缩级别
	EnablePriority    bool `json:"enable_priority"`    // 启用优先级处理
	AdaptiveBatching  bool `json:"adaptive_batching"`  // 自适应批处理
}

// DefaultBatchConfig 默认批处理配置
func DefaultBatchConfig() *BatchConfig {
	return &BatchConfig{
		MaxBatchSize:      1000,
		MaxBatchDelay:     10 * time.Millisecond,
		MaxMemoryUsage:    128 * 1024 * 1024, // 128MB
		FlushThreshold:    0.8,               // 80%
		MaxWorkers:        4,
		QueueSize:         10000,
		WorkerIdleTime:    30 * time.Second,
		EnableCompression: true,
		CompressionLevel:  6,
		EnablePriority:    true,
		AdaptiveBatching:  true,
	}
}

// BatchProcessor 批处理器
type BatchProcessor struct {
	config *BatchConfig
	logger *zap.Logger

	// 工作队列
	workQueue  chan *BatchItem
	batchQueue chan *Batch

	// 工作协程管理
	workers      sync.WaitGroup
	batchWorkers sync.WaitGroup

	// 批次管理
	currentBatch *Batch
	batchMutex   sync.Mutex
	batchTimer   *time.Timer

	// 统计信息
	stats struct {
		itemsProcessed   atomic.Int64
		batchesProcessed atomic.Int64
		totalLatency     atomic.Int64
		avgBatchSize     atomic.Int64
		compressionRatio atomic.Int64 // * 1000 for precision
		memoryUsage      atomic.Int64
		queueLength      atomic.Int64
	}

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
}

// BatchItem 批处理项
type BatchItem struct {
	ID        string
	Data      []byte
	Priority  int
	Timestamp time.Time
	Metadata  map[string]interface{}
	Callback  func(item *BatchItem, result *BatchResult)

	// 内部字段
	size       int
	compressed bool
}

// Batch 批次
type Batch struct {
	ID        string
	Items     []*BatchItem
	CreatedAt time.Time
	Size      int
	Priority  int // 批次优先级（最高优先级项的优先级）

	// 统计
	ItemCount      int
	TotalSize      int
	CompressedSize int
	ProcessingTime time.Duration
}

// BatchResult 批处理结果
type BatchResult struct {
	Success        bool
	ProcessedCount int
	FailedCount    int
	TotalSize      int64
	ProcessingTime time.Duration
	Error          error
	Metadata       map[string]interface{}
}

// BatchHandler 批处理处理器接口
type BatchHandler interface {
	ProcessBatch(batch *Batch) *BatchResult
	GetHandlerType() string
}

// NewBatchProcessor 创建批处理器
func NewBatchProcessor(config *BatchConfig) *BatchProcessor {
	if config == nil {
		config = DefaultBatchConfig()
	}

	processor := &BatchProcessor{
		config:     config,
		logger:     zap.L().Named("batch-processor"),
		workQueue:  make(chan *BatchItem, config.QueueSize),
		batchQueue: make(chan *Batch, config.MaxWorkers*2),
	}

	return processor
}

// Start 启动批处理器
func (bp *BatchProcessor) Start(ctx context.Context) error {
	bp.ctx, bp.cancel = context.WithCancel(ctx)

	// 启动批次收集器
	bp.workers.Add(1)
	go bp.batchCollector()

	// 启动批处理工作协程
	for i := 0; i < bp.config.MaxWorkers; i++ {
		bp.batchWorkers.Add(1)
		go bp.batchWorker(i)
	}

	// 启动监控协程
	bp.workers.Add(1)
	go bp.monitorLoop()

	bp.logger.Info("📦 批处理器启动",
		zap.Int("max_batch_size", bp.config.MaxBatchSize),
		zap.Duration("max_delay", bp.config.MaxBatchDelay),
		zap.Int("workers", bp.config.MaxWorkers),
		zap.Bool("compression", bp.config.EnableCompression))

	return nil
}

// Stop 停止批处理器
func (bp *BatchProcessor) Stop() error {
	bp.logger.Info("🛑 正在停止批处理器...")

	if bp.cancel != nil {
		bp.cancel()
	}

	// 处理剩余的批次
	bp.flushCurrentBatch()

	// 关闭队列
	close(bp.workQueue)

	// 等待工作协程结束
	bp.workers.Wait()

	// 关闭批处理队列
	close(bp.batchQueue)
	bp.batchWorkers.Wait()

	bp.logger.Info("✅ 批处理器已停止")
	return nil
}

// Submit 提交批处理项
func (bp *BatchProcessor) Submit(item *BatchItem) error {
	if item == nil {
		return fmt.Errorf("批处理项不能为空")
	}

	// 设置默认值
	if item.ID == "" {
		item.ID = fmt.Sprintf("item-%d", time.Now().UnixNano())
	}
	if item.Timestamp.IsZero() {
		item.Timestamp = time.Now()
	}
	if item.size == 0 {
		item.size = len(item.Data)
	}

	// 更新队列长度统计
	bp.stats.queueLength.Store(int64(len(bp.workQueue)))

	select {
	case bp.workQueue <- item:
		return nil
	case <-bp.ctx.Done():
		return bp.ctx.Err()
	default:
		return fmt.Errorf("工作队列已满")
	}
}

// SubmitAsync 异步提交批处理项
func (bp *BatchProcessor) SubmitAsync(item *BatchItem) {
	go func() {
		if err := bp.Submit(item); err != nil {
			bp.logger.Error("异步提交失败",
				zap.String("item_id", item.ID),
				zap.Error(err))

			if item.Callback != nil {
				item.Callback(item, &BatchResult{
					Success: false,
					Error:   err,
				})
			}
		}
	}()
}

// batchCollector 批次收集器
func (bp *BatchProcessor) batchCollector() {
	defer bp.workers.Done()

	bp.logger.Debug("启动批次收集器")

	for {
		select {
		case item := <-bp.workQueue:
			bp.addToBatch(item)

		case <-bp.ctx.Done():
			bp.flushCurrentBatch()
			return
		}
	}
}

// addToBatch 添加到批次
func (bp *BatchProcessor) addToBatch(item *BatchItem) {
	bp.batchMutex.Lock()
	defer bp.batchMutex.Unlock()

	// 创建新批次（如果需要）
	if bp.currentBatch == nil {
		bp.createNewBatch()
	}

	// 压缩数据（如果启用）
	if bp.config.EnableCompression && !item.compressed {
		bp.compressItem(item)
	}

	// 添加到当前批次
	bp.currentBatch.Items = append(bp.currentBatch.Items, item)
	bp.currentBatch.ItemCount++
	bp.currentBatch.TotalSize += item.size

	// 更新批次优先级
	if bp.config.EnablePriority && item.Priority > bp.currentBatch.Priority {
		bp.currentBatch.Priority = item.Priority
	}

	// 检查是否需要刷新批次
	shouldFlush := bp.shouldFlushBatch()
	if shouldFlush {
		bp.flushCurrentBatchUnsafe()
	}
}

// createNewBatch 创建新批次
func (bp *BatchProcessor) createNewBatch() {
	bp.currentBatch = &Batch{
		ID:        fmt.Sprintf("batch-%d", time.Now().UnixNano()),
		Items:     make([]*BatchItem, 0, bp.config.MaxBatchSize),
		CreatedAt: time.Now(),
		Priority:  0,
	}

	// 设置刷新定时器
	if bp.batchTimer != nil {
		bp.batchTimer.Stop()
	}
	bp.batchTimer = time.AfterFunc(bp.config.MaxBatchDelay, func() {
		bp.batchMutex.Lock()
		bp.flushCurrentBatchUnsafe()
		bp.batchMutex.Unlock()
	})
}

// shouldFlushBatch 检查是否应该刷新批次
func (bp *BatchProcessor) shouldFlushBatch() bool {
	if bp.currentBatch == nil {
		return false
	}

	// 检查批次大小
	if bp.currentBatch.ItemCount >= bp.config.MaxBatchSize {
		return true
	}

	// 检查内存使用
	currentMemory := bp.stats.memoryUsage.Load()
	if float64(currentMemory) > float64(bp.config.MaxMemoryUsage)*bp.config.FlushThreshold {
		return true
	}

	// 自适应批处理
	if bp.config.AdaptiveBatching {
		return bp.shouldAdaptiveFlush()
	}

	return false
}

// shouldAdaptiveFlush 自适应刷新判断
func (bp *BatchProcessor) shouldAdaptiveFlush() bool {
	// 基于队列长度的自适应策略
	queueLen := bp.stats.queueLength.Load()
	avgBatchSize := bp.stats.avgBatchSize.Load()

	// 如果队列很长，使用较小的批次以减少延迟
	if queueLen > int64(bp.config.QueueSize/2) {
		return bp.currentBatch.ItemCount >= int(avgBatchSize/2)
	}

	// 如果队列很短，等待更大的批次以提高效率
	if queueLen < int64(bp.config.QueueSize/10) {
		return false
	}

	return false
}

// flushCurrentBatch 刷新当前批次
func (bp *BatchProcessor) flushCurrentBatch() {
	bp.batchMutex.Lock()
	defer bp.batchMutex.Unlock()
	bp.flushCurrentBatchUnsafe()
}

// flushCurrentBatchUnsafe 刷新当前批次（不安全版本）
func (bp *BatchProcessor) flushCurrentBatchUnsafe() {
	if bp.currentBatch == nil || bp.currentBatch.ItemCount == 0 {
		return
	}

	// 停止定时器
	if bp.batchTimer != nil {
		bp.batchTimer.Stop()
		bp.batchTimer = nil
	}

	// 发送批次到处理队列
	select {
	case bp.batchQueue <- bp.currentBatch:
		bp.logger.Debug("批次已发送到处理队列",
			zap.String("batch_id", bp.currentBatch.ID),
			zap.Int("item_count", bp.currentBatch.ItemCount),
			zap.Int("total_size", bp.currentBatch.TotalSize))

	case <-bp.ctx.Done():
		bp.logger.Warn("上下文已取消，丢弃批次",
			zap.String("batch_id", bp.currentBatch.ID))
	default:
		bp.logger.Warn("批处理队列已满，丢弃批次",
			zap.String("batch_id", bp.currentBatch.ID))
	}

	// 更新统计
	bp.updateBatchStats(bp.currentBatch)

	// 重置当前批次
	bp.currentBatch = nil
}

// batchWorker 批处理工作协程
func (bp *BatchProcessor) batchWorker(id int) {
	defer bp.batchWorkers.Done()

	bp.logger.Debug("启动批处理工作协程", zap.Int("worker_id", id))

	for batch := range bp.batchQueue {
		start := time.Now()
		result := bp.processBatch(batch)
		batch.ProcessingTime = time.Since(start)

		// 更新统计
		bp.stats.batchesProcessed.Add(1)
		bp.stats.totalLatency.Add(batch.ProcessingTime.Nanoseconds())

		// 调用回调
		bp.callItemCallbacks(batch, result)

		bp.logger.Debug("批次处理完成",
			zap.String("batch_id", batch.ID),
			zap.Int("item_count", batch.ItemCount),
			zap.Duration("processing_time", batch.ProcessingTime),
			zap.Bool("success", result.Success))
	}

	bp.logger.Debug("批处理工作协程结束", zap.Int("worker_id", id))
}

// processBatch 处理批次
func (bp *BatchProcessor) processBatch(batch *Batch) *BatchResult {
	result := &BatchResult{
		Success:        true,
		ProcessedCount: 0,
		FailedCount:    0,
		TotalSize:      int64(batch.TotalSize),
		ProcessingTime: 0,
		Metadata:       make(map[string]interface{}),
	}

	start := time.Now()
	defer func() {
		result.ProcessingTime = time.Since(start)
	}()

	// 处理每个项目
	for _, item := range batch.Items {
		if err := bp.processItem(item); err != nil {
			result.FailedCount++
			result.Success = false
			if result.Error == nil {
				result.Error = err
			}
			bp.logger.Error("处理项目失败",
				zap.String("item_id", item.ID),
				zap.Error(err))
		} else {
			result.ProcessedCount++
			bp.stats.itemsProcessed.Add(1)
		}
	}

	return result
}

// processItem 处理单个项目
func (bp *BatchProcessor) processItem(item *BatchItem) error {
	// 这里是具体的处理逻辑
	// 实际应用中，这会根据项目类型进行不同的处理

	// 模拟处理时间
	if item.size > 1024 {
		time.Sleep(time.Microsecond * time.Duration(item.size/1024))
	}

	return nil
}

// callItemCallbacks 调用项目回调
func (bp *BatchProcessor) callItemCallbacks(batch *Batch, result *BatchResult) {
	for i, item := range batch.Items {
		if item.Callback != nil {
			itemResult := &BatchResult{
				Success:        i < result.ProcessedCount,
				ProcessedCount: 1,
				TotalSize:      int64(item.size),
				ProcessingTime: result.ProcessingTime / time.Duration(len(batch.Items)),
				Error:          result.Error,
			}

			go item.Callback(item, itemResult)
		}
	}
}

// compressItem 压缩项目数据
func (bp *BatchProcessor) compressItem(item *BatchItem) {
	// 简化的压缩实现
	// 实际应用中会使用真正的压缩算法
	if len(item.Data) > 128 {
		// 模拟压缩
		originalSize := len(item.Data)
		compressedSize := originalSize * 7 / 10 // 假设70%的压缩率

		// 更新压缩统计
		ratio := int64(compressedSize * 1000 / originalSize)
		bp.stats.compressionRatio.Store(ratio)

		item.size = compressedSize
		item.compressed = true

		bp.logger.Debug("数据已压缩",
			zap.String("item_id", item.ID),
			zap.Int("original_size", originalSize),
			zap.Int("compressed_size", compressedSize))
	}
}

// updateBatchStats 更新批次统计
func (bp *BatchProcessor) updateBatchStats(batch *Batch) {
	// 更新平均批次大小
	oldAvg := bp.stats.avgBatchSize.Load()
	newAvg := (oldAvg*9 + int64(batch.ItemCount)) / 10
	bp.stats.avgBatchSize.Store(newAvg)

	// 更新内存使用
	bp.stats.memoryUsage.Add(int64(batch.TotalSize))
}

// monitorLoop 监控循环
func (bp *BatchProcessor) monitorLoop() {
	defer bp.workers.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bp.logStats()
			bp.adjustParameters()

		case <-bp.ctx.Done():
			return
		}
	}
}

// logStats 记录统计信息
func (bp *BatchProcessor) logStats() {
	stats := bp.GetStats()

	bp.logger.Info("📊 批处理器统计",
		zap.Int64("items_processed", stats["items_processed"].(int64)),
		zap.Int64("batches_processed", stats["batches_processed"].(int64)),
		zap.Float64("avg_batch_size", stats["avg_batch_size"].(float64)),
		zap.Float64("avg_latency_ms", stats["avg_latency_ms"].(float64)),
		zap.Int64("queue_length", stats["queue_length"].(int64)),
		zap.Float64("compression_ratio", stats["compression_ratio"].(float64)))
}

// adjustParameters 调整参数
func (bp *BatchProcessor) adjustParameters() {
	if !bp.config.AdaptiveBatching {
		return
	}

	queueLen := bp.stats.queueLength.Load()
	avgLatency := float64(bp.stats.totalLatency.Load()) / 1e6 // 转换为毫秒

	// 如果队列积压严重，减小批次大小
	if queueLen > int64(bp.config.QueueSize*3/4) {
		newSize := bp.config.MaxBatchSize * 3 / 4
		if newSize > 10 {
			bp.config.MaxBatchSize = newSize
			bp.logger.Info("🔧 调整批次大小（减小）", zap.Int("new_size", newSize))
		}
	}

	// 如果延迟过高，减小批次延迟
	if avgLatency > 100 { // 100ms
		newDelay := bp.config.MaxBatchDelay * 3 / 4
		if newDelay > time.Millisecond {
			bp.config.MaxBatchDelay = newDelay
			bp.logger.Info("🔧 调整批次延迟（减小）", zap.Duration("new_delay", newDelay))
		}
	}
}

// GetStats 获取统计信息
func (bp *BatchProcessor) GetStats() map[string]interface{} {
	batchesProcessed := bp.stats.batchesProcessed.Load()
	var avgBatchSize float64
	var avgLatencyMs float64

	if batchesProcessed > 0 {
		avgBatchSize = float64(bp.stats.itemsProcessed.Load()) / float64(batchesProcessed)
		avgLatencyMs = float64(bp.stats.totalLatency.Load()) / float64(batchesProcessed) / 1e6
	}

	return map[string]interface{}{
		"items_processed":    bp.stats.itemsProcessed.Load(),
		"batches_processed":  batchesProcessed,
		"avg_batch_size":     avgBatchSize,
		"avg_latency_ms":     avgLatencyMs,
		"compression_ratio":  float64(bp.stats.compressionRatio.Load()) / 1000.0,
		"memory_usage":       bp.stats.memoryUsage.Load(),
		"queue_length":       bp.stats.queueLength.Load(),
		"current_batch_size": bp.getCurrentBatchSize(),
		"config":             bp.config,
	}
}

// getCurrentBatchSize 获取当前批次大小
func (bp *BatchProcessor) getCurrentBatchSize() int {
	bp.batchMutex.Lock()
	defer bp.batchMutex.Unlock()

	if bp.currentBatch != nil {
		return bp.currentBatch.ItemCount
	}
	return 0
}

// CreateBatchItem 创建批处理项（辅助函数）
func CreateBatchItem(data []byte, priority int, callback func(*BatchItem, *BatchResult)) *BatchItem {
	return &BatchItem{
		ID:        fmt.Sprintf("item-%d", time.Now().UnixNano()),
		Data:      data,
		Priority:  priority,
		Timestamp: time.Now(),
		Callback:  callback,
		size:      len(data),
	}
}

// BatchWriter 批处理写入器
type BatchWriter struct {
	processor *BatchProcessor
	priority  int
	metadata  map[string]interface{}
}

// NewBatchWriter 创建批处理写入器
func NewBatchWriter(processor *BatchProcessor, priority int) *BatchWriter {
	return &BatchWriter{
		processor: processor,
		priority:  priority,
		metadata:  make(map[string]interface{}),
	}
}

// Write 实现io.Writer接口
func (bw *BatchWriter) Write(data []byte) (int, error) {
	item := &BatchItem{
		ID:        fmt.Sprintf("write-%d", time.Now().UnixNano()),
		Data:      make([]byte, len(data)),
		Priority:  bw.priority,
		Timestamp: time.Now(),
		Metadata:  bw.metadata,
		size:      len(data),
	}

	copy(item.Data, data)

	if err := bw.processor.Submit(item); err != nil {
		return 0, err
	}

	return len(data), nil
}

// SetMetadata 设置元数据
func (bw *BatchWriter) SetMetadata(key string, value interface{}) {
	bw.metadata[key] = value
}

// 确保实现了接口
var _ io.Writer = (*BatchWriter)(nil)



