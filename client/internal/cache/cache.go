package cache

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

var (
	ErrCacheNotFound = errors.New("cache not found")
	ErrCacheExpired  = errors.New("cache expired")
	ErrCacheFull     = errors.New("cache is full")
)

// CacheConfig 缓存配置
type CacheConfig struct {
	// 基本配置
	MaxSize       int64         `json:"max_size"`       // 最大缓存大小（字节）
	MaxEntries    int           `json:"max_entries"`    // 最大条目数
	DefaultTTL    time.Duration `json:"default_ttl"`    // 默认TTL
	CleanInterval time.Duration `json:"clean_interval"` // 清理间隔

	// 策略配置
	EvictionPolicy string  `json:"eviction_policy"` // 淘汰策略: lru, lfu, fifo, random
	LoadFactor     float64 `json:"load_factor"`     // 负载因子

	// 预取配置
	EnablePrefetch    bool    `json:"enable_prefetch"`    // 启用预取
	PrefetchThreshold float64 `json:"prefetch_threshold"` // 预取阈值
	PrefetchWindow    int     `json:"prefetch_window"`    // 预取窗口大小
	PrefetchWorkers   int     `json:"prefetch_workers"`   // 预取工作协程数

	// 性能配置
	EnableCompression bool `json:"enable_compression"` // 启用压缩
	CompressionLevel  int  `json:"compression_level"`  // 压缩级别
	EnableSharding    bool `json:"enable_sharding"`    // 启用分片
	ShardCount        int  `json:"shard_count"`        // 分片数量

	// 持久化配置
	EnablePersistence bool          `json:"enable_persistence"` // 启用持久化
	PersistencePath   string        `json:"persistence_path"`   // 持久化路径
	SyncInterval      time.Duration `json:"sync_interval"`      // 同步间隔
}

// DefaultCacheConfig 默认缓存配置
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxSize:           100 * 1024 * 1024, // 100MB
		MaxEntries:        10000,
		DefaultTTL:        1 * time.Hour,
		CleanInterval:     5 * time.Minute,
		EvictionPolicy:    "lru",
		LoadFactor:        0.75,
		EnablePrefetch:    true,
		PrefetchThreshold: 0.8,
		PrefetchWindow:    10,
		PrefetchWorkers:   2,
		EnableCompression: true,
		CompressionLevel:  6,
		EnableSharding:    true,
		ShardCount:        16,
		EnablePersistence: false,
		PersistencePath:   "/tmp/cache",
		SyncInterval:      30 * time.Second,
	}
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Key         string
	Value       []byte
	Size        int64
	TTL         time.Duration
	CreatedAt   time.Time
	AccessedAt  time.Time
	AccessCount int64
	Compressed  bool

	// LRU链表节点
	prev *CacheEntry
	next *CacheEntry
}

// IsExpired 检查是否过期
func (e *CacheEntry) IsExpired() bool {
	if e.TTL <= 0 {
		return false
	}
	return time.Since(e.CreatedAt) > e.TTL
}

// Age 获取年龄
func (e *CacheEntry) Age() time.Duration {
	return time.Since(e.CreatedAt)
}

// SmartCache 智能缓存
type SmartCache struct {
	config *CacheConfig
	logger *zap.Logger

	// 分片缓存
	shards    []*CacheShard
	shardMask uint64

	// 预取器
	prefetcher *Prefetcher

	// 统计信息
	stats struct {
		hits         atomic.Int64
		misses       atomic.Int64
		evictions    atomic.Int64
		prefetches   atomic.Int64
		currentSize  atomic.Int64
		currentCount atomic.Int64
	}

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// CacheShard 缓存分片
type CacheShard struct {
	mutex       sync.RWMutex
	entries     map[string]*CacheEntry
	lruHead     *CacheEntry
	lruTail     *CacheEntry
	currentSize int64
	maxSize     int64
	maxEntries  int
	policy      EvictionPolicy
}

// EvictionPolicy 淘汰策略接口
type EvictionPolicy interface {
	OnAccess(entry *CacheEntry)
	OnAdd(entry *CacheEntry)
	OnEvict() *CacheEntry
	Name() string
}

// NewSmartCache 创建智能缓存
func NewSmartCache(config *CacheConfig) *SmartCache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	shardCount := config.ShardCount
	if shardCount <= 0 {
		shardCount = 16
	}

	cache := &SmartCache{
		config:    config,
		logger:    zap.L().Named("smart-cache"),
		shards:    make([]*CacheShard, shardCount),
		shardMask: uint64(shardCount - 1),
	}

	// 初始化分片
	shardMaxSize := config.MaxSize / int64(shardCount)
	shardMaxEntries := config.MaxEntries / shardCount

	for i := 0; i < shardCount; i++ {
		cache.shards[i] = &CacheShard{
			entries:    make(map[string]*CacheEntry),
			maxSize:    shardMaxSize,
			maxEntries: shardMaxEntries,
			policy:     createEvictionPolicy(config.EvictionPolicy),
		}
		cache.initLRUList(cache.shards[i])
	}

	// 初始化预取器
	if config.EnablePrefetch {
		cache.prefetcher = NewPrefetcher(config, cache)
	}

	return cache
}

// Start 启动缓存
func (sc *SmartCache) Start(ctx context.Context) error {
	sc.ctx, sc.cancel = context.WithCancel(ctx)

	// 启动清理协程
	sc.wg.Add(1)
	go sc.cleanupLoop()

	// 启动统计协程
	sc.wg.Add(1)
	go sc.statsLoop()

	// 启动预取器
	if sc.prefetcher != nil {
		if err := sc.prefetcher.Start(sc.ctx); err != nil {
			return fmt.Errorf("启动预取器失败: %w", err)
		}
	}

	sc.logger.Info("🗄️ 智能缓存启动",
		zap.Int64("max_size_mb", sc.config.MaxSize/1024/1024),
		zap.Int("max_entries", sc.config.MaxEntries),
		zap.String("eviction_policy", sc.config.EvictionPolicy),
		zap.Int("shard_count", len(sc.shards)),
		zap.Bool("prefetch_enabled", sc.config.EnablePrefetch))

	return nil
}

// Stop 停止缓存
func (sc *SmartCache) Stop() error {
	sc.logger.Info("🛑 正在停止智能缓存...")

	if sc.cancel != nil {
		sc.cancel()
	}

	// 停止预取器
	if sc.prefetcher != nil {
		if err := sc.prefetcher.Stop(); err != nil {
			sc.logger.Error("停止预取器失败", zap.Error(err))
		}
	}

	// 等待协程结束
	sc.wg.Wait()

	// 清理所有分片
	for _, shard := range sc.shards {
		shard.mutex.Lock()
		shard.entries = make(map[string]*CacheEntry)
		shard.currentSize = 0
		sc.initLRUList(shard)
		shard.mutex.Unlock()
	}

	sc.logger.Info("✅ 智能缓存已停止")
	return nil
}

// Get 获取缓存项
func (sc *SmartCache) Get(key string) ([]byte, error) {
	shard := sc.getShard(key)

	shard.mutex.RLock()
	entry, exists := shard.entries[key]
	shard.mutex.RUnlock()

	if !exists {
		sc.stats.misses.Add(1)

		// 触发预取
		if sc.prefetcher != nil {
			sc.prefetcher.OnMiss(key)
		}

		return nil, ErrCacheNotFound
	}

	// 检查过期
	if entry.IsExpired() {
		sc.Delete(key)
		sc.stats.misses.Add(1)
		return nil, ErrCacheExpired
	}

	// 更新访问信息
	shard.mutex.Lock()
	entry.AccessedAt = time.Now()
	entry.AccessCount++
	shard.policy.OnAccess(entry)
	sc.moveToFront(shard, entry)
	shard.mutex.Unlock()

	sc.stats.hits.Add(1)

	// 触发预取
	if sc.prefetcher != nil {
		sc.prefetcher.OnHit(key)
	}

	// 解压缩（如果需要）
	if entry.Compressed {
		return sc.decompress(entry.Value)
	}

	return entry.Value, nil
}

// Set 设置缓存项
func (sc *SmartCache) Set(key string, value []byte, ttl time.Duration) error {
	if len(value) == 0 {
		return fmt.Errorf("缓存值不能为空")
	}

	// 压缩（如果启用）
	finalValue := value
	compressed := false
	if sc.config.EnableCompression && len(value) > 1024 {
		if compressedValue, err := sc.compress(value); err == nil && len(compressedValue) < len(value) {
			finalValue = compressedValue
			compressed = true
		}
	}

	entry := &CacheEntry{
		Key:         key,
		Value:       finalValue,
		Size:        int64(len(finalValue)),
		TTL:         ttl,
		CreatedAt:   time.Now(),
		AccessedAt:  time.Now(),
		AccessCount: 1,
		Compressed:  compressed,
	}

	if ttl <= 0 {
		entry.TTL = sc.config.DefaultTTL
	}

	shard := sc.getShard(key)

	shard.mutex.Lock()
	defer shard.mutex.Unlock()

	// 检查是否已存在
	if oldEntry, exists := shard.entries[key]; exists {
		// 更新现有条目
		shard.currentSize -= oldEntry.Size
		sc.removeFromLRU(shard, oldEntry)
	}

	// 检查容量限制
	if err := sc.ensureCapacity(shard, entry.Size); err != nil {
		return err
	}

	// 添加新条目
	shard.entries[key] = entry
	shard.currentSize += entry.Size
	shard.policy.OnAdd(entry)
	sc.addToFront(shard, entry)

	// 更新统计
	sc.stats.currentSize.Add(entry.Size)
	sc.stats.currentCount.Add(1)

	return nil
}

// Delete 删除缓存项
func (sc *SmartCache) Delete(key string) error {
	shard := sc.getShard(key)

	shard.mutex.Lock()
	defer shard.mutex.Unlock()

	entry, exists := shard.entries[key]
	if !exists {
		return ErrCacheNotFound
	}

	delete(shard.entries, key)
	shard.currentSize -= entry.Size
	sc.removeFromLRU(shard, entry)

	// 更新统计
	sc.stats.currentSize.Add(-entry.Size)
	sc.stats.currentCount.Add(-1)

	return nil
}

// Exists 检查缓存项是否存在
func (sc *SmartCache) Exists(key string) bool {
	shard := sc.getShard(key)

	shard.mutex.RLock()
	entry, exists := shard.entries[key]
	shard.mutex.RUnlock()

	if !exists {
		return false
	}

	return !entry.IsExpired()
}

// Clear 清空缓存
func (sc *SmartCache) Clear() error {
	for _, shard := range sc.shards {
		shard.mutex.Lock()
		shard.entries = make(map[string]*CacheEntry)
		shard.currentSize = 0
		sc.initLRUList(shard)
		shard.mutex.Unlock()
	}

	// 重置统计
	sc.stats.currentSize.Store(0)
	sc.stats.currentCount.Store(0)

	sc.logger.Info("🧹 缓存已清空")
	return nil
}

// getShard 获取分片
func (sc *SmartCache) getShard(key string) *CacheShard {
	hash := sc.hash(key)
	return sc.shards[hash&sc.shardMask]
}

// hash 计算哈希值
func (sc *SmartCache) hash(key string) uint64 {
	h := md5.Sum([]byte(key))
	return uint64(h[0])<<56 | uint64(h[1])<<48 | uint64(h[2])<<40 | uint64(h[3])<<32 |
		uint64(h[4])<<24 | uint64(h[5])<<16 | uint64(h[6])<<8 | uint64(h[7])
}

// ensureCapacity 确保容量足够
func (sc *SmartCache) ensureCapacity(shard *CacheShard, newSize int64) error {
	// 检查单个条目大小
	if newSize > shard.maxSize {
		return fmt.Errorf("条目大小超过分片限制: %d > %d", newSize, shard.maxSize)
	}

	// 检查条目数量
	if len(shard.entries) >= shard.maxEntries {
		if err := sc.evictEntries(shard, 1); err != nil {
			return err
		}
	}

	// 检查内存大小
	for shard.currentSize+newSize > shard.maxSize {
		if err := sc.evictEntries(shard, 1); err != nil {
			return err
		}
	}

	return nil
}

// evictEntries 淘汰条目
func (sc *SmartCache) evictEntries(shard *CacheShard, count int) error {
	for i := 0; i < count; i++ {
		entry := shard.policy.OnEvict()
		if entry == nil {
			// 尝试从LRU尾部淘汰
			entry = shard.lruTail
			if entry == nil {
				return ErrCacheFull
			}
		}

		delete(shard.entries, entry.Key)
		shard.currentSize -= entry.Size
		sc.removeFromLRU(shard, entry)

		sc.stats.evictions.Add(1)
		sc.stats.currentSize.Add(-entry.Size)
		sc.stats.currentCount.Add(-1)
	}

	return nil
}

// LRU链表操作
func (sc *SmartCache) initLRUList(shard *CacheShard) {
	shard.lruHead = &CacheEntry{}
	shard.lruTail = &CacheEntry{}
	shard.lruHead.next = shard.lruTail
	shard.lruTail.prev = shard.lruHead
}

func (sc *SmartCache) addToFront(shard *CacheShard, entry *CacheEntry) {
	entry.next = shard.lruHead.next
	entry.prev = shard.lruHead
	shard.lruHead.next.prev = entry
	shard.lruHead.next = entry
}

func (sc *SmartCache) removeFromLRU(shard *CacheShard, entry *CacheEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	}
	entry.prev = nil
	entry.next = nil
}

func (sc *SmartCache) moveToFront(shard *CacheShard, entry *CacheEntry) {
	sc.removeFromLRU(shard, entry)
	sc.addToFront(shard, entry)
}

// 压缩和解压缩
func (sc *SmartCache) compress(data []byte) ([]byte, error) {
	// 简化的压缩实现
	// 实际应用中会使用gzip、lz4等压缩算法
	return data, nil
}

func (sc *SmartCache) decompress(data []byte) ([]byte, error) {
	// 简化的解压缩实现
	return data, nil
}

// cleanupLoop 清理循环
func (sc *SmartCache) cleanupLoop() {
	defer sc.wg.Done()

	ticker := time.NewTicker(sc.config.CleanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sc.cleanup()
		case <-sc.ctx.Done():
			return
		}
	}
}

// cleanup 清理过期条目
func (sc *SmartCache) cleanup() {
	var totalCleaned int64

	for _, shard := range sc.shards {
		shard.mutex.Lock()

		var toDelete []string
		for key, entry := range shard.entries {
			if entry.IsExpired() {
				toDelete = append(toDelete, key)
			}
		}

		for _, key := range toDelete {
			entry := shard.entries[key]
			delete(shard.entries, key)
			shard.currentSize -= entry.Size
			sc.removeFromLRU(shard, entry)
			totalCleaned++
		}

		shard.mutex.Unlock()
	}

	if totalCleaned > 0 {
		sc.stats.currentCount.Add(-totalCleaned)
		sc.logger.Debug("🧹 清理过期条目", zap.Int64("count", totalCleaned))
	}
}

// statsLoop 统计循环
func (sc *SmartCache) statsLoop() {
	defer sc.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sc.logStats()
		case <-sc.ctx.Done():
			return
		}
	}
}

// logStats 记录统计信息
func (sc *SmartCache) logStats() {
	stats := sc.GetStats()

	sc.logger.Info("📊 缓存统计",
		zap.Int64("hits", stats["hits"].(int64)),
		zap.Int64("misses", stats["misses"].(int64)),
		zap.Float64("hit_rate", stats["hit_rate"].(float64)),
		zap.Int64("entries", stats["entries"].(int64)),
		zap.Int64("size_mb", stats["size_mb"].(int64)),
		zap.Int64("evictions", stats["evictions"].(int64)))
}

// GetStats 获取统计信息
func (sc *SmartCache) GetStats() map[string]interface{} {
	hits := sc.stats.hits.Load()
	misses := sc.stats.misses.Load()

	var hitRate float64
	if hits+misses > 0 {
		hitRate = float64(hits) / float64(hits+misses)
	}

	stats := map[string]interface{}{
		"hits":       hits,
		"misses":     misses,
		"hit_rate":   hitRate,
		"evictions":  sc.stats.evictions.Load(),
		"prefetches": sc.stats.prefetches.Load(),
		"entries":    sc.stats.currentCount.Load(),
		"size":       sc.stats.currentSize.Load(),
		"size_mb":    sc.stats.currentSize.Load() / 1024 / 1024,
		"config":     sc.config,
	}

	// 添加预取器统计
	if sc.prefetcher != nil {
		stats["prefetcher"] = sc.prefetcher.GetStats()
	}

	return stats
}

// GetSize 获取当前大小
func (sc *SmartCache) GetSize() int64 {
	return sc.stats.currentSize.Load()
}

// GetCount 获取当前条目数
func (sc *SmartCache) GetCount() int64 {
	return sc.stats.currentCount.Load()
}

// 全局缓存实例
var (
	globalCache     *SmartCache
	globalCacheOnce sync.Once
)

// GetGlobalCache 获取全局缓存
func GetGlobalCache() *SmartCache {
	globalCacheOnce.Do(func() {
		globalCache = NewSmartCache(DefaultCacheConfig())
	})
	return globalCache
}
