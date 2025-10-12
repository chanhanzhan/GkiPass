package auth

import "time"

// ManagerConfig 认证管理器配置
type ManagerConfig struct {
	PlaneURL       string        // 平面服务器URL
	APIEndpoint    string        // API端点路径
	NodeID         string        // 节点ID
	Token          string        // 令牌
	APIKey         string        // API密钥
	TokenFile      string        // 令牌文件路径
	RefreshWindow  time.Duration // 刷新窗口时间
	ConnectTimeout time.Duration // 连接超时
	RequestTimeout time.Duration // 请求超时
	RetryInterval  time.Duration // 重试间隔
	MaxRetries     int           // 最大重试次数
	TLSVerify      bool          // 是否验证TLS
	Enabled        bool          // 是否启用认证
}

// DefaultManagerConfig 默认认证管理器配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		PlaneURL:       "https://api.example.com",
		APIEndpoint:    "/api/v1",
		RefreshWindow:  5 * time.Minute,
		TokenFile:      "./data/auth.json",
		ConnectTimeout: 10 * time.Second,
		RequestTimeout: 30 * time.Second,
		RetryInterval:  5 * time.Second,
		MaxRetries:     3,
		TLSVerify:      true,
		Enabled:        true,
	}
}
