package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"
)

// IdentityInfo 节点身份信息
type IdentityInfo struct {
	NodeID           string            `json:"node_id"`
	NodeName         string            `json:"node_name"`
	HardwareID       string            `json:"hardware_id"`
	SystemInfo       map[string]string `json:"system_info"`
	NetworkInfo      []NetworkInfo     `json:"network_info"`
	CreatedAt        time.Time         `json:"created_at"`
	LastAuth         time.Time         `json:"last_auth"`
	AuthToken        string            `json:"auth_token"`
	Role             string            `json:"role"`
	Groups           []string          `json:"groups"`
	AllowedProtocols []string          `json:"allowed_protocols"`
}

// NetworkInfo 网络接口信息
type NetworkInfo struct {
	Name       string   `json:"name"`
	MACAddress string   `json:"mac_address"`
	IPAddress  []string `json:"ip_address"`
	IsUp       bool     `json:"is_up"`
	IsLoopback bool     `json:"is_loopback"`
}

// ManagerConfig 身份管理器配置
type ManagerConfig struct {
	DataDir             string        `json:"data_dir"`              // 数据目录
	IdentityFile        string        `json:"identity_file"`         // 身份文件名
	Token               string        `json:"token"`                 // 初始认证令牌
	PlaneAddr           string        `json:"plane_addr"`            // 面板服务器地址
	APIKey              string        `json:"api_key"`               // API密钥
	AutoGenerate        bool          `json:"auto_generate"`         // 自动生成身份
	EnableHardwareID    bool          `json:"enable_hardware_id"`    // 启用硬件ID
	RefreshInterval     time.Duration `json:"refresh_interval"`      // 刷新间隔
	TokenRefreshEnabled bool          `json:"token_refresh_enabled"` // 启用令牌刷新
	TokenRefreshWindow  time.Duration `json:"token_refresh_window"`  // 令牌刷新窗口
}

// DefaultManagerConfig 默认身份管理器配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		DataDir:             "./data",
		IdentityFile:        "identity.json",
		AutoGenerate:        true,
		EnableHardwareID:    true,
		RefreshInterval:     24 * time.Hour,
		TokenRefreshEnabled: true,
		TokenRefreshWindow:  7 * 24 * time.Hour,
	}
}

// Manager 身份管理器
type Manager struct {
	config *ManagerConfig
	logger *zap.Logger

	identity    *IdentityInfo
	initialized bool
	lock        sync.RWMutex
}

// NewManager 创建身份管理器
func NewManager(config *ManagerConfig) (*Manager, error) {
	if config == nil {
		config = DefaultManagerConfig()
	}

	m := &Manager{
		config: config,
		logger: zap.L().Named("identity"),
	}

	// 确保数据目录存在
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}

	return m, nil
}

// Initialize 初始化身份
func (m *Manager) Initialize() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.initialized {
		return nil
	}

	// 尝试加载身份
	identity, err := m.loadIdentity()
	if err != nil {
		if os.IsNotExist(err) {
			// 如果启用自动生成，则生成新身份
			if m.config.AutoGenerate {
				m.logger.Info("身份文件不存在，自动生成")
				identity, err = m.generateIdentity()
				if err != nil {
					return fmt.Errorf("生成身份失败: %w", err)
				}

				// 保存新身份
				if err := m.saveIdentity(identity); err != nil {
					return fmt.Errorf("保存身份失败: %w", err)
				}
			} else {
				return fmt.Errorf("身份文件不存在，且未启用自动生成")
			}
		} else {
			return fmt.Errorf("加载身份失败: %w", err)
		}
	}

	// 设置身份
	m.identity = identity
	m.initialized = true

	m.logger.Info("身份初始化完成",
		zap.String("node_id", identity.NodeID),
		zap.String("node_name", identity.NodeName),
		zap.String("role", identity.Role))

	return nil
}

// GetNodeID 获取节点ID
func (m *Manager) GetNodeID() string {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if !m.initialized || m.identity == nil {
		return ""
	}

	return m.identity.NodeID
}

// GetAuthToken 获取认证令牌
func (m *Manager) GetAuthToken() string {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if !m.initialized || m.identity == nil {
		return ""
	}

	// 如果配置了固定令牌，使用配置的令牌
	if m.config.Token != "" {
		return m.config.Token
	}

	return m.identity.AuthToken
}

// UpdateToken 更新认证令牌
func (m *Manager) UpdateToken(token string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if !m.initialized || m.identity == nil {
		return fmt.Errorf("身份未初始化")
	}

	m.identity.AuthToken = token
	m.identity.LastAuth = time.Now()

	return m.saveIdentity(m.identity)
}

// GetIdentity 获取身份信息
func (m *Manager) GetIdentity() *IdentityInfo {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if !m.initialized || m.identity == nil {
		return nil
	}

	// 返回副本，避免外部修改
	identity := *m.identity

	return &identity
}

// loadIdentity 加载身份
func (m *Manager) loadIdentity() (*IdentityInfo, error) {
	identityPath := filepath.Join(m.config.DataDir, m.config.IdentityFile)

	// 检查文件是否存在
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		return nil, err
	}

	// 读取文件内容
	file, err := os.Open(identityPath)
	if err != nil {
		return nil, fmt.Errorf("打开身份文件失败: %w", err)
	}
	defer file.Close()

	// 解析JSON
	var identity IdentityInfo
	err = json.NewDecoder(file).Decode(&identity)
	if err != nil {
		return nil, fmt.Errorf("解析身份文件失败: %w", err)
	}

	// 验证身份
	if identity.NodeID == "" {
		return nil, fmt.Errorf("身份文件中缺少NodeID")
	}

	// 更新网络信息
	networkInfo, err := collectNetworkInfo()
	if err != nil {
		m.logger.Warn("收集网络信息失败", zap.Error(err))
	} else {
		identity.NetworkInfo = networkInfo
	}

	return &identity, nil
}

// saveIdentity 保存身份
func (m *Manager) saveIdentity(identity *IdentityInfo) error {
	identityPath := filepath.Join(m.config.DataDir, m.config.IdentityFile)

	// 创建或截断文件
	file, err := os.Create(identityPath)
	if err != nil {
		return fmt.Errorf("创建身份文件失败: %w", err)
	}
	defer file.Close()

	// 序列化为JSON
	err = json.NewEncoder(file).Encode(identity)
	if err != nil {
		return fmt.Errorf("写入身份文件失败: %w", err)
	}

	return nil
}

// generateIdentity 生成身份
func (m *Manager) generateIdentity() (*IdentityInfo, error) {
	// 生成随机节点ID
	nodeID, err := generateRandomID()
	if err != nil {
		return nil, fmt.Errorf("生成节点ID失败: %w", err)
	}

	// 获取主机名作为节点名称
	nodeName, err := os.Hostname()
	if err != nil {
		nodeName = "node-" + nodeID[:8]
	}

	// 生成硬件ID
	hardwareID := ""
	if m.config.EnableHardwareID {
		hardwareID, err = generateHardwareID()
		if err != nil {
			m.logger.Warn("生成硬件ID失败", zap.Error(err))
		}
	}

	// 收集系统信息
	systemInfo, err := collectSystemInfo()
	if err != nil {
		m.logger.Warn("收集系统信息失败", zap.Error(err))
	}

	// 收集网络信息
	networkInfo, err := collectNetworkInfo()
	if err != nil {
		m.logger.Warn("收集网络信息失败", zap.Error(err))
	}

	// 创建身份信息
	identity := &IdentityInfo{
		NodeID:      nodeID,
		NodeName:    nodeName,
		HardwareID:  hardwareID,
		SystemInfo:  systemInfo,
		NetworkInfo: networkInfo,
		CreatedAt:   time.Now(),
		LastAuth:    time.Now(),
		AuthToken:   m.config.Token, // 使用配置中的初始令牌
		Role:        "client",       // 默认角色为客户端
		Groups:      []string{},     // 初始无组
	}

	return identity, nil
}

// generateRandomID 生成随机ID
func generateRandomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// generateHardwareID 生成硬件ID
func generateHardwareID() (string, error) {
	// 收集网络接口信息
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("获取网络接口失败: %w", err)
	}

	// 使用第一个非本地环回接口的MAC地址
	var macAddr string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback == 0 && len(iface.HardwareAddr) > 0 {
			macAddr = iface.HardwareAddr.String()
			break
		}
	}

	if macAddr == "" {
		return "", fmt.Errorf("未找到可用的MAC地址")
	}

	// 结合主机名和MAC地址生成硬件ID
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// 创建哈希
	h := sha256.New()
	h.Write([]byte(macAddr))
	h.Write([]byte(hostname))

	// 返回16进制编码的结果
	return hex.EncodeToString(h.Sum(nil)), nil
}

// collectSystemInfo 收集系统信息
func collectSystemInfo() (map[string]string, error) {
	info := make(map[string]string)

	// 操作系统
	info["os"] = runtime.GOOS

	// 架构
	info["arch"] = runtime.GOARCH

	// CPU核心数
	info["cpu_cores"] = fmt.Sprintf("%d", runtime.NumCPU())

	// 主机名
	hostname, err := os.Hostname()
	if err == nil {
		info["hostname"] = hostname
	}

	// TODO: 收集更多系统信息

	return info, nil
}

// collectNetworkInfo 收集网络信息
func collectNetworkInfo() ([]NetworkInfo, error) {
	var networkInfos []NetworkInfo

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("获取网络接口失败: %w", err)
	}

	for _, iface := range interfaces {
		info := NetworkInfo{
			Name:       iface.Name,
			MACAddress: iface.HardwareAddr.String(),
			IsUp:       iface.Flags&net.FlagUp != 0,
			IsLoopback: iface.Flags&net.FlagLoopback != 0,
		}

		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
					info.IPAddress = append(info.IPAddress, ipNet.IP.String())
				}
			}
		}

		networkInfos = append(networkInfos, info)
	}

	return networkInfos, nil
}

// NeedsTokenRefresh 检查是否需要刷新令牌
func (m *Manager) NeedsTokenRefresh() bool {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if !m.initialized || m.identity == nil {
		return false
	}

	// 如果不启用令牌刷新，则不需要刷新
	if !m.config.TokenRefreshEnabled {
		return false
	}

	// 检查上次认证时间
	return time.Since(m.identity.LastAuth) > m.config.TokenRefreshWindow
}

// End of identity/manager.go
