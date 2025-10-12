package pool

import (
	"fmt"
	"time"
)

// PoolConfig 连接池配置
type PoolConfig struct {
	// 基本配置
	MinConnections int           `json:"min_connections" yaml:"min_connections"`
	MaxConnections int           `json:"max_connections" yaml:"max_connections"`
	MaxIdleTime    time.Duration `json:"max_idle_time" yaml:"max_idle_time"`
	MaxConnAge     time.Duration `json:"max_conn_age" yaml:"max_conn_age"`

	// 质量阈值
	MinQualityScore float64       `json:"min_quality_score" yaml:"min_quality_score"`
	MaxRTT          time.Duration `json:"max_rtt" yaml:"max_rtt"`
	MaxPacketLoss   float64       `json:"max_packet_loss" yaml:"max_packet_loss"`
	MaxErrorRate    float64       `json:"max_error_rate" yaml:"max_error_rate"`

	// 自适应参数
	ScaleUpThreshold   float64       `json:"scale_up_threshold" yaml:"scale_up_threshold"`
	ScaleDownThreshold float64       `json:"scale_down_threshold" yaml:"scale_down_threshold"`
	AdaptInterval      time.Duration `json:"adapt_interval" yaml:"adapt_interval"`

	// 监控配置
	QualityCheckInterval time.Duration `json:"quality_check_interval" yaml:"quality_check_interval"`
	CleanupInterval      time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
	RTTSampleSize        int           `json:"rtt_sample_size" yaml:"rtt_sample_size"`

	// 高级配置
	EnablePreWarming    bool          `json:"enable_pre_warming" yaml:"enable_pre_warming"`
	PreWarmingTargets   int           `json:"pre_warming_targets" yaml:"pre_warming_targets"`
	LoadBalanceStrategy string        `json:"load_balance_strategy" yaml:"load_balance_strategy"`
	HealthCheckEnabled  bool          `json:"health_check_enabled" yaml:"health_check_enabled"`
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
}

// DefaultPoolConfig 默认连接池配置
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		// 基本配置
		MinConnections: 2,
		MaxConnections: 20,
		MaxIdleTime:    5 * time.Minute,
		MaxConnAge:     30 * time.Minute,

		// 质量阈值
		MinQualityScore: 60.0,
		MaxRTT:          500 * time.Millisecond,
		MaxPacketLoss:   0.05, // 5%
		MaxErrorRate:    0.02, // 2%

		// 自适应参数
		ScaleUpThreshold:   80.0, // 平均质量低于80分时扩容
		ScaleDownThreshold: 95.0, // 平均质量高于95分时缩容
		AdaptInterval:      30 * time.Second,

		// 监控配置
		QualityCheckInterval: 10 * time.Second,
		CleanupInterval:      1 * time.Minute,
		RTTSampleSize:        20,

		// 高级配置
		EnablePreWarming:    true,
		PreWarmingTargets:   3,
		LoadBalanceStrategy: "quality_based", // quality_based, round_robin, least_conn
		HealthCheckEnabled:  true,
		HealthCheckInterval: 30 * time.Second,
	}
}

// HighPerformanceConfig 高性能配置
func HighPerformanceConfig() *PoolConfig {
	config := DefaultPoolConfig()

	// 增加连接数
	config.MinConnections = 5
	config.MaxConnections = 50

	// 更严格的质量要求
	config.MinQualityScore = 75.0
	config.MaxRTT = 200 * time.Millisecond
	config.MaxPacketLoss = 0.02 // 2%
	config.MaxErrorRate = 0.01  // 1%

	// 更积极的扩缩容
	config.ScaleUpThreshold = 85.0
	config.ScaleDownThreshold = 98.0
	config.AdaptInterval = 15 * time.Second

	// 更频繁的监控
	config.QualityCheckInterval = 5 * time.Second
	config.HealthCheckInterval = 15 * time.Second

	// 预热更多连接
	config.PreWarmingTargets = 8

	return config
}

// LowResourceConfig 低资源配置
func LowResourceConfig() *PoolConfig {
	config := DefaultPoolConfig()

	// 减少连接数
	config.MinConnections = 1
	config.MaxConnections = 8

	// 放宽质量要求
	config.MinQualityScore = 50.0
	config.MaxRTT = 1 * time.Second
	config.MaxPacketLoss = 0.1 // 10%
	config.MaxErrorRate = 0.05 // 5%

	// 较慢的适应速度
	config.AdaptInterval = 1 * time.Minute

	// 较少的监控频率
	config.QualityCheckInterval = 30 * time.Second
	config.CleanupInterval = 2 * time.Minute
	config.HealthCheckInterval = 1 * time.Minute

	// 禁用预热
	config.EnablePreWarming = false
	config.PreWarmingTargets = 1

	return config
}

// Validate 验证配置
func (c *PoolConfig) Validate() error {
	if c.MinConnections < 1 {
		return fmt.Errorf("最小连接数必须大于0")
	}

	if c.MaxConnections < c.MinConnections {
		return fmt.Errorf("最大连接数必须大于等于最小连接数")
	}

	if c.MaxIdleTime <= 0 {
		return fmt.Errorf("最大空闲时间必须大于0")
	}

	if c.MaxConnAge <= 0 {
		return fmt.Errorf("最大连接年龄必须大于0")
	}

	if c.MinQualityScore < 0 || c.MinQualityScore > 100 {
		return fmt.Errorf("最小质量分数必须在0-100之间")
	}

	if c.MaxPacketLoss < 0 || c.MaxPacketLoss > 1 {
		return fmt.Errorf("最大丢包率必须在0-1之间")
	}

	if c.MaxErrorRate < 0 || c.MaxErrorRate > 1 {
		return fmt.Errorf("最大错误率必须在0-1之间")
	}

	if c.ScaleUpThreshold < 0 || c.ScaleUpThreshold > 100 {
		return fmt.Errorf("扩容阈值必须在0-100之间")
	}

	if c.ScaleDownThreshold < 0 || c.ScaleDownThreshold > 100 {
		return fmt.Errorf("缩容阈值必须在0-100之间")
	}

	if c.ScaleUpThreshold >= c.ScaleDownThreshold {
		return fmt.Errorf("扩容阈值必须小于缩容阈值")
	}

	if c.RTTSampleSize < 1 {
		return fmt.Errorf("RTT样本大小必须大于0")
	}

	validStrategies := map[string]bool{
		"quality_based": true,
		"round_robin":   true,
		"least_conn":    true,
		"random":        true,
	}

	if !validStrategies[c.LoadBalanceStrategy] {
		return fmt.Errorf("无效的负载均衡策略: %s", c.LoadBalanceStrategy)
	}

	return nil
}

// Apply 应用配置到连接池
func (c *PoolConfig) Apply(pool *AdaptivePool) {
	pool.minConnections = c.MinConnections
	pool.maxConnections = c.MaxConnections
	pool.maxIdleTime = c.MaxIdleTime
	pool.maxConnAge = c.MaxConnAge
	pool.minQualityScore = c.MinQualityScore
	pool.maxRTT = c.MaxRTT
	pool.maxPacketLoss = c.MaxPacketLoss
	pool.maxErrorRate = c.MaxErrorRate
	pool.scaleUpThreshold = c.ScaleUpThreshold
	pool.scaleDownThreshold = c.ScaleDownThreshold
	pool.adaptInterval = c.AdaptInterval
}

// LoadBalanceStrategy 负载均衡策略
type LoadBalanceStrategy int

const (
	StrategyQualityBased LoadBalanceStrategy = iota
	StrategyRoundRobin
	StrategyLeastConn
	StrategyRandom
)

// String 返回策略名称
func (s LoadBalanceStrategy) String() string {
	switch s {
	case StrategyQualityBased:
		return "quality_based"
	case StrategyRoundRobin:
		return "round_robin"
	case StrategyLeastConn:
		return "least_conn"
	case StrategyRandom:
		return "random"
	default:
		return "unknown"
	}
}

// ParseLoadBalanceStrategy 解析负载均衡策略
func ParseLoadBalanceStrategy(strategy string) LoadBalanceStrategy {
	switch strategy {
	case "quality_based":
		return StrategyQualityBased
	case "round_robin":
		return StrategyRoundRobin
	case "least_conn":
		return StrategyLeastConn
	case "random":
		return StrategyRandom
	default:
		return StrategyQualityBased
	}
}
