package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// 节点指标
	NodesOnline = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gkipass_nodes_online",
			Help: "在线节点数量",
		},
		[]string{"type", "group_id"},
	)

	// 隧道指标
	TunnelsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gkipass_tunnels_active",
			Help: "活跃隧道数量",
		},
		[]string{"user_id", "protocol"},
	)

	// 流量指标
	TrafficBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gkipass_traffic_bytes_total",
			Help: "总流量（字节）",
		},
		[]string{"direction", "node_id"},
	)

	// API请求指标
	HTTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gkipass_http_requests_total",
			Help: "HTTP请求总数",
		},
		[]string{"method", "endpoint", "status"},
	)

	// API延迟指标
	HTTPDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gkipass_http_duration_seconds",
			Help:    "HTTP请求延迟（秒）",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// WebSocket连接数
	WSConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gkipass_websocket_connections",
			Help: "WebSocket连接数",
		},
	)

	// 用户配额使用
	UserQuotaUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gkipass_user_quota_usage",
			Help: "用户配额使用率",
		},
		[]string{"user_id", "quota_type"},
	)
)
