package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gkipass/plane/internal/model"
)

// NodeGroupService 节点组服务
type NodeGroupService struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewNodeGroupService 创建节点组服务
func NewNodeGroupService(db *sql.DB) *NodeGroupService {
	return &NodeGroupService{
		db:     db,
		logger: zap.L().Named("node-group-service"),
	}
}

// CreateNodeGroup 创建节点组
func (s *NodeGroupService) CreateNodeGroup(group *model.NodeGroup) error {
	// 验证必填字段
	if group.Name == "" {
		return errors.New("组名不能为空")
	}

	if group.Role == "" {
		return errors.New("角色不能为空")
	}

	// 生成ID
	if group.ID == "" {
		group.ID = uuid.New().String()
	}

	// 创建节点组
	err := model.CreateNodeGroup(s.db, group)
	if err != nil {
		s.logger.Error("创建节点组失败",
			zap.String("name", group.Name),
			zap.Error(err))
		return fmt.Errorf("创建节点组失败: %w", err)
	}

	// 添加节点到组
	if len(group.Nodes) > 0 {
		for _, nodeID := range group.Nodes {
			if err := model.AddNodeToGroup(s.db, group.ID, nodeID); err != nil {
				s.logger.Error("添加节点到组失败",
					zap.String("group_id", group.ID),
					zap.String("node_id", nodeID),
					zap.Error(err))
				// 继续添加其他节点
			}
		}
	}

	s.logger.Info("创建节点组成功",
		zap.String("id", group.ID),
		zap.String("name", group.Name),
		zap.String("role", string(group.Role)))

	return nil
}

// GetNodeGroup 获取节点组
func (s *NodeGroupService) GetNodeGroup(id string) (*model.NodeGroup, error) {
	group, err := model.GetNodeGroup(s.db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("节点组不存在: %s", id)
		}
		s.logger.Error("获取节点组失败",
			zap.String("id", id),
			zap.Error(err))
		return nil, fmt.Errorf("获取节点组失败: %w", err)
	}

	return group, nil
}

// UpdateNodeGroup 更新节点组
func (s *NodeGroupService) UpdateNodeGroup(group *model.NodeGroup) error {
	// 验证必填字段
	if group.ID == "" {
		return errors.New("组ID不能为空")
	}

	if group.Name == "" {
		return errors.New("组名不能为空")
	}

	if group.Role == "" {
		return errors.New("角色不能为空")
	}

	// 检查组是否存在
	_, err := model.GetNodeGroup(s.db, group.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("节点组不存在: %s", group.ID)
		}
		return fmt.Errorf("获取节点组失败: %w", err)
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 更新节点组
	if err := model.UpdateNodeGroup(s.db, group); err != nil {
		tx.Rollback()
		s.logger.Error("更新节点组失败",
			zap.String("id", group.ID),
			zap.Error(err))
		return fmt.Errorf("更新节点组失败: %w", err)
	}

	// 更新节点关联
	// 1. 删除现有关联
	_, err = tx.Exec("DELETE FROM node_group_nodes WHERE group_id = ?", group.ID)
	if err != nil {
		tx.Rollback()
		s.logger.Error("删除节点关联失败",
			zap.String("group_id", group.ID),
			zap.Error(err))
		return fmt.Errorf("删除节点关联失败: %w", err)
	}

	// 2. 添加新关联
	if len(group.Nodes) > 0 {
		for _, nodeID := range group.Nodes {
			_, err = tx.Exec(
				"INSERT INTO node_group_nodes (group_id, node_id, added_at) VALUES (?, ?, ?)",
				group.ID, nodeID, time.Now(),
			)
			if err != nil {
				tx.Rollback()
				s.logger.Error("添加节点关联失败",
					zap.String("group_id", group.ID),
					zap.String("node_id", nodeID),
					zap.Error(err))
				return fmt.Errorf("添加节点关联失败: %w", err)
			}
		}
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		s.logger.Error("提交事务失败",
			zap.String("id", group.ID),
			zap.Error(err))
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.logger.Info("更新节点组成功",
		zap.String("id", group.ID),
		zap.String("name", group.Name))

	return nil
}

// DeleteNodeGroup 删除节点组
func (s *NodeGroupService) DeleteNodeGroup(id string) error {
	// 检查是否为默认组
	if strings.HasPrefix(id, "default-") {
		return errors.New("不能删除系统默认组")
	}

	// 删除节点组
	err := model.DeleteNodeGroup(s.db, id)
	if err != nil {
		s.logger.Error("删除节点组失败",
			zap.String("id", id),
			zap.Error(err))
		return fmt.Errorf("删除节点组失败: %w", err)
	}

	s.logger.Info("删除节点组成功", zap.String("id", id))

	return nil
}

// ListNodeGroups 列出所有节点组
func (s *NodeGroupService) ListNodeGroups() ([]*model.NodeGroup, error) {
	groups, err := model.ListNodeGroups(s.db)
	if err != nil {
		s.logger.Error("列出节点组失败", zap.Error(err))
		return nil, fmt.Errorf("列出节点组失败: %w", err)
	}

	return groups, nil
}

// AddNodeToGroup 将节点添加到组
func (s *NodeGroupService) AddNodeToGroup(groupID, nodeID string) error {
	// 检查组是否存在
	_, err := model.GetNodeGroup(s.db, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("节点组不存在: %s", groupID)
		}
		return fmt.Errorf("获取节点组失败: %w", err)
	}

	// 检查节点是否存在
	// TODO: 实现节点存在性检查

	// 添加节点到组
	err = model.AddNodeToGroup(s.db, groupID, nodeID)
	if err != nil {
		s.logger.Error("添加节点到组失败",
			zap.String("group_id", groupID),
			zap.String("node_id", nodeID),
			zap.Error(err))
		return fmt.Errorf("添加节点到组失败: %w", err)
	}

	s.logger.Info("添加节点到组成功",
		zap.String("group_id", groupID),
		zap.String("node_id", nodeID))

	return nil
}

// RemoveNodeFromGroup 从组中移除节点
func (s *NodeGroupService) RemoveNodeFromGroup(groupID, nodeID string) error {
	// 移除节点
	err := model.RemoveNodeFromGroup(s.db, groupID, nodeID)
	if err != nil {
		s.logger.Error("从组中移除节点失败",
			zap.String("group_id", groupID),
			zap.String("node_id", nodeID),
			zap.Error(err))
		return fmt.Errorf("从组中移除节点失败: %w", err)
	}

	s.logger.Info("从组中移除节点成功",
		zap.String("group_id", groupID),
		zap.String("node_id", nodeID))

	return nil
}

// GetNodeGroups 获取节点所属的组
func (s *NodeGroupService) GetNodeGroups(nodeID string) ([]*model.NodeGroup, error) {
	groups, err := model.GetNodeGroups(s.db, nodeID)
	if err != nil {
		s.logger.Error("获取节点所属组失败",
			zap.String("node_id", nodeID),
			zap.Error(err))
		return nil, fmt.Errorf("获取节点所属组失败: %w", err)
	}

	return groups, nil
}

// GetIngressGroups 获取所有入口组
func (s *NodeGroupService) GetIngressGroups() ([]*model.NodeGroup, error) {
	groups, err := model.GetIngressGroups(s.db)
	if err != nil {
		s.logger.Error("获取入口组失败", zap.Error(err))
		return nil, fmt.Errorf("获取入口组失败: %w", err)
	}

	return groups, nil
}

// GetEgressGroups 获取所有出口组
func (s *NodeGroupService) GetEgressGroups() ([]*model.NodeGroup, error) {
	groups, err := model.GetEgressGroups(s.db)
	if err != nil {
		s.logger.Error("获取出口组失败", zap.Error(err))
		return nil, fmt.Errorf("获取出口组失败: %w", err)
	}

	return groups, nil
}
