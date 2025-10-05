package sqlite

import (
	"sync"
)

// Pool SQLite连接池（简单封装）
type Pool struct {
	db *SQLiteDB
	mu sync.RWMutex
}

// NewPool 创建新的连接池
func NewPool(dbPath string) (*Pool, error) {
	db, err := NewSQLiteDB(dbPath)
	if err != nil {
		return nil, err
	}

	return &Pool{
		db: db,
	}, nil
}

// Get 获取数据库连接
func (p *Pool) Get() *SQLiteDB {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.db
}

// Close 关闭连接池
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.db.Close()
}
