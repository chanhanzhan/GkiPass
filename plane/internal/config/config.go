package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 全局配置
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	TLS      TLSConfig      `yaml:"tls"`
	Log      LogConfig      `yaml:"log"`
	Captcha  CaptchaConfig  `yaml:"captcha"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	HTTP3Port    int    `yaml:"http3_port"`   // HTTP/3 (QUIC) 端口
	Mode         string `yaml:"mode"`         // debug, release
	EnableHTTP3  bool   `yaml:"enable_http3"` // 启用 HTTP/3
	ReadTimeout  int    `yaml:"read_timeout"`
	WriteTimeout int    `yaml:"write_timeout"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	SQLitePath    string `yaml:"sqlite_path"`
	RedisAddr     string `yaml:"redis_addr"`
	RedisPassword string `yaml:"redis_password"`
	RedisDB       int    `yaml:"redis_db"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	JWTSecret     string      `yaml:"jwt_secret"`
	JWTExpiration int         `yaml:"jwt_expiration"` // 单位：小时
	AdminPassword string      `yaml:"admin_password"`
	GitHub        GitHubOAuth `yaml:"github"`
}

// GitHubOAuth GitHub OAuth2 配置
type GitHubOAuth struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
}

// TLSConfig TLS配置
type TLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	CAFile     string `yaml:"ca_file"`
	MinVersion string `yaml:"min_version"` // TLS 1.2, TLS 1.3
	EnableALPN bool   `yaml:"enable_alpn"` // ALPN for HTTP/2, HTTP/3
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `yaml:"level"`       // debug, info, warn, error
	Format     string `yaml:"format"`      // json, console
	OutputPath string `yaml:"output_path"` // 日志文件路径
	MaxSize    int    `yaml:"max_size"`    // 单个日志文件大小(MB)
	MaxBackups int    `yaml:"max_backups"` // 保留的旧日志文件数量
	MaxAge     int    `yaml:"max_age"`     // 保留天数
	Compress   bool   `yaml:"compress"`    // 是否压缩
}

// CaptchaConfig 验证码配置
type CaptchaConfig struct {
	Enabled        bool   `yaml:"enabled"`         // 是否启用验证码
	Type           string `yaml:"type"`            // 类型: image, turnstile, both
	EnableLogin    bool   `yaml:"enable_login"`    // 登录页面启用
	EnableRegister bool   `yaml:"enable_register"` // 注册页面启用
	// 图片验证码配置
	ImageWidth  int `yaml:"image_width"`  // 验证码图片宽度
	ImageHeight int `yaml:"image_height"` // 验证码图片高度
	CodeLength  int `yaml:"code_length"`  // 验证码长度
	Expiration  int `yaml:"expiration"`   // 过期时间（秒）
	// Cloudflare Turnstile 配置
	TurnstileSiteKey   string `yaml:"turnstile_site_key"`   // Turnstile站点密钥
	TurnstileSecretKey string `yaml:"turnstile_secret_key"` // Turnstile服务端密钥
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// LoadConfigOrDefault 加载配置或使用默认值
func LoadConfigOrDefault(path string) *Config {
	if path == "" {
		return DefaultConfig()
	}

	config, err := LoadConfig(path)
	if err != nil {
		fmt.Printf("Failed to load config: %v, using defaults\n", err)
		return DefaultConfig()
	}

	return config
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         8080,
			HTTP3Port:    8443,
			Mode:         "debug",
			EnableHTTP3:  false, // 默认关闭 HTTP/3
			ReadTimeout:  30,
			WriteTimeout: 30,
		},
		Database: DatabaseConfig{
			SQLitePath:    "./data/gkipass.db",
			RedisAddr:     "localhost:6379",
			RedisPassword: "",
			RedisDB:       0,
		},
		Auth: AuthConfig{
			JWTSecret:     "change-this-secret-in-production",
			JWTExpiration: 24,
			AdminPassword: "admin123",
			GitHub: GitHubOAuth{
				Enabled:      false,
				ClientID:     "",
				ClientSecret: "",
				RedirectURL:  "http://localhost:3000/auth/callback/github",
			},
		},
		TLS: TLSConfig{
			Enabled:    false,
			CertFile:   "",
			KeyFile:    "",
			CAFile:     "",
			MinVersion: "TLS 1.3",
			EnableALPN: true,
		},
		Log: LogConfig{
			Level:      "info",
			Format:     "console",
			OutputPath: "./logs/gkipass.log",
			MaxSize:    100,
			MaxBackups: 10,
			MaxAge:     30,
			Compress:   true,
		},
		Captcha: CaptchaConfig{
			Enabled:            false, // 默认关闭
			Type:               "image",
			EnableLogin:        false,
			EnableRegister:     false,
			ImageWidth:         240,
			ImageHeight:        80,
			CodeLength:         6,
			Expiration:         300, // 5分钟
			TurnstileSiteKey:   "",
			TurnstileSecretKey: "",
		},
	}
}

// SaveConfig 保存配置到文件
func SaveConfig(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
