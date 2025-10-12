package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.uber.org/zap"

	"gkipass/plane/internal/api/handler"
	"gkipass/plane/internal/api/middleware"
	"gkipass/plane/internal/config"
	"gkipass/plane/internal/service"
)

// Server API服务器
type Server struct {
	config     *config.APIConfig
	router     *mux.Router
	httpServer *http.Server
	logger     *zap.Logger
	services   *service.ServiceContainer
}

// NewServer 创建API服务器
func NewServer(config *config.APIConfig, services *service.ServiceContainer) *Server {
	router := mux.NewRouter()

	server := &Server{
		config:   config,
		router:   router,
		logger:   zap.L().Named("api-server"),
		services: services,
	}

	// 创建HTTP服务器
	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.ListenAddr, config.ListenPort),
		Handler:      server.buildHandler(),
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	return server
}

// buildHandler 构建HTTP处理器
func (s *Server) buildHandler() http.Handler {
	// 注册中间件
	s.router.Use(middleware.RequestLogger(s.logger))
	s.router.Use(middleware.Recover(s.logger))

	// 注册API版本前缀
	apiRouter := s.router.PathPrefix("/api/v1").Subrouter()

	// 注册认证中间件
	authMiddleware := middleware.NewAuthMiddleware(s.services.AuthService)
	securedRouter := apiRouter.NewRoute().Subrouter()
	securedRouter.Use(authMiddleware.Middleware)

	// TODO: 注释掉旧的HTTP handler路由，因为已经使用Gin框架在router.go中设置路由
	// 注册健康检查路由
	// apiRouter.HandleFunc("/health", handler.HealthCheck).Methods("GET")

	// 注册认证路由
	// authHandler := handler.NewAuthHandler(s.services.AuthService)
	// authHandler.RegisterRoutes(apiRouter)

	// 注册节点路由
	// nodeHandler := handler.NewNodeHandler(s.services.NodeService)
	// nodeHandler.RegisterRoutes(securedRouter)

	// 注册节点组路由
	// nodeGroupHandler := handler.NewNodeGroupHandler(s.services.NodeGroupService)
	// nodeGroupHandler.RegisterRoutes(securedRouter)

	// 注册隧道路由
	// tunnelHandler := handler.NewTunnelHandler(s.services.TunnelService)
	// tunnelHandler.RegisterRoutes(securedRouter)

	// 注册用户路由
	// userHandler := handler.NewUserHandler(s.services.UserService)
	// userHandler.RegisterRoutes(securedRouter)

	// 注册监控路由
	// monitoringHandler := handler.NewMonitoringHandler(s.services.MonitoringService)
	// monitoringHandler.RegisterRoutes(securedRouter)

	// 注册WebSocket路由
	wsHandler := handler.NewWebSocketHandler(s.services.WebSocketService)
	wsHandler.RegisterRoutes(securedRouter)

	// 添加CORS支持
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   s.config.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-API-Key"},
		AllowCredentials: true,
		MaxAge:           86400, // 24小时
	})

	return corsHandler.Handler(s.router)
}

// Start 启动服务器
func (s *Server) Start() error {
	// 启动HTTP服务器
	go func() {
		s.logger.Info("API服务器启动",
			zap.String("addr", s.httpServer.Addr))

		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatal("HTTP服务器启动失败", zap.Error(err))
		}
	}()

	// 处理信号
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// 等待信号
	<-stop

	// 优雅关闭
	s.logger.Info("正在关闭API服务器...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("关闭HTTP服务器失败", zap.Error(err))
		return err
	}

	s.logger.Info("API服务器已关闭")

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}
