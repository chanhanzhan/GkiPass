package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"gkipass/client/internal/app"
	"gkipass/client/internal/config"
)

const (
	// 应用信息
	AppName    = "gkipass-node"
	AppVersion = "1.0.0"

	// 默认配置
	DefaultConfigPath = "./config.json"
	DefaultLogLevel   = "info"
	DefaultDataDir    = "./data"
)

// 命令行参数
var (
	configPath = flag.String("config", DefaultConfigPath, "配置文件路径")
	logLevel   = flag.String("log-level", DefaultLogLevel, "日志级别 (debug, info, warn, error)")
	dataDir    = flag.String("data-dir", DefaultDataDir, "数据目录")
	daemon     = flag.Bool("daemon", false, "以守护进程模式运行")
	version    = flag.Bool("version", false, "显示版本信息")
	help       = flag.Bool("help", false, "显示帮助信息")

	// 运行时参数
	maxProcs  = flag.Int("max-procs", 0, "最大CPU核心数 (0=自动)")
	memLimit  = flag.String("mem-limit", "", "内存限制 (如: 1GB, 512MB)")
	pprofAddr = flag.String("pprof", "", "pprof监听地址 (如: :6060)")

	// 调试参数 - 模拟客户端/服务端
	debugMode     = flag.String("debug", "", "调试模式 (server[:port]/client:ip:port) 支持协议: tcp/udp/ws/wss/tls/tls-mux/kcp/quic")
	debugProtocol = flag.String("protocol", "tcp", "调试协议 (tcp/udp/ws/wss/tls/tls-mux/kcp/quic)")

	// 客户端调试参数
	token     = flag.String("token", "", "客户端认证令牌 (客户端模式必需)")
	planeAddr = flag.String("s", "", "Plane服务器地址 (客户端模式必需)")
	apiKey    = flag.String("key", "", "服务端API密钥 (可设置或随机生成)")

	// 其他调试参数
	trafficTest  = flag.Bool("traffic-test", false, "启用流量测试")
	testDataSize = flag.Int("test-data-size", 1024, "测试数据大小 (字节)")

	// 开发参数
	devMode = flag.Bool("dev", false, "开发模式")
)

func main() {
	// 自定义flag用法信息
	flag.Usage = printHelp

	// 解析命令行参数
	flag.Parse()

	// 显示版本信息
	if *version {
		printVersion()
		os.Exit(0)
	}

	// 显示帮助信息
	if *help {
		printHelp()
		os.Exit(0)
	}

	// 没有参数时显示帮助
	if len(os.Args) == 1 {
		printHelp()
		os.Exit(0)
	}

	// 设置运行时参数
	setupRuntime()

	// 初始化日志
	logger := setupLogger(*logLevel)
	defer logger.Sync()

	// 设置全局日志
	zap.ReplaceGlobals(logger)

	logger.Info("启动GKI Pass节点",
		zap.String("version", AppVersion),
		zap.String("config_path", *configPath),
		zap.String("data_dir", *dataDir),
		zap.String("log_level", *logLevel),
		zap.Bool("daemon", *daemon),
		zap.Bool("dev_mode", *devMode))

	// 创建数据目录
	if err := createDataDir(*dataDir); err != nil {
		logger.Fatal("创建数据目录失败", zap.Error(err))
	}

	// 加载配置
	cfg, err := loadConfig(*configPath)
	if err != nil {
		logger.Fatal("加载配置失败", zap.Error(err))
	}

	// 从命令行参数覆盖部分配置
	if *apiKey != "" {
		cfg.Plane.APIKey = *apiKey
		logger.Debug("使用命令行指定的API密钥", zap.String("api_key", "*****"))
	}

	// 验证配置
	if err := validateConfig(cfg); err != nil {
		logger.Fatal("配置验证失败", zap.Error(err))
	}

	// 设置数据目录
	cfg.DataDir = *dataDir

	// 配置调试选项
	if *debugMode != "" {
		debugConfig, err := parseDebugMode(*debugMode, *debugProtocol, *token, *planeAddr, *apiKey, *trafficTest, *testDataSize, *logLevel)
		if err != nil {
			logger.Fatal("调试模式配置错误", zap.Error(err))
		}

		cfg.Debug = debugConfig

		logger.Info("启用调试模式",
			zap.String("mode", debugConfig.Mode),
			zap.String("protocol", debugConfig.Protocol),
			zap.String("target_addr", debugConfig.TargetAddr),
			zap.Int("listen_port", debugConfig.ListenPort),
			zap.Bool("traffic_test", debugConfig.TrafficTest))
	}

	// 创建应用实例
	application, err := app.New(cfg)
	if err != nil {
		logger.Fatal("创建应用实例失败", zap.Error(err))
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	setupSignalHandlers(ctx, cancel, application, logger)

	// 启动应用
	logger.Info("启动应用服务")
	if err := application.Start(); err != nil {
		logger.Fatal("启动应用失败", zap.Error(err))
	}

	// 等待停止信号
	<-ctx.Done()

	// 优雅停止应用
	logger.Info("开始优雅停止应用")

	if err := application.Stop(); err != nil {
		logger.Error("停止应用时发生错误", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("应用已优雅停止")
}

// printVersion 打印版本信息
func printVersion() {
	fmt.Printf("%s version %s\n", AppName, AppVersion)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Printf("Usage: %s [options]\n\n", AppName)

	fmt.Println("调试模式 (Debug Mode):")
	fmt.Println("  服务端模式: --debug server[:port] --protocol <协议>")
	fmt.Println("  客户端模式: --debug client:ip:port --protocol <协议> --token <令牌> -s <plane-server> [--key <API密钥>]")
	fmt.Println("")
	fmt.Println("支持的协议: tcp, udp, ws, wss, tls, tls-mux, kcp, quic")
	fmt.Println("")

	fmt.Println("Options:")
	flag.PrintDefaults()

	fmt.Println("\n调试模式示例:")
	fmt.Printf("  # 启动TCP服务端 (端口9230)\n")
	fmt.Printf("  %s --debug server --protocol tcp\n", AppName)
	fmt.Printf("\n")
	fmt.Printf("  # 启动TCP服务端 (自定义端口)\n")
	fmt.Printf("  %s --debug server:8080 --protocol tcp\n", AppName)
	fmt.Printf("\n")
	fmt.Printf("  # 连接到服务端\n")
	fmt.Printf("  %s --debug client:127.0.0.1:9230 --protocol tcp --token mytoken -s ws://plane.example.com/ws --key myapikey\n", AppName)
	fmt.Printf("\n")
	fmt.Printf("  # WebSocket服务端\n")
	fmt.Printf("  %s --debug server:8080 --protocol ws --traffic-test\n", AppName)
	fmt.Printf("\n")

	fmt.Println("常规模式示例:")
	fmt.Printf("  %s -config ./config.json\n", AppName)
	fmt.Printf("  %s -config ./config.json -log-level debug\n", AppName)
	fmt.Printf("  %s -config ./config.json -daemon\n", AppName)
	fmt.Printf("  %s -version\n", AppName)
}

// setupRuntime 设置运行时参数
func setupRuntime() {
	// 设置最大CPU核心数
	if *maxProcs > 0 {
		runtime.GOMAXPROCS(*maxProcs)
	}

	// 这里可以添加更多运行时设置
	// 如内存限制、GC调优等
}

// setupLogger 设置日志
func setupLogger(level string) *zap.Logger {
	// 解析日志级别
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	// 配置日志编码器
	var encoderConfig zapcore.EncoderConfig
	if *devMode {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		encoderConfig = zap.NewProductionEncoderConfig()
		encoderConfig.TimeKey = "timestamp"
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	// 创建编码器
	var encoder zapcore.Encoder
	if *devMode {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// 配置输出
	writeSyncer := zapcore.AddSync(os.Stdout)

	// 创建核心
	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)

	// 创建日志器
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return logger
}

// createDataDir 创建数据目录
func createDataDir(dataDir string) error {
	// 转换为绝对路径
	absPath, err := filepath.Abs(dataDir)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %w", err)
	}

	// 检查目录是否存在
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		// 创建目录
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
	}

	// 检查目录权限
	if err := checkDirPermissions(absPath); err != nil {
		return fmt.Errorf("检查目录权限失败: %w", err)
	}

	return nil
}

// checkDirPermissions 检查目录权限
func checkDirPermissions(dir string) error {
	// 检查读权限
	if _, err := os.Open(dir); err != nil {
		return fmt.Errorf("目录不可读: %w", err)
	}

	// 检查写权限
	testFile := filepath.Join(dir, ".write_test")
	file, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("目录不可写: %w", err)
	}
	file.Close()
	os.Remove(testFile)

	return nil
}

// loadConfig 加载配置
func loadConfig(configPath string) (*config.Config, error) {
	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// 如果配置文件不存在，创建默认配置
		cfg := config.DefaultConfig()
		if err := config.SaveConfig(cfg, configPath); err != nil {
			return nil, fmt.Errorf("保存默认配置失败: %w", err)
		}
		fmt.Printf("已创建默认配置文件: %s\n", configPath)
		return cfg, nil
	}

	// 加载配置
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	return cfg, nil
}

// validateConfig 验证配置
func validateConfig(cfg *config.Config) error {
	// 节点ID可以为空，将由身份管理器自动生成

	// 验证Plane地址
	if len(cfg.PlaneURLs) == 0 {
		return fmt.Errorf("至少需要配置一个Plane地址")
	}

	// 验证端口范围
	if cfg.Network.PortRange.Min >= cfg.Network.PortRange.Max {
		return fmt.Errorf("端口范围配置无效: %d >= %d",
			cfg.Network.PortRange.Min, cfg.Network.PortRange.Max)
	}

	// 验证证书配置
	if cfg.TLS.CertDir == "" {
		return fmt.Errorf("证书目录不能为空")
	}

	return nil
}

// setupSignalHandlers 设置信号处理
func setupSignalHandlers(ctx context.Context, cancel context.CancelFunc, app *app.Application, logger *zap.Logger) {
	sigChan := make(chan os.Signal, 1)

	// 监听信号
	signal.Notify(sigChan,
		syscall.SIGINT,  // Ctrl+C
		syscall.SIGTERM, // 终止信号
		syscall.SIGQUIT, // 退出信号
		syscall.SIGHUP,  // 挂起信号（用于重载配置）
	)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sig := <-sigChan:
				logger.Info("收到信号", zap.String("signal", sig.String()))

				switch sig {
				case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
					// 停止信号
					logger.Info("收到停止信号，开始优雅停止")
					cancel()
					return

				case syscall.SIGHUP:
					// 重载配置信号
					logger.Info("收到重载信号，开始重载配置")
					if err := reloadConfig(app, logger); err != nil {
						logger.Error("重载配置失败", zap.Error(err))
					}
				}
			}
		}
	}()
}

// reloadConfig 重载配置
func reloadConfig(app *app.Application, logger *zap.Logger) error {
	logger.Info("开始重载配置", zap.String("config_path", *configPath))

	// 加载新配置
	newCfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("加载新配置失败: %w", err)
	}

	// 验证新配置
	if err := validateConfig(newCfg); err != nil {
		return fmt.Errorf("新配置验证失败: %w", err)
	}

	// TODO: 实现配置重载功能
	// 重载应用配置
	// if err := app.ReloadConfig(newCfg); err != nil {
	// 	return fmt.Errorf("应用配置重载失败: %w", err)
	// }

	logger.Info("配置重载完成（功能未实现，需要重启应用）")
	return nil
}

// parseDebugMode 解析调试模式
func parseDebugMode(debugMode, protocol, token, planeAddr string, apiKey string, trafficTest bool, testDataSize int, logLevel string) (*config.DebugConfig, error) {
	// 验证协议
	validProtocols := map[string]bool{
		"tcp":     true,
		"udp":     true,
		"ws":      true,
		"wss":     true,
		"tls":     true,
		"tls-mux": true,
		"kcp":     true,
		"quic":    true,
	}

	if !validProtocols[protocol] {
		return nil, fmt.Errorf("不支持的协议: %s, 支持的协议: tcp/udp/ws/wss/tls/tls-mux/kcp/quic", protocol)
	}

	config := &config.DebugConfig{
		Enabled:      true,
		Protocol:     protocol,
		TrafficTest:  trafficTest,
		TestDataSize: testDataSize,
		TestInterval: 5 * time.Second,
		TestDuration: 30 * time.Second,
		LogLevel:     logLevel,
	}

	// 解析调试模式
	if strings.HasPrefix(debugMode, "server") {
		// 服务端模式: server 或 server:port
		config.Mode = "server"
		config.ListenPort = 9230 // 默认端口

		if strings.Contains(debugMode, ":") {
			parts := strings.Split(debugMode, ":")
			if len(parts) != 2 {
				return nil, fmt.Errorf("服务端模式格式错误，应为: server[:port]")
			}

			port, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("端口格式错误: %s", parts[1])
			}

			if port <= 0 || port > 65535 {
				return nil, fmt.Errorf("端口范围错误: %d，应在1-65535之间", port)
			}

			config.ListenPort = port
		}

	} else if strings.HasPrefix(debugMode, "client:") {
		// 客户端模式: client:ip:port
		config.Mode = "client"

		// 验证必需参数
		if token == "" {
			return nil, fmt.Errorf("客户端模式需要 --token 参数")
		}
		if planeAddr == "" {
			return nil, fmt.Errorf("客户端模式需要 -s plane-server 参数")
		}

		// 解析地址
		parts := strings.Split(debugMode, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("客户端模式格式错误，应为: client:ip:port")
		}

		ip := parts[1]
		if ip == "" {
			return nil, fmt.Errorf("IP地址不能为空")
		}

		port, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("端口格式错误: %s", parts[2])
		}

		if port <= 0 || port > 65535 {
			return nil, fmt.Errorf("端口范围错误: %d，应在1-65535之间", port)
		}

		config.TargetAddr = fmt.Sprintf("%s:%d", ip, port)
		config.Token = token
		config.PlaneAddr = planeAddr

		// API密钥，如果未指定则为空（服务端可以自动生成）
		if apiKey != "" {
			config.APIKey = apiKey
		}

	} else {
		return nil, fmt.Errorf("调试模式格式错误，应为: server[:port] 或 client:ip:port")
	}

	return config, nil
}

// handlePanic 处理panic
func handlePanic() {
	if r := recover(); r != nil {
		zap.L().Error("应用发生panic",
			zap.Any("panic", r),
			zap.Stack("stack"))
		os.Exit(1)
	}
}

// init 初始化函数
func init() {
	// 设置panic处理
	defer handlePanic()
}
