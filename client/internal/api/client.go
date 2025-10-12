package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"

	"gkipass/client/internal/auth"
)

// ClientConfig API客户端配置
type ClientConfig struct {
	BaseURL         string        `json:"base_url"`          // 基础URL
	APIVersion      string        `json:"api_version"`       // API版本
	Timeout         time.Duration `json:"timeout"`           // 超时时间
	RetryCount      int           `json:"retry_count"`       // 重试次数
	RetryWait       time.Duration `json:"retry_wait"`        // 重试等待时间
	UserAgent       string        `json:"user_agent"`        // User-Agent
	TLSSkipVerify   bool          `json:"tls_skip_verify"`   // 跳过TLS验证
	EnableTracing   bool          `json:"enable_tracing"`    // 启用跟踪
	MaxResponseSize int64         `json:"max_response_size"` // 最大响应大小
}

// DefaultClientConfig 默认API客户端配置
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		APIVersion:      "v1",
		Timeout:         30 * time.Second,
		RetryCount:      3,
		RetryWait:       5 * time.Second,
		UserAgent:       "GKiPass-Client/1.0",
		TLSSkipVerify:   false,
		EnableTracing:   false,
		MaxResponseSize: 10 * 1024 * 1024, // 10MB
	}
}

// Client API客户端
type Client struct {
	config       *ClientConfig
	httpClient   *http.Client
	authManager  *auth.Manager
	logger       *zap.Logger
	baseURL      *url.URL
	lastAPIError *APIError
}

// NewClient 创建API客户端
func NewClient(config *ClientConfig, authManager *auth.Manager) (*Client, error) {
	if config == nil {
		config = DefaultClientConfig()
	}

	// 解析基础URL
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("无效的基础URL: %w", err)
	}

	// 创建HTTP客户端
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.TLSSkipVerify,
			MinVersion:         tls.VersionTLS12,
		},
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	client := &Client{
		config:      config,
		httpClient:  httpClient,
		authManager: authManager,
		logger:      zap.L().Named("api-client"),
		baseURL:     baseURL,
	}

	return client, nil
}

// APIError API错误
type APIError struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	ErrorCode  string `json:"error_code"`
	RequestID  string `json:"request_id"`
	Timestamp  int64  `json:"timestamp"`
	Details    map[string]interface{}
}

func (e *APIError) Error() string {
	if e.ErrorCode != "" {
		return fmt.Sprintf("API错误 [%s]: %s (状态码: %d, 请求ID: %s)",
			e.ErrorCode, e.Message, e.StatusCode, e.RequestID)
	}
	return fmt.Sprintf("API错误: %s (状态码: %d, 请求ID: %s)",
		e.Message, e.StatusCode, e.RequestID)
}

// Request 发送API请求
func (c *Client) Request(ctx context.Context, method, endpoint string, body, result interface{}) error {
	// 构建完整URL
	reqURL := c.buildURL(endpoint)

	// 序列化请求体
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("序列化请求体失败: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	// 设置头部
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.config.UserAgent)

	// 设置认证信息
	if c.authManager != nil {
		authResult := c.authManager.GetAuthResult()
		if authResult != nil && authResult.Success {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authResult.Token))
		} else {
			// 尝试重新认证
			if c.authManager.NeedsAuth() {
				if _, err := c.authManager.Authenticate(); err != nil {
					return fmt.Errorf("认证失败: %w", err)
				}

				// 重新获取认证结果
				authResult = c.authManager.GetAuthResult()
				if authResult != nil && authResult.Success {
					req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authResult.Token))
				} else {
					return fmt.Errorf("获取认证信息失败")
				}
			}
		}
	}

	// 执行请求，带重试
	var resp *http.Response
	var lastErr error

	for i := 0; i <= c.config.RetryCount; i++ {
		if i > 0 {
			// 等待重试
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.config.RetryWait):
				// 继续重试
			}
		}

		c.logger.Debug("发送API请求",
			zap.String("method", method),
			zap.String("url", reqURL),
			zap.Int("attempt", i+1),
			zap.Int("max_attempts", c.config.RetryCount+1))

		// 发送请求
		resp, err = c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("发送请求失败: %w", err)
			c.logger.Warn("请求失败，将重试",
				zap.Error(err),
				zap.Int("attempt", i+1),
				zap.Int("max_attempts", c.config.RetryCount+1))
			continue
		}

		// 请求成功，退出重试循环
		break
	}

	// 检查是否所有请求都失败
	if resp == nil {
		return lastErr
	}

	// 确保响应体被关闭
	defer resp.Body.Close()

	// 处理非成功状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := c.handleErrorResponse(resp)
		c.lastAPIError = apiErr
		return apiErr
	}

	// 处理成功响应
	if result != nil {
		// 限制读取大小
		limitedReader := io.LimitReader(resp.Body, c.config.MaxResponseSize)

		// 解析响应体
		if err := json.NewDecoder(limitedReader).Decode(result); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	return nil
}

// GET 发送GET请求
func (c *Client) GET(ctx context.Context, endpoint string, result interface{}) error {
	return c.Request(ctx, http.MethodGet, endpoint, nil, result)
}

// POST 发送POST请求
func (c *Client) POST(ctx context.Context, endpoint string, body, result interface{}) error {
	return c.Request(ctx, http.MethodPost, endpoint, body, result)
}

// PUT 发送PUT请求
func (c *Client) PUT(ctx context.Context, endpoint string, body, result interface{}) error {
	return c.Request(ctx, http.MethodPut, endpoint, body, result)
}

// DELETE 发送DELETE请求
func (c *Client) DELETE(ctx context.Context, endpoint string, result interface{}) error {
	return c.Request(ctx, http.MethodDelete, endpoint, nil, result)
}

// PATCH 发送PATCH请求
func (c *Client) PATCH(ctx context.Context, endpoint string, body, result interface{}) error {
	return c.Request(ctx, http.MethodPatch, endpoint, body, result)
}

// buildURL 构建完整URL
func (c *Client) buildURL(endpoint string) string {
	// 复制基础URL
	u := *c.baseURL

	// 构建路径
	if c.config.APIVersion != "" {
		u.Path = path.Join(u.Path, "api", c.config.APIVersion, strings.TrimPrefix(endpoint, "/"))
	} else {
		u.Path = path.Join(u.Path, "api", strings.TrimPrefix(endpoint, "/"))
	}

	return u.String()
}

// handleErrorResponse 处理错误响应
func (c *Client) handleErrorResponse(resp *http.Response) *APIError {
	// 创建默认错误
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Message:    fmt.Sprintf("HTTP错误: %s", resp.Status),
		Timestamp:  time.Now().Unix(),
		RequestID:  resp.Header.Get("X-Request-ID"),
		Details:    make(map[string]interface{}),
	}

	// 尝试读取错误详情
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.config.MaxResponseSize))
	if err != nil {
		c.logger.Warn("读取错误响应失败", zap.Error(err))
		return apiErr
	}

	// 尝试解析JSON错误
	if len(body) > 0 {
		var jsonErr struct {
			Message   string                 `json:"message"`
			ErrorCode string                 `json:"error_code"`
			RequestID string                 `json:"request_id"`
			Details   map[string]interface{} `json:"details"`
		}

		if err := json.Unmarshal(body, &jsonErr); err == nil {
			if jsonErr.Message != "" {
				apiErr.Message = jsonErr.Message
			}
			if jsonErr.ErrorCode != "" {
				apiErr.ErrorCode = jsonErr.ErrorCode
			}
			if jsonErr.RequestID != "" {
				apiErr.RequestID = jsonErr.RequestID
			}
			if jsonErr.Details != nil {
				apiErr.Details = jsonErr.Details
			}
		}
	}

	return apiErr
}

// GetLastError 获取最后一个API错误
func (c *Client) GetLastError() *APIError {
	return c.lastAPIError
}
