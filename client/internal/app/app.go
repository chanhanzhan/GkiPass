package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"gkipass/client/internal/auth"
	"gkipass/client/internal/cache"
	"gkipass/client/internal/config"
	"gkipass/client/internal/debug"
	"gkipass/client/internal/diagnostics"
	"gkipass/client/internal/identity"
	"gkipass/client/internal/optimizer"
	"gkipass/client/internal/performance"
	"gkipass/client/internal/plane"
	"gkipass/client/internal/pool"
	"gkipass/client/internal/protocol"
	"gkipass/client/internal/rules"
	"gkipass/client/internal/tls"
	"gkipass/client/internal/transport"
)

// Application 应用程序
type Application struct {
	cfg                 *config.Config
	ctx                 context.Context
	cancel              context.CancelFunc
	identityManager     *identity.Manager
	authManager         *auth.Manager
	planeManager        *plane.Manager
	ruleManager         *rules.Manager
	poolManager         *pool.Manager
	protocolManager     *protocol.Manager
	tlsManager          *tls.Manager
	debugManager        *debug.Manager
	diagnosticsManager  *diagnostics.Manager
	performanceAnalyzer *performance.PerformanceAnalyzer
	memoryOptimizer     *optimizer.MemoryOptimizer
	goroutineOptimizer  *optimizer.GoroutineOptimizer
	cacheManager        *cache.SmartCache
	trafficManager      *protocol.TrafficManager
	logger              *zap.Logger
}

// New 创建应用程序
func New(cfg *config.Config) (*Application, error) {
	ctx, cancel := context.WithCancel(context.Background())

	logger := zap.L().Named("app")

	app := &Application{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		logger: logger,
	}

	if err := app.initComponents(); err != nil {
		cancel()
		return nil, err
	}

	return app, nil
}

// initComponents 初始化组件
func (a *Application) initComponents() error {
	var err error

	// 初始化身份管理器
	identityConfig := &identity.ManagerConfig{
		DataDir:             a.cfg.DataDir,
		IdentityFile:        "identity.json",
		PlaneAddr:           a.cfg.Plane.URL,
		APIKey:              a.cfg.Plane.APIKey,
		Token:               a.cfg.Plane.Token,
		AutoGenerate:        true,
		EnableHardwareID:    true,
		RefreshInterval:     time.Hour,
		TokenRefreshEnabled: true,
		TokenRefreshWindow:  time.Minute * 5,
	}
	a.identityManager, err = identity.NewManager(identityConfig)
	if err != nil {
		return fmt.Errorf("初始化身份管理器失败: %w", err)
	}

	// 初始化认证管理器
	authConfig := &auth.ManagerConfig{
		PlaneURL:       a.cfg.Plane.URL,
		APIEndpoint:    "/api/v1",
		Token:          a.cfg.Plane.Token,
		APIKey:         a.cfg.Plane.APIKey,
		ConnectTimeout: a.cfg.Plane.ConnectTimeout,
		RequestTimeout: time.Second * 30,
		RetryInterval:  time.Second * 5,
		MaxRetries:     3,
		RefreshWindow:  time.Minute * 5,
		TLSVerify:      true,
	}
	a.authManager = auth.NewManager(authConfig)
	a.authManager.SetIdentityManager(a.identityManager)

	// 初始化Plane管理器
	planeConfig := &plane.Config{
		URL:                  a.cfg.Plane.URL,
		Token:                a.cfg.Plane.Token,
		APIKey:               a.cfg.Plane.APIKey,
		ConnectTimeout:       a.cfg.Plane.ConnectTimeout,
		ReconnectInterval:    a.cfg.Plane.ReconnectInterval,
		MaxReconnectAttempts: a.cfg.Plane.MaxReconnectAttempts,
		HeartbeatInterval:    a.cfg.Plane.HeartbeatInterval,
	}
	a.planeManager = plane.NewManager(planeConfig)
	a.planeManager.SetIdentityManager(a.identityManager)
	a.planeManager.SetAuthManager(a.authManager)

	// 初始化连接池管理器
	poolConfig := pool.DefaultPoolConfig()
	if a.cfg.Debug != nil && a.cfg.Debug.Enabled {
		poolConfig = pool.HighPerformanceConfig()
	} else {
		poolConfig = pool.LowResourceConfig()
	}
	a.poolManager = pool.NewPoolManagerWithConfig(poolConfig)

	// 初始化传输管理器（用于协议管理器）
	transportManager, err := transport.New(nil)
	if err != nil {
		return fmt.Errorf("初始化传输管理器失败: %w", err)
	}

	// 初始化协议管理器
	a.protocolManager, err = protocol.New(transportManager)
	if err != nil {
		return fmt.Errorf("初始化协议管理器失败: %w", err)
	}
	if a.cfg.Debug != nil && a.cfg.Debug.Enabled {
		a.protocolManager.SetOptimizationMode("latency")
	} else {
		a.protocolManager.SetOptimizationMode("throughput")
	}

	// 初始化协议转发器
	forwarderConfig := &protocol.ForwarderConfig{
		BufferSize:        32 * 1024,
		IdleTimeout:       5 * time.Minute,
		MaxConnections:    1000,
		EnableCompression: false,
		RateLimitBPS:      0,
		Logger:            a.logger,
	}
	forwarder := protocol.NewForwarder(forwarderConfig)

	// 初始化规则管理器
	rulesConfig := &rules.ManagerConfig{
		RulesDir:         a.cfg.DataDir + "/rules",
		CacheRules:       true,
		AutoReload:       true,
		ReloadInterval:   time.Minute * 5,
		BackupRules:      true,
		MaxBackups:       10,
		EnableVersioning: true,
	}
	a.ruleManager, err = rules.NewManager(rulesConfig, forwarder)
	if err != nil {
		return fmt.Errorf("初始化规则管理器失败: %w", err)
	}

	// 初始化TLS管理器
	tlsConfig := &tls.Config{
		CertFile:            a.cfg.TLS.CertFile,
		KeyFile:             a.cfg.TLS.KeyFile,
		CAFile:              a.cfg.TLS.CAFile,
		ServerName:          a.cfg.TLS.ServerName,
		InsecureSkipVerify:  a.cfg.TLS.SkipVerify,
		CertRefreshInterval: time.Hour,
	}
	a.tlsManager = tls.NewManager(tlsConfig)

	// 初始化调试管理器
	if a.cfg.Debug != nil && a.cfg.Debug.Enabled {
		a.debugManager = debug.NewManager(a.cfg.Debug)
	}

	// 初始化诊断管理器
	a.diagnosticsManager = diagnostics.NewManager(nil)

	// 初始化性能分析器
	performanceConfig := performance.DefaultAnalyzerConfig()
	a.performanceAnalyzer = performance.NewPerformanceAnalyzer(performanceConfig)

	// 初始化内存优化器
	a.memoryOptimizer = optimizer.NewMemoryOptimizer(nil)

	// 初始化Goroutine优化器
	a.goroutineOptimizer = optimizer.NewGoroutineOptimizer(nil)

	// 初始化缓存管理器
	cacheConfig := &cache.CacheConfig{
		MaxSize:           1024 * 1024 * 100,
		DefaultTTL:        time.Hour,
		ShardCount:        32,
		EnableCompression: false,
		EvictionPolicy:    "lru",
	}
	a.cacheManager = cache.NewSmartCache(cacheConfig)

	// 初始化流量管理器
	a.trafficManager = protocol.NewTrafficManager()

	return nil
}

// Start 启动应用程序
func (a *Application) Start() error {
	a.logger.Info("正在启动应用程序...")

	// 启动认证管理器
	if err := a.authManager.Start(); err != nil {
		return fmt.Errorf("启动认证管理器失败: %w", err)
	}

	// 启动Plane管理器
	if err := a.planeManager.Start(); err != nil {
		return fmt.Errorf("启动Plane管理器失败: %w", err)
	}

	// 启动规则管理器
	if err := a.ruleManager.Start(); err != nil {
		return fmt.Errorf("启动规则管理器失败: %w", err)
	}

	// 启动连接池管理器
	if err := a.poolManager.Start(); err != nil {
		return fmt.Errorf("启动连接池管理器失败: %w", err)
	}

	// 启动协议管理器
	if err := a.protocolManager.Start(a.ctx); err != nil {
		return fmt.Errorf("启动协议管理器失败: %w", err)
	}

	// 启动TLS管理器
	if err := a.tlsManager.Start(); err != nil {
		return fmt.Errorf("启动TLS管理器失败: %w", err)
	}

	// 启动调试管理器
	if a.debugManager != nil {
		if err := a.debugManager.Start(); err != nil {
			return fmt.Errorf("启动调试管理器失败: %w", err)
		}
	}

	// 启动诊断管理器
	if err := a.diagnosticsManager.Start(); err != nil {
		return fmt.Errorf("启动诊断管理器失败: %w", err)
	}

	// 启动性能分析器
	if err := a.performanceAnalyzer.Start(); err != nil {
		return fmt.Errorf("启动性能分析器失败: %w", err)
	}

	// 启动内存优化器
	if err := a.memoryOptimizer.Start(); err != nil {
		return fmt.Errorf("启动内存优化器失败: %w", err)
	}

	// 启动Goroutine优化器
	if err := a.goroutineOptimizer.Start(); err != nil {
		return fmt.Errorf("启动Goroutine优化器失败: %w", err)
	}

	// 启动缓存管理器
	if err := a.cacheManager.Start(a.ctx); err != nil {
		return fmt.Errorf("启动缓存管理器失败: %w", err)
	}

	// 启动流量管理器
	if err := a.trafficManager.Start(); err != nil {
		return fmt.Errorf("启动流量管理器失败: %w", err)
	}

	a.logger.Info("应用程序已启动")

	return nil
}

// Stop 停止应用程序
func (a *Application) Stop() error {
	a.logger.Info("正在停止应用程序...")

	// 创建一个包含所有组件的列表，按照依赖关系的相反顺序停止
	stopComponents := []struct {
		name string
		stop func() error
	}{
		{"流量管理器", a.trafficManager.Stop},
		{"缓存管理器", a.cacheManager.Stop},
		{"Goroutine优化器", a.goroutineOptimizer.Stop},
		{"内存优化器", a.memoryOptimizer.Stop},
		{"性能分析器", a.performanceAnalyzer.Stop},
		{"诊断管理器", a.diagnosticsManager.Stop},
	}

	if a.debugManager != nil {
		stopComponents = append(stopComponents, struct {
			name string
			stop func() error
		}{"调试管理器", a.debugManager.Stop})
	}

	stopComponents = append(stopComponents,
		struct {
			name string
			stop func() error
		}{"TLS管理器", a.tlsManager.Stop},
		struct {
			name string
			stop func() error
		}{"协议管理器", a.protocolManager.Stop},
		struct {
			name string
			stop func() error
		}{"连接池管理器", a.poolManager.Stop},
		struct {
			name string
			stop func() error
		}{"规则管理器", a.ruleManager.Stop},
		struct {
			name string
			stop func() error
		}{"Plane管理器", a.planeManager.Stop},
		struct {
			name string
			stop func() error
		}{"认证管理器", a.authManager.Stop},
	)

	// 停止所有组件
	for _, component := range stopComponents {
		if err := component.stop(); err != nil {
			a.logger.Error(fmt.Sprintf("停止%s失败", component.name), zap.Error(err))
		}
	}

	// 取消上下文
	a.cancel()

	a.logger.Info("应用程序已停止")

	return nil
}

// Run 运行应用程序
func (a *Application) Run() error {
	// 启动应用程序
	if err := a.Start(); err != nil {
		return err
	}

	// 处理信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	sig := <-sigCh
	a.logger.Info("收到信号，正在优雅关闭", zap.String("signal", sig.String()))

	// 创建一个带有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 在新的上下文中停止应用程序
	go func() {
		if err := a.Stop(); err != nil {
			a.logger.Error("停止应用程序失败", zap.Error(err))
		}
		cancel()
	}()

	// 等待停止完成或超时
	<-ctx.Done()
	if ctx.Err() == context.DeadlineExceeded {
		a.logger.Warn("优雅关闭超时，强制退出")
	}

	return nil
}

// GetStatus 获取应用程序状态
func (a *Application) GetStatus() map[string]interface{} {
	status := map[string]interface{}{
		"node_id":    a.identityManager.GetNodeID(),
		"start_time": time.Now().Format(time.RFC3339),
		"components": map[string]interface{}{
			"auth":        a.authManager.GetStatus(),
			"plane":       a.planeManager.GetStatus(),
			"pool":        a.poolManager.GetStats(),
			"protocol":    a.protocolManager.GetStats(),
			"tls":         a.tlsManager.GetStatus(),
			"diagnostics": a.diagnosticsManager.GetStats(),
			"performance": a.performanceAnalyzer.GetStats(),
			"memory":      a.memoryOptimizer.GetStats(),
			"goroutine":   a.goroutineOptimizer.GetStats(),
			"cache":       a.cacheManager.GetStats(),
			"traffic":     a.trafficManager.GetStats(),
		},
	}

	if a.debugManager != nil {
		status["components"].(map[string]interface{})["debug"] = a.debugManager.GetStats()
	}

	return status
}
