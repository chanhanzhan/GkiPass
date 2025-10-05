package core

import (
	"net/http"
	_ "net/http/pprof"
	"runtime"

	"gkipass/client/logger"

	"go.uber.org/zap"
)

// StartPProfServer 启动pprof性能分析服务器
func StartPProfServer(addr string) {
	if addr == "" {
		addr = "localhost:6060"
	}

	// 设置内存采样率
	runtime.MemProfileRate = 1

	go func() {
		logger.Info("pprof服务器已启动",
			zap.String("addr", addr),
			zap.String("cpu_profile", "http://"+addr+"/debug/pprof/profile"),
			zap.String("heap_profile", "http://"+addr+"/debug/pprof/heap"),
			zap.String("goroutine", "http://"+addr+"/debug/pprof/goroutine"))

		if err := http.ListenAndServe(addr, nil); err != nil {
			logger.Error("pprof服务器错误", zap.Error(err))
		}
	}()
}

// ForceGC 强制垃圾回收
func ForceGC() {
	runtime.GC()
}

// GetMemStats 获取内存统计
func GetMemStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// PrintMemStats 打印内存统计
func PrintMemStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	logger.Info("内存统计",
		zap.Uint64("alloc_mb", m.Alloc/1024/1024),
		zap.Uint64("total_alloc_mb", m.TotalAlloc/1024/1024),
		zap.Uint64("sys_mb", m.Sys/1024/1024),
		zap.Uint32("num_gc", m.NumGC),
		zap.Uint32("goroutines", uint32(runtime.NumGoroutine())))
}








