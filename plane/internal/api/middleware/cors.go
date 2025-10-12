package middleware

import (
	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 允许所有来源
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		// 允许使用凭证（但要注意当 Allow-Origin 为 * 时，浏览器会忽略这个设置）
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		// 允许所有常用头部
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-API-Key, accept, origin, Cache-Control, X-Requested-With, X-Request-ID, token")
		// 允许所有常用方法
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH, HEAD")
		// 允许暴露的响应头部
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type, X-Request-ID")
		// 缓存预检请求结果 1 小时
		c.Writer.Header().Set("Access-Control-Max-Age", "3600")

		// 处理 OPTIONS 预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
