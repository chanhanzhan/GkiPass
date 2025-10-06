package service

import (
	"fmt"
	"sync"
)

// MultiTenantManager 多租户管理器
type MultiTenantManager struct {
	tenants map[string]*TenantConfig
	quotas  map[string]*ResourceQuota
	mu      sync.RWMutex
}

// TenantConfig 租户配置
type TenantConfig struct {
	TenantID   string
	Name       string
	Enabled    bool
	MaxNodes   int
	MaxTunnels int
	MaxTraffic int64 // 字节
}

// ResourceQuota 资源配额
type ResourceQuota struct {
	TenantID    string
	UsedNodes   int
	UsedTunnels int
	UsedTraffic int64
	MaxNodes    int
	MaxTunnels  int
	MaxTraffic  int64
}

// NewMultiTenantManager 创建多租户管理器
func NewMultiTenantManager() *MultiTenantManager {
	return &MultiTenantManager{
		tenants: make(map[string]*TenantConfig),
		quotas:  make(map[string]*ResourceQuota),
	}
}

// RegisterTenant 注册租户
func (mtm *MultiTenantManager) RegisterTenant(config *TenantConfig) error {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	mtm.tenants[config.TenantID] = config
	mtm.quotas[config.TenantID] = &ResourceQuota{
		TenantID:   config.TenantID,
		MaxNodes:   config.MaxNodes,
		MaxTunnels: config.MaxTunnels,
		MaxTraffic: config.MaxTraffic,
	}

	return nil
}

// CheckQuota 检查配额
func (mtm *MultiTenantManager) CheckQuota(tenantID string, resourceType string) error {
	mtm.mu.RLock()
	defer mtm.mu.RUnlock()

	quota, exists := mtm.quotas[tenantID]
	if !exists {
		return fmt.Errorf("租户不存在: %s", tenantID)
	}

	switch resourceType {
	case "node":
		if quota.UsedNodes >= quota.MaxNodes {
			return fmt.Errorf("节点配额已满: %d/%d", quota.UsedNodes, quota.MaxNodes)
		}
	case "tunnel":
		if quota.UsedTunnels >= quota.MaxTunnels {
			return fmt.Errorf("隧道配额已满: %d/%d", quota.UsedTunnels, quota.MaxTunnels)
		}
	case "traffic":
		if quota.UsedTraffic >= quota.MaxTraffic {
			return fmt.Errorf("流量配额已满: %d/%d", quota.UsedTraffic, quota.MaxTraffic)
		}
	}

	return nil
}

// UpdateQuota 更新配额使用情况
func (mtm *MultiTenantManager) UpdateQuota(tenantID string, resourceType string, delta int) {
	mtm.mu.Lock()
	defer mtm.mu.Unlock()

	quota, exists := mtm.quotas[tenantID]
	if !exists {
		return
	}

	switch resourceType {
	case "node":
		quota.UsedNodes += delta
	case "tunnel":
		quota.UsedTunnels += delta
	}
}

