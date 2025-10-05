package middleware

import (
	"strings"
	"time"

	"gkipass/plane/db"
	dbinit "gkipass/plane/db/init"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims JWT声明
type JWTClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// JWTAuth JWT认证中间件
func JWTAuth(secret string, dbManager *db.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(401, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// 解析Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(401, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 检查Redis缓存（如果可用）
		if dbManager.HasCache() {
			session, err := dbManager.Cache.Redis.GetSession(tokenString)
			if err == nil && session != nil {
				// 检查是否过期
				if time.Now().Before(session.ExpiresAt) {
					c.Set("user_id", session.UserID)
					c.Set("username", session.Username)
					c.Set("role", session.Role)
					c.Next()
					return
				}
			}
		}

		// 解析JWT token
		token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(401, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(*JWTClaims)
		if !ok {
			c.JSON(401, gin.H{"error": "Invalid token claims"})
			c.Abort()
			return
		}

		// 设置用户信息到上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		// 如果Redis可用，缓存session
		if dbManager.HasCache() {
			session := &dbinit.Session{
				Token:     tokenString,
				UserID:    claims.UserID,
				Username:  claims.Username,
				Role:      claims.Role,
				CreatedAt: time.Now(),
				ExpiresAt: claims.ExpiresAt.Time,
			}
			_ = dbManager.Cache.Redis.SetSession(tokenString, session, time.Until(claims.ExpiresAt.Time))
		}

		c.Next()
	}
}

// RequireRole 角色检查中间件
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists {
			c.JSON(403, gin.H{"error": "Role not found in context"})
			c.Abort()
			return
		}

		roleStr := userRole.(string)
		allowed := false
		for _, role := range roles {
			if roleStr == role {
				allowed = true
				break
			}
		}

		if !allowed {
			c.JSON(403, gin.H{"error": "Insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// GenerateJWT 生成JWT token
func GenerateJWT(userID, username, role, secret string, expirationHours int) (string, error) {
	claims := JWTClaims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expirationHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}


