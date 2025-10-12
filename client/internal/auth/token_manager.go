package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TokenType 表示令牌类型
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// Token 表示认证令牌
type Token struct {
	Type      TokenType `json:"type"`
	Value     string    `json:"value"`
	ExpiresAt int64     `json:"expires_at"`
	IssuedAt  int64     `json:"issued_at"`
	ExpiresIn int       `json:"expires_in"`
	Scope     string    `json:"scope"`
}

// TokenPair 表示访问令牌和刷新令牌的对
type TokenPair struct {
	AccessToken  Token `json:"access_token"`
	RefreshToken Token `json:"refresh_token"`
}

// IsExpired 检查令牌是否已过期，includeBuffer为true时会提前5分钟判定过期以便刷新
func (t *Token) IsExpired(includeBuffer bool) bool {
	if t == nil || t.Value == "" {
		return true
	}

	now := time.Now().Unix()
	if includeBuffer {
		// 提前5分钟视为过期（用于刷新）
		return now >= t.ExpiresAt-300
	}
	return now >= t.ExpiresAt
}

// TokenManagerOptions 令牌管理器选项
type TokenManagerOptions struct {
	Storage         string           // 存储方式: memory, file
	FilePath        string           // 令牌文件路径
	AutoRefresh     bool             // 是否自动刷新
	RefreshInterval time.Duration    // 刷新检查间隔
	Logger          *zap.Logger      // 日志记录器
	OnRefresh       func(*TokenPair) // 刷新回调
	OnExpired       func()           // 令牌过期回调
}

// DefaultTokenManagerOptions 默认令牌管理器选项
func DefaultTokenManagerOptions() *TokenManagerOptions {
	return &TokenManagerOptions{
		Storage:         "memory",
		FilePath:        "./data/token.json",
		AutoRefresh:     true,
		RefreshInterval: 5 * time.Minute,
		Logger:          zap.L().Named("token-manager"),
	}
}

// TokenManager 管理认证令牌
type TokenManager struct {
	options   *TokenManagerOptions
	tokens    *TokenPair
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	refreshFn func(string) (*TokenPair, error) // 令牌刷新函数
}

// NewTokenManager 创建新的令牌管理器
func NewTokenManager(options *TokenManagerOptions) *TokenManager {
	if options == nil {
		options = DefaultTokenManagerOptions()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TokenManager{
		options: options,
		tokens:  nil,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// SetTokens 设置令牌对
func (tm *TokenManager) SetTokens(tokens *TokenPair) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.tokens = tokens

	// 如果配置了文件存储，保存令牌到文件
	if tm.options.Storage == "file" && tm.options.FilePath != "" {
		if err := tm.saveTokensToFile(); err != nil {
			tm.options.Logger.Error("保存令牌到文件失败", zap.Error(err))
			return err
		}
	}

	return nil
}

// GetAccessToken 获取访问令牌
func (tm *TokenManager) GetAccessToken() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.tokens == nil || tm.tokens.AccessToken.IsExpired(false) {
		return ""
	}

	return tm.tokens.AccessToken.Value
}

// GetTokenPair 获取令牌对
func (tm *TokenManager) GetTokenPair() *TokenPair {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.tokens == nil {
		return nil
	}

	// 返回副本，避免外部修改
	tokenCopy := *tm.tokens
	return &tokenCopy
}

// HasValidTokens 检查是否有有效令牌
func (tm *TokenManager) HasValidTokens() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.tokens != nil && !tm.tokens.AccessToken.IsExpired(false)
}

// NeedsRefresh 检查是否需要刷新令牌
func (tm *TokenManager) NeedsRefresh() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if tm.tokens == nil {
		return false // 无令牌，需要完整登录，而不是刷新
	}

	// 访问令牌即将过期但刷新令牌有效时需要刷新
	return tm.tokens.AccessToken.IsExpired(true) && !tm.tokens.RefreshToken.IsExpired(false)
}

// Start 开始令牌管理
func (tm *TokenManager) Start() error {
	// 尝试从文件加载令牌
	if tm.options.Storage == "file" && tm.options.FilePath != "" {
		if err := tm.loadTokensFromFile(); err != nil {
			tm.options.Logger.Warn("从文件加载令牌失败", zap.Error(err))
		}
	}

	// 如果配置了自动刷新，启动刷新循环
	if tm.options.AutoRefresh {
		go tm.refreshLoop()
	}

	return nil
}

// Stop 停止令牌管理
func (tm *TokenManager) Stop() {
	tm.cancel()
}

// SetRefreshFunction 设置令牌刷新函数
func (tm *TokenManager) SetRefreshFunction(fn func(string) (*TokenPair, error)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.refreshFn = fn
}

// RefreshTokens 刷新令牌
func (tm *TokenManager) RefreshTokens() (*TokenPair, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.tokens == nil || tm.refreshFn == nil {
		return nil, errors.New("无法刷新令牌：令牌不存在或未设置刷新函数")
	}

	if tm.tokens.RefreshToken.IsExpired(false) {
		if tm.options.OnExpired != nil {
			go tm.options.OnExpired()
		}
		return nil, errors.New("刷新令牌已过期")
	}

	// 调用刷新函数获取新令牌
	newTokens, err := tm.refreshFn(tm.tokens.RefreshToken.Value)
	if err != nil {
		return nil, fmt.Errorf("刷新令牌失败: %w", err)
	}

	// 更新令牌
	tm.tokens = newTokens

	// 保存到文件
	if tm.options.Storage == "file" && tm.options.FilePath != "" {
		if err := tm.saveTokensToFile(); err != nil {
			tm.options.Logger.Error("保存令牌到文件失败", zap.Error(err))
		}
	}

	// 调用刷新回调
	if tm.options.OnRefresh != nil {
		go tm.options.OnRefresh(newTokens)
	}

	return newTokens, nil
}

// ClearTokens 清除令牌
func (tm *TokenManager) ClearTokens() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.tokens = nil

	// 如果使用文件存储，删除令牌文件
	if tm.options.Storage == "file" && tm.options.FilePath != "" {
		if err := os.Remove(tm.options.FilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除令牌文件失败: %w", err)
		}
	}

	return nil
}

// saveTokensToFile 保存令牌到文件
func (tm *TokenManager) saveTokensToFile() error {
	if tm.tokens == nil {
		return nil
	}

	// 创建目录（如果不存在）
	dir := tm.options.FilePath[:len(tm.options.FilePath)-len("/token.json")]
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 序列化令牌
	data, err := json.MarshalIndent(tm.tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化令牌失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(tm.options.FilePath, data, 0600); err != nil {
		return fmt.Errorf("写入令牌文件失败: %w", err)
	}

	return nil
}

// loadTokensFromFile 从文件加载令牌
func (tm *TokenManager) loadTokensFromFile() error {
	// 检查文件是否存在
	if _, err := os.Stat(tm.options.FilePath); os.IsNotExist(err) {
		return nil // 文件不存在，不是错误
	}

	// 读取文件内容
	data, err := os.ReadFile(tm.options.FilePath)
	if err != nil {
		return fmt.Errorf("读取令牌文件失败: %w", err)
	}

	if len(data) == 0 {
		return io.EOF // 文件为空
	}

	// 解析令牌
	var tokens TokenPair
	if err := json.Unmarshal(data, &tokens); err != nil {
		return fmt.Errorf("解析令牌文件失败: %w", err)
	}

	// 检查令牌有效性
	if tokens.AccessToken.IsExpired(false) && tokens.RefreshToken.IsExpired(false) {
		return errors.New("令牌已过期")
	}

	tm.mu.Lock()
	tm.tokens = &tokens
	tm.mu.Unlock()

	return nil
}

// refreshLoop 令牌刷新循环
func (tm *TokenManager) refreshLoop() {
	ticker := time.NewTicker(tm.options.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 检查是否需要刷新令牌
			if tm.NeedsRefresh() {
				tm.options.Logger.Debug("正在刷新令牌...")

				if _, err := tm.RefreshTokens(); err != nil {
					tm.options.Logger.Error("刷新令牌失败", zap.Error(err))

					// 如果刷新失败且访问令牌已过期，通知过期回调
					if tm.GetTokenPair() != nil && tm.GetTokenPair().AccessToken.IsExpired(false) {
						if tm.options.OnExpired != nil {
							tm.options.OnExpired()
						}
					}
				} else {
					tm.options.Logger.Debug("令牌刷新成功")
				}
			}

		case <-tm.ctx.Done():
			return
		}
	}
}
