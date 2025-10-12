package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"gkipass/plane/internal/api/response"
	"gkipass/plane/internal/service"
)

// AuthMiddleware 认证中间件
type AuthMiddleware struct {
	authService *service.AuthService
	logger      *zap.Logger
}

// NewAuthMiddleware 创建认证中间件
func NewAuthMiddleware(authService *service.AuthService) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
		logger:      zap.L().Named("auth-middleware"),
	}
}

// Middleware 认证中间件处理函数
func (m *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 获取认证头
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// 尝试从API密钥头获取认证信息
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// 尝试从查询参数获取认证信息
				apiKey = r.URL.Query().Get("api_key")
				if apiKey == "" {
					m.logger.Debug("请求未包含认证信息")
					response.Unauthorized(w, "未提供认证信息", nil)
					return
				}

				// 验证API密钥
				if err := m.authService.ValidateAPIKey(apiKey); err != nil {
					m.logger.Debug("API密钥验证失败", zap.Error(err))
					response.Unauthorized(w, "无效的API密钥", err)
					return
				}

				// API密钥验证成功，继续处理请求
				next.ServeHTTP(w, r)
				return
			}

			// 验证API密钥
			if err := m.authService.ValidateAPIKey(apiKey); err != nil {
				m.logger.Debug("API密钥验证失败", zap.Error(err))
				response.Unauthorized(w, "无效的API密钥", err)
				return
			}

			// API密钥验证成功，继续处理请求
			next.ServeHTTP(w, r)
			return
		}

		// 解析认证头
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			m.logger.Debug("认证头格式无效", zap.String("header", authHeader))
			response.Unauthorized(w, "认证头格式无效", nil)
			return
		}

		token := parts[1]

		// 验证令牌
		claims, err := m.authService.ValidateToken(token)
		if err != nil {
			m.logger.Debug("令牌验证失败", zap.Error(err))
			response.Unauthorized(w, "无效的令牌", err)
			return
		}

		// 将用户信息添加到请求上下文
		ctx := r.Context()
		ctx = service.WithUserClaims(ctx, claims)

		// 继续处理请求
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GenerateJWT 生成JWT令牌
func GenerateJWT(userID, username, role, jwtSecret string, expiresIn int) (string, error) {
	// TODO: Implement actual JWT token generation with proper signing
	// For now, return a placeholder token
	return "jwt_token_placeholder_" + userID, nil
}

// JWTAuth 返回Gin JWT认证中间件
func JWTAuth(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Header中获取token
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少认证令牌"})
			c.Abort()
			return
		}

		// 验证token
		claims, err := authService.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的令牌"})
			c.Abort()
			return
		}

		// 将用户信息存储到上下文
		c.Set("user_claims", claims)
		c.Next()
	}
}
