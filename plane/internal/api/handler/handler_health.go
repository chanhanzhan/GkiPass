package handler

import (
	"net/http"
	"runtime"
	"time"

	"gkipass/plane/internal/api/response"
)

// HealthCheck 健康检查处理器
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	// 构建健康状态信息
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0", // 从配置或构建信息中获取
		"uptime":    time.Since(startTime).String(),
		"system": map[string]interface{}{
			"go_version": runtime.Version(),
			"go_os":      runtime.GOOS,
			"go_arch":    runtime.GOARCH,
			"cpu_cores":  runtime.NumCPU(),
			"goroutines": runtime.NumGoroutine(),
		},
	}

	response.Success(w, health)
}

// 应用启动时间
var startTime = time.Now()
