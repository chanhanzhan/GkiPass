package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gkipass/client/config"
	"gkipass/client/core"
	"gkipass/client/logger"
	"gkipass/client/metrics"
	"gkipass/client/ws"

	"go.uber.org/zap"
)

var (
	token     = flag.String("token", "", "节点认证Token（必填）")
	server    = flag.String("s", "", "Plane服务器地址，例如: ws://plane:8080 （必填）")
	nodeType  = flag.String("type", "entry", "节点类型: entry/exit")
	nodeName  = flag.String("name", "", "节点名称（可选）")
	groupID   = flag.String("group", "", "节点组ID（可选）")
	logLevel  = flag.String("log", "info", "日志级别: debug/info/warn/error")
	enableTLS = flag.Bool("tls", false, "启用节点间TLS加密")
	certFile  = flag.String("cert", "", "TLS证书文件")
	keyFile   = flag.String("key", "", "TLS私钥文件")
	version   = "1.0.0"
)

func main() {
	flag.Parse()

	// 验证必填参数
	if *token == "" || *server == "" {
		fmt.Println("错误: --token 和 -s 参数为必填项")
		fmt.Println("\n使用示例:")
		fmt.Println("  ./client --token <your-token> -s ws://plane:8080")
		fmt.Println("\n完整参数:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	printBanner()

	// 构建配置
	cfg := buildConfigFromFlags()

	// 初始化日志
	if err := logger.Init(*logLevel, "console", "stdout"); err != nil {
		fmt.Printf("初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("GKI Pass Client 启动",
		zap.String("version", version),
		zap.String("node_type", *nodeType),
		zap.String("server", *server),
		zap.Bool("tls", *enableTLS))

	// 创建隧道管理器
	tunnelManager := core.NewTunnelManager(cfg.Node.Type)

	// 创建WebSocket客户端
	wsClient := ws.NewClient(&cfg.Plane)

	// 创建指标收集器
	collector := metrics.NewCollector(cfg.Node.ID, wsClient, tunnelManager)

	// 创建消息处理器
	handler := ws.NewHandler(wsClient, tunnelManager)

	// 设置消息回调
	wsClient.SetOnMessage(func(msg *ws.Message) {
		if err := handler.HandleMessage(msg); err != nil {
			logger.Error("处理消息失败", zap.Error(err))
		}
	})

	wsClient.SetOnConnected(func() {
		logger.Info("WebSocket连接成功，开始注册节点")
		if err := registerNode(wsClient, cfg); err != nil {
			logger.Error("注册节点失败", zap.Error(err))
		}
	})

	wsClient.SetOnDisconnected(func() {
		logger.Warn("WebSocket连接断开")
	})

	// 连接到Plane
	if err := wsClient.Connect(); err != nil {
		logger.Fatal("连接到Plane失败", zap.Error(err))
	}

	// 等待注册确认
	logger.Info("等待注册确认...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ackMsg, err := wsClient.WaitForMessage(ctx, ws.MsgTypeRegisterAck)
	if err != nil {
		logger.Fatal("等待注册确认超时", zap.Error(err))
	}

	var ackResp ws.NodeRegisterResponse
	if err := ackMsg.ParseData(&ackResp); err != nil {
		logger.Fatal("解析注册响应失败", zap.Error(err))
	}

	if !ackResp.Success {
		logger.Fatal("节点注册失败", zap.String("message", ackResp.Message))
	}

	firstRuleReceived := make(chan bool, 1)

	wsClient.SetOnMessage(func(msg *ws.Message) {
		if msg.Type == ws.MsgTypeSyncRules {
			select {
			case firstRuleReceived <- true:
			default:
			}
		}
		if err := handler.HandleMessage(msg); err != nil {
			logger.Error("处理消息失败", zap.Error(err))
		}
	})

	// 等待首次配置（最多30秒）
	select {
	case <-firstRuleReceived:
		logger.Info("✅ 配置已下发，节点完全就绪")
	case <-time.After(30 * time.Second):
		logger.Warn("⚠️  未收到配置，节点以默认配置运行")
	}

	// 启动流量收集器
	collector.Start()

	// 启动心跳
	go startHeartbeat(wsClient, cfg, collector)

	// 启用pprof性能分析
	go StartPProfServer(":6060")

	logger.Info("🎉 节点已完全启动，等待流量转发...")

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("收到停止信号，开始优雅关闭...")

	// 停止收集器
	collector.Stop()

	// 停止隧道管理器
	tunnelManager.Stop()

	// 关闭WebSocket
	if err := wsClient.Close(); err != nil {
		logger.Error("关闭WebSocket失败", zap.Error(err))
	}

	logger.Info("节点已停止")
}

// registerNode 注册节点
func registerNode(wsClient *ws.Client, cfg *config.Config) error {
	ip, err := getLocalIP()
	if err != nil {
		ip = "127.0.0.1"
	}

	req := ws.NodeRegisterRequest{
		NodeID:   cfg.Node.ID,
		NodeName: cfg.Node.Name,
		NodeType: cfg.Node.Type,
		GroupID:  cfg.Node.GroupID,
		Version:  version,
		IP:       ip,
		Port:     8080, // SOCKS5监听端口
		CK:       cfg.Plane.CK,
		Capabilities: map[string]bool{
			"tcp":           true,
			"udp":           true,
			"http":          true,
			"tls":           true,
			"socks":         true,
			"load_balance":  true,
			"health_check":  true,
			"traffic_stats": true,
		},
	}

	msg, err := ws.NewMessage(ws.MsgTypeNodeRegister, req)
	if err != nil {
		return fmt.Errorf("创建注册消息失败: %w", err)
	}

	logger.Info("发送注册请求",
		zap.String("node_id", req.NodeID),
		zap.String("node_type", req.NodeType),
		zap.String("group_id", req.GroupID))

	return wsClient.Send(msg)
}

// startHeartbeat 启动心跳
func startHeartbeat(wsClient *ws.Client, cfg *config.Config, collector *metrics.Collector) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !wsClient.IsConnected() {
			continue
		}

		// 获取完整监控快照
		snapshot := collector.GetMonitorSnapshot()

		req := ws.HeartbeatRequest{
			NodeID:      cfg.Node.ID,
			Status:      "online",
			CPUUsage:    snapshot.CPUUsage,
			MemoryUsage: snapshot.MemUsage,
			Connections: int(snapshot.Connections),
		}

		msg, err := ws.NewMessage(ws.MsgTypeHeartbeat, req)
		if err != nil {
			logger.Error("创建心跳消息失败", zap.Error(err))
			continue
		}

		if err := wsClient.Send(msg); err != nil {
			logger.Error("发送心跳失败", zap.Error(err))
		} else {
			logger.Debug("心跳已发送",
				zap.Int("connections", int(snapshot.Connections)),
				zap.Float64("cpu", snapshot.CPUUsage),
				zap.Int64("mem_mb", snapshot.MemUsage/1024/1024))
		}
	}
}

// getLocalIP 获取本机IP
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("未找到有效IP")
}

// buildConfigFromFlags 从命令行参数构建配置
func buildConfigFromFlags() *config.Config {
	nodeID := fmt.Sprintf("node-%d", time.Now().Unix())
	if *nodeName == "" {
		*nodeName = fmt.Sprintf("%s-node-%d", *nodeType, time.Now().Unix()%1000)
	}

	return &config.Config{
		Node: config.NodeConfig{
			ID:      nodeID,
			Name:    *nodeName,
			Type:    *nodeType,
			GroupID: *groupID,
		},
		Plane: config.PlaneConfig{
			URL:                  *server + "/ws/node",
			CK:                   *token,
			ReconnectInterval:    5,
			MaxReconnectAttempts: 0,
			Timeout:              30,
		},
		Pool: config.PoolConfig{
			MinConns:          5,
			MaxConns:          100,
			IdleTimeout:       300,
			HeartbeatInterval: 30,
			AutoScale:         true,
		},
		TLS: config.TLSConfig{
			Enabled:         *enableTLS,
			Cert:            *certFile,
			Key:             *keyFile,
			PinVerification: false,
		},
	}
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
║           Client Node - Bidirectional Tunnel         ║
║                      v1.0.0                           ║
║                                                       ║
╚═══════════════════════════════════════════════════════╝
`
	fmt.Println(banner)
}
