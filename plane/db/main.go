package db

import (
	"fmt"
	"log"

	"gkipass/plane/db/cache"
	"gkipass/plane/db/sqlite"
)

// Manager 数据库管理器
type Manager struct {
	DB    *DB
	Cache *Cache

	sqlitePool *sqlite.Pool
	redisPool  *cache.Pool
}

// Config 数据库配置
type Config struct {
	// SQLite配置
	SQLitePath string

	// Redis配置
	RedisAddr     string
	RedisPassword string
	RedisDB       int
}

// NewManager 创建新的数据库管理器
func NewManager(cfg *Config) (*Manager, error) {
	// 初始化SQLite
	sqlitePool, err := sqlite.NewPool(cfg.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("failed to init SQLite: %w", err)
	}
	log.Printf("✓ SQLite initialized: %s", cfg.SQLitePath)

	// 初始化Redis
	redisPool, err := cache.NewPool(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		// Redis是可选的，如果连接失败只打印警告
		log.Printf("⚠ Redis connection failed: %v (continuing without cache)", err)
		redisPool = nil
	} else {
		log.Printf("✓ Redis connected: %s", cfg.RedisAddr)
	}

	manager := &Manager{
		DB:         NewDB(sqlitePool.Get()),
		sqlitePool: sqlitePool,
		redisPool:  redisPool,
	}

	// 如果Redis可用，设置缓存
	if redisPool != nil {
		manager.Cache = NewCache(redisPool.Get())
	}

	return manager, nil
}

// Close 关闭所有数据库连接
func (m *Manager) Close() error {
	var errs []error

	if m.sqlitePool != nil {
		if err := m.sqlitePool.Close(); err != nil {
			errs = append(errs, fmt.Errorf("SQLite close error: %w", err))
		}
	}

	if m.redisPool != nil {
		if err := m.redisPool.Close(); err != nil {
			errs = append(errs, fmt.Errorf("Redis close error: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// HasCache 检查是否有缓存可用
func (m *Manager) HasCache() bool {
	return m.Cache != nil && m.Cache.Redis != nil
}
