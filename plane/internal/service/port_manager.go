package service

import (
	"fmt"
	"sync"

	"gkipass/plane/db"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

// PortManager 端口管理器（按隧道组共享端口）
type PortManager struct {
	db         *db.Manager
	groupPorts map[string]map[int]string // groupID -> (port -> tunnelID)
	mu         sync.RWMutex
}

var (
	portManagerInstance *PortManager
	portManagerOnce     sync.Once
)

// GetPortManager 获取端口管理器单例
func GetPortManager(dbManager *db.Manager) *PortManager {
	portManagerOnce.Do(func() {
		portManagerInstance = &PortManager{
			db:         dbManager,
			groupPorts: make(map[string]map[int]string),
		}
		// 初始化时加载已占用端口
		portManagerInstance.loadOccupiedPorts()
	})
	return portManagerInstance
}

// loadOccupiedPorts 从数据库加载已占用端口
func (pm *PortManager) loadOccupiedPorts() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 查询所有启用的隧道
	enabled := true
	tunnels, err := pm.db.DB.SQLite.ListTunnels("", &enabled)
	if err != nil {
		logger.Error("加载已占用端口失败", zap.Error(err))
		return err
	}

	// 重建端口占用映射（按组）
	pm.groupPorts = make(map[string]map[int]string)
	for _, tunnel := range tunnels {
		// 使用入口组ID作为key
		groupID := tunnel.EntryGroupID
		if pm.groupPorts[groupID] == nil {
			pm.groupPorts[groupID] = make(map[int]string)
		}
		pm.groupPorts[groupID][tunnel.LocalPort] = tunnel.ID
	}

	totalPorts := 0
	for _, ports := range pm.groupPorts {
		totalPorts += len(ports)
	}

	logger.Info("端口占用加载完成（按组共享）",
		zap.Int("groups", len(pm.groupPorts)),
		zap.Int("totalPorts", totalPorts))
	return nil
}

// IsPortAvailableInGroup 检查端口在指定组内是否可用
func (pm *PortManager) IsPortAvailableInGroup(groupID string, port int, excludeTunnelID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 检查该组是否存在
	groupPorts, exists := pm.groupPorts[groupID]
	if !exists {
		return true // 组内没有占用任何端口
	}

	tunnelID, occupied := groupPorts[port]
	if !occupied {
		return true // 端口未占用
	}

	// 如果是同一个隧道（更新操作），则认为可用
	return tunnelID == excludeTunnelID
}

// OccupyPortInGroup 在指定组内占用端口
func (pm *PortManager) OccupyPortInGroup(groupID string, port int, tunnelID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 确保组存在
	if pm.groupPorts[groupID] == nil {
		pm.groupPorts[groupID] = make(map[int]string)
	}

	if existingTunnelID, occupied := pm.groupPorts[groupID][port]; occupied {
		if existingTunnelID != tunnelID {
			return fmt.Errorf("端口 %d 已被同组内其他隧道占用", port)
		}
	}

	pm.groupPorts[groupID][port] = tunnelID
	logger.Info("端口已占用（组内）",
		zap.String("groupID", groupID),
		zap.Int("port", port),
		zap.String("tunnelID", tunnelID))

	return nil
}

// ReleasePortInGroup 在指定组内释放端口
func (pm *PortManager) ReleasePortInGroup(groupID string, port int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if groupPorts, exists := pm.groupPorts[groupID]; exists {
		if tunnelID, portExists := groupPorts[port]; portExists {
			delete(groupPorts, port)
			logger.Info("端口已释放（组内）",
				zap.String("groupID", groupID),
				zap.Int("port", port),
				zap.String("tunnelID", tunnelID))
		}
	}
}

// GetOccupiedPortsInGroup 获取指定组内已占用端口
func (pm *PortManager) GetOccupiedPortsInGroup(groupID string) map[int]string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 返回副本
	ports := make(map[int]string)
	if groupPorts, exists := pm.groupPorts[groupID]; exists {
		for port, tunnelID := range groupPorts {
			ports[port] = tunnelID
		}
	}

	return ports
}

// GetAllOccupiedPorts 获取所有组的占用端口（用于管理员查看）
func (pm *PortManager) GetAllOccupiedPorts() map[string]map[int]string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 返回完整副本
	allPorts := make(map[string]map[int]string)
	for groupID, ports := range pm.groupPorts {
		allPorts[groupID] = make(map[int]string)
		for port, tunnelID := range ports {
			allPorts[groupID][port] = tunnelID
		}
	}

	return allPorts
}

// ValidatePort 验证端口范围
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("端口必须在 1-65535 范围内")
	}

	// 保留端口检查（1-1024为系统保留）
	if port < 1024 {
		return fmt.Errorf("端口 %d 为系统保留端口", port)
	}

	return nil
}
