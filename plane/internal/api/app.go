package api

import (
	"gkipass/plane/db"
	"gkipass/plane/internal/config"
)

// App 应用实例
type App struct {
	Config *config.Config
	DB     *db.Manager
}

// NewApp 创建新的应用实例
func NewApp(cfg *config.Config, dbManager *db.Manager) *App {
	return &App{
		Config: cfg,
		DB:     dbManager,
	}
}

