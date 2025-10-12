package rules

import (
	"fmt"
	"net"
	"time"
)

// RuleType 规则类型
type RuleType int

const (
	RuleTypeTCP RuleType = iota
	RuleTypeUDP
	RuleTypeHTTP
	RuleTypeHTTPS
	RuleTypeSOCKS5
	RuleTypeMixed // 混合协议
)

// String 返回规则类型名称
func (rt RuleType) String() string {
	switch rt {
	case RuleTypeTCP:
		return "TCP"
	case RuleTypeUDP:
		return "UDP"
	case RuleTypeHTTP:
		return "HTTP"
	case RuleTypeHTTPS:
		return "HTTPS"
	case RuleTypeSOCKS5:
		return "SOCKS5"
	case RuleTypeMixed:
		return "MIXED"
	default:
		return "UNKNOWN"
	}
}

// RuleAction 规则动作
type RuleAction int

const (
	RuleActionAllow RuleAction = iota
	RuleActionDeny
	RuleActionReject
	RuleActionRelay // 中继转发
)

// String 返回规则动作名称
func (ra RuleAction) String() string {
	switch ra {
	case RuleActionAllow:
		return "ALLOW"
	case RuleActionDeny:
		return "DENY"
	case RuleActionReject:
		return "REJECT"
	case RuleActionRelay:
		return "RELAY"
	default:
		return "UNKNOWN"
	}
}

// RuleStatus 规则状态
type RuleStatus int

const (
	RuleStatusPending RuleStatus = iota
	RuleStatusActive
	RuleStatusInactive
	RuleStatusDeleted
)

// String 返回规则状态名称
func (rs RuleStatus) String() string {
	switch rs {
	case RuleStatusPending:
		return "PENDING"
	case RuleStatusActive:
		return "ACTIVE"
	case RuleStatusInactive:
		return "INACTIVE"
	case RuleStatusDeleted:
		return "DELETED"
	default:
		return "UNKNOWN"
	}
}

// PortRange 端口范围
type PortRange struct {
	Start uint16 `json:"start"`
	End   uint16 `json:"end"`
}

// Contains 检查端口是否在范围内
func (pr PortRange) Contains(port uint16) bool {
	return port >= pr.Start && port <= pr.End
}

// String 返回端口范围字符串
func (pr PortRange) String() string {
	if pr.Start == pr.End {
		return fmt.Sprintf("%d", pr.Start)
	}
	return fmt.Sprintf("%d-%d", pr.Start, pr.End)
}

// IPRange IP地址范围
type IPRange struct {
	Network *net.IPNet `json:"network"`
	Start   net.IP     `json:"start,omitempty"`
	End     net.IP     `json:"end,omitempty"`
}

// Contains 检查IP是否在范围内
func (ir IPRange) Contains(ip net.IP) bool {
	if ir.Network != nil {
		return ir.Network.Contains(ip)
	}

	if ir.Start != nil && ir.End != nil {
		return bytesCompare(ip, ir.Start) >= 0 && bytesCompare(ip, ir.End) <= 0
	}

	return false
}

// String 返回IP范围字符串
func (ir IPRange) String() string {
	if ir.Network != nil {
		return ir.Network.String()
	}
	if ir.Start != nil && ir.End != nil {
		return fmt.Sprintf("%s-%s", ir.Start.String(), ir.End.String())
	}
	return "0.0.0.0/0"
}

// bytesCompare 比较两个IP地址的字节
func bytesCompare(a, b net.IP) int {
	a = a.To16()
	b = b.To16()

	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// TrafficLimit 流量限制
type TrafficLimit struct {
	BytesPerSecond   int64 `json:"bytes_per_second"`  // 每秒字节数
	ConnectionsLimit int   `json:"connections_limit"` // 连接数限制
	RequestsPerMin   int   `json:"requests_per_min"`  // 每分钟请求数
	BurstSize        int64 `json:"burst_size"`        // 突发大小
	Enabled          bool  `json:"enabled"`
}

// IsEmpty 检查限制是否为空
func (tl TrafficLimit) IsEmpty() bool {
	return tl.BytesPerSecond == 0 && tl.ConnectionsLimit == 0 && tl.RequestsPerMin == 0
}

// Rule 隧道规则
type Rule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// 规则匹配条件
	Type        RuleType    `json:"type"`
	SourceIPs   []IPRange   `json:"source_ips"`
	TargetIPs   []IPRange   `json:"target_ips"`
	SourcePorts []PortRange `json:"source_ports"`
	TargetPorts []PortRange `json:"target_ports"`
	Domains     []string    `json:"domains"`  // 域名匹配
	Keywords    []string    `json:"keywords"` // 关键字匹配

	// 规则动作
	Action     RuleAction `json:"action"`
	TargetAddr string     `json:"target_addr"` // 目标地址（用于RELAY）

	// 限制配置
	TrafficLimit TrafficLimit `json:"traffic_limit"`

	// 时间配置
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`

	// 状态信息
	Status    RuleStatus `json:"status"`
	Priority  int        `json:"priority"` // 优先级，数字越大优先级越高
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Version   int64      `json:"version"` // 版本号

	// 统计信息
	HitCount   int64      `json:"hit_count"`   // 命中次数
	BytesTotal int64      `json:"bytes_total"` // 总字节数
	LastHit    *time.Time `json:"last_hit,omitempty"`

	// 元数据
	Metadata map[string]interface{} `json:"metadata"`
}

// NewRule 创建新规则
func NewRule(id, name string, ruleType RuleType, action RuleAction) *Rule {
	now := time.Now()
	return &Rule{
		ID:         id,
		Name:       name,
		Type:       ruleType,
		Action:     action,
		Status:     RuleStatusPending,
		Priority:   0,
		CreatedAt:  now,
		UpdatedAt:  now,
		Version:    1,
		HitCount:   0,
		BytesTotal: 0,
		Metadata:   make(map[string]interface{}),
	}
}

// IsValid 检查规则是否有效
func (r *Rule) IsValid() bool {
	now := time.Now()

	// 检查状态
	if r.Status != RuleStatusActive {
		return false
	}

	// 检查时间有效性
	if r.ValidFrom != nil && now.Before(*r.ValidFrom) {
		return false
	}
	if r.ValidUntil != nil && now.After(*r.ValidUntil) {
		return false
	}

	return true
}

// IsExpired 检查规则是否过期
func (r *Rule) IsExpired() bool {
	if r.ValidUntil == nil {
		return false
	}
	return time.Now().After(*r.ValidUntil)
}

// MatchesIP 检查IP是否匹配规则
func (r *Rule) MatchesIP(srcIP, dstIP net.IP) bool {
	// 检查源IP
	if len(r.SourceIPs) > 0 {
		matched := false
		for _, ipRange := range r.SourceIPs {
			if ipRange.Contains(srcIP) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 检查目标IP
	if len(r.TargetIPs) > 0 {
		matched := false
		for _, ipRange := range r.TargetIPs {
			if ipRange.Contains(dstIP) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// MatchesPort 检查端口是否匹配规则
func (r *Rule) MatchesPort(srcPort, dstPort uint16) bool {
	// 检查源端口
	if len(r.SourcePorts) > 0 {
		matched := false
		for _, portRange := range r.SourcePorts {
			if portRange.Contains(srcPort) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 检查目标端口
	if len(r.TargetPorts) > 0 {
		matched := false
		for _, portRange := range r.TargetPorts {
			if portRange.Contains(dstPort) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// MatchesDomain 检查域名是否匹配规则
func (r *Rule) MatchesDomain(domain string) bool {
	if len(r.Domains) == 0 {
		return true
	}

	for _, ruleDomain := range r.Domains {
		if matchDomain(domain, ruleDomain) {
			return true
		}
	}

	return false
}

// MatchesKeyword 检查关键字是否匹配规则
func (r *Rule) MatchesKeyword(content string) bool {
	if len(r.Keywords) == 0 {
		return true
	}

	for _, keyword := range r.Keywords {
		if containsIgnoreCase(content, keyword) {
			return true
		}
	}

	return false
}

// IncrementHit 增加命中计数
func (r *Rule) IncrementHit(bytes int64) {
	r.HitCount++
	r.BytesTotal += bytes
	now := time.Now()
	r.LastHit = &now
}

// UpdateVersion 更新版本号
func (r *Rule) UpdateVersion() {
	r.Version++
	r.UpdatedAt = time.Now()
}

// Clone 克隆规则
func (r *Rule) Clone() *Rule {
	clone := *r

	// 深拷贝切片
	clone.SourceIPs = make([]IPRange, len(r.SourceIPs))
	copy(clone.SourceIPs, r.SourceIPs)

	clone.TargetIPs = make([]IPRange, len(r.TargetIPs))
	copy(clone.TargetIPs, r.TargetIPs)

	clone.SourcePorts = make([]PortRange, len(r.SourcePorts))
	copy(clone.SourcePorts, r.SourcePorts)

	clone.TargetPorts = make([]PortRange, len(r.TargetPorts))
	copy(clone.TargetPorts, r.TargetPorts)

	clone.Domains = make([]string, len(r.Domains))
	copy(clone.Domains, r.Domains)

	clone.Keywords = make([]string, len(r.Keywords))
	copy(clone.Keywords, r.Keywords)

	// 深拷贝元数据
	clone.Metadata = make(map[string]interface{})
	for k, v := range r.Metadata {
		clone.Metadata[k] = v
	}

	return &clone
}

// RuleSet 规则集
type RuleSet struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Rules       []*Rule   `json:"rules"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NewRuleSet 创建新规则集
func NewRuleSet(id, name string) *RuleSet {
	now := time.Now()
	return &RuleSet{
		ID:        id,
		Name:      name,
		Rules:     make([]*Rule, 0),
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// AddRule 添加规则
func (rs *RuleSet) AddRule(rule *Rule) {
	rs.Rules = append(rs.Rules, rule)
	rs.Version++
	rs.UpdatedAt = time.Now()
}

// RemoveRule 移除规则
func (rs *RuleSet) RemoveRule(ruleID string) bool {
	for i, rule := range rs.Rules {
		if rule.ID == ruleID {
			rs.Rules = append(rs.Rules[:i], rs.Rules[i+1:]...)
			rs.Version++
			rs.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// GetRule 获取规则
func (rs *RuleSet) GetRule(ruleID string) *Rule {
	for _, rule := range rs.Rules {
		if rule.ID == ruleID {
			return rule
		}
	}
	return nil
}

// SortByPriority 按优先级排序规则
func (rs *RuleSet) SortByPriority() {
	// 使用冒泡排序，优先级高的在前
	n := len(rs.Rules)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if rs.Rules[j].Priority < rs.Rules[j+1].Priority {
				rs.Rules[j], rs.Rules[j+1] = rs.Rules[j+1], rs.Rules[j]
			}
		}
	}
}

// GetActiveRules 获取活跃规则
func (rs *RuleSet) GetActiveRules() []*Rule {
	var activeRules []*Rule
	for _, rule := range rs.Rules {
		if rule.IsValid() {
			activeRules = append(activeRules, rule)
		}
	}
	return activeRules
}

// Clone 克隆规则集
func (rs *RuleSet) Clone() *RuleSet {
	clone := *rs
	clone.Rules = make([]*Rule, len(rs.Rules))
	for i, rule := range rs.Rules {
		clone.Rules[i] = rule.Clone()
	}
	return &clone
}

// 辅助函数

// matchDomain 匹配域名（支持通配符）
func matchDomain(domain, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if pattern == domain {
		return true
	}

	// 支持 *.example.com 形式的通配符
	if len(pattern) > 2 && pattern[:2] == "*." {
		suffix := pattern[1:] // .example.com
		return len(domain) >= len(suffix) && domain[len(domain)-len(suffix):] == suffix
	}

	return false
}

// containsIgnoreCase 不区分大小写的字符串包含检查
func containsIgnoreCase(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	return contains(s, substr)
}

// toLower 转换为小写
func toLower(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			result[i] = r + 32
		} else {
			result[i] = r
		}
	}
	return string(result)
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

// indexOf 查找子串位置
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}





