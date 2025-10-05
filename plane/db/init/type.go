package init

import (
	"database/sql"
	"time"
)

// Node 节点信息
type Node struct {
	ID          string    `json:"id" db:"id"`                   // 节点唯一ID
	Name        string    `json:"name" db:"name"`               // 节点名称
	Type        string    `json:"type" db:"type"`               // 节点类型: client/server
	Status      string    `json:"status" db:"status"`           // 状态: online/offline/error
	IP          string    `json:"ip" db:"ip"`                   // 节点IP地址
	Port        int       `json:"port" db:"port"`               // 节点端口
	Version     string    `json:"version" db:"version"`         // 客户端版本
	CertID      string    `json:"cert_id" db:"cert_id"`         // 关联的证书ID
	APIKey      string    `json:"api_key" db:"api_key"`         // 节点API密钥
	GroupID     string    `json:"group_id" db:"group_id"`       // 所属节点组ID
	UserID      string    `json:"user_id" db:"user_id"`         // 所属用户ID
	LastSeen    time.Time `json:"last_seen" db:"last_seen"`     // 最后在线时间
	CreatedAt   time.Time `json:"created_at" db:"created_at"`   // 创建时间
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`   // 更新时间
	Tags        string    `json:"tags" db:"tags"`               // 标签（JSON）
	Description string    `json:"description" db:"description"` // 描述
}

// NodeGroup 节点组
type NodeGroup struct {
	ID          string    `json:"id" db:"id"`                   // 组ID
	Name        string    `json:"name" db:"name"`               // 组名称
	Type        string    `json:"type" db:"type"`               // 组类型: entry(入口组)/exit(出口组)
	UserID      string    `json:"user_id" db:"user_id"`         // 所属用户ID
	NodeCount   int       `json:"node_count" db:"node_count"`   // 节点数量
	CreatedAt   time.Time `json:"created_at" db:"created_at"`   // 创建时间
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`   // 更新时间
	Description string    `json:"description" db:"description"` // 描述
}

// Policy 策略配置
type Policy struct {
	ID          string    `json:"id" db:"id"`                   // 策略ID
	Name        string    `json:"name" db:"name"`               // 策略名称
	Type        string    `json:"type" db:"type"`               // 策略类型: protocol/acl/routing
	Priority    int       `json:"priority" db:"priority"`       // 优先级（数字越小优先级越高）
	Enabled     bool      `json:"enabled" db:"enabled"`         // 是否启用
	Config      string    `json:"config" db:"config"`           // 策略配置（JSON）
	NodeIDs     string    `json:"node_ids" db:"node_ids"`       // 应用的节点ID列表（JSON数组）
	CreatedAt   time.Time `json:"created_at" db:"created_at"`   // 创建时间
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`   // 更新时间
	Description string    `json:"description" db:"description"` // 描述
}

// PolicyConfig 策略配置详情（用于解析JSON）
type PolicyConfig struct {
	// 协议策略
	Protocols []string `json:"protocols,omitempty"` // tcp, udp, http, tls, socks

	// ACL规则
	AllowIPs   []string `json:"allow_ips,omitempty"`   // 允许的IP列表
	DenyIPs    []string `json:"deny_ips,omitempty"`    // 拒绝的IP列表
	AllowPorts []int    `json:"allow_ports,omitempty"` // 允许的端口列表
	DenyPorts  []int    `json:"deny_ports,omitempty"`  // 拒绝的端口列表

	// 路由策略
	TargetHost string `json:"target_host,omitempty"` // 目标主机
	TargetPort int    `json:"target_port,omitempty"` // 目标端口
}

// Certificate 证书信息
type Certificate struct {
	ID          string    `json:"id" db:"id"`                   // 证书ID
	Type        string    `json:"type" db:"type"`               // 证书类型: ca/leaf
	Name        string    `json:"name" db:"name"`               // 证书名称
	CommonName  string    `json:"common_name" db:"common_name"` // CN
	PublicKey   string    `json:"public_key" db:"public_key"`   // 公钥（PEM格式）
	PrivateKey  string    `json:"private_key" db:"private_key"` // 私钥（PEM格式，加密存储）
	Pin         string    `json:"pin" db:"pin"`                 // SPKI Pin
	ParentID    string    `json:"parent_id" db:"parent_id"`     // 父证书ID（用于链）
	NotBefore   time.Time `json:"not_before" db:"not_before"`   // 生效时间
	NotAfter    time.Time `json:"not_after" db:"not_after"`     // 过期时间
	CreatedAt   time.Time `json:"created_at" db:"created_at"`   // 创建时间
	Revoked     bool      `json:"revoked" db:"revoked"`         // 是否已吊销
	Description string    `json:"description" db:"description"` // 描述
}

// Statistics 统计数据
type Statistics struct {
	ID             string    `json:"id" db:"id"`                           // 统计ID
	NodeID         string    `json:"node_id" db:"node_id"`                 // 节点ID
	Timestamp      time.Time `json:"timestamp" db:"timestamp"`             // 时间戳
	BytesIn        int64     `json:"bytes_in" db:"bytes_in"`               // 入站字节数
	BytesOut       int64     `json:"bytes_out" db:"bytes_out"`             // 出站字节数
	PacketsIn      int64     `json:"packets_in" db:"packets_in"`           // 入站包数
	PacketsOut     int64     `json:"packets_out" db:"packets_out"`         // 出站包数
	Connections    int       `json:"connections" db:"connections"`         // 当前连接数
	ActiveSessions int       `json:"active_sessions" db:"active_sessions"` // 活跃会话数
	ErrorCount     int       `json:"error_count" db:"error_count"`         // 错误次数
	AvgLatency     float64   `json:"avg_latency" db:"avg_latency"`         // 平均延迟(ms)
	CPUUsage       float64   `json:"cpu_usage" db:"cpu_usage"`             // CPU使用率
	MemoryUsage    int64     `json:"memory_usage" db:"memory_usage"`       // 内存使用(bytes)
}

// User 用户信息（用于控制面板登录）
type User struct {
	ID           string       `json:"id" db:"id"`                       // 用户ID
	Username     string       `json:"username" db:"username"`           // 用户名
	PasswordHash string       `json:"-" db:"password_hash"`             // 密码哈希
	Email        string       `json:"email" db:"email"`                 // 邮箱
	Avatar       string       `json:"avatar" db:"avatar"`               // 头像URL
	Provider     string       `json:"provider" db:"provider"`           // 登录提供商: local/github
	ProviderID   string       `json:"provider_id" db:"provider_id"`     // 提供商用户ID
	Role         string       `json:"role" db:"role"`                   // 角色: admin/user
	Enabled      bool         `json:"enabled" db:"enabled"`             // 是否启用
	LastLoginAt  sql.NullTime `json:"last_login_at" db:"last_login_at"` // 最后登录时间（可为空）
	CreatedAt    time.Time    `json:"created_at" db:"created_at"`       // 创建时间
	UpdatedAt    time.Time    `json:"updated_at" db:"updated_at"`       // 更新时间
}

// Session 会话信息（存储在Redis）
type Session struct {
	Token     string    `json:"token"`      // JWT token
	UserID    string    `json:"user_id"`    // 用户ID
	Username  string    `json:"username"`   // 用户名
	Role      string    `json:"role"`       // 角色
	CreatedAt time.Time `json:"created_at"` // 创建时间
	ExpiresAt time.Time `json:"expires_at"` // 过期时间
}

// NodeStatus 节点实时状态（存储在Redis）
type NodeStatus struct {
	NodeID        string    `json:"node_id"`
	Online        bool      `json:"online"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	CurrentLoad   float64   `json:"current_load"`
	Connections   int       `json:"connections"`
}

// Plan 套餐
type Plan struct {
	ID             string    `json:"id" db:"id"`                             // 套餐ID
	Name           string    `json:"name" db:"name"`                         // 套餐名称
	MaxRules       int       `json:"max_rules" db:"max_rules"`               // 最大规则数（0=无限）
	MaxTraffic     int64     `json:"max_traffic" db:"max_traffic"`           // 最大流量(bytes)（0=无限）
	MaxBandwidth   int64     `json:"max_bandwidth" db:"max_bandwidth"`       // 最大带宽(bps)（0=无限）
	MaxConnections int       `json:"max_connections" db:"max_connections"`   // 最大连接数（0=无限）
	MaxConnectIPs  int       `json:"max_connect_ips" db:"max_connect_ips"`   // 最大连接IP数（0=无限）
	AllowedNodeIDs string    `json:"allowed_node_ids" db:"allowed_node_ids"` // 允许使用的节点ID列表（JSON，空=全部）
	BillingCycle   string    `json:"billing_cycle" db:"billing_cycle"`       // 计费周期: monthly/yearly/permanent
	Price          float64   `json:"price" db:"price"`                       // 价格
	Enabled        bool      `json:"enabled" db:"enabled"`                   // 是否启用
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
	Description    string    `json:"description" db:"description"`
}

// UserSubscription 用户订阅
type UserSubscription struct {
	ID           string    `json:"id" db:"id"`
	UserID       string    `json:"user_id" db:"user_id"`
	PlanID       string    `json:"plan_id" db:"plan_id"`
	StartDate    time.Time `json:"start_date" db:"start_date"`
	EndDate      time.Time `json:"end_date" db:"end_date"`
	Status       string    `json:"status" db:"status"` // active/expired/cancelled
	UsedRules    int       `json:"used_rules" db:"used_rules"`
	UsedTraffic  int64     `json:"used_traffic" db:"used_traffic"`
	TrafficReset time.Time `json:"traffic_reset" db:"traffic_reset"` // 流量重置时间
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// Tunnel 隧道（转发规则）
type Tunnel struct {
	ID              string    `json:"id" db:"id"`
	UserID          string    `json:"user_id" db:"user_id"`
	Name            string    `json:"name" db:"name"`
	Protocol        string    `json:"protocol" db:"protocol"`             // tcp/udp/http/https
	EntryGroupID    string    `json:"entry_group_id" db:"entry_group_id"` // 入口组ID
	ExitGroupID     string    `json:"exit_group_id" db:"exit_group_id"`   // 出口组ID
	LocalPort       int       `json:"local_port" db:"local_port"`         // 入口端口（全局唯一）
	Targets         string    `json:"targets" db:"targets"`               // 目标列表（JSON数组，多目标支持）
	Enabled         bool      `json:"enabled" db:"enabled"`
	TrafficIn       int64     `json:"traffic_in" db:"traffic_in"`
	TrafficOut      int64     `json:"traffic_out" db:"traffic_out"`
	ConnectionCount int       `json:"connection_count" db:"connection_count"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	Description     string    `json:"description" db:"description"`
}

// TunnelTarget 隧道目标
type TunnelTarget struct {
	Host   string `json:"host"`   // 域名或IP
	Port   int    `json:"port"`   // 端口
	Weight int    `json:"weight"` // 权重（用于负载均衡）
}

// ConnectionKey CK认证密钥
type ConnectionKey struct {
	ID        string    `json:"id" db:"id"`
	Key       string    `json:"key" db:"key"` // CK密钥
	NodeID    string    `json:"node_id" db:"node_id"`
	Type      string    `json:"type" db:"type"` // node/user
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Wallet 用户钱包
type Wallet struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`       // 用户ID
	Balance   float64   `json:"balance" db:"balance"`       // 余额
	Frozen    float64   `json:"frozen" db:"frozen"`         // 冻结金额
	CreatedAt time.Time `json:"created_at" db:"created_at"` // 创建时间
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"` // 更新时间
}

// WalletTransaction 钱包交易记录
type WalletTransaction struct {
	ID            string    `json:"id" db:"id"`
	WalletID      string    `json:"wallet_id" db:"wallet_id"`           // 钱包ID
	UserID        string    `json:"user_id" db:"user_id"`               // 用户ID
	Type          string    `json:"type" db:"type"`                     // 类型: recharge/consume/refund/withdraw
	Amount        float64   `json:"amount" db:"amount"`                 // 金额（正数为收入，负数为支出）
	Balance       float64   `json:"balance" db:"balance"`               // 交易后余额
	RelatedID     string    `json:"related_id" db:"related_id"`         // 关联ID（订单ID等）
	RelatedType   string    `json:"related_type" db:"related_type"`     // 关联类型: subscription/plan/manual
	Status        string    `json:"status" db:"status"`                 // 状态: pending/completed/failed/cancelled
	PaymentMethod string    `json:"payment_method" db:"payment_method"` // 支付方式: alipay/wechat/card/admin
	TransactionNo string    `json:"transaction_no" db:"transaction_no"` // 交易流水号
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	Description   string    `json:"description" db:"description"` // 描述
}

// Notification 通知
type Notification struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`       // 用户ID（空表示全局通知）
	Type      string    `json:"type" db:"type"`             // 类型: system/subscription/traffic/security
	Title     string    `json:"title" db:"title"`           // 标题
	Content   string    `json:"content" db:"content"`       // 内容
	Link      string    `json:"link" db:"link"`             // 相关链接
	IsRead    bool      `json:"is_read" db:"is_read"`       // 是否已读
	Priority  string    `json:"priority" db:"priority"`     // 优先级: low/normal/high/urgent
	CreatedAt time.Time `json:"created_at" db:"created_at"` // 创建时间
}

// Announcement 公告
type Announcement struct {
	ID        string    `json:"id" db:"id"`
	Title     string    `json:"title" db:"title"`           // 标题
	Content   string    `json:"content" db:"content"`       // 内容（支持Markdown）
	Type      string    `json:"type" db:"type"`             // 类型: notice/maintenance/update/warning
	Priority  string    `json:"priority" db:"priority"`     // 优先级: low/normal/high
	Enabled   bool      `json:"enabled" db:"enabled"`       // 是否启用
	StartTime time.Time `json:"start_time" db:"start_time"` // 开始显示时间
	EndTime   time.Time `json:"end_time" db:"end_time"`     // 结束显示时间
	CreatedBy string    `json:"created_by" db:"created_by"` // 创建人ID
	CreatedAt time.Time `json:"created_at" db:"created_at"` // 创建时间
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"` // 更新时间
}

// SystemSettings 系统设置
type SystemSettings struct {
	ID          string    `json:"id" db:"id"`
	Key         string    `json:"key" db:"key"`                 // 设置键（唯一）
	Value       string    `json:"value" db:"value"`             // 设置值（JSON格式）
	Category    string    `json:"category" db:"category"`       // 分类: captcha/security/general/notification
	UpdatedBy   string    `json:"updated_by" db:"updated_by"`   // 更新人ID
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`   // 更新时间
	Description string    `json:"description" db:"description"` // 描述
}

// CaptchaSettings 验证码设置（存储在SystemSettings的value中）
type CaptchaSettings struct {
	Enabled   bool   `json:"enabled"`    // 是否启用验证码
	Type      string `json:"type"`       // 类型: image/turnstile
	SiteKey   string `json:"site_key"`   // Turnstile站点密钥
	SecretKey string `json:"secret_key"` // Turnstile密钥
	// 图片验证码设置
	ImageWidth  int `json:"image_width"`  // 宽度
	ImageHeight int `json:"image_height"` // 高度
	CodeLength  int `json:"code_length"`  // 验证码长度
}

// CaptchaSession 验证码会话（存储在Redis）
type CaptchaSession struct {
	ID        string    `json:"id"`         // 会话ID
	Code      string    `json:"code"`       // 验证码（加密存储）
	CreatedAt time.Time `json:"created_at"` // 创建时间
	ExpiresAt time.Time `json:"expires_at"` // 过期时间
}
