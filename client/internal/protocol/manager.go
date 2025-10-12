package protocol

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"gkipass/client/internal/transport"

	"go.uber.org/zap"
)

// NodeRole 节点角色
type NodeRole string

const (
	RoleUnknown NodeRole = "unknown"
	RoleEntry   NodeRole = "entry"  // 入口节点
	RoleExit    NodeRole = "exit"   // 出口节点
	RoleBridge  NodeRole = "bridge" // 桥接节点
	RoleRelay   NodeRole = "relay"  // 中继节点
)

func (r NodeRole) String() string {
	return string(r)
}

// NodeGroup 节点组信息
type NodeGroup struct {
	ID        string            `json:"id"`         // 组ID
	Name      string            `json:"name"`       // 组名称
	Role      NodeRole          `json:"role"`       // 组角色
	Region    string            `json:"region"`     // 地区
	Priority  int               `json:"priority"`   // 优先级
	Capacity  int               `json:"capacity"`   // 容量
	Current   int               `json:"current"`    // 当前节点数
	Metadata  map[string]string `json:"metadata"`   // 元数据
	CreatedAt time.Time         `json:"created_at"` // 创建时间
	UpdatedAt time.Time         `json:"updated_at"` // 更新时间
}

// RoleAssignment 角色分配信息
type RoleAssignment struct {
	NodeID        string                 `json:"node_id"`       // 节点ID
	Role          NodeRole               `json:"role"`          // 分配的角色
	Group         *NodeGroup             `json:"group"`         // 所属组
	AssignedAt    time.Time              `json:"assigned_at"`   // 分配时间
	ExpiresAt     *time.Time             `json:"expires_at"`    // 过期时间
	Capabilities  []string               `json:"capabilities"`  // 要求的能力
	Configuration map[string]interface{} `json:"configuration"` // 角色特定配置
}

// RuleInfo 规则信息
type RuleInfo struct {
	ID         string                 `json:"id"`          // 规则ID
	Name       string                 `json:"name"`        // 规则名称
	EntryGroup string                 `json:"entry_group"` // 入口组ID
	ExitGroup  string                 `json:"exit_group"`  // 出口组ID
	Protocol   string                 `json:"protocol"`    // 协议
	Port       int                    `json:"port"`        // 端口
	TargetHost string                 `json:"target_host"` // 目标主机
	TargetPort int                    `json:"target_port"` // 目标端口
	Config     map[string]interface{} `json:"config"`      // 配置
	Status     string                 `json:"status"`      // 状态
	CreatedAt  time.Time              `json:"created_at"`  // 创建时间
	UpdatedAt  time.Time              `json:"updated_at"`  // 更新时间
}

// Manager 协议管理器
type Manager struct {
	transport *transport.Manager
	logger    *zap.Logger

	// 协议优化组件
	zeroCopyEngine *ZeroCopyEngine
	batchProcessor *BatchProcessor
	bufferManager  *BufferManager

	// 角色管理
	currentRole  atomic.Value // *RoleAssignment
	roleHistory  []*RoleAssignment
	roleHandlers []RoleChangeHandler
	roleMutex    sync.RWMutex

	// 规则管理
	activeRules  map[string]*RuleInfo
	ruleHandlers []RuleChangeHandler
	rulesMutex   sync.RWMutex

	// 状态
	ctx    context.Context
	cancel context.CancelFunc

	// 统计
	stats struct {
		roleChanges        atomic.Int64
		ruleUpdates        atomic.Int64
		tcpConnections     atomic.Int64
		udpSessions        atomic.Int64
		httpRequests       atomic.Int64
		protocolDetections atomic.Int64
		zeroCopyBytes      atomic.Int64
		batchesProcessed   atomic.Int64
		bufferAllocations  atomic.Int64
	}
}

// RoleChangeHandler 角色变更处理器
type RoleChangeHandler func(oldRole, newRole *RoleAssignment) error

// RuleChangeHandler 规则变更处理器
type RuleChangeHandler func(action string, rule *RuleInfo) error

// NewManager 创建协议管理器 (通用接口)
func NewManager() (*Manager, error) {
	// 传入nil transport，稍后会由app进行初始化
	return New(nil)
}

// New 创建协议管理器
func New(transport *transport.Manager) (*Manager, error) {
	manager := &Manager{
		transport:   transport,
		logger:      zap.L().Named("protocol"),
		activeRules: make(map[string]*RuleInfo),
	}

	// 初始化角色为未知
	manager.currentRole.Store((*RoleAssignment)(nil))

	// 初始化协议优化组件
	var err error

	// 1. 初始化零拷贝引擎
	zeroCopyConfig := DefaultZeroCopyConfig()
	manager.zeroCopyEngine = NewZeroCopyEngine(zeroCopyConfig)

	// 2. 初始化批处理器
	batchConfig := DefaultBatchConfig()
	manager.batchProcessor = NewBatchProcessor(batchConfig)

	// 3. 初始化缓冲区管理器
	bufferConfig := DefaultBufferConfig()
	manager.bufferManager = NewBufferManager(bufferConfig)

	manager.logger.Info("🚀 协议管理器初始化完成",
		zap.Bool("zero_copy", zeroCopyConfig.EnableSendfile),
		zap.Int("batch_size", batchConfig.MaxBatchSize),
		zap.Int("buffer_pools", len(bufferConfig.PoolSizes)))

	return manager, err
}

// Start 启动协议管理器
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	m.logger.Info("📡 协议管理器启动")

	// 启动优化组件
	if err := m.startOptimizationComponents(); err != nil {
		return fmt.Errorf("启动优化组件失败: %w", err)
	}

	// 启动角色协商
	go m.startRoleNegotiation()

	// 启动性能监控
	go m.startPerformanceMonitoring()

	return nil
}

// Stop 停止协议管理器
func (m *Manager) Stop() error {
	m.logger.Info("🛑 正在停止协议管理器...")

	if m.cancel != nil {
		m.cancel()
	}

	// 停止优化组件
	m.stopOptimizationComponents()

	m.logger.Info("✅ 协议管理器已停止")
	return nil
}

// startRoleNegotiation 启动角色协商
func (m *Manager) startRoleNegotiation() {
	m.logger.Info("🎭 开始角色协商...")

	// TODO: 发送角色请求到Plane
	// 这里暂时模拟，实际应该通过Plane连接发送

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if m.GetCurrentRole() == nil {
				m.logger.Debug("尚未分配角色，等待Plane分配...")
			}

		case <-m.ctx.Done():
			return
		}
	}
}

// HandleRoleAssignment 处理角色分配
func (m *Manager) HandleRoleAssignment(assignment *RoleAssignment) error {
	m.roleMutex.Lock()
	defer m.roleMutex.Unlock()

	oldRole := m.getCurrentRoleUnsafe()

	// 验证角色分配
	if err := m.validateRoleAssignment(assignment); err != nil {
		return fmt.Errorf("角色分配验证失败: %w", err)
	}

	// 更新角色
	m.currentRole.Store(assignment)
	m.roleHistory = append(m.roleHistory, assignment)
	m.stats.roleChanges.Add(1)

	m.logger.Info("🎭 角色分配成功",
		zap.String("old_role", m.getRoleString(oldRole)),
		zap.String("new_role", assignment.Role.String()),
		zap.String("group_id", assignment.Group.ID),
		zap.String("group_name", assignment.Group.Name))

	// 通知角色变更处理器
	for _, handler := range m.roleHandlers {
		if err := handler(oldRole, assignment); err != nil {
			m.logger.Error("角色变更处理器执行失败", zap.Error(err))
		}
	}

	return nil
}

// validateRoleAssignment 验证角色分配
func (m *Manager) validateRoleAssignment(assignment *RoleAssignment) error {
	if assignment == nil {
		return fmt.Errorf("角色分配不能为空")
	}

	if assignment.NodeID == "" {
		return fmt.Errorf("节点ID不能为空")
	}

	if assignment.Role == RoleUnknown {
		return fmt.Errorf("无效的角色")
	}

	if assignment.Group == nil {
		return fmt.Errorf("节点组信息不能为空")
	}

	// 检查过期时间
	if assignment.ExpiresAt != nil && assignment.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("角色分配已过期")
	}

	return nil
}

// HandleRuleSync 处理规则同步
func (m *Manager) HandleRuleSync(rules []*RuleInfo) error {
	m.rulesMutex.Lock()
	defer m.rulesMutex.Unlock()

	m.logger.Info("📋 接收到规则同步", zap.Int("count", len(rules)))

	// 创建新的规则映射
	newRules := make(map[string]*RuleInfo)
	for _, rule := range rules {
		if err := m.validateRule(rule); err != nil {
			m.logger.Error("规则验证失败",
				zap.String("rule_id", rule.ID),
				zap.Error(err))
			continue
		}
		newRules[rule.ID] = rule
	}

	// 处理规则变更
	m.processRuleChanges(newRules)

	// 更新活跃规则
	m.activeRules = newRules
	m.stats.ruleUpdates.Add(1)

	m.logger.Info("📋 规则同步完成", zap.Int("active_rules", len(m.activeRules)))
	return nil
}

// validateRule 验证规则
func (m *Manager) validateRule(rule *RuleInfo) error {
	if rule == nil {
		return fmt.Errorf("规则不能为空")
	}

	if rule.ID == "" {
		return fmt.Errorf("规则ID不能为空")
	}

	if rule.EntryGroup == "" || rule.ExitGroup == "" {
		return fmt.Errorf("入口组和出口组不能为空")
	}

	if rule.Port <= 0 || rule.Port > 65535 {
		return fmt.Errorf("无效的端口号: %d", rule.Port)
	}

	return nil
}

// processRuleChanges 处理规则变更
func (m *Manager) processRuleChanges(newRules map[string]*RuleInfo) {
	// 检查新增的规则
	for id, newRule := range newRules {
		if oldRule, exists := m.activeRules[id]; !exists {
			// 新增规则
			m.logger.Info("📋 新增规则",
				zap.String("rule_id", id),
				zap.String("protocol", newRule.Protocol),
				zap.Int("port", newRule.Port))

			for _, handler := range m.ruleHandlers {
				if err := handler("add", newRule); err != nil {
					m.logger.Error("规则新增处理器执行失败", zap.Error(err))
				}
			}
		} else if !m.ruleEquals(oldRule, newRule) {
			// 更新规则
			m.logger.Info("📋 更新规则",
				zap.String("rule_id", id),
				zap.String("protocol", newRule.Protocol),
				zap.Int("port", newRule.Port))

			for _, handler := range m.ruleHandlers {
				if err := handler("update", newRule); err != nil {
					m.logger.Error("规则更新处理器执行失败", zap.Error(err))
				}
			}
		}
	}

	// 检查删除的规则
	for id, oldRule := range m.activeRules {
		if _, exists := newRules[id]; !exists {
			// 删除规则
			m.logger.Info("📋 删除规则",
				zap.String("rule_id", id),
				zap.String("protocol", oldRule.Protocol),
				zap.Int("port", oldRule.Port))

			for _, handler := range m.ruleHandlers {
				if err := handler("delete", oldRule); err != nil {
					m.logger.Error("规则删除处理器执行失败", zap.Error(err))
				}
			}
		}
	}
}

// ruleEquals 比较两个规则是否相等
func (m *Manager) ruleEquals(rule1, rule2 *RuleInfo) bool {
	if rule1 == nil || rule2 == nil {
		return rule1 == rule2
	}

	return rule1.ID == rule2.ID &&
		rule1.EntryGroup == rule2.EntryGroup &&
		rule1.ExitGroup == rule2.ExitGroup &&
		rule1.Protocol == rule2.Protocol &&
		rule1.Port == rule2.Port &&
		rule1.TargetHost == rule2.TargetHost &&
		rule1.TargetPort == rule2.TargetPort &&
		rule1.Status == rule2.Status
}

// RegisterRoleChangeHandler 注册角色变更处理器
func (m *Manager) RegisterRoleChangeHandler(handler RoleChangeHandler) {
	m.roleMutex.Lock()
	defer m.roleMutex.Unlock()

	m.roleHandlers = append(m.roleHandlers, handler)
	m.logger.Debug("注册角色变更处理器", zap.Int("total", len(m.roleHandlers)))
}

// RegisterRuleChangeHandler 注册规则变更处理器
func (m *Manager) RegisterRuleChangeHandler(handler RuleChangeHandler) {
	m.rulesMutex.Lock()
	defer m.rulesMutex.Unlock()

	m.ruleHandlers = append(m.ruleHandlers, handler)
	m.logger.Debug("注册规则变更处理器", zap.Int("total", len(m.ruleHandlers)))
}

// GetCurrentRole 获取当前角色
func (m *Manager) GetCurrentRole() *RoleAssignment {
	if role := m.currentRole.Load(); role != nil {
		return role.(*RoleAssignment)
	}
	return nil
}

// getCurrentRoleUnsafe 获取当前角色（不安全版本，需要持有锁）
func (m *Manager) getCurrentRoleUnsafe() *RoleAssignment {
	if role := m.currentRole.Load(); role != nil {
		return role.(*RoleAssignment)
	}
	return nil
}

// getRoleString 获取角色字符串
func (m *Manager) getRoleString(role *RoleAssignment) string {
	if role == nil {
		return "none"
	}
	return role.Role.String()
}

// GetRoleHistory 获取角色历史
func (m *Manager) GetRoleHistory() []*RoleAssignment {
	m.roleMutex.RLock()
	defer m.roleMutex.RUnlock()

	// 返回副本
	history := make([]*RoleAssignment, len(m.roleHistory))
	copy(history, m.roleHistory)
	return history
}

// GetActiveRules 获取活跃规则
func (m *Manager) GetActiveRules() map[string]*RuleInfo {
	m.rulesMutex.RLock()
	defer m.rulesMutex.RUnlock()

	// 返回副本
	rules := make(map[string]*RuleInfo)
	for id, rule := range m.activeRules {
		rules[id] = rule
	}
	return rules
}

// IsEntryNode 检查是否为入口节点
func (m *Manager) IsEntryNode() bool {
	role := m.GetCurrentRole()
	return role != nil && role.Role == RoleEntry
}

// IsExitNode 检查是否为出口节点
func (m *Manager) IsExitNode() bool {
	role := m.GetCurrentRole()
	return role != nil && role.Role == RoleExit
}

// IsBridgeNode 检查是否为桥接节点
func (m *Manager) IsBridgeNode() bool {
	role := m.GetCurrentRole()
	return role != nil && role.Role == RoleBridge
}

// IsRelayNode 检查是否为中继节点
func (m *Manager) IsRelayNode() bool {
	role := m.GetCurrentRole()
	return role != nil && role.Role == RoleRelay
}

// GetStats 获取协议统计
func (m *Manager) GetStats() map[string]interface{} {
	currentRole := m.GetCurrentRole()

	stats := map[string]interface{}{
		"tcp_connections":     m.stats.tcpConnections.Load(),
		"udp_sessions":        m.stats.udpSessions.Load(),
		"http_requests":       m.stats.httpRequests.Load(),
		"protocol_detections": m.stats.protocolDetections.Load(),
		"role_changes":        m.stats.roleChanges.Load(),
		"rule_updates":        m.stats.ruleUpdates.Load(),
		"active_rules":        len(m.activeRules),
	}

	if currentRole != nil {
		stats["current_role"] = currentRole.Role.String()
		stats["group_id"] = currentRole.Group.ID
		stats["group_name"] = currentRole.Group.Name
		stats["assigned_at"] = currentRole.AssignedAt
	} else {
		stats["current_role"] = "none"
	}

	// 添加优化组件统计
	if m.zeroCopyEngine != nil {
		zeroCopyStats := m.zeroCopyEngine.GetStats()
		stats["zero_copy"] = zeroCopyStats
		stats["zero_copy_bytes"] = m.stats.zeroCopyBytes.Load()
	}

	if m.batchProcessor != nil {
		batchStats := m.batchProcessor.GetStats()
		stats["batch_processor"] = batchStats
		stats["batches_processed"] = m.stats.batchesProcessed.Load()
	}

	if m.bufferManager != nil {
		bufferStats := m.bufferManager.GetStats()
		stats["buffer_manager"] = bufferStats
		stats["buffer_allocations"] = m.stats.bufferAllocations.Load()
	}

	return stats
}

// startOptimizationComponents 启动优化组件
func (m *Manager) startOptimizationComponents() error {
	// 启动缓冲区管理器
	if err := m.bufferManager.Start(m.ctx); err != nil {
		return fmt.Errorf("启动缓冲区管理器失败: %w", err)
	}

	// 启动批处理器
	if err := m.batchProcessor.Start(m.ctx); err != nil {
		return fmt.Errorf("启动批处理器失败: %w", err)
	}

	// 启动零拷贝引擎
	if err := m.zeroCopyEngine.Start(m.ctx); err != nil {
		return fmt.Errorf("启动零拷贝引擎失败: %w", err)
	}

	m.logger.Info("✅ 优化组件启动完成")
	return nil
}

// stopOptimizationComponents 停止优化组件
func (m *Manager) stopOptimizationComponents() {
	// 按相反顺序停止
	if m.zeroCopyEngine != nil {
		if err := m.zeroCopyEngine.Stop(); err != nil {
			m.logger.Error("停止零拷贝引擎失败", zap.Error(err))
		}
	}

	if m.batchProcessor != nil {
		if err := m.batchProcessor.Stop(); err != nil {
			m.logger.Error("停止批处理器失败", zap.Error(err))
		}
	}

	if m.bufferManager != nil {
		if err := m.bufferManager.Stop(); err != nil {
			m.logger.Error("停止缓冲区管理器失败", zap.Error(err))
		}
	}

	m.logger.Info("✅ 优化组件已停止")
}

// startPerformanceMonitoring 启动性能监控
func (m *Manager) startPerformanceMonitoring() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.logPerformanceStats()
			m.optimizeComponents()
		case <-m.ctx.Done():
			return
		}
	}
}

// logPerformanceStats 记录性能统计
func (m *Manager) logPerformanceStats() {
	stats := m.GetStats()

	m.logger.Info("📈 协议管理器性能统计",
		zap.String("current_role", stats["current_role"].(string)),
		zap.Int("active_rules", stats["active_rules"].(int)),
		zap.Int64("tcp_connections", stats["tcp_connections"].(int64)),
		zap.Int64("udp_sessions", stats["udp_sessions"].(int64)),
		zap.Int64("zero_copy_bytes", m.stats.zeroCopyBytes.Load()),
		zap.Int64("batches_processed", m.stats.batchesProcessed.Load()))
}

// optimizeComponents 优化组件
func (m *Manager) optimizeComponents() {
	// 根据当前负载调整优化策略
	currentRole := m.GetCurrentRole()
	if currentRole == nil {
		return
	}

	// 根据角色优化
	switch currentRole.Role {
	case RoleEntry:
		// 入口节点优化延迟
		m.zeroCopyEngine.OptimizeForLatency()
	case RoleExit:
		// 出口节点优化吞吐量
		m.zeroCopyEngine.OptimizeForThroughput()
	default:
		// 默认平衡优化
	}
}

// ProcessData 使用优化组件处理数据
func (m *Manager) ProcessData(data []byte, priority int) error {
	// 使用批处理器处理数据
	item := CreateBatchItem(data, priority, func(item *BatchItem, result *BatchResult) {
		if result.Success {
			m.stats.batchesProcessed.Add(1)
		} else {
			m.logger.Error("批处理失败",
				zap.String("item_id", item.ID),
				zap.Error(result.Error))
		}
	})

	return m.batchProcessor.Submit(item)
}

// GetBuffer 获取缓冲区
func (m *Manager) GetBuffer(size int) *SmartBuffer {
	m.stats.bufferAllocations.Add(1)
	return m.bufferManager.GetBuffer(size)
}

// ZeroCopy 执行零拷贝传输
func (m *Manager) ZeroCopy(source io.Reader, dest io.Writer, size int64) (int64, error) {
	copied, err := m.zeroCopyEngine.Copy(source, dest, size)
	if err == nil {
		m.stats.zeroCopyBytes.Add(copied)
	}
	return copied, err
}

// SetOptimizationMode 设置优化模式
func (m *Manager) SetOptimizationMode(mode string) error {
	switch mode {
	case "latency":
		m.zeroCopyEngine.OptimizeForLatency()
		m.logger.Info("🏃 已切换到延迟优先模式")
	case "throughput":
		m.zeroCopyEngine.OptimizeForThroughput()
		m.logger.Info("📦 已切换到吞吐量优先模式")
	default:
		return fmt.Errorf("不支持的优化模式: %s", mode)
	}
	return nil
}
