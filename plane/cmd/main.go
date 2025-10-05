package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gkipass/plane/db"
	"gkipass/plane/internal/api"
	"gkipass/plane/internal/config"
	"gkipass/plane/internal/server"
	"gkipass/plane/internal/service"
	"gkipass/plane/internal/ws"
	"gkipass/plane/pkg/initializer"
	"gkipass/plane/pkg/logger"

	"go.uber.org/zap"
)

var (
	configPath = flag.String("config", "./config.yaml", "Path to config file")
	port       = flag.Int("port", 0, "Override server port")
)

func main() {
	flag.Parse()
	if err := logger.Init(&logger.Config{
		Level:  "info",
		Format: "console",
	}); err != nil {
		log.Fatalf("初始化日志系统失败: %v", err)
	}
	defer logger.Sync()

	// 检查是否首次运行
	isFirstRun := initializer.IsFirstRun(*configPath)

	// 初始化基础目录
	if err := initializer.InitDirectories(); err != nil {
		logger.Fatal("初始化目录失败", zap.Error(err))
	}
	if isFirstRun {
		initializer.PrintWelcome()
	} else {
		printBanner()
	}

	// 首次运行初始化
	if isFirstRun {
		// 初始化配置文件
		if err := initializer.InitConfig(*configPath); err != nil {
			logger.Fatal("初始化配置失败", zap.Error(err))
		}

		// 初始化证书
		if err := initializer.InitCertificates("./certs"); err != nil {
			logger.Fatal("初始化证书失败", zap.Error(err))
		}
	}
	cfg := config.LoadConfigOrDefault(*configPath)
	if *port > 0 {
		cfg.Server.Port = *port
	}

	// 重新初始化日志系统（使用配置）
	if err := logger.Init(&logger.Config{
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		OutputPath: cfg.Log.OutputPath,
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
		Compress:   cfg.Log.Compress,
	}); err != nil {
		logger.Fatal("重新初始化日志系统失败", zap.Error(err))
	}

	dbManager, err := db.NewManager(&db.Config{
		SQLitePath:    cfg.Database.SQLitePath,
		RedisAddr:     cfg.Database.RedisAddr,
		RedisPassword: cfg.Database.RedisPassword,
		RedisDB:       cfg.Database.RedisDB,
	})
	if err != nil {
		logger.Fatal("初始化数据库失败", zap.Error(err))
	}
	defer dbManager.Close()

	logger.Info("✓ 数据库初始化成功")

	// 创建应用实例
	app := api.NewApp(cfg, dbManager)
	jwtManager := service.NewJWTManager(dbManager)
	if err := jwtManager.Start(); err != nil {			
	}
	defer jwtManager.Stop()
	cfg.Auth.JWTSecret = jwtManager.GetSecret()
	service.GetPortManager(dbManager)
	cleanupService := service.NewCleanupService(dbManager)
	go cleanupService.Start()
	defer cleanupService.Stop()
	wsServer := ws.NewServer(dbManager)
	wsServer.Start()
	router := api.SetupRouter(app, wsServer)
	http2Addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	var tlsConfig *tls.Config
	if cfg.TLS.Enabled && cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		tlsConfig = createTLSConfig(cfg)
	}

	// 创建 HTTP/2 服务器
	http2Server := server.NewHTTP2Server(
		http2Addr,
		router,
		tlsConfig,
		time.Duration(cfg.Server.ReadTimeout)*time.Second,
		time.Duration(cfg.Server.WriteTimeout)*time.Second,
	)
	go func() {
		if isFirstRun {
		} else {
		}
		var err error
		if cfg.TLS.Enabled {
			err = http2Server.Start(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		} else {
			err = http2Server.StartInsecure()
		}
		if err != nil && err != http.ErrServerClosed {
		}
	}()

	var http3Server *server.HTTP3Server
	if cfg.Server.EnableHTTP3 && cfg.TLS.Enabled {
		http3Addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.HTTP3Port)
		http3Server = server.NewHTTP3Server(http3Addr, router, tlsConfig)

		go func() {
			if err := http3Server.Start(); err != nil {
				logger.Error("HTTP/3 服务器错误", zap.Error(err))
			}
		}()
	} else if cfg.Server.EnableHTTP3 {
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 关闭 HTTP/2 服务器
	if err := http2Server.Shutdown(ctx); err != nil {
	}
	// 关闭 HTTP/3 服务器
	if http3Server != nil {
		if err := http3Server.Shutdown(ctx); err != nil {
		}
	}

	logger.Info("✓ 所有服务器已停止")
}

func printBanner() {
	banner := `
╔═══════════════════════════════════════════════════════╗
║                                                       ║
║   ██████╗ ██╗  ██╗██╗    ██████╗  █████╗ ███████╗███╗
║  ██╔════╝ ██║ ██╔╝██║    ██╔══██╗██╔══██╗██╔════╝████║
║  ██║  ███╗█████╔╝ ██║    ██████╔╝███████║███████╗╚═██║
║  ██║   ██║██╔═██╗ ██║    ██╔═══╝ ██╔══██║╚════██║  ██║
║  ╚██████╔╝██║  ██╗██║    ██║     ██║  ██║███████║  ██║
║   ╚═════╝ ╚═╝  ╚═╝╚═╝    ╚═╝     ╚═╝  ╚═╝╚══════╝  ╚═╝
║                                                       ║
║           Bidirectional Tunnel Control Plane         ║
║                      v2.0.0                           ║
║                                                       ║
╚═══════════════════════════════════════════════════════╝
`
	fmt.Println(banner)
}
