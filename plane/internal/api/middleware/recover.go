package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"

	"gkipass/plane/internal/api/response"
)

// Recover 恢复中间件
func Recover(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// 记录错误和堆栈跟踪
					stackTrace := debug.Stack()
					logger.Error("请求处理异常",
						zap.Any("error", err),
						zap.String("stack", string(stackTrace)),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
						zap.String("remote_addr", r.RemoteAddr),
					)

					// 返回500错误
					errMsg := "服务器内部错误"
					if logger.Core().Enabled(zap.DebugLevel) {
						errMsg = fmt.Sprintf("服务器内部错误: %v", err)
					}
					response.InternalServerError(w, errMsg, nil)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
