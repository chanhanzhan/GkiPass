package auth

import (
	"time"

	"gkipass/client/internal/identity"
)

// SetIdentityManager 设置身份管理器
func (m *Manager) SetIdentityManager(identityManager *identity.Manager) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 存储身份管理器引用并同步节点ID
	if identityManager != nil {
		m.config.NodeID = identityManager.GetNodeID()
	}
}

// GetStatus 获取认证管理器状态
func (m *Manager) GetStatus() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 获取令牌状态
	tokenPair := m.tokenManager.GetTokenPair()
	var accessTokenValid bool
	var refreshTokenValid bool
	var expiresIn int64

	if tokenPair != nil {
		accessTokenValid = !tokenPair.AccessToken.IsExpired(false)
		refreshTokenValid = !tokenPair.RefreshToken.IsExpired(false)
		if accessTokenValid {
			expiresIn = tokenPair.AccessToken.ExpiresAt - time.Now().Unix()
		}
	}

	// 构建状态信息
	status := map[string]interface{}{
		"auth_method":       string(m.authMethod),
		"has_valid_token":   accessTokenValid,
		"has_refresh_token": refreshTokenValid,
		"token_expires_in":  expiresIn,
		"auto_refresh":      m.tokenManager.options.AutoRefresh,
		"refresh_interval":  m.tokenManager.options.RefreshInterval.String(),
		"refresh_window":    m.config.RefreshInterval.String(),
	}

	// 添加认证结果信息（如果有）
	if m.authResult != nil {
		status["last_auth"] = map[string]interface{}{
			"success":    m.authResult.Success,
			"expires_at": m.authResult.ExpiresAt,
			"role":       m.authResult.Role,
			"node_id":    m.authResult.NodeID,
		}
	}

	return status
}
