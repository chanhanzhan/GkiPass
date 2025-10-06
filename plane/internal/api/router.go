package api

import (
	"gkipass/plane/internal/api/middleware"
	"gkipass/plane/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// SetupRouter 设置路由
func SetupRouter(app *App, wsServer *ws.Server) *gin.Engine {
	// 设置Gin模式
	if app.Config.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// 全局中间件
	router.Use(middleware.Recovery())
	router.Use(middleware.Logger())
	router.Use(middleware.CORS())

	// 健康检查
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"cache":  app.DB.HasCache(),
		})
	})

	// Prometheus 监控指标
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// WebSocket 端点（节点连接）
	router.GET("/ws/node", wsServer.HandleWebSocket)

	// WebSocket 状态
	router.GET("/ws/stats", func(c *gin.Context) {
		c.JSON(200, wsServer.GetStats())
	})

	// API v1
	v1 := router.Group("/api/v1")
	{
		captchaHandler := NewCaptchaHandler(app)
		v1.GET("/captcha/config", captchaHandler.GetCaptchaConfig)
		v1.GET("/captcha/image", captchaHandler.GenerateImageCaptcha)

		// 公开公告
		announcementHandler := NewAnnouncementHandler(app)
		v1.GET("/announcements", announcementHandler.ListActiveAnnouncements)
		v1.GET("/announcements/:id", announcementHandler.GetAnnouncement)

		// 认证路由（无需JWT）
		auth := v1.Group("/auth")
		{
			authHandler := NewAuthHandler(app)
			userHandler := NewUserHandler(app)

			auth.POST("/register", userHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/logout", authHandler.Logout)

			// GitHub OAuth
			if app.Config.Auth.GitHub.Enabled {
				oauthHandler := NewOAuthHandler(app)
				auth.GET("/github", oauthHandler.GitHubLoginURL)
				auth.POST("/github/callback", oauthHandler.GitHubCallback)
			}
		}

		// 需要JWT认证的路由
		authorized := v1.Group("")
		authorized.Use(middleware.JWTAuth(app.Config.Auth.JWTSecret, app.DB))
		{
			// 用户管理
			users := authorized.Group("/users")
			{
				userHandler := NewUserHandler(app)
				users.GET("/profile", userHandler.GetProfile)
				users.PUT("/password", userHandler.UpdatePassword)

				// 管理员功能
				users.GET("", middleware.AdminAuth(), userHandler.ListUsers)
				users.PUT("/:id/status", middleware.AdminAuth(), userHandler.ToggleUserStatus)
				users.PUT("/:id/role", middleware.AdminAuth(), userHandler.UpdateUserRole)
				users.DELETE("/:id", middleware.AdminAuth(), userHandler.DeleteUser)
			}

			// 节点组管理
			groups := authorized.Group("/node-groups")
			{
				groupHandler := NewNodeGroupHandler(app)
				configHandler := NewNodeGroupConfigHandler(app)

				groups.POST("", groupHandler.Create)
				groups.GET("", groupHandler.List)
				groups.GET("/:id", groupHandler.Get)
				groups.PUT("/:id", groupHandler.Update)
				groups.DELETE("/:id", groupHandler.Delete)

				// 节点组配置
				groups.GET("/:id/config", configHandler.GetNodeGroupConfig)
				groups.PUT("/:id/config", configHandler.UpdateNodeGroupConfig)
				groups.POST("/:id/config/reset", configHandler.ResetNodeGroupConfig)
			}

			// 节点管理
			nodes := authorized.Group("/nodes")
			{
				nodeHandler := NewNodeHandler(app)
				ckHandler := NewCKHandler(app)
				statusHandler := NewNodeStatusHandler(app)
				certHandler := NewNodeCertHandler(app)

				// 用户可用节点（根据套餐过滤）
				nodes.GET("/available", nodeHandler.GetAvailableNodes)

				nodes.POST("", nodeHandler.Create)
				nodes.GET("", nodeHandler.List)
				nodes.GET("/:id", nodeHandler.Get)
				nodes.PUT("/:id", nodeHandler.Update)
				nodes.DELETE("/:id", nodeHandler.Delete)
				nodes.POST("/:id/heartbeat", nodeHandler.Heartbeat)

				// 节点状态 API
				nodes.GET("/:id/status", statusHandler.GetNodeStatus)
				nodes.GET("/status/list", statusHandler.ListNodesStatus)
				nodes.GET("/group/:group_id/status", statusHandler.GetNodesByGroup)

				// Connection Key 管理
				nodes.POST("/:id/generate-ck", ckHandler.GenerateNodeCK)
				nodes.GET("/:id/connection-keys", ckHandler.ListNodeCKs)
				nodes.DELETE("/connection-keys/:ck_id", ckHandler.RevokeCK)

				// 节点证书管理
				nodes.POST("/:id/cert/generate", certHandler.GenerateCert)
				nodes.GET("/:id/cert/download", certHandler.DownloadCert)
				nodes.POST("/:id/cert/renew", certHandler.RenewCert)
				nodes.GET("/:id/cert/info", certHandler.GetCertInfo)
			}

			// 节点部署 API
			deployHandler := NewNodeDeployHandler(app)

			// 节点组内创建节点
			groups.POST("/:id/nodes", deployHandler.CreateNode)
			groups.GET("/:id/nodes", deployHandler.ListNodesInGroup)

			// 节点注册（公开API，供服务器调用）
			v1.POST("/nodes/register", deployHandler.RegisterNode)
			// 注意：心跳API已在上面的nodes路由组中注册（nodes.POST("/:id/heartbeat", nodeHandler.Heartbeat)）

			// 策略管理
			policies := authorized.Group("/policies")
			{
				policyHandler := NewPolicyHandler(app)
				policies.POST("", policyHandler.Create)
				policies.GET("", policyHandler.List)
				policies.GET("/:id", policyHandler.Get)
				policies.PUT("/:id", policyHandler.Update)
				policies.DELETE("/:id", policyHandler.Delete)
				policies.POST("/:id/deploy", policyHandler.Deploy)
			}

			// 证书管理
			certs := authorized.Group("/certificates")
			{
				certHandler := NewCertificateHandler(app)
				certs.POST("/ca", certHandler.GenerateCA)
				certs.POST("/leaf", certHandler.GenerateLeaf)
				certs.GET("", certHandler.List)
				certs.GET("/:id", certHandler.Get)
				certs.POST("/:id/revoke", certHandler.Revoke)
				certs.GET("/:id/download", certHandler.Download)
			}

			// 套餐管理
			plans := authorized.Group("/plans")
			{
				planHandler := NewPlanHandler(app)
				plans.GET("", planHandler.List)
				plans.GET("/:id", planHandler.Get)
				plans.GET("/:id/subscribe", middleware.QuotaCheck(app.DB), planHandler.Subscribe)
				plans.GET("/my/subscription", planHandler.MySubscription)

				// 仅管理员
				adminPlans := plans.Group("")
				adminPlans.Use(middleware.AdminAuth())
				{
					adminPlans.POST("", planHandler.Create)
					adminPlans.PUT("/:id", planHandler.Update)
					adminPlans.DELETE("/:id", planHandler.Delete)
				}
			}

			// 隧道管理
			tunnels := authorized.Group("/tunnels")
			tunnels.Use(middleware.QuotaCheck(app.DB))
			{
				tunnelHandler := NewTunnelHandler(app)
				tunnels.GET("", tunnelHandler.List)
				tunnels.GET("/:id", tunnelHandler.Get)
				tunnels.POST("", middleware.RuleQuotaCheck(app.DB), tunnelHandler.Create)
				tunnels.PUT("/:id", tunnelHandler.Update)
				tunnels.DELETE("/:id", tunnelHandler.Delete)
				tunnels.POST("/:id/toggle", tunnelHandler.Toggle)
			}

			// 统计和监控
			stats := authorized.Group("/statistics")
			{
				statsHandler := NewStatisticsHandler(app)
				stats.GET("/nodes/:id", statsHandler.GetNodeStats)
				stats.GET("/overview", statsHandler.GetOverview)
				stats.POST("/report", statsHandler.ReportStats)
			}

			// 流量统计
			traffic := authorized.Group("/traffic")
			{
				trafficHandler := NewTrafficStatsHandler(app)
				traffic.GET("/stats", trafficHandler.ListTrafficStats)
				traffic.GET("/summary", trafficHandler.GetTrafficSummary)
				traffic.POST("/report", trafficHandler.ReportTraffic) // 节点上报流量
			}

			// 管理员专用统计
			adminStats := authorized.Group("/admin/statistics")
			adminStats.Use(middleware.AdminAuth())
			{
				statsHandler := NewStatisticsHandler(app)
				adminStats.GET("/overview", statsHandler.GetAdminOverview)
			}

			// 钱包管理
			wallet := authorized.Group("/wallet")
			{
				walletHandler := NewWalletHandler(app)
				wallet.GET("/balance", walletHandler.GetBalance)
				wallet.GET("/transactions", walletHandler.ListTransactions)
				wallet.POST("/recharge", walletHandler.Recharge) // 旧版保留兼容
			}

			// 支付管理
			payment := authorized.Group("/payment")
			{
				paymentHandler := NewPaymentHandler(app)
				payment.POST("/recharge", paymentHandler.CreateRechargeOrder)
				payment.GET("/orders/:id", paymentHandler.QueryOrderStatus)
			}

			// 订阅管理
			subscriptions := authorized.Group("/subscriptions")
			{
				subscriptionHandler := NewSubscriptionHandler(app)
				subscriptions.GET("/current", subscriptionHandler.GetCurrentSubscription)
				subscriptions.GET("", middleware.AdminAuth(), subscriptionHandler.ListSubscriptions)
			}

			// 通知管理
			notificationHandler := NewNotificationHandler(app)
			notifications := authorized.Group("/notifications")
			{
				notifications.GET("", notificationHandler.List)
				notifications.POST("/:id/read", notificationHandler.MarkAsRead)
				notifications.POST("/read-all", notificationHandler.MarkAllAsRead)
				notifications.DELETE("/:id", notificationHandler.Delete)
			}

			// 管理员专用路由
			admin := authorized.Group("/admin")
			admin.Use(middleware.AdminAuth())
			{
				// 支付配置管理
				paymentConfigHandler := NewPaymentConfigHandler(app)
				paymentHandler := NewPaymentHandler(app)
				admin.GET("/payment/configs", paymentConfigHandler.ListConfigs)
				admin.GET("/payment/config/:id", paymentConfigHandler.GetConfig)
				admin.PUT("/payment/config/:id", paymentConfigHandler.UpdateConfig)
				admin.POST("/payment/config/:id/toggle", paymentConfigHandler.ToggleConfig)
				admin.POST("/payment/manual-recharge", paymentHandler.ManualRecharge)

				// 系统设置
				settingsHandler := NewSettingsHandler(app)
				admin.GET("/settings/captcha", settingsHandler.GetCaptchaSettings)
				admin.PUT("/settings/captcha", settingsHandler.UpdateCaptchaSettings)

				// 公告管理
				admin.GET("/announcements", announcementHandler.ListAll)
				admin.POST("/announcements", announcementHandler.Create)
				admin.PUT("/announcements/:id", announcementHandler.Update)
				admin.DELETE("/announcements/:id", announcementHandler.Delete)

				// 通知管理（创建全局通知）
				admin.POST("/notifications", notificationHandler.Create)
			}
		}
	}

	return router
}
