package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"
)

// Config 应用配置
type Config struct {
	// 基础配置
	NodeID    string   `json:"node_id"`    // 节点ID
	PlaneURLs []string `json:"plane_urls"` // Plane服务器地址列表
	DataDir   string   `json:"data_dir"`   // 数据目录

	// 组件配置
	Plane      PlaneConfig      `json:"plane"`
	Node       NodeConfig       `json:"node"`
	Paths      PathsConfig      `json:"paths"`
	Log        LogConfig        `json:"log"`
	TLS        TLSConfig        `json:"tls"`
	Network    NetworkConfig    `json:"network"`
	Transport  TransportConfig  `json:"transport"`
	Protocol   ProtocolConfig   `json:"protocol"`
	Traffic    TrafficConfig    `json:"traffic"`
	Monitoring MonitoringConfig `json:"monitoring"`
	HotReload  *HotReloadConfig `json:"hot_reload,omitempty"`
	Debug      *DebugConfig     `json:"debug,omitempty"`
}

// PlaneConfig Plane服务器配置
type PlaneConfig struct {
	URL                  string        `json:"url"`                    // Plane服务器地址
	Token                string        `json:"token"`                  // 认证Token
	APIKey               string        `json:"api_key"`                // API密钥
	ConnectTimeout       time.Duration `json:"connect_timeout"`        // 连接超时
	ReconnectInterval    time.Duration `json:"reconnect_interval"`     // 重连间隔
	MaxReconnectAttempts int           `json:"max_reconnect_attempts"` // 最大重连次数，0表示无限
	HeartbeatInterval    time.Duration `json:"heartbeat_interval"`     // 心跳间隔
}

// NodeConfig 节点配置
type NodeConfig struct {
	ID      string `json:"id"`      // 节点ID，留空自动生成
	Name    string `json:"name"`    // 节点名称，留空自动生成
	Version string `json:"version"` // 节点版本
}

// PathsConfig 路径配置
type PathsConfig struct {
	CertDir string `json:"cert_dir"` // 证书存储目录
	DataDir string `json:"data_dir"` // 数据存储目录
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `json:"level"`       // 日志级别
	Format     string `json:"format"`      // 日志格式：json/console
	OutputPath string `json:"output_path"` // 输出路径，空为stdout
}

// TransportConfig 传输配置
type TransportConfig struct {
	Type           string        `json:"type"`            // 传输类型：tcp/tls/mtls/ws/wss/none
	MaxConnections int           `json:"max_connections"` // 最大连接数
	MinConnections int           `json:"min_connections"` // 最小连接数
	ConnectTimeout time.Duration `json:"connect_timeout"` // 连接超时
	IdleTimeout    time.Duration `json:"idle_timeout"`    // 空闲超时
}

// ProtocolConfig 协议配置
type ProtocolConfig struct {
	TCP   ProtocolSettings `json:"tcp"`   // TCP协议设置
	UDP   ProtocolSettings `json:"udp"`   // UDP协议设置
	HTTP  ProtocolSettings `json:"http"`  // HTTP协议设置
	TLS   ProtocolSettings `json:"tls"`   // TLS协议设置
	SOCKS ProtocolSettings `json:"socks"` // SOCKS协议设置
}

// ProtocolSettings 协议设置
type ProtocolSettings struct {
	Enabled    bool          `json:"enabled"`     // 是否启用
	Timeout    time.Duration `json:"timeout"`     // 超时时间
	BufferSize int           `json:"buffer_size"` // 缓冲区大小
}

// TrafficConfig 流量控制配置
type TrafficConfig struct {
	EnableRateLimit bool `json:"enable_rate_limit"` // 启用限速
	RateLimitMbps   int  `json:"rate_limit_mbps"`   // 限速 Mbps
	MaxConnections  int  `json:"max_connections"`   // 最大连接数
	BufferSize      int  `json:"buffer_size"`       // 缓冲区大小
	EnableQoS       bool `json:"enable_qos"`        // 启用QoS
}

// MonitoringConfig 监控配置
type MonitoringConfig struct {
	Enabled        bool          `json:"enabled"`         // 启用监控
	ReportInterval time.Duration `json:"report_interval"` // 上报间隔
	MetricsPort    int           `json:"metrics_port"`    // 指标端口
	EnablePprof    bool          `json:"enable_pprof"`    // 启用pprof
	PprofPort      int           `json:"pprof_port"`      // pprof端口
}

// TLSConfig TLS配置
type TLSConfig struct {
	CertDir      string   `json:"cert_dir"`      // 证书目录
	CertFile     string   `json:"cert_file"`     // 证书文件
	KeyFile      string   `json:"key_file"`      // 私钥文件
	CAFile       string   `json:"ca_file"`       // CA文件
	ServerName   string   `json:"server_name"`   // 服务器名称
	SkipVerify   bool     `json:"skip_verify"`   // 跳过验证
	MinVersion   string   `json:"min_version"`   // 最小TLS版本
	MaxVersion   string   `json:"max_version"`   // 最大TLS版本
	CipherSuites []string `json:"cipher_suites"` // 加密套件
}

// NetworkConfig 网络配置
type NetworkConfig struct {
	ListenAddr     string        `json:"listen_addr"`     // 监听地址
	PortRange      PortRange     `json:"port_range"`      // 端口范围
	MaxConnections int           `json:"max_connections"` // 最大连接数
	Timeout        time.Duration `json:"timeout"`         // 网络超时
	KeepAlive      bool          `json:"keep_alive"`      // 保持连接
	TCPNoDelay     bool          `json:"tcp_no_delay"`    // TCP_NODELAY
}

// PortRange 端口范围
type PortRange struct {
	Min uint16 `json:"min"` // 最小端口
	Max uint16 `json:"max"` // 最大端口
}

// HotReloadConfig 热重载配置
type HotReloadConfig struct {
	Enabled        bool          `json:"enabled"`
	WatchInterval  time.Duration `json:"watch_interval"`
	ReloadDelay    time.Duration `json:"reload_delay"`
	BackupEnabled  bool          `json:"backup_enabled"`
	BackupDir      string        `json:"backup_dir"`
	ValidateConfig bool          `json:"validate_config"`
}

// DebugConfig 调试配置
type DebugConfig struct {
	Enabled  bool   `json:"enabled"`  // 启用调试
	Mode     string `json:"mode"`     // 调试模式: server/client
	Protocol string `json:"protocol"` // 调试协议: tcp/udp/ws/wss/tls/tls-mux/kcp/quic

	// 服务端配置
	ListenPort int `json:"listen_port"` // 服务端监听端口

	// 客户端配置
	TargetAddr string `json:"target_addr"` // 客户端目标地址 (ip:port)
	Token      string `json:"token"`       // 客户端认证令牌
	PlaneAddr  string `json:"plane_addr"`  // Plane服务器地址
	APIKey     string `json:"api_key"`     // 服务端API密钥

	// 测试配置
	TrafficTest  bool          `json:"traffic_test"`   // 启用流量测试
	TestInterval time.Duration `json:"test_interval"`  // 测试间隔
	TestDuration time.Duration `json:"test_duration"`  // 测试持续时间
	TestDataSize int           `json:"test_data_size"` // 测试数据大小

	// 其他配置
	LogLevel    string `json:"log_level"`    // 调试日志级别
	EnablePprof bool   `json:"enable_pprof"` // 启用性能分析
	PprofPort   int    `json:"pprof_port"`   // pprof端口
}

// LoadFromFile 从文件加载配置
func (c *Config) LoadFromFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	return nil
}

// SaveToFile 保存配置到文件
func (c *Config) SaveToFile(filename string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证Plane配置
	if c.Plane.URL == "" {
		return fmt.Errorf("plane URL不能为空")
	}

	// 验证URL格式
	if _, err := url.Parse(c.Plane.URL); err != nil {
		return fmt.Errorf("plane URL格式无效: %w", err)
	}

	if c.Plane.Token == "" {
		return fmt.Errorf("认证token不能为空")
	}

	// 设置默认值
	c.setDefaults()

	return nil
}

// setDefaults 设置默认值
func (c *Config) setDefaults() {
	if c.Plane.ConnectTimeout == 0 {
		c.Plane.ConnectTimeout = 30 * time.Second
	}

	if c.Plane.ReconnectInterval == 0 {
		c.Plane.ReconnectInterval = 5 * time.Second
	}

	if c.Plane.HeartbeatInterval == 0 {
		c.Plane.HeartbeatInterval = 30 * time.Second
	}

	if c.Paths.CertDir == "" {
		c.Paths.CertDir = "./certs"
	}

	if c.Paths.DataDir == "" {
		c.Paths.DataDir = "./data"
	}

	if c.Log.Level == "" {
		c.Log.Level = "info"
	}

	if c.Log.Format == "" {
		c.Log.Format = "console"
	}
}

// GetPlaneWebSocketURL 获取Plane WebSocket URL
func (c *Config) GetPlaneWebSocketURL() string {
	u, err := url.Parse(c.Plane.URL)
	if err != nil {
		return c.Plane.URL
	}

	// 转换HTTP/HTTPS为WS/WSS
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	// 确保有WebSocket路径
	if u.Path == "" || u.Path == "/" {
		u.Path = "/ws/node"
	}

	return u.String()
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	cfg := &Config{
		NodeID:    "", // 自动生成
		PlaneURLs: []string{"ws://127.0.0.1:8080/ws"},
		DataDir:   "./data",

		Plane: PlaneConfig{
			ConnectTimeout:       30 * time.Second,
			ReconnectInterval:    5 * time.Second,
			MaxReconnectAttempts: 0, // 无限重连
			HeartbeatInterval:    30 * time.Second,
			APIKey:               "", // 默认为空，可由服务端自动生成或通过--key参数设置
		},
		Node: NodeConfig{
			Version: "2.0.0",
		},
		Paths: PathsConfig{
			CertDir: "./certs",
			DataDir: "./data",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "console",
		},
		TLS: TLSConfig{
			CertDir:    "./certs",
			MinVersion: "1.2",
			MaxVersion: "1.3",
		},
		Network: NetworkConfig{
			ListenAddr:     ":0",
			PortRange:      PortRange{Min: 10000, Max: 65535},
			MaxConnections: 1000,
			Timeout:        30 * time.Second,
			KeepAlive:      true,
			TCPNoDelay:     true,
		},
		Transport: TransportConfig{
			Type:           "tcp",
			MaxConnections: 100,
			MinConnections: 5,
			ConnectTimeout: 10 * time.Second,
			IdleTimeout:    300 * time.Second,
		},
		Protocol: ProtocolConfig{
			TCP:   ProtocolSettings{Enabled: true, Timeout: 30 * time.Second, BufferSize: 32768},
			UDP:   ProtocolSettings{Enabled: true, Timeout: 30 * time.Second, BufferSize: 65536},
			HTTP:  ProtocolSettings{Enabled: true, Timeout: 60 * time.Second, BufferSize: 32768},
			TLS:   ProtocolSettings{Enabled: true, Timeout: 30 * time.Second, BufferSize: 32768},
			SOCKS: ProtocolSettings{Enabled: true, Timeout: 30 * time.Second, BufferSize: 32768},
		},
		Traffic: TrafficConfig{
			EnableRateLimit: false,
			RateLimitMbps:   100,
			MaxConnections:  1000,
			BufferSize:      65536,
			EnableQoS:       false,
		},
		Monitoring: MonitoringConfig{
			Enabled:        true,
			ReportInterval: 60 * time.Second,
			MetricsPort:    9090,
			EnablePprof:    false,
			PprofPort:      6060,
		},
		HotReload: &HotReloadConfig{
			Enabled:        true,
			WatchInterval:  1 * time.Second,
			ReloadDelay:    3 * time.Second,
			BackupEnabled:  true,
			BackupDir:      "./backups",
			ValidateConfig: true,
		},
	}

	return cfg
}

// LoadConfig 加载配置文件
func LoadConfig(filename string) (*Config, error) {
	cfg := DefaultConfig()
	if err := cfg.LoadFromFile(filename); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveConfig 保存配置到文件
func SaveConfig(cfg *Config, filename string) error {
	return cfg.SaveToFile(filename)
}
