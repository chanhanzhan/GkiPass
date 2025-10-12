package cache

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// PrefetchStrategy 预取策略
type PrefetchStrategy interface {
	// ShouldPrefetch 判断是否应该预取
	ShouldPrefetch(key string, accessPattern *AccessPattern) bool

	// GeneratePrefetchKeys 生成预取键
	GeneratePrefetchKeys(key string, accessPattern *AccessPattern) []string

	// GetStrategyName 获取策略名称
	GetStrategyName() string
}

// AccessPattern 访问模式
type AccessPattern struct {
	Key          string
	AccessCount  int64
	LastAccess   time.Time
	AccessTimes  []time.Time
	Neighbors    []string      // 相关的键
	AccessRate   float64       // 访问频率
	TimeInterval time.Duration // 平均访问间隔
}

// Prefetcher 预取器
type Prefetcher struct {
	config *CacheConfig
	cache  *SmartCache
	logger *zap.Logger

	// 访问模式跟踪
	patterns   map[string]*AccessPattern
	patternMux sync.RWMutex

	// 预取策略
	strategies []PrefetchStrategy

	// 工作队列
	prefetchQueue chan *PrefetchRequest
	workers       []chan struct{} // 用于停止worker

	// 统计信息
	stats struct {
		prefetchRequests atomic.Int64
		prefetchHits     atomic.Int64
		prefetchMisses   atomic.Int64
		prefetchWasted   atomic.Int64 // 预取但未使用的项
		patternUpdates   atomic.Int64
		totalPrefetched  atomic.Int64
	}

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// PrefetchRequest 预取请求
type PrefetchRequest struct {
	Key       string
	Priority  int
	Strategy  string
	Metadata  map[string]interface{}
	Callback  func(key string, success bool, err error)
	CreatedAt time.Time
}

// NewPrefetcher 创建预取器
func NewPrefetcher(config *CacheConfig, cache *SmartCache) *Prefetcher {
	prefetcher := &Prefetcher{
		config:        config,
		cache:         cache,
		logger:        zap.L().Named("prefetcher"),
		patterns:      make(map[string]*AccessPattern),
		prefetchQueue: make(chan *PrefetchRequest, config.PrefetchWorkers*10),
		workers:       make([]chan struct{}, config.PrefetchWorkers),
	}

	// 初始化预取策略
	prefetcher.strategies = []PrefetchStrategy{
		NewSequentialPrefetchStrategy(),
		NewSpatialPrefetchStrategy(),
		NewTemporalPrefetchStrategy(),
		NewMLPrefetchStrategy(),
	}

	return prefetcher
}

// Start 启动预取器
func (p *Prefetcher) Start(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	// 启动工作协程
	for i := 0; i < p.config.PrefetchWorkers; i++ {
		stopChan := make(chan struct{})
		p.workers[i] = stopChan

		p.wg.Add(1)
		go p.worker(i, stopChan)
	}

	// 启动模式分析协程
	p.wg.Add(1)
	go p.patternAnalyzer()

	// 启动清理协程
	p.wg.Add(1)
	go p.cleanup()

	p.logger.Info("🔮 预取器启动",
		zap.Int("workers", p.config.PrefetchWorkers),
		zap.Int("strategies", len(p.strategies)),
		zap.Float64("threshold", p.config.PrefetchThreshold))

	return nil
}

// Stop 停止预取器
func (p *Prefetcher) Stop() error {
	p.logger.Info("🛑 正在停止预取器...")

	if p.cancel != nil {
		p.cancel()
	}

	// 停止所有工作协程
	for _, stopChan := range p.workers {
		close(stopChan)
	}

	// 关闭预取队列
	close(p.prefetchQueue)

	// 等待所有协程结束
	p.wg.Wait()

	p.logger.Info("✅ 预取器已停止")
	return nil
}

// OnHit 处理缓存命中
func (p *Prefetcher) OnHit(key string) {
	p.updateAccessPattern(key, true)
	p.triggerPrefetch(key)
}

// OnMiss 处理缓存未命中
func (p *Prefetcher) OnMiss(key string) {
	p.updateAccessPattern(key, false)
	p.triggerPrefetch(key)
}

// updateAccessPattern 更新访问模式
func (p *Prefetcher) updateAccessPattern(key string, hit bool) {
	p.patternMux.Lock()
	defer p.patternMux.Unlock()

	pattern, exists := p.patterns[key]
	if !exists {
		pattern = &AccessPattern{
			Key:         key,
			AccessTimes: make([]time.Time, 0, 10),
			Neighbors:   make([]string, 0),
		}
		p.patterns[key] = pattern
	}

	now := time.Now()
	pattern.AccessCount++
	pattern.LastAccess = now
	pattern.AccessTimes = append(pattern.AccessTimes, now)

	// 保持最近的访问时间
	if len(pattern.AccessTimes) > 10 {
		pattern.AccessTimes = pattern.AccessTimes[1:]
	}

	// 计算访问频率和间隔
	if len(pattern.AccessTimes) > 1 {
		totalInterval := pattern.AccessTimes[len(pattern.AccessTimes)-1].Sub(pattern.AccessTimes[0])
		pattern.TimeInterval = totalInterval / time.Duration(len(pattern.AccessTimes)-1)
		pattern.AccessRate = float64(len(pattern.AccessTimes)) / totalInterval.Seconds()
	}

	p.stats.patternUpdates.Add(1)
}

// triggerPrefetch 触发预取
func (p *Prefetcher) triggerPrefetch(key string) {
	p.patternMux.RLock()
	pattern, exists := p.patterns[key]
	p.patternMux.RUnlock()

	if !exists {
		return
	}

	// 使用各种策略生成预取请求
	for _, strategy := range p.strategies {
		if strategy.ShouldPrefetch(key, pattern) {
			prefetchKeys := strategy.GeneratePrefetchKeys(key, pattern)

			for _, prefetchKey := range prefetchKeys {
				// 检查是否已在缓存中
				if p.cache.Exists(prefetchKey) {
					continue
				}

				request := &PrefetchRequest{
					Key:       prefetchKey,
					Priority:  p.calculatePriority(prefetchKey, pattern),
					Strategy:  strategy.GetStrategyName(),
					Metadata:  map[string]interface{}{"original_key": key},
					CreatedAt: time.Now(),
				}

				select {
				case p.prefetchQueue <- request:
					p.stats.prefetchRequests.Add(1)
				default:
					// 队列满，丢弃请求
				}
			}
		}
	}
}

// calculatePriority 计算预取优先级
func (p *Prefetcher) calculatePriority(key string, pattern *AccessPattern) int {
	priority := 5 // 默认优先级

	// 基于访问频率调整
	if pattern.AccessRate > 1.0 { // 每秒超过1次
		priority += 3
	} else if pattern.AccessRate > 0.1 { // 每秒超过0.1次
		priority += 1
	}

	// 基于访问间隔调整
	if pattern.TimeInterval < 1*time.Minute {
		priority += 2
	} else if pattern.TimeInterval < 5*time.Minute {
		priority += 1
	}

	return priority
}

// worker 预取工作协程
func (p *Prefetcher) worker(id int, stopChan chan struct{}) {
	defer p.wg.Done()

	p.logger.Debug("启动预取工作协程", zap.Int("worker_id", id))

	for {
		select {
		case request := <-p.prefetchQueue:
			p.processPrefetchRequest(request)

		case <-stopChan:
			p.logger.Debug("预取工作协程停止", zap.Int("worker_id", id))
			return

		case <-p.ctx.Done():
			return
		}
	}
}

// processPrefetchRequest 处理预取请求
func (p *Prefetcher) processPrefetchRequest(request *PrefetchRequest) {
	// 检查是否已在缓存中
	if p.cache.Exists(request.Key) {
		p.stats.prefetchHits.Add(1)
		if request.Callback != nil {
			request.Callback(request.Key, true, nil)
		}
		return
	}

	// 执行预取逻辑
	success, err := p.performPrefetch(request)

	if success {
		p.stats.prefetchHits.Add(1)
		p.stats.totalPrefetched.Add(1)
	} else {
		p.stats.prefetchMisses.Add(1)
	}

	if request.Callback != nil {
		request.Callback(request.Key, success, err)
	}

	p.logger.Debug("预取请求处理完成",
		zap.String("key", request.Key),
		zap.String("strategy", request.Strategy),
		zap.Bool("success", success),
		zap.Error(err))
}

// performPrefetch 执行预取
func (p *Prefetcher) performPrefetch(request *PrefetchRequest) (bool, error) {
	// 这里是预取的具体实现
	// 实际应用中，这里会调用数据源获取数据
	// 为了演示，我们模拟一个简单的预取逻辑

	// 模拟数据获取
	data := []byte(fmt.Sprintf("prefetched-data-for-%s", request.Key))

	// 设置较短的TTL用于预取数据
	prefetchTTL := p.config.DefaultTTL / 2

	// 存储到缓存
	err := p.cache.Set(request.Key, data, prefetchTTL)
	return err == nil, err
}

// patternAnalyzer 模式分析器
func (p *Prefetcher) patternAnalyzer() {
	defer p.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.analyzePatterns()
		case <-p.ctx.Done():
			return
		}
	}
}

// analyzePatterns 分析访问模式
func (p *Prefetcher) analyzePatterns() {
	p.patternMux.Lock()
	defer p.patternMux.Unlock()

	now := time.Now()
	var activePatterns int

	// 分析所有模式，寻找相关性
	for key, pattern := range p.patterns {
		// 检查模式是否过期
		if now.Sub(pattern.LastAccess) > 1*time.Hour {
			delete(p.patterns, key)
			continue
		}

		activePatterns++

		// 寻找相关的键（简单的前缀匹配）
		p.findRelatedKeys(pattern)
	}

	p.logger.Debug("模式分析完成", zap.Int("active_patterns", activePatterns))
}

// findRelatedKeys 寻找相关键
func (p *Prefetcher) findRelatedKeys(pattern *AccessPattern) {
	pattern.Neighbors = pattern.Neighbors[:0] // 清空

	// 简单的相关性分析：基于键的前缀
	keyPrefix := extractPrefix(pattern.Key)

	for key := range p.patterns {
		if key != pattern.Key && strings.HasPrefix(key, keyPrefix) {
			pattern.Neighbors = append(pattern.Neighbors, key)
			if len(pattern.Neighbors) >= 5 { // 限制邻居数量
				break
			}
		}
	}
}

// extractPrefix 提取键前缀
func extractPrefix(key string) string {
	// 简单实现：返回最后一个斜杠之前的部分
	if lastSlash := strings.LastIndex(key, "/"); lastSlash > 0 {
		return key[:lastSlash+1]
	}
	return key
}

// cleanup 清理过期数据
func (p *Prefetcher) cleanup() {
	defer p.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanupExpiredPatterns()
		case <-p.ctx.Done():
			return
		}
	}
}

// cleanupExpiredPatterns 清理过期模式
func (p *Prefetcher) cleanupExpiredPatterns() {
	p.patternMux.Lock()
	defer p.patternMux.Unlock()

	now := time.Now()
	var cleaned int

	for key, pattern := range p.patterns {
		if now.Sub(pattern.LastAccess) > 2*time.Hour {
			delete(p.patterns, key)
			cleaned++
		}
	}

	if cleaned > 0 {
		p.logger.Debug("清理过期访问模式", zap.Int("cleaned", cleaned))
	}
}

// GetStats 获取预取器统计
func (p *Prefetcher) GetStats() map[string]interface{} {
	p.patternMux.RLock()
	patternCount := len(p.patterns)
	p.patternMux.RUnlock()

	prefetchRequests := p.stats.prefetchRequests.Load()
	prefetchHits := p.stats.prefetchHits.Load()

	var prefetchHitRate float64
	if prefetchRequests > 0 {
		prefetchHitRate = float64(prefetchHits) / float64(prefetchRequests)
	}

	return map[string]interface{}{
		"prefetch_requests": prefetchRequests,
		"prefetch_hits":     prefetchHits,
		"prefetch_misses":   p.stats.prefetchMisses.Load(),
		"prefetch_hit_rate": prefetchHitRate,
		"prefetch_wasted":   p.stats.prefetchWasted.Load(),
		"pattern_updates":   p.stats.patternUpdates.Load(),
		"total_prefetched":  p.stats.totalPrefetched.Load(),
		"active_patterns":   patternCount,
		"queue_size":        len(p.prefetchQueue),
		"strategies":        len(p.strategies),
	}
}

// 预取策略实现

// SequentialPrefetchStrategy 顺序预取策略
type SequentialPrefetchStrategy struct{}

func NewSequentialPrefetchStrategy() *SequentialPrefetchStrategy {
	return &SequentialPrefetchStrategy{}
}

func (s *SequentialPrefetchStrategy) ShouldPrefetch(key string, pattern *AccessPattern) bool {
	// 如果访问频率较高，考虑顺序预取
	return pattern.AccessCount > 2 && pattern.AccessRate > 0.1
}

func (s *SequentialPrefetchStrategy) GeneratePrefetchKeys(key string, pattern *AccessPattern) []string {
	var keys []string

	// 尝试生成顺序键
	if strings.Contains(key, "_") {
		parts := strings.Split(key, "_")
		if len(parts) > 1 {
			// 尝试解析最后一部分为数字
			lastPart := parts[len(parts)-1]
			if len(lastPart) > 0 {
				// 简单的数字递增
				for i := 1; i <= 3; i++ {
					newKey := strings.Join(parts[:len(parts)-1], "_") + fmt.Sprintf("_%s%d", lastPart, i)
					keys = append(keys, newKey)
				}
			}
		}
	}

	return keys
}

func (s *SequentialPrefetchStrategy) GetStrategyName() string {
	return "sequential"
}

// SpatialPrefetchStrategy 空间预取策略
type SpatialPrefetchStrategy struct{}

func NewSpatialPrefetchStrategy() *SpatialPrefetchStrategy {
	return &SpatialPrefetchStrategy{}
}

func (s *SpatialPrefetchStrategy) ShouldPrefetch(key string, pattern *AccessPattern) bool {
	return len(pattern.Neighbors) > 0
}

func (s *SpatialPrefetchStrategy) GeneratePrefetchKeys(key string, pattern *AccessPattern) []string {
	// 返回相关的邻居键
	return pattern.Neighbors
}

func (s *SpatialPrefetchStrategy) GetStrategyName() string {
	return "spatial"
}

// TemporalPrefetchStrategy 时间预取策略
type TemporalPrefetchStrategy struct{}

func NewTemporalPrefetchStrategy() *TemporalPrefetchStrategy {
	return &TemporalPrefetchStrategy{}
}

func (s *TemporalPrefetchStrategy) ShouldPrefetch(key string, pattern *AccessPattern) bool {
	// 基于时间间隔的预测
	return pattern.TimeInterval > 0 && pattern.TimeInterval < 5*time.Minute
}

func (s *TemporalPrefetchStrategy) GeneratePrefetchKeys(key string, pattern *AccessPattern) []string {
	// 基于时间模式生成键
	// 这里简化实现
	var keys []string

	if strings.Contains(key, "daily") {
		// 预取明天的数据
		keys = append(keys, strings.Replace(key, "daily", "daily_tomorrow", 1))
	}

	return keys
}

func (s *TemporalPrefetchStrategy) GetStrategyName() string {
	return "temporal"
}

// MLPrefetchStrategy 机器学习预取策略
type MLPrefetchStrategy struct{}

func NewMLPrefetchStrategy() *MLPrefetchStrategy {
	return &MLPrefetchStrategy{}
}

func (s *MLPrefetchStrategy) ShouldPrefetch(key string, pattern *AccessPattern) bool {
	// 简化的ML决策：基于多个因素的综合评分
	score := 0.0

	// 访问频率因子
	if pattern.AccessRate > 1.0 {
		score += 0.4
	} else if pattern.AccessRate > 0.5 {
		score += 0.2
	}

	// 访问规律性因子
	if pattern.TimeInterval > 0 && pattern.TimeInterval < 10*time.Minute {
		score += 0.3
	}

	// 相关性因子
	if len(pattern.Neighbors) > 2 {
		score += 0.3
	}

	return score > 0.6
}

func (s *MLPrefetchStrategy) GeneratePrefetchKeys(key string, pattern *AccessPattern) []string {
	// 结合多种策略生成预取键
	keys := make([]string, 0)

	// 添加邻居键
	keys = append(keys, pattern.Neighbors...)

	// 添加模式推导的键
	if strings.Contains(key, "/") {
		parts := strings.Split(key, "/")
		if len(parts) > 1 {
			// 生成同级别的相关键
			base := strings.Join(parts[:len(parts)-1], "/")
			keys = append(keys, base+"/related_1", base+"/related_2")
		}
	}

	return keys
}

func (s *MLPrefetchStrategy) GetStrategyName() string {
	return "ml"
}



