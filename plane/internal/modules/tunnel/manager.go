package tunnel

import (
	"gkipass/plane/db"
	"gkipass/plane/internal/service"
)

// Manager 隧道管理器（模块封装）
type Manager struct {
	tunnelService *service.TunnelService
	planService   *service.PlanService
}

// NewManager 创建隧道管理器
func NewManager(dbManager *db.Manager) *Manager {
	planService := service.NewPlanService(dbManager)
	// TODO: Pass correct services when available
	tunnelService := service.NewTunnelService(nil, nil, nil, nil)

	return &Manager{
		tunnelService: tunnelService,
		planService:   planService,
	}
}

// GetTunnelService 获取隧道服务
func (m *Manager) GetTunnelService() *service.TunnelService {
	return m.tunnelService
}

// GetPlanService 获取套餐服务
func (m *Manager) GetPlanService() *service.PlanService {
	return m.planService
}
