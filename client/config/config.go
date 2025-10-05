package config

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Config 节点配置
type Config struct {
	Node      NodeConfig      `yaml:"node"`
	Plane     PlaneConfig     `yaml:"plane"`
	Listen    ListenConfig    `yaml:"listen"`
	Pool      PoolConfig      `yaml:"pool"`
	RateLimit RateLimitConfig `yaml:"ratelimit"`
	TLS       TLSConfig       `yaml:"tls"`
	Log       LogConfig       `yaml:"log"`
}

// NodeConfig 节点信息
type NodeConfig struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Type    string `yaml:"type"` // entry/exit
	GroupID string `yaml:"group_id"`
}

// PlaneConfig Plane连接配置
type PlaneConfig struct {
	URL                  string `yaml:"url"`
	CK                   string `yaml:"ck"`
	ReconnectInterval    int    `yaml:"reconnect_interval"`
	MaxReconnectAttempts int    `yaml:"max_reconnect_attempts"`
	Timeout              int    `yaml:"timeout"`
}

// ListenConfig 监听配置
type ListenConfig struct {
	Host  string `yaml:"host"`
	Ports []int  `yaml:"ports"`
}

// PoolConfig 连接池配置
type PoolConfig struct {
	MinConns          int  `yaml:"min_conns"`
	MaxConns          int  `yaml:"max_conns"`
	IdleTimeout       int  `yaml:"idle_timeout"`
	HeartbeatInterval int  `yaml:"heartbeat_interval"`
	AutoScale         bool `yaml:"auto_scale"`
}

// RateLimitConfig 限速配置
type RateLimitConfig struct {
	Enabled          bool  `yaml:"enabled"`
	DefaultBandwidth int64 `yaml:"default_bandwidth"`
	Burst            int64 `yaml:"burst"`
}

// TLSConfig TLS配置
type TLSConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Cert            string `yaml:"cert"`
	Key             string `yaml:"key"`
	CA              string `yaml:"ca"`
	PinVerification bool   `yaml:"pin_verification"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	Output     string `yaml:"output"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
}

// LoadConfig 加载配置文件
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 首次运行，生成节点ID
	if cfg.Node.ID == "" {
		cfg.Node.ID = uuid.New().String()
		if err := SaveConfig(path, &cfg); err != nil {
			return nil, fmt.Errorf("保存节点ID失败: %w", err)
		}
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return &cfg, nil
}

// SaveConfig 保存配置文件
func SaveConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证节点类型
	if c.Node.Type != "entry" && c.Node.Type != "exit" {
		return fmt.Errorf("无效的节点类型: %s，必须是 entry 或 exit", c.Node.Type)
	}

	// 验证Plane地址
	if c.Plane.URL == "" {
		return fmt.Errorf("Plane URL 不能为空")
	}

	// 验证日志级别
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Log.Level] {
		return fmt.Errorf("无效的日志级别: %s", c.Log.Level)
	}

	return nil
}






