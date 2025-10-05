package pool

import (
	"context"
	"sync"
	"time"
)

// RateLimiter 令牌桶限速器
type RateLimiter struct {
	rate     int64     // 每秒字节数
	burst    int64     // 突发大小
	tokens   int64     // 当前令牌数
	lastTime time.Time // 上次更新时间
	mu       sync.Mutex
}

// NewRateLimiter 创建限速器
func NewRateLimiter(rate, burst int64) *RateLimiter {
	return &RateLimiter{
		rate:     rate,
		burst:    burst,
		tokens:   burst,
		lastTime: time.Now(),
	}
}

// Allow 检查是否允许传输n字节
func (rl *RateLimiter) Allow(n int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// 补充令牌
	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.tokens += int64(elapsed * float64(rl.rate))
	if rl.tokens > rl.burst {
		rl.tokens = rl.burst
	}
	rl.lastTime = now

	// 消耗令牌
	if rl.tokens >= n {
		rl.tokens -= n
		return true
	}

	return false
}

// Wait 等待直到可以传输n字节
func (rl *RateLimiter) Wait(n int64) {
	for !rl.Allow(n) {
		time.Sleep(10 * time.Millisecond)
	}
}

// WaitContext 带上下文的等待
func (rl *RateLimiter) WaitContext(ctx context.Context, n int64) error {
	for !rl.Allow(n) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return nil
}

// SetRate 动态调整速率
func (rl *RateLimiter) SetRate(rate int64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.rate = rate
}

// SetBurst 动态调整突发大小
func (rl *RateLimiter) SetBurst(burst int64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.burst = burst
	if rl.tokens > burst {
		rl.tokens = burst
	}
}

// GetRate 获取当前速率
func (rl *RateLimiter) GetRate() int64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.rate
}

// GetTokens 获取当前令牌数
func (rl *RateLimiter) GetTokens() int64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.tokens
}

// LimitedReader 限速读取器
type LimitedReader struct {
	reader  interface{ Read([]byte) (int, error) }
	limiter *RateLimiter
}

// NewLimitedReader 创建限速读取器
func NewLimitedReader(reader interface{ Read([]byte) (int, error) }, limiter *RateLimiter) *LimitedReader {
	return &LimitedReader{
		reader:  reader,
		limiter: limiter,
	}
}

// Read 实现io.Reader接口
func (lr *LimitedReader) Read(p []byte) (int, error) {
	n, err := lr.reader.Read(p)
	if n > 0 && lr.limiter != nil {
		lr.limiter.Wait(int64(n))
	}
	return n, err
}

// LimitedWriter 限速写入器
type LimitedWriter struct {
	writer  interface{ Write([]byte) (int, error) }
	limiter *RateLimiter
}

// NewLimitedWriter 创建限速写入器
func NewLimitedWriter(writer interface{ Write([]byte) (int, error) }, limiter *RateLimiter) *LimitedWriter {
	return &LimitedWriter{
		writer:  writer,
		limiter: limiter,
	}
}

// Write 实现io.Writer接口
func (lw *LimitedWriter) Write(p []byte) (int, error) {
	if lw.limiter != nil {
		lw.limiter.Wait(int64(len(p)))
	}
	return lw.writer.Write(p)
}






