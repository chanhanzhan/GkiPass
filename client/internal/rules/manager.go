package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"gkipass/client/internal/protocol"
)

// ManagerConfig 规则管理器配置
type ManagerConfig struct {
	RulesDir         string        `json:"rules_dir"`         // 规则目录
	CacheRules       bool          `json:"cache_rules"`       // 缓存规则
	AutoReload       bool          `json:"auto_reload"`       // 自动重新加载
	ReloadInterval   time.Duration `json:"reload_interval"`   // 重新加载间隔
	BackupRules      bool          `json:"backup_rules"`      // 备份规则
	MaxBackups       int           `json:"max_backups"`       // 最大备份数
	EnableVersioning bool          `json:"enable_versioning"` // 启用版本控制
}

// DefaultManagerConfig 默认规则管理器配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		RulesDir:         "./data/rules",
		CacheRules:       true,
		AutoReload:       true,
		ReloadInterval:   5 * time.Minute,
		BackupRules:      true,
		MaxBackups:       10,
		EnableVersioning: true,
	}
}

// Manager 规则管理器
type Manager struct {
	config      *ManagerConfig
	logger      *zap.Logger
	forwarder   *protocol.Forwarder
	rules       map[string]*Rule
	rulesByPort map[int][]*Rule
	version     int64
	lock        sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewManager 创建规则管理器
func NewManager(config *ManagerConfig, forwarder *protocol.Forwarder) (*Manager, error) {
	if config == nil {
		config = DefaultManagerConfig()
	}

	// 确保规则目录存在
	if err := os.MkdirAll(config.RulesDir, 0755); err != nil {
		return nil, fmt.Errorf("创建规则目录失败: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		config:      config,
		logger:      zap.L().Named("rules-manager"),
		forwarder:   forwarder,
		rules:       make(map[string]*Rule),
		rulesByPort: make(map[int][]*Rule),
		ctx:         ctx,
		cancel:      cancel,
	}

	return m, nil
}

// Start 启动规则管理器
func (m *Manager) Start() error {
	m.logger.Info("启动规则管理器")

	// 加载规则
	if err := m.loadRules(); err != nil {
		m.logger.Error("加载规则失败", zap.Error(err))
	}

	// 如果启用自动重新加载，启动重新加载协程
	if m.config.AutoReload {
		m.wg.Add(1)
		go m.autoReloadLoop()
	}

	return nil
}

// Stop 停止规则管理器
func (m *Manager) Stop() error {
	m.logger.Info("停止规则管理器")

	// 取消上下文
	m.cancel()

	// 等待协程结束
	m.wg.Wait()

	return nil
}

// loadRules 加载规则
func (m *Manager) loadRules() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// 清空规则
	m.rules = make(map[string]*Rule)
	m.rulesByPort = make(map[int][]*Rule)

	// 读取规则文件
	files, err := ioutil.ReadDir(m.config.RulesDir)
	if err != nil {
		return fmt.Errorf("读取规则目录失败: %w", err)
	}

	// 加载每个规则文件
	for _, file := range files {
		// 跳过目录和非JSON文件
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		// 读取规则文件
		filePath := filepath.Join(m.config.RulesDir, file.Name())
		ruleData, err := ioutil.ReadFile(filePath)
		if err != nil {
			m.logger.Error("读取规则文件失败",
				zap.String("file", filePath),
				zap.Error(err))
			continue
		}

		// 解析规则
		var rule Rule
		if err := json.Unmarshal(ruleData, &rule); err != nil {
			m.logger.Error("解析规则文件失败",
				zap.String("file", filePath),
				zap.Error(err))
			continue
		}

		// 添加规则
		m.rules[rule.ID] = &rule

		// TODO: Port indexing needs to be reimplemented for new Rule structure
		// 按端口索引规则
		/*
			if rule.Status == RuleStatusActive {
				// Port indexing logic here
			}
		*/

		// 更新版本
		if rule.Version > m.version {
			m.version = rule.Version
		}
	}

	m.logger.Info("加载规则完成",
		zap.Int("rule_count", len(m.rules)),
		zap.Int64("version", m.version))

	// 应用规则
	return m.applyRules()
}

// applyRules 应用规则
func (m *Manager) applyRules() error {
	// 如果没有转发器，跳过应用
	if m.forwarder == nil {
		m.logger.Warn("未设置转发器，跳过应用规则")
		return nil
	}

	// TODO: Implement rule forwarding when ForwardRule and related types are available
	// 遍历规则
	/*
		for _, rule := range m.rules {
			if !rule.Status != RuleStatusActive {
				continue
			}
			// Add forwarding logic here
		}
	*/

	return nil
}

// autoReloadLoop 自动重新加载循环
func (m *Manager) autoReloadLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.ReloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if err := m.loadRules(); err != nil {
				m.logger.Error("自动重新加载规则失败", zap.Error(err))
			}
		}
	}
}

// GetRule 获取规则
func (m *Manager) GetRule(id string) *Rule {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.rules[id]
}

// GetRules 获取所有规则
func (m *Manager) GetRules() []*Rule {
	m.lock.RLock()
	defer m.lock.RUnlock()

	rules := make([]*Rule, 0, len(m.rules))
	for _, rule := range m.rules {
		rules = append(rules, rule)
	}

	return rules
}

// GetRulesByPort 获取指定端口的规则
func (m *Manager) GetRulesByPort(port int) []*Rule {
	m.lock.RLock()
	defer m.lock.RUnlock()

	rules := m.rulesByPort[port]
	if rules == nil {
		return []*Rule{}
	}

	// 返回副本
	result := make([]*Rule, len(rules))
	copy(result, rules)

	return result
}

// AddRule 添加规则
func (m *Manager) AddRule(rule *Rule) error {
	if rule == nil {
		return fmt.Errorf("规则为空")
	}

	if rule.ID == "" {
		return fmt.Errorf("规则ID为空")
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	// 检查规则是否已存在
	if _, exists := m.rules[rule.ID]; exists {
		return fmt.Errorf("规则已存在: %s", rule.ID)
	}

	// 设置创建时间和更新时间
	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	// 设置版本
	rule.Version = m.version + 1
	m.version = rule.Version

	// 添加规则
	m.rules[rule.ID] = rule

	// TODO: Port indexing needs to be reimplemented for new Rule structure
	/*
		if rule.Status == RuleStatusActive {
			// Port indexing logic here
		}
	*/

	// 保存规则
	if err := m.saveRule(rule); err != nil {
		return fmt.Errorf("保存规则失败: %w", err)
	}

	// TODO: Apply rule forwarding when ForwardRule and related types are available
	/*
		if m.forwarder != nil && rule.Status == RuleStatusActive {
			// Add forwarding logic here
		}
	*/

	return nil
}

// UpdateRule 更新规则
func (m *Manager) UpdateRule(rule *Rule) error {
	if rule == nil {
		return fmt.Errorf("规则为空")
	}

	if rule.ID == "" {
		return fmt.Errorf("规则ID为空")
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	// 检查规则是否存在
	oldRule, exists := m.rules[rule.ID]
	if !exists {
		return fmt.Errorf("规则不存在: %s", rule.ID)
	}

	// 备份旧规则
	if m.config.BackupRules {
		if err := m.backupRule(oldRule); err != nil {
			m.logger.Error("备份规则失败",
				zap.String("rule_id", oldRule.ID),
				zap.Error(err))
		}
	}

	// 保留创建时间
	rule.CreatedAt = oldRule.CreatedAt

	// 设置更新时间
	rule.UpdatedAt = time.Now()

	// 设置版本
	rule.Version = oldRule.Version + 1
	if rule.Version > m.version {
		m.version = rule.Version
	}

	// TODO: Port-based rule removal needs to be reimplemented
	/*
		// 从旧端口的规则列表中移除
		if oldRule.Status == RuleStatusActive {
			// Port removal logic here
		}
	*/

	// 更新规则
	m.rules[rule.ID] = rule

	// TODO: Port indexing needs to be reimplemented for new Rule structure
	/*
		if rule.Status == RuleStatusActive {
			// Port indexing logic here
		}
	*/

	// 保存规则
	if err := m.saveRule(rule); err != nil {
		return fmt.Errorf("保存规则失败: %w", err)
	}

	// 应用规则
	// TODO: 实现更新转发规则

	return nil
}

// DeleteRule 删除规则
func (m *Manager) DeleteRule(id string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// 检查规则是否存在
	rule, exists := m.rules[id]
	if !exists {
		return fmt.Errorf("规则不存在: %s", id)
	}

	// 备份规则
	if m.config.BackupRules {
		if err := m.backupRule(rule); err != nil {
			m.logger.Error("备份规则失败",
				zap.String("rule_id", rule.ID),
				zap.Error(err))
		}
	}

	// TODO: Port-based rule removal needs to be reimplemented
	/*
		// 从端口的规则列表中移除
		if rule.Status == RuleStatusActive {
			// Port removal logic here
		}
	*/

	// 删除规则
	delete(m.rules, id)

	// 删除规则文件
	filePath := filepath.Join(m.config.RulesDir, fmt.Sprintf("%s.json", id))
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除规则文件失败: %w", err)
	}

	// 应用规则
	// TODO: 实现删除转发规则

	return nil
}

// saveRule 保存规则
func (m *Manager) saveRule(rule *Rule) error {
	// 序列化规则
	data, err := json.MarshalIndent(rule, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化规则失败: %w", err)
	}

	// 保存规则文件
	filePath := filepath.Join(m.config.RulesDir, fmt.Sprintf("%s.json", rule.ID))
	if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入规则文件失败: %w", err)
	}

	return nil
}

// backupRule 备份规则
func (m *Manager) backupRule(rule *Rule) error {
	// 创建备份目录
	backupDir := filepath.Join(m.config.RulesDir, "backups", rule.ID)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("创建备份目录失败: %w", err)
	}

	// 序列化规则
	data, err := json.MarshalIndent(rule, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化规则失败: %w", err)
	}

	// 保存备份文件
	backupPath := filepath.Join(backupDir, fmt.Sprintf("v%d_%s.json", rule.Version, time.Now().Format("20060102_150405")))
	if err := ioutil.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}

	// 如果超过最大备份数，删除旧备份
	if m.config.MaxBackups > 0 {
		files, err := ioutil.ReadDir(backupDir)
		if err != nil {
			m.logger.Error("读取备份目录失败",
				zap.String("dir", backupDir),
				zap.Error(err))
			return nil
		}

		if len(files) > m.config.MaxBackups {
			// 按修改时间排序
			sort.Slice(files, func(i, j int) bool {
				return files[i].ModTime().Before(files[j].ModTime())
			})

			// 删除旧文件
			for i := 0; i < len(files)-m.config.MaxBackups; i++ {
				oldPath := filepath.Join(backupDir, files[i].Name())
				if err := os.Remove(oldPath); err != nil {
					m.logger.Error("删除旧备份文件失败",
						zap.String("file", oldPath),
						zap.Error(err))
				}
			}
		}
	}

	return nil
}

// SyncRules 同步规则
func (m *Manager) SyncRules(rules []*Rule, version int64, fullSync bool) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// 检查版本
	if version <= m.version && !fullSync {
		m.logger.Info("规则版本未变更，跳过同步",
			zap.Int64("current_version", m.version),
			zap.Int64("new_version", version))
		return nil
	}

	// 全量同步时，清空现有规则
	if fullSync {
		m.rules = make(map[string]*Rule)
		m.rulesByPort = make(map[int][]*Rule)
	}

	// 添加或更新规则
	for _, rule := range rules {
		// 设置更新时间
		rule.UpdatedAt = time.Now()

		// 保存规则
		m.rules[rule.ID] = rule

		// TODO: Port indexing needs to be reimplemented for new Rule structure
		/*
			// 按端口索引规则
			if rule.Status == RuleStatusActive {
				// Port indexing logic here
			}
		*/

		// 保存规则文件
		if err := m.saveRule(rule); err != nil {
			m.logger.Error("保存规则失败",
				zap.String("rule_id", rule.ID),
				zap.Error(err))
		}
	}

	// 更新版本
	m.version = version

	// 应用规则
	if err := m.applyRules(); err != nil {
		return fmt.Errorf("应用规则失败: %w", err)
	}

	m.logger.Info("同步规则完成",
		zap.Int("rule_count", len(rules)),
		zap.Int64("version", version),
		zap.Bool("full_sync", fullSync))

	return nil
}

// GetVersion 获取规则版本
func (m *Manager) GetVersion() int64 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.version
}

// GetStats 获取统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.lock.RLock()
	defer m.lock.RUnlock()

	enabledCount := 0
	for _, rule := range m.rules {
		if rule.Status == RuleStatusActive {
			enabledCount++
		}
	}

	portCount := len(m.rulesByPort)

	return map[string]interface{}{
		"total_rules":    len(m.rules),
		"enabled_rules":  enabledCount,
		"disabled_rules": len(m.rules) - enabledCount,
		"port_count":     portCount,
		"version":        m.version,
	}
}
