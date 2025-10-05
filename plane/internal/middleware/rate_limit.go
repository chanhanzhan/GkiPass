package middleware

import (
	"fmt"
	"time"

	"gkipass/plane/db"

	"github.com/gin-gonic/gin"
)

// RateLimiter 请求限流中间件
type RateLimiter struct {
	db *db.Manager
}

// NewRateLimiter 创建限流器
func NewRateLimiter(dbManager *db.Manager) *RateLimiter {
	return &RateLimiter{db: dbManager}
}

// Limit 限流中间件
func (rl *RateLimiter) Limit(requests int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !rl.db.HasCache() {
			c.Next()
			return
		}

		// 获取用户标识
		userID, exists := c.Get("user_id")
		if !exists {
			userID = c.ClientIP()
		}

		key := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), userID)

		// 检查限流
		var count int
		_ = rl.db.Cache.Redis.Get(key, &count)

		if count >= requests {
			c.JSON(429, gin.H{"error": "Too many requests"})
			c.Abort()
			return
		}

		// 增加计数
		count++
		_ = rl.db.Cache.Redis.Set(key, count, window)

		c.Next()
	}
}
