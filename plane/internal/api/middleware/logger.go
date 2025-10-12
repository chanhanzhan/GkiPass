package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RequestLogger 请求日志中间件
func RequestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// 生成请求ID
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// 添加请求ID到响应头
			w.Header().Set("X-Request-ID", requestID)

			// 包装响应写入器以捕获状态码
			ww := &responseWriter{w: w, status: http.StatusOK}

			// 处理请求
			next.ServeHTTP(ww, r)

			// 记录请求日志
			duration := time.Since(start)

			// 根据状态码选择日志级别
			logFunc := logger.Info
			if ww.status >= 500 {
				logFunc = logger.Error
			} else if ww.status >= 400 {
				logFunc = logger.Warn
			}

			logFunc("HTTP请求",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("query", r.URL.RawQuery),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("user_agent", r.UserAgent()),
				zap.Int("status", ww.status),
				zap.Duration("duration", duration),
				zap.String("request_id", requestID),
			)
		})
	}
}

// responseWriter 响应写入器包装器
type responseWriter struct {
	w      http.ResponseWriter
	status int
	size   int
}

// Header 实现http.ResponseWriter接口
func (rw *responseWriter) Header() http.Header {
	return rw.w.Header()
}

// Write 实现http.ResponseWriter接口
func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.w.Write(b)
	rw.size += size
	return size, err
}

// WriteHeader 实现http.ResponseWriter接口
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.w.WriteHeader(statusCode)
}

// Flush 实现http.Flusher接口（如果底层ResponseWriter支持）
func (rw *responseWriter) Flush() {
	if f, ok := rw.w.(http.Flusher); ok {
		f.Flush()
	}
}

// Logger 返回Gin日志中间件
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 记录日志
		duration := time.Since(start)
		logger := zap.L()

		logFunc := logger.Info
		if c.Writer.Status() >= 500 {
			logFunc = logger.Error
		} else if c.Writer.Status() >= 400 {
			logFunc = logger.Warn
		}

		logFunc("HTTP请求",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("remote_addr", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("status", c.Writer.Status()),
			zap.Int("size", c.Writer.Size()),
			zap.Duration("duration", duration),
		)
	}
}
