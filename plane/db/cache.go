package db

import (
	"gkipass/plane/db/cache"
)

// Cache 缓存接口（Redis专用）
type Cache struct {
	Redis *cache.RedisCache
}

// NewCache 创建新的缓存实例
func NewCache(redisCache *cache.RedisCache) *Cache {
	return &Cache{
		Redis: redisCache,
	}
}

// Close 关闭缓存连接
func (c *Cache) Close() error {
	if c.Redis != nil {
		return c.Redis.Close()
	}
	return nil
}
