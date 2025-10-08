package api

import (
	"gkipass/plane/db"
	"gkipass/plane/internal/config"
	"gkipass/plane/internal/types"
)

// App 应用实例 - 重新导出types.App以保持兼容性
type App = types.App

// NewApp 创建新的应用实例
func NewApp(cfg *config.Config, dbManager *db.Manager) *App {
	return types.NewApp(cfg, dbManager)
}
