package cache

import (
	"sync"
)

// Pool Redis连接池（简单封装）
type Pool struct {
	cache *RedisCache
	mu    sync.RWMutex
}

// NewPool 创建新的Redis连接池
func NewPool(addr, password string, db int) (*Pool, error) {
	cache, err := NewRedisCache(addr, password, db)
	if err != nil {
		return nil, err
	}

	return &Pool{
		cache: cache,
	}, nil
}

// Get 获取Redis客户端
func (p *Pool) Get() *RedisCache {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cache
}

// Close 关闭连接池
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cache.Close()
}
