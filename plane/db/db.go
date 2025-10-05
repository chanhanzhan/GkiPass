package db

import (
	"gkipass/plane/db/sqlite"
)

// DB 数据库接口（SQLite专用）
type DB struct {
	SQLite *sqlite.SQLiteDB
}

// NewDB 创建新的数据库实例
func NewDB(sqliteDB *sqlite.SQLiteDB) *DB {
	return &DB{
		SQLite: sqliteDB,
	}
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	if db.SQLite != nil {
		return db.SQLite.Close()
	}
	return nil
}
