package plane

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"gkipass/client/internal/auth"
	"gkipass/client/internal/identity"
)

// Config 面板配置
type Config struct {
	URL                  string        `json:"url"`                    // 面板URL
	Token                string        `json:"token"`                  // 认证Token
	APIKey               string        `json:"api_key"`                // API密钥
	ConnectTimeout       time.Duration `json:"connect_timeout"`        // 连接超时
	ReconnectInterval    time.Duration `json:"reconnect_interval"`     // 重连间隔
	MaxReconnectAttempts int           `json:"max_reconnect_attempts"` // 最大重连次数
	HeartbeatInterval    time.Duration `json:"heartbeat_interval"`     // 心跳间隔
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		ConnectTimeout:       30 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 0, // 无限重连
		HeartbeatInterval:    30 * time.Second,
	}
}

// Manager 面板管理器
type Manager struct {
	config          *Config
	identityManager *identity.Manager
	authManager     *auth.Manager
	logger          *zap.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	lock   sync.RWMutex
}

// NewManager 创建面板管理器
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		config: config,
		logger: zap.L().Named("plane"),
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetIdentityManager 设置身份管理器
func (m *Manager) SetIdentityManager(identityManager *identity.Manager) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.identityManager = identityManager
}

// SetAuthManager 设置认证管理器
func (m *Manager) SetAuthManager(authManager *auth.Manager) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.authManager = authManager
}

// Start 启动面板管理器
func (m *Manager) Start() error {
	m.logger.Info("启动面板管理器")

	// 启动连接协程
	m.wg.Add(1)
	go m.connectionLoop()

	return nil
}

// Stop 停止面板管理器
func (m *Manager) Stop() error {
	m.logger.Info("停止面板管理器")

	// 取消上下文
	m.cancel()

	// 等待协程结束
	m.wg.Wait()

	return nil
}

// GetStatus 获取状态
func (m *Manager) GetStatus() map[string]interface{} {
	m.lock.RLock()
	defer m.lock.RUnlock()

	status := map[string]interface{}{
		"url": m.config.URL,
	}

	return status
}

// connectionLoop 连接循环
func (m *Manager) connectionLoop() {
	defer m.wg.Done()

	// 等待身份和认证管理器准备好
	if err := m.waitForDependencies(); err != nil {
		m.logger.Error("等待依赖失败", zap.Error(err))
		return
	}

	// TODO: 实现连接逻辑

	// 等待取消
	<-m.ctx.Done()
}

// waitForDependencies 等待依赖
func (m *Manager) waitForDependencies() error {
	// 等待身份管理器
	for i := 0; i < 10; i++ {
		m.lock.RLock()
		if m.identityManager != nil && m.authManager != nil {
			m.lock.RUnlock()
			return nil
		}
		m.lock.RUnlock()

		select {
		case <-m.ctx.Done():
			return context.Canceled
		case <-time.After(500 * time.Millisecond):
			// 继续等待
		}
	}

	return nil
}
