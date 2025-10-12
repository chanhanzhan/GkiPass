package traffic

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// TokenBucket 令牌桶
type TokenBucket struct {
	capacity   int64 // 桶容量
	tokens     int64 // 当前令牌数
	rate       int64 // 填充速率 (tokens/second)
	lastRefill int64 // 上次填充时间戳 (nanoseconds)

	mutex  sync.Mutex
	logger *zap.Logger
}

// NewTokenBucket 创建令牌桶
func NewTokenBucket(capacity, rate int64) *TokenBucket {
	now := time.Now().UnixNano()

	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity, // 初始满桶
		rate:       rate,
		lastRefill: now,
		logger:     zap.L().Named("token-bucket"),
	}
}

// TryConsume 尝试消费令牌（非阻塞）
func (tb *TokenBucket) TryConsume(tokens int64) bool {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	tb.refill()

	if tb.tokens >= tokens {
		tb.tokens -= tokens
		tb.logger.Debug("消费令牌成功",
			zap.Int64("consumed", tokens),
			zap.Int64("remaining", tb.tokens))
		return true
	}

	tb.logger.Debug("令牌不足",
		zap.Int64("required", tokens),
		zap.Int64("available", tb.tokens))
	return false
}

// Consume 消费令牌（阻塞等待）
func (tb *TokenBucket) Consume(ctx context.Context, tokens int64) error {
	for {
		if tb.TryConsume(tokens) {
			return nil
		}

		// 计算等待时间
		waitTime := tb.calculateWaitTime(tokens)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// 继续尝试
		}
	}
}

// calculateWaitTime 计算等待时间
func (tb *TokenBucket) calculateWaitTime(tokens int64) time.Duration {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	tb.refill()

	if tb.tokens >= tokens {
		return 0
	}

	needed := tokens - tb.tokens
	if tb.rate <= 0 {
		return time.Hour // 如果没有填充速率，等待很长时间
	}

	waitSeconds := float64(needed) / float64(tb.rate)
	return time.Duration(waitSeconds * float64(time.Second))
}

// refill 填充令牌（需要在锁内调用）
func (tb *TokenBucket) refill() {
	now := time.Now().UnixNano()
	elapsed := now - tb.lastRefill

	if elapsed <= 0 {
		return
	}

	// 计算需要添加的令牌数
	tokensToAdd := int64(float64(elapsed) * float64(tb.rate) / float64(time.Second))

	if tokensToAdd > 0 {
		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now

		tb.logger.Debug("填充令牌",
			zap.Int64("added", tokensToAdd),
			zap.Int64("current", tb.tokens),
			zap.Int64("capacity", tb.capacity))
	}
}

// GetTokens 获取当前令牌数
func (tb *TokenBucket) GetTokens() int64 {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	tb.refill()
	return tb.tokens
}

// SetRate 设置填充速率
func (tb *TokenBucket) SetRate(rate int64) {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	tb.rate = rate
	tb.logger.Info("更新令牌桶速率", zap.Int64("rate", rate))
}

// SetCapacity 设置桶容量
func (tb *TokenBucket) SetCapacity(capacity int64) {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	tb.capacity = capacity
	if tb.tokens > capacity {
		tb.tokens = capacity
	}
	tb.logger.Info("更新令牌桶容量", zap.Int64("capacity", capacity))
}

// GetStats 获取统计信息
func (tb *TokenBucket) GetStats() map[string]interface{} {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	tb.refill()

	return map[string]interface{}{
		"capacity":    tb.capacity,
		"tokens":      tb.tokens,
		"rate":        tb.rate,
		"utilization": float64(tb.capacity-tb.tokens) / float64(tb.capacity) * 100,
	}
}

// RateLimiter 速率限制器
type RateLimiter struct {
	buckets map[string]*TokenBucket // 按标识符分组的令牌桶
	mutex   sync.RWMutex

	// 默认配置
	defaultCapacity int64
	defaultRate     int64

	logger *zap.Logger
}

// NewRateLimiter 创建速率限制器
func NewRateLimiter(defaultCapacity, defaultRate int64) *RateLimiter {
	return &RateLimiter{
		buckets:         make(map[string]*TokenBucket),
		defaultCapacity: defaultCapacity,
		defaultRate:     defaultRate,
		logger:          zap.L().Named("rate-limiter"),
	}
}

// GetBucket 获取或创建令牌桶
func (rl *RateLimiter) GetBucket(identifier string) *TokenBucket {
	rl.mutex.RLock()
	bucket, exists := rl.buckets[identifier]
	rl.mutex.RUnlock()

	if exists {
		return bucket
	}

	// 创建新的令牌桶
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// 双重检查
	if bucket, exists := rl.buckets[identifier]; exists {
		return bucket
	}

	bucket = NewTokenBucket(rl.defaultCapacity, rl.defaultRate)
	rl.buckets[identifier] = bucket

	rl.logger.Debug("创建新令牌桶",
		zap.String("identifier", identifier),
		zap.Int64("capacity", rl.defaultCapacity),
		zap.Int64("rate", rl.defaultRate))

	return bucket
}

// TryConsume 尝试消费令牌
func (rl *RateLimiter) TryConsume(identifier string, tokens int64) bool {
	bucket := rl.GetBucket(identifier)
	return bucket.TryConsume(tokens)
}

// Consume 消费令牌（阻塞）
func (rl *RateLimiter) Consume(ctx context.Context, identifier string, tokens int64) error {
	bucket := rl.GetBucket(identifier)
	return bucket.Consume(ctx, tokens)
}

// SetBucketConfig 设置特定桶的配置
func (rl *RateLimiter) SetBucketConfig(identifier string, capacity, rate int64) {
	bucket := rl.GetBucket(identifier)
	bucket.SetCapacity(capacity)
	bucket.SetRate(rate)

	rl.logger.Info("更新令牌桶配置",
		zap.String("identifier", identifier),
		zap.Int64("capacity", capacity),
		zap.Int64("rate", rate))
}

// RemoveBucket 移除令牌桶
func (rl *RateLimiter) RemoveBucket(identifier string) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	delete(rl.buckets, identifier)
	rl.logger.Debug("移除令牌桶", zap.String("identifier", identifier))
}

// GetStats 获取所有令牌桶统计
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	stats := map[string]interface{}{
		"bucket_count":     len(rl.buckets),
		"default_capacity": rl.defaultCapacity,
		"default_rate":     rl.defaultRate,
		"buckets":          make(map[string]interface{}),
	}

	bucketsStats := stats["buckets"].(map[string]interface{})
	for id, bucket := range rl.buckets {
		bucketsStats[id] = bucket.GetStats()
	}

	return stats
}

// LeakyBucket 漏桶算法
type LeakyBucket struct {
	capacity int64 // 桶容量
	current  int64 // 当前水位
	leakRate int64 // 泄露速率 (bytes/second)
	lastLeak int64 // 上次泄露时间戳

	droppedPackets atomic.Int64 // 丢弃的包数
	droppedBytes   atomic.Int64 // 丢弃的字节数

	mutex  sync.Mutex
	logger *zap.Logger
}

// NewLeakyBucket 创建漏桶
func NewLeakyBucket(capacity, leakRate int64) *LeakyBucket {
	return &LeakyBucket{
		capacity: capacity,
		current:  0,
		leakRate: leakRate,
		lastLeak: time.Now().UnixNano(),
		logger:   zap.L().Named("leaky-bucket"),
	}
}

// TryAdd 尝试添加数据
func (lb *LeakyBucket) TryAdd(size int64) bool {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	lb.leak()

	if lb.current+size <= lb.capacity {
		lb.current += size
		lb.logger.Debug("添加数据成功",
			zap.Int64("size", size),
			zap.Int64("current", lb.current),
			zap.Int64("capacity", lb.capacity))
		return true
	}

	// 桶满，丢弃数据
	lb.droppedPackets.Add(1)
	lb.droppedBytes.Add(size)

	lb.logger.Debug("桶满，丢弃数据",
		zap.Int64("size", size),
		zap.Int64("current", lb.current),
		zap.Int64("capacity", lb.capacity))

	return false
}

// leak 泄露数据（需要在锁内调用）
func (lb *LeakyBucket) leak() {
	now := time.Now().UnixNano()
	elapsed := now - lb.lastLeak

	if elapsed <= 0 {
		return
	}

	// 计算泄露的字节数
	leaked := int64(float64(elapsed) * float64(lb.leakRate) / float64(time.Second))

	if leaked > 0 {
		lb.current -= leaked
		if lb.current < 0 {
			lb.current = 0
		}
		lb.lastLeak = now

		lb.logger.Debug("泄露数据",
			zap.Int64("leaked", leaked),
			zap.Int64("current", lb.current))
	}
}

// GetUtilization 获取使用率
func (lb *LeakyBucket) GetUtilization() float64 {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	lb.leak()
	return float64(lb.current) / float64(lb.capacity) * 100
}

// GetStats 获取统计信息
func (lb *LeakyBucket) GetStats() map[string]interface{} {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	lb.leak()

	return map[string]interface{}{
		"capacity":        lb.capacity,
		"current":         lb.current,
		"leak_rate":       lb.leakRate,
		"utilization":     float64(lb.current) / float64(lb.capacity) * 100,
		"dropped_packets": lb.droppedPackets.Load(),
		"dropped_bytes":   lb.droppedBytes.Load(),
	}
}





