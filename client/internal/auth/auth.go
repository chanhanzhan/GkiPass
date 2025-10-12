package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AuthConfig 认证配置
type AuthConfig struct {
	APIEndpoint     string            // API端点
	NodeID          string            // 节点ID
	CKValue         string            // 连接密钥值
	APIKey          string            // API密钥
	ClientCert      string            // 客户端证书路径
	ClientKey       string            // 客户端私钥路径
	CAFile          string            // CA证书路径
	TokenFilePath   string            // 令牌文件路径
	AutoRefresh     bool              // 是否自动刷新令牌
	RefreshInterval time.Duration     // 刷新间隔
	ExtraHeaders    map[string]string // 额外的请求头
}

// DefaultAuthConfig 默认认证配置
func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		APIEndpoint:     "https://api.example.com",
		RefreshInterval: 5 * time.Minute,
		AutoRefresh:     true,
		TokenFilePath:   "./data/auth.json",
		ExtraHeaders:    make(map[string]string),
	}
}

// AuthResult 认证结果
type AuthResult struct {
	Success   bool      `json:"success"`    // 认证是否成功
	Token     string    `json:"token"`      // 认证令牌
	ExpiresAt time.Time `json:"expires_at"` // 过期时间
	UserID    string    `json:"user_id"`    // 用户ID
	NodeID    string    `json:"node_id"`    // 节点ID
	Role      string    `json:"role"`       // 角色
	Message   string    `json:"message"`    // 认证消息
}

// AuthMethod 认证方法
type AuthMethod string

const (
	AuthMethodCK     AuthMethod = "ck"      // 连接密钥认证
	AuthMethodAPIKey AuthMethod = "api_key" // API密钥认证
	AuthMethodToken  AuthMethod = "token"   // 令牌认证
	AuthMethodTLS    AuthMethod = "tls"     // TLS证书认证
)

// Manager 认证管理器
type Manager struct {
	config       *AuthConfig
	httpClient   *http.Client
	authResult   *AuthResult
	authMethod   AuthMethod
	tokenManager *TokenManager
	mu           sync.RWMutex
	logger       *zap.Logger
}

// NewManager 创建新的认证管理器
func NewManager(config *ManagerConfig) *Manager {
	// 获取配置
	if config == nil {
		config = DefaultManagerConfig()
	}

	// 创建内部配置
	intConfig := &AuthConfig{
		APIEndpoint:     config.PlaneURL + config.APIEndpoint,
		NodeID:          config.NodeID,
		APIKey:          config.APIKey,
		TokenFilePath:   config.TokenFile,
		AutoRefresh:     true,
		RefreshInterval: config.RefreshWindow,
		ExtraHeaders:    make(map[string]string),
		CKValue:         "", // 简化处理
		ClientCert:      "", // 简化处理
		ClientKey:       "", // 简化处理
	}

	tokenOptions := &TokenManagerOptions{
		Storage:         "file",
		FilePath:        config.TokenFile,
		AutoRefresh:     true,
		RefreshInterval: config.RefreshWindow,
		Logger:          zap.L().Named("auth-token"),
	}

	m := &Manager{
		config:       intConfig,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		tokenManager: NewTokenManager(tokenOptions),
		logger:       zap.L().Named("auth"),
	}

	// 设置令牌刷新函数
	m.tokenManager.SetRefreshFunction(m.refreshToken)

	// 确定认证方法
	if intConfig.CKValue != "" {
		m.authMethod = AuthMethodCK
	} else if intConfig.APIKey != "" {
		m.authMethod = AuthMethodAPIKey
	} else if intConfig.ClientCert != "" && intConfig.ClientKey != "" {
		m.authMethod = AuthMethodTLS
	} else {
		m.authMethod = AuthMethodToken
	}

	return m
}

// Start 启动认证管理
func (m *Manager) Start() error {
	// 启动令牌管理器
	if err := m.tokenManager.Start(); err != nil {
		return fmt.Errorf("启动令牌管理器失败: %w", err)
	}

	// 检查是否有有效令牌
	if !m.tokenManager.HasValidTokens() {
		// 没有有效令牌，尝试认证
		if m.NeedsAuth() {
			m.logger.Info("没有有效令牌，将尝试认证")
			if _, err := m.Authenticate(); err != nil {
				m.logger.Warn("认证失败", zap.Error(err))
				return err
			}
		}
	}

	return nil
}

// Stop 停止认证管理
func (m *Manager) Stop() error {
	m.tokenManager.Stop()
	return nil
}

// Authenticate 认证
func (m *Manager) Authenticate() (*AuthResult, error) {
	var err error
	var result *AuthResult

	// 根据认证方法进行认证
	switch m.authMethod {
	case AuthMethodCK:
		result, err = m.authenticateByCK()
	case AuthMethodAPIKey:
		result, err = m.authenticateByAPIKey()
	case AuthMethodTLS:
		result, err = m.authenticateByTLS()
	default:
		// 默认使用令牌认证，但这种情况下应该已经有令牌了
		if m.tokenManager.HasValidTokens() {
			tokenPair := m.tokenManager.GetTokenPair()
			result = &AuthResult{
				Success:   true,
				Token:     tokenPair.AccessToken.Value,
				ExpiresAt: time.Unix(tokenPair.AccessToken.ExpiresAt, 0),
				// 其他字段可能从令牌中提取，或者返回空值
			}
			err = nil
		} else {
			err = fmt.Errorf("没有可用的认证方法")
		}
	}

	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.authResult = result
	m.mu.Unlock()

	// 如果认证成功，更新令牌
	if result.Success {
		// 构造令牌对
		now := time.Now().Unix()
		expiresAt := result.ExpiresAt.Unix()
		expiresIn := int(expiresAt - now)

		tokenPair := &TokenPair{
			AccessToken: Token{
				Type:      TokenTypeAccess,
				Value:     result.Token,
				ExpiresAt: expiresAt,
				IssuedAt:  now,
				ExpiresIn: expiresIn,
				Scope:     "api",
			},
			RefreshToken: Token{
				Type:      TokenTypeRefresh,
				Value:     result.Token,          // 实际应用中，刷新令牌应该单独获取
				ExpiresAt: expiresAt + 3600*24*7, // 假设刷新令牌有效期比访问令牌长
				IssuedAt:  now,
				ExpiresIn: expiresIn + 3600*24*7,
				Scope:     "refresh",
			},
		}

		if err := m.tokenManager.SetTokens(tokenPair); err != nil {
			m.logger.Error("设置令牌失败", zap.Error(err))
		}
	}

	return result, nil
}

// NeedsAuth 检查是否需要认证
func (m *Manager) NeedsAuth() bool {
	// 首先检查是否有有效令牌
	if m.tokenManager.HasValidTokens() {
		return false
	}

	// 然后检查是否配置了其他认证方法
	return m.authMethod != ""
}

// GetAuthResult 获取认证结果
func (m *Manager) GetAuthResult() *AuthResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 如果没有认证结果或令牌已过期，返回nil
	if m.authResult == nil {
		return nil
	}

	// 检查令牌是否过期
	if time.Now().After(m.authResult.ExpiresAt) {
		return nil
	}

	return m.authResult
}

// GetAuthHeader 获取认证头部
func (m *Manager) GetAuthHeader() (string, string) {
	// 首先尝试从令牌管理器获取访问令牌
	if token := m.tokenManager.GetAccessToken(); token != "" {
		return "Authorization", "Bearer " + token
	}

	// 如果没有令牌，使用API密钥
	if m.config.APIKey != "" {
		return "X-API-Key", m.config.APIKey
	}

	// 如果没有认证信息，返回空
	return "", ""
}

// authenticateByCK 使用连接密钥进行认证
func (m *Manager) authenticateByCK() (*AuthResult, error) {
	url := fmt.Sprintf("%s/api/v1/auth/node", m.config.APIEndpoint)

	// 构造请求体
	reqBody := map[string]interface{}{
		"node_id": m.config.NodeID,
		"ck":      m.config.CKValue,
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// 创建请求
	req, err := http.NewRequest("POST", url, strings.NewReader(string(reqData)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	for k, v := range m.config.ExtraHeaders {
		req.Header.Set(k, v)
	}

	// 发送请求
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析响应
	var response struct {
		Success bool   `json:"success"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Token     string    `json:"token"`
			ExpiresAt time.Time `json:"expires_at"`
			NodeID    string    `json:"node_id"`
			Role      string    `json:"role"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查认证结果
	if !response.Success || response.Data.Token == "" {
		return &AuthResult{
			Success: false,
			Message: response.Message,
		}, nil
	}

	// 返回认证结果
	return &AuthResult{
		Success:   true,
		Token:     response.Data.Token,
		ExpiresAt: response.Data.ExpiresAt,
		NodeID:    response.Data.NodeID,
		Role:      response.Data.Role,
		Message:   response.Message,
	}, nil
}

// authenticateByAPIKey 使用API密钥进行认证
func (m *Manager) authenticateByAPIKey() (*AuthResult, error) {
	// 在实际应用中，这里应该调用API进行认证
	// 这里简化处理，直接返回一个临时认证结果
	return &AuthResult{
		Success:   true,
		Token:     m.config.APIKey,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		NodeID:    m.config.NodeID,
		Role:      "node",
		Message:   "API Key authentication",
	}, nil
}

// authenticateByTLS 使用TLS证书进行认证
func (m *Manager) authenticateByTLS() (*AuthResult, error) {
	// 在实际应用中，这里应该配置TLS客户端证书并进行认证
	// 这里简化处理，直接返回错误
	return nil, fmt.Errorf("TLS认证未实现")
}

// refreshToken 刷新令牌
func (m *Manager) refreshToken(refreshToken string) (*TokenPair, error) {
	// 构造刷新请求
	url := fmt.Sprintf("%s/api/v1/auth/refresh", m.config.APIEndpoint)

	reqBody := map[string]string{
		"refresh_token": refreshToken,
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	// 创建请求
	req, err := http.NewRequest("POST", url, strings.NewReader(string(reqData)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	for k, v := range m.config.ExtraHeaders {
		req.Header.Set(k, v)
	}

	// 发送请求
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析响应
	var response struct {
		Success bool   `json:"success"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
			TokenType    string `json:"token_type"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查响应结果
	if !response.Success || response.Data.AccessToken == "" {
		return nil, fmt.Errorf("刷新令牌失败: %s", response.Message)
	}

	// 构造新令牌对
	now := time.Now().Unix()
	accessExpiresAt := now + int64(response.Data.ExpiresIn)
	refreshExpiresAt := now + int64(response.Data.ExpiresIn)*2 // 假设刷新令牌有效期是访问令牌的两倍

	return &TokenPair{
		AccessToken: Token{
			Type:      TokenTypeAccess,
			Value:     response.Data.AccessToken,
			ExpiresAt: accessExpiresAt,
			IssuedAt:  now,
			ExpiresIn: response.Data.ExpiresIn,
			Scope:     "api",
		},
		RefreshToken: Token{
			Type:      TokenTypeRefresh,
			Value:     response.Data.RefreshToken,
			ExpiresAt: refreshExpiresAt,
			IssuedAt:  now,
			ExpiresIn: response.Data.ExpiresIn * 2,
			Scope:     "refresh",
		},
	}, nil
}
